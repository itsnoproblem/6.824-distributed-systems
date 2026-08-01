package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type CourseRepo interface{ Course() *course.Course }

type ProgressMarker interface {
	SetComplete(ctx context.Context, ref course.StepRef, done bool) error
}

// LLM is the evaluation model provider; nil unlocks nothing — evaluation
// mode is locked and submissions are stored without review.
type LLM interface {
	Complete(ctx context.Context, system, user string) (string, error)
	Model() string
}

// LabRepo abstracts the student's mounted lab repository (implemented by
// FSLabRepo; may be nil in tests that don't exercise labs).
type LabRepo interface {
	Snapshot(workdir string, globs []string) (map[string]string, error)
	RunTests(ctx context.Context, workdir string, cmd []string, timeout time.Duration) (string, error)
}

type SubmissionRepo interface {
	InsertSubmission(ctx context.Context, s Submission) (int64, error)
	UpdateSubmission(ctx context.Context, id int64, status Status, testOutput string) error
	GetSubmission(ctx context.Context, id int64) (Submission, error)
	LatestForStep(ctx context.Context, ref course.StepRef) (*Submission, error)
	InsertEvaluation(ctx context.Context, e Evaluation) (int64, error)
	EvaluationForSubmission(ctx context.Context, submissionID int64) (*Evaluation, error)
}

type StepEvalView struct {
	Enabled    bool
	Step       course.Step
	Submission *Submission
	Evaluation *Evaluation
}

type Service struct {
	course      CourseRepo
	subs        SubmissionRepo
	progress    ProgressMarker
	llm         LLM
	lab         LabRepo
	rubrics     map[string]Rubric
	guidanceDir string
	runAsync    func(func())
	now         func() time.Time
}

type Option func(*Service)

// WithRunAsync overrides how lab evaluations are scheduled; tests run them inline.
func WithRunAsync(f func(func())) Option { return func(s *Service) { s.runAsync = f } }

func NewService(c CourseRepo, subs SubmissionRepo, p ProgressMarker, llm LLM, lab LabRepo,
	contentDir string, opts ...Option) (*Service, error) {
	rubrics := map[string]Rubric{}
	for _, name := range []string{"question", "lab"} {
		r, err := LoadRubric(filepath.Join(contentDir, "rubric", name+".md"))
		if err != nil {
			return nil, fmt.Errorf("load rubric: %w", err)
		}
		rubrics[name] = r
	}
	s := &Service{
		course: c, subs: subs, progress: p, llm: llm, lab: lab,
		rubrics: rubrics, guidanceDir: filepath.Join(contentDir, "guidance"),
		runAsync: func(f func()) { go f() }, now: time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

func (s *Service) Enabled() bool { return s.llm != nil }

// SubmitAnswer stores a reading-question answer and marks the step complete.
// In locked mode (nil LLM) it does so without review. Otherwise it hands off
// to evaluateQuestion, which owns the submission's status from there.
func (s *Service) SubmitAnswer(ctx context.Context, ref course.StepRef, answer string) error {
	_, step, ok := s.course.Course().Step(ref)
	if !ok || step.Type != course.StepQuestion {
		return fmt.Errorf("%w: question step %s", api.ErrNotFound, ref)
	}
	if strings.TrimSpace(answer) == "" {
		return fmt.Errorf("%w: answer is empty", api.ErrInvalid)
	}
	id, err := s.subs.InsertSubmission(ctx, Submission{
		Ref: ref, Kind: KindQuestion, Content: answer,
		Status: StatusPending, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}
	if s.llm == nil {
		// Locked mode: status flips to complete before progress is marked,
		// so a late DB failure never leaves progress "done" while the
		// submission itself is stuck pending.
		if err := s.subs.UpdateSubmission(ctx, id, StatusComplete, ""); err != nil {
			return err
		}
		return s.progress.SetComplete(ctx, ref, true)
	}
	if err := s.progress.SetComplete(ctx, ref, true); err != nil {
		return err
	}
	return s.evaluateQuestion(ctx, id)
}

// evaluateQuestion runs the synchronous question-evaluation pipeline; LLM
// failures land on the submission as StatusFailed, never as an HTTP error.
func (s *Service) evaluateQuestion(ctx context.Context, id int64) error {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return err
	}
	mod, step, ok := s.course.Course().Step(sub.Ref)
	if !ok {
		return fmt.Errorf("%w: step %s", api.ErrNotFound, sub.Ref)
	}
	if err := s.subs.UpdateSubmission(ctx, id, StatusRunning, ""); err != nil {
		return err
	}
	// From here on, persist outcomes on a context that survives cancelation
	// of the inbound request. If the HTTP client disconnects during the
	// (long) LLM call, r.Context() is canceled; without this, the
	// failure-recording write below would itself fail and strand the
	// submission at StatusRunning forever, with no recovery path since
	// Retry only accepts StatusFailed. The LLM call itself intentionally
	// keeps the original ctx so a client disconnect still aborts the
	// in-flight model request.
	persistCtx := context.WithoutCancel(ctx)
	rubric := s.rubrics["question"]
	system, user := BuildQuestionPrompt(rubric, *mod, *step, sub.Content)
	raw, err := s.llm.Complete(ctx, system, user)
	if err != nil {
		return s.subs.UpdateSubmission(persistCtx, id, StatusFailed, "LLM error: "+err.Error())
	}
	verdict, err := ParseVerdict(raw)
	if err != nil {
		return s.subs.UpdateSubmission(persistCtx, id, StatusFailed, "verdict parse error: "+err.Error())
	}
	if _, err := s.subs.InsertEvaluation(persistCtx, Evaluation{
		SubmissionID: id, Model: s.llm.Model(), RubricVersion: rubric.Version,
		Verdict: verdict, CreatedAt: s.now().UTC(),
	}); err != nil {
		return err
	}
	return s.subs.UpdateSubmission(persistCtx, id, StatusComplete, "")
}

// SubmitLab snapshots the lab code, records the submission, marks the step
// complete, and schedules the async run-tests-then-evaluate pipeline.
func (s *Service) SubmitLab(ctx context.Context, ref course.StepRef) error {
	_, step, ok := s.course.Course().Step(ref)
	if !ok || step.Type != course.StepSubmit || step.Eval == nil {
		return fmt.Errorf("%w: submit step %s", api.ErrNotFound, ref)
	}
	if s.lab == nil {
		return fmt.Errorf("%w: no lab repository configured (set LAB_REPO_DIR)", api.ErrInvalid)
	}
	files, err := s.lab.Snapshot(step.Eval.Workdir, step.Eval.Globs)
	if err != nil {
		return fmt.Errorf("%w: snapshot: %v", api.ErrInvalid, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("%w: no files matched %v under %s",
			api.ErrInvalid, step.Eval.Globs, step.Eval.Workdir)
	}
	content, err := json.Marshal(files)
	if err != nil {
		return err
	}
	id, err := s.subs.InsertSubmission(ctx, Submission{
		Ref: ref, Kind: KindLab, Content: string(content),
		Status: StatusPending, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}
	if err := s.progress.SetComplete(ctx, ref, true); err != nil {
		return err
	}
	s.runAsync(func() { s.evaluateLab(id) })
	return nil
}

// evaluateLab runs in the background with its own context; every failure
// lands on the submission row, never in a log the user can't see.
func (s *Service) evaluateLab(id int64) {
	ctx := context.Background()
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return
	}
	mod, step, ok := s.course.Course().Step(sub.Ref)
	if !ok || step.Eval == nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, "step no longer exists in content")
		return
	}
	_ = s.subs.UpdateSubmission(ctx, id, StatusRunning, "")
	out, err := s.lab.RunTests(ctx, step.Eval.Workdir, step.Eval.TestCmd, step.Eval.Timeout)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nRUNNER ERROR: "+err.Error())
		return
	}
	if s.llm == nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusComplete, out)
		return
	}
	var files map[string]string
	if err := json.Unmarshal([]byte(sub.Content), &files); err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nSNAPSHOT DECODE ERROR: "+err.Error())
		return
	}
	rubric := s.rubrics["lab"]
	system, user := BuildLabPrompt(rubric, s.loadGuidance(sub.Ref.Module), *mod, *step, files, out)
	raw, err := s.llm.Complete(ctx, system, user)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nLLM ERROR: "+err.Error())
		return
	}
	verdict, err := ParseVerdict(raw)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nVERDICT PARSE ERROR: "+err.Error())
		return
	}
	if _, err := s.subs.InsertEvaluation(ctx, Evaluation{
		SubmissionID: id, Model: s.llm.Model(), RubricVersion: rubric.Version,
		Verdict: verdict, CreatedAt: s.now().UTC(),
	}); err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nSTORE ERROR: "+err.Error())
		return
	}
	_ = s.subs.UpdateSubmission(ctx, id, StatusComplete, out)
}

