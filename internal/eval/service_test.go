package eval_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

func TestLoadRubric(t *testing.T) {
	r, err := eval.LoadRubric("../../content/rubric/question.md")
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "1" || r.Body == "" {
		t.Fatalf("rubric: %+v", r)
	}
}

func fixtureCourse() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Module One", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "r1", Title: "Read", Type: course.StepReading},
			{Slug: "q1", Title: "Question", Type: course.StepQuestion, Question: "Why?"},
		}},
		{Slug: "m2", Title: "Module Two", Kind: course.KindLab, Order: 2, Steps: []course.Step{
			{Slug: "lab1", Title: "Lab", Type: course.StepSubmit, Eval: &course.EvalMeta{
				Workdir: "src/x", Globs: []string{"*.go"}, TestCmd: []string{"go", "test"},
				Timeout: time.Minute,
			}},
		}},
	}}
}

// stubLabRepo is a minimal eval.LabRepo: Snapshot always succeeds, and
// RunTests fails once before succeeding, so tests can drive a submission
// through StatusFailed and back via Retry.
type stubLabRepo struct{ runCalls int }

func (s *stubLabRepo) Snapshot(_ string, _ []string) (map[string]string, error) {
	return map[string]string{"src/x/x.go": "package x"}, nil
}

func (s *stubLabRepo) RunTests(_ context.Context, _ string, _ []string, _ time.Duration) (string, error) {
	s.runCalls++
	if s.runCalls == 1 {
		return "", errors.New("boom")
	}
	return "PASS ok", nil
}

type testEnv struct {
	svc      *eval.Service
	progress *sqlite.ProgressRepo
	subs     *sqlite.SubmissionRepo
}

func newEnv(t *testing.T, llm eval.LLM) testEnv {
	t.Helper()
	return newEnvWithLab(t, llm, nil)
}

func newEnvWithLab(t *testing.T, llm eval.LLM, lab eval.LabRepo) testEnv {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	progress := sqlite.NewProgressRepo(db)
	subs := sqlite.NewSubmissionRepo(db)
	svc, err := eval.NewService(coursefs.NewRepo(fixtureCourse()), subs, progress, llm, lab,
		"../../content", eval.WithRunAsync(func(f func()) { f() }))
	if err != nil {
		t.Fatal(err)
	}
	return testEnv{svc: svc, progress: progress, subs: subs}
}

func TestSubmitAnswerLocked(t *testing.T) {
	env := newEnv(t, nil)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "q1"}

	if env.svc.Enabled() {
		t.Fatal("nil LLM must mean locked")
	}
	if err := env.svc.SubmitAnswer(ctx, ref, "because"); err != nil {
		t.Fatal(err)
	}
	view, err := env.svc.StepState(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if view.Submission == nil || view.Submission.Status != eval.StatusComplete ||
		view.Submission.Content != "because" || view.Evaluation != nil {
		t.Fatalf("view: %+v", view)
	}
	done, _ := env.progress.Completed(ctx)
	if _, ok := done[ref]; !ok {
		t.Fatal("answer should mark step complete")
	}
}

// cancelingLLM simulates an HTTP client disconnecting mid-request: the
// context passed to SubmitAnswer is canceled from within Complete, mimicking
// r.Context() being canceled while the LLM call is in flight.
type cancelingLLM struct{ cancel context.CancelFunc }

func (f cancelingLLM) Complete(_ context.Context, _, _ string) (string, error) {
	f.cancel()
	return "", context.Canceled
}
func (f cancelingLLM) Model() string { return "fake/model" }

func TestSubmitAnswerRecordsFailureAfterRequestCancelation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := newEnv(t, cancelingLLM{cancel: cancel})
	ref := course.StepRef{Module: "m1", Step: "q1"}

	if err := env.svc.SubmitAnswer(ctx, ref, "because"); err != nil {
		t.Fatalf("SubmitAnswer returned error: %v", err)
	}

	view, err := env.svc.StepState(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if view.Submission == nil || view.Submission.Status != eval.StatusFailed {
		t.Fatalf("submission = %+v, want StatusFailed", view.Submission)
	}
	if !strings.Contains(view.Submission.TestOutput, "LLM error") {
		t.Fatalf("test output = %q, want it to contain %q", view.Submission.TestOutput, "LLM error")
	}
}

func TestSubmitAnswerValidates(t *testing.T) {
	env := newEnv(t, nil)
	ctx := context.Background()
	if err := env.svc.SubmitAnswer(ctx, course.StepRef{Module: "m1", Step: "r1"}, "x"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("non-question step err = %v", err)
	}
	if err := env.svc.SubmitAnswer(ctx, course.StepRef{Module: "m1", Step: "q1"}, "  "); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("empty answer err = %v", err)
	}
}

// TestLockedModeLabRetry: locked mode's only possible lab failure is a
// runner error, and re-running the test runner alone can resolve it, so
// Retry must work for labs even with a nil LLM.
func TestLockedModeLabRetry(t *testing.T) {
	lab := &stubLabRepo{}
	env := newEnvWithLab(t, nil, lab)
	ctx := context.Background()
	ref := course.StepRef{Module: "m2", Step: "lab1"}

	if err := env.svc.SubmitLab(ctx, ref); err != nil {
		t.Fatalf("SubmitLab returned error: %v", err)
	}
	view, err := env.svc.StepState(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if view.Submission == nil || view.Submission.Status != eval.StatusFailed ||
		!strings.Contains(view.Submission.TestOutput, "RUNNER ERROR") {
		t.Fatalf("submission after failed run = %+v", view.Submission)
	}

	if err := env.svc.Retry(ctx, view.Submission.ID); err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}

	view, err = env.svc.StepState(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if view.Submission == nil || view.Submission.Status != eval.StatusComplete ||
		!strings.Contains(view.Submission.TestOutput, "PASS ok") {
		t.Fatalf("submission after retry = %+v", view.Submission)
	}
}

// TestLockedModeQuestionRetryRejected: unlike labs, a question's only
// failure mode is the LLM itself, so locked mode must keep rejecting
// question retries.
func TestLockedModeQuestionRetryRejected(t *testing.T) {
	env := newEnv(t, nil)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "q1"}

	id, err := env.subs.InsertSubmission(ctx, eval.Submission{
		Ref: ref, Kind: eval.KindQuestion, Content: "because",
		Status: eval.StatusFailed, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := env.svc.Retry(ctx, id); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("Retry err = %v, want ErrInvalid", err)
	}
}
