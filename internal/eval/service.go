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
// (The LLM evaluation branch is added with the OpenRouter provider task.)
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
	if err := s.progress.SetComplete(ctx, ref, true); err != nil {
		return err
	}
	return s.subs.UpdateSubmission(ctx, id, StatusComplete, "")
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