// loadGuidance returns per-module evaluator guidance, or "" when none is authored.
func (s *Service) loadGuidance(moduleSlug string) string {
	raw, err := os.ReadFile(filepath.Join(s.guidanceDir, moduleSlug+".md"))
	if err != nil {
		return ""
	}
	return string(raw)
}

func (s *Service) RefForSubmission(ctx context.Context, id int64) (course.StepRef, error) {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return course.StepRef{}, fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	return sub.Ref, nil
}

// Retry re-evaluates a failed submission: synchronously for questions,
// via the async pipeline for labs. Question retries require the LLM (their
// only failure mode is an LLM/verdict error), so locked mode rejects them.
// Lab retries proceed regardless of LLM presence: a lab's only possible
// locked-mode failure is a runner error (timeout/exec), which re-running
// the test runner alone can resolve; evaluateLab already handles a nil LLM
// by completing with test output only.
func (s *Service) Retry(ctx context.Context, id int64) error {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	if sub.Status != StatusFailed {
		return fmt.Errorf("%w: submission %d is %s, not failed", api.ErrInvalid, id, sub.Status)
	}
	switch sub.Kind {
	case KindQuestion:
		if s.llm == nil {
			return fmt.Errorf("%w: evaluation mode is locked", api.ErrInvalid)
		}
		return s.evaluateQuestion(ctx, id)
	default:
		s.runAsync(func() { s.evaluateLab(id) })
		return nil
	}
}

func (s *Service) StepState(ctx context.Context, ref course.StepRef) (StepEvalView, error) {
	_, step, ok := s.course.Course().Step(ref)
	if !ok {
		return StepEvalView{}, fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	sub, err := s.subs.LatestForStep(ctx, ref)
	if err != nil {
		return StepEvalView{}, err
	}
	view := StepEvalView{Enabled: s.Enabled(), Step: *step, Submission: sub}
	if sub != nil {
		if view.Evaluation, err = s.subs.EvaluationForSubmission(ctx, sub.ID); err != nil {
			return StepEvalView{}, err
		}
	}
	return view, nil
}
