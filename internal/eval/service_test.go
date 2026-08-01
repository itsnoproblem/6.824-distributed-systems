package eval_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

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
	}}
}

type testEnv struct {
	svc      *eval.Service
	progress *sqlite.ProgressRepo
	subs     *sqlite.SubmissionRepo
}

func newEnv(t *testing.T, llm eval.LLM) testEnv {
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
	svc, err := eval.NewService(coursefs.NewRepo(fixtureCourse()), subs, progress, llm, nil,
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
