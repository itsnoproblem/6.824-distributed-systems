package exercise

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/runstream"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type CourseRepo interface{ Course() *course.Course }

type ProgressMarker interface {
	SetComplete(ctx context.Context, ref course.StepRef, done bool) error
}

type DraftRepo interface {
	Upsert(ctx context.Context, ref course.StepRef, files map[string]string) error
	Get(ctx context.Context, ref course.StepRef) (map[string]string, bool, error)
	Delete(ctx context.Context, ref course.StepRef) error
}

type SubmissionRepo interface {
	InsertSubmission(ctx context.Context, s eval.Submission) (int64, error)
	UpdateSubmission(ctx context.Context, id int64, status eval.Status, testOutput string) error
	SetPassed(ctx context.Context, id int64, passed bool) error
	GetSubmission(ctx context.Context, id int64) (eval.Submission, error)
	LatestForStep(ctx context.Context, ref course.StepRef) (*eval.Submission, error)
	InsertEvaluation(ctx context.Context, e eval.Evaluation) (int64, error)
	EvaluationForSubmission(ctx context.Context, submissionID int64) (*eval.Evaluation, error)
}

type Runner interface {
	RunExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string,
		sink func(string)) (output string, exitCode int, err error)
	CheckExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string) ([]Diagnostic, error)
	// Materialize returns the full workspace file set (generated go.mod +
	// every scaffold file, with editable files overlaid) without touching
	// disk — used to snapshot exactly what a run will execute.
	Materialize(meta *course.CodeMeta, editable map[string]string) map[string]string
}

type Service struct {
	course   CourseRepo
	drafts   DraftRepo
	subs     SubmissionRepo
	progress ProgressMarker
	runner   Runner
	llm      eval.LLM
	rubric   eval.Rubric
	runAsync func(func())
	now      func() time.Time
	broker   *runstream.Broker
}

type Option func(*Service)

// WithRunAsync overrides how runs are scheduled; tests run them inline.
func WithRunAsync(f func(func())) Option { return func(s *Service) { s.runAsync = f } }

