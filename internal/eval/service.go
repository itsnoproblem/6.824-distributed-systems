package eval

import (
	"context"
	"fmt"
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
// FSLabRepo in the lab-evaluation task; nil until then).
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

func (s *Service) RefForSubmission(ctx context.Context, id int64) (course.StepRef, error) {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return course.StepRef{}, fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	return sub.Ref, nil
}

// Retry re-evaluates a failed submission. (Lab retries arrive with the lab
// pipeline task; until then only questions are retryable.)
func (s *Service) Retry(ctx context.Context, id int64) error {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	if sub.Status != StatusFailed {
		return fmt.Errorf("%w: submission %d is %s, not failed", api.ErrInvalid, id, sub.Status)
	}
	if s.llm == nil {
		return fmt.Errorf("%w: evaluation mode is locked", api.ErrInvalid)
	}
	if sub.Kind != KindQuestion {
		return fmt.Errorf("%w: lab retry requires the lab pipeline", api.ErrInvalid)
	}
	return s.evaluateQuestion(ctx, id)
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