// NewService wires the exercise engine. llm is nil in locked mode: the
// service still saves drafts and runs tests, but FeedbackEnabled reports
// false and Feedback rejects requests.
func NewService(c CourseRepo, d DraftRepo, subs SubmissionRepo, p ProgressMarker, r Runner,
	llm eval.LLM, contentDir string, opts ...Option) (*Service, error) {
	rubric, err := eval.LoadRubric(filepath.Join(contentDir, "rubric", "exercise.md"))
	if err != nil {
		return nil, fmt.Errorf("load exercise rubric: %w", err)
	}
	s := &Service{
		course: c, drafts: d, subs: subs, progress: p, runner: r,
		llm: llm, rubric: rubric,
		runAsync: func(f func()) { go f() }, now: time.Now,
		broker: runstream.NewBroker(),
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// FeedbackEnabled reports whether evaluation mode is unlocked — an LLM was
// configured at construction. When false, Feedback rejects every request.
func (s *Service) FeedbackEnabled() bool { return s.llm != nil }

func (s *Service) codeStep(ref course.StepRef) (*course.Step, error) {
	_, step, ok := s.course.Course().Step(ref)
	if !ok || step.Type != course.StepCode || step.Code == nil {
		return nil, fmt.Errorf("%w: code step %s", api.ErrNotFound, ref)
	}
	return step, nil
}

// effective returns the editable file set: scaffold overlaid with any draft.
func (s *Service) effective(ctx context.Context, ref course.StepRef, meta *course.CodeMeta) (map[string]string, bool, error) {
	files := map[string]string{}
	for _, name := range meta.Editable {
		files[name] = meta.Files[name]
	}
	draft, ok, err := s.drafts.Get(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	if ok {
		for name, src := range draft {
			if _, editable := files[name]; editable {
				files[name] = src
			}
		}
	}
	return files, ok, nil
}

func (s *Service) State(ctx context.Context, ref course.StepRef) (View, error) {
	step, err := s.codeStep(ref)
	if err != nil {
		return View{}, err
	}
	editable, hasDraft, err := s.effective(ctx, ref, step.Code)
	if err != nil {
		return View{}, err
	}
	view := View{Meta: step.Code, Step: *step, HasDraft: hasDraft}
	for _, name := range step.Code.Editable {
		view.Files = append(view.Files, FileView{Name: name, Content: editable[name]})
	}
	for _, name := range step.Code.Readonly {
		view.Files = append(view.Files, FileView{Name: name, Content: step.Code.Files[name], Readonly: true})
	}
	if view.Submission, err = s.subs.LatestForStep(ctx, ref); err != nil {
		return View{}, err
	}
	if view.Submission != nil {
		if view.Evaluation, err = s.subs.EvaluationForSubmission(ctx, view.Submission.ID); err != nil {
			return View{}, err
		}
		if view.Submission.Status == eval.StatusPending || view.Submission.Status == eval.StatusRunning {
			_, view.Live = s.broker.Get(runKey(view.Submission.ID))
		}
	}
	return view, nil
}

func (s *Service) SaveDraft(ctx context.Context, ref course.StepRef, files map[string]string) error {
	step, err := s.codeStep(ref)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%w: no files in draft", api.ErrInvalid)
	}
	editable := map[string]bool{}
	for _, f := range step.Code.Editable {
		editable[f] = true
	}
	for name := range files {
		if !editable[name] {
			return fmt.Errorf("%w: %q is not an editable file", api.ErrInvalid, name)
		}
	}
	return s.drafts.Upsert(ctx, ref, files)
}

func (s *Service) ResetDraft(ctx context.Context, ref course.StepRef) error {
	if _, err := s.codeStep(ref); err != nil {
		return err
	}
	return s.drafts.Delete(ctx, ref)
}

func (s *Service) Check(ctx context.Context, ref course.StepRef) ([]Diagnostic, error) {
	step, err := s.codeStep(ref)
	if err != nil {
		return nil, err
	}
	editable, _, err := s.effective(ctx, ref, step.Code)
	if err != nil {
		return nil, err
	}
	return s.runner.CheckExercise(ctx, step.Code, editable)
}

// Run snapshots the full materialized file set — generated go.mod plus
// every scaffold file, editable and readonly, with the draft overlay
// applied to the editable ones — into a submission and schedules the async
// test run. The snapshot is stored before any side effect, and it's the
// complete workspace, not just the editable subset: submissions.content
// keeps the full materialized file set for reproducibility, matching how
// v1 labs snapshot everything the test run touched.
func (s *Service) Run(ctx context.Context, ref course.StepRef) error {
	step, err := s.codeStep(ref)
	if err != nil {
		return err
	}
	editable, _, err := s.effective(ctx, ref, step.Code)
	if err != nil {
		return err
	}
	full := s.runner.Materialize(step.Code, editable)
	content, err := json.Marshal(full)
	if err != nil {
		return err
	}
	id, err := s.subs.InsertSubmission(ctx, eval.Submission{
		Ref: ref, Kind: eval.KindExercise, Content: string(content),
		Status: eval.StatusPending, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}
	s.startRun(id)
	return nil
}

func runKey(id int64) string { return "exercise/" + strconv.FormatInt(id, 10) }

// startRun registers the live run BEFORE scheduling the pipeline goroutine,
// so a Watch/Cancel arriving right after the Run response always finds it.
func (s *Service) startRun(id int64) {
	runCtx, cancel := context.WithCancel(context.Background())
	live := s.broker.Register(runKey(id), cancel)
	s.runAsync(func() {
		defer cancel()
		s.evaluate(runCtx, id, live)
	})
}

// evaluate runs in the background; every failure lands on the submission
// row. Persistence failures along the way are logged rather than silently
// discarded — they can't be returned (there is no caller left to hear
// about them), but they must be observable. Persistence always uses a fresh
// background context (it must survive after runCtx is canceled by Cancel or
// by startRun's deferred cancel on exit); only the test run itself is bound
// to runCtx, so a cancel actually stops the subprocess.
func (s *Service) evaluate(runCtx context.Context, id int64, live *runstream.Run) {
	defer live.Finish() // idempotent; releases subscribers on every exit path
	ctx := context.Background()
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		log.Printf("exercise evaluate: load submission %d: %v", id, err)
		return
	}
	step, err := s.codeStep(sub.Ref)
	if err != nil {
		if uerr := s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, "step no longer exists in content"); uerr != nil {
			log.Printf("exercise evaluate: update submission %d to failed (missing step): %v", id, uerr)
		}
		return
	}
	if err := s.subs.UpdateSubmission(ctx, id, eval.StatusRunning, ""); err != nil {
		log.Printf("exercise evaluate: update submission %d to running: %v", id, err)
	}
	// sub.Content is the full materialized file set stored by Run (go.mod +
	// every scaffold file, editable and readonly, with the draft overlay
	// already applied) — unmarshal and execute exactly that snapshot.
	// RunExercise only overlays keys named in meta.Editable, so passing the
	// full set here is safe: the readonly files and go.mod it also carries
	// are ignored as overlay and reconstructed identically from meta.
	var files map[string]string
	if err := json.Unmarshal([]byte(sub.Content), &files); err != nil {
		if uerr := s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, "snapshot decode error: "+err.Error()); uerr != nil {
			log.Printf("exercise evaluate: update submission %d to failed (decode error): %v", id, uerr)
		}
		return
	}
	out, code, err := s.runner.RunExercise(runCtx, step.Code, files, live.Append)
	live.Finish() // test phase over: release stream subscribers before this returns
	if live.Canceled() {
		if uerr := s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, out+"\n\ncanceled by user"); uerr != nil {
			log.Printf("exercise evaluate: update submission %d to failed (canceled): %v", id, uerr)
		}
		return
	}
	if err != nil {
		if uerr := s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, out+"\n\nRUNNER ERROR: "+err.Error()); uerr != nil {
			log.Printf("exercise evaluate: update submission %d to failed (runner error): %v", id, uerr)
		}
		return
	}
	passed := code == 0
	if err := s.subs.SetPassed(ctx, id, passed); err != nil {
		log.Printf("exercise evaluate: set passed=%v for submission %d: %v", passed, id, err)
	}
	if err := s.subs.UpdateSubmission(ctx, id, eval.StatusComplete, out); err != nil {
		log.Printf("exercise evaluate: update submission %d to complete: %v", id, err)
	}
	if passed {
		// sticky completion: only ever set true
		if err := s.progress.SetComplete(ctx, sub.Ref, true); err != nil {
			log.Printf("exercise evaluate: mark %s complete: %v", sub.Ref, err)
		}
	}
}

// Feedback reviews the latest completed run with the LLM. Synchronous —
// exercise code is small and the wait is a click away from the result.
//
// sub.Content holds the full materialized workspace (generated go.mod plus
// every scaffold file, editable and readonly, with the draft overlay
// applied) — the same snapshot the runner executed, kept for
// reproducibility. That's more than a reviewer needs: the prompt shows only
// the student's editable files, extracted via step.Code.Editable, so go.mod
// and other read-only scaffold noise never reach the model.
func (s *Service) Feedback(ctx context.Context, ref course.StepRef) error {
	step, err := s.codeStep(ref)
	if err != nil {
		return err
	}
	if s.llm == nil {
		return fmt.Errorf("%w: evaluation mode is locked", api.ErrInvalid)
	}
	sub, err := s.subs.LatestForStep(ctx, ref)
	if err != nil {
		return err
	}
	if sub == nil || sub.Status != eval.StatusComplete {
		return fmt.Errorf("%w: run the exercise before requesting feedback", api.ErrInvalid)
	}
	var full map[string]string
	if err := json.Unmarshal([]byte(sub.Content), &full); err != nil {
		return err
	}
	editable := make(map[string]string, len(step.Code.Editable))
	for _, name := range step.Code.Editable {
		editable[name] = full[name]
	}
	mod, _, _ := s.course.Course().Step(ref)
	passed := sub.Passed != nil && *sub.Passed
	system, user := BuildExercisePrompt(s.rubric, *mod, *step, editable, sub.TestOutput, passed)
	raw, err := s.llm.Complete(ctx, system, user)
	if err != nil {
		return fmt.Errorf("feedback: %w", err)
	}
	verdict, err := eval.ParseVerdict(raw)
	if err != nil {
		return fmt.Errorf("feedback verdict: %w", err)
	}
	_, err = s.subs.InsertEvaluation(ctx, eval.Evaluation{
		SubmissionID: sub.ID, Model: s.llm.Model(), RubricVersion: s.rubric.Version,
		Verdict: verdict, CreatedAt: s.now().UTC(),
	})
	return err
}

func (s *Service) RefForSubmission(ctx context.Context, id int64) (course.StepRef, error) {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return course.StepRef{}, fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	return sub.Ref, nil
}

// Watch subscribes to the live output of a submission's run. For a
// submission with no live run (finished or interrupted) it synthesizes an
// immediate done event so late connections degrade gracefully instead of
// erroring.
func (s *Service) Watch(ctx context.Context, id int64) (<-chan runstream.Event, error) {
	if _, err := s.subs.GetSubmission(ctx, id); err != nil {
		return nil, fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	if run, ok := s.broker.Get(runKey(id)); ok {
		return run.Subscribe(ctx), nil
	}
	ch := make(chan runstream.Event, 1)
	ch <- runstream.Event{Kind: runstream.KindDone}
	close(ch)
	return ch, nil
}

// Cancel kills the live run for a submission. Only the test-execution phase
// is cancelable; afterwards the run is no longer live and this rejects.
func (s *Service) Cancel(ctx context.Context, id int64) error {
	if _, err := s.subs.GetSubmission(ctx, id); err != nil {
		return fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	run, ok := s.broker.Get(runKey(id))
	if !ok {
		return fmt.Errorf("%w: submission %d has no live run", api.ErrInvalid, id)
	}
	run.Cancel()
	return nil
}
