package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func subRepo(t *testing.T) *sqlite.SubmissionRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewSubmissionRepo(db)
}

func TestSubmissionLifecycle(t *testing.T) {
	repo := subRepo(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "q1"}

	if latest, err := repo.LatestForStep(ctx, ref); err != nil || latest != nil {
		t.Fatalf("expected no submission yet: %v %v", latest, err)
	}
	id, err := repo.InsertSubmission(ctx, eval.Submission{
		Ref: ref, Kind: eval.KindQuestion, Content: "answer",
		Status: eval.StatusPending, CreatedAt: time.Now(),
	})
	if err != nil || id == 0 {
		t.Fatalf("insert: %v", err)
	}
	if err := repo.UpdateSubmission(ctx, id, eval.StatusComplete, "out"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetSubmission(ctx, id)
	if err != nil || got.Status != eval.StatusComplete || got.TestOutput != "out" ||
		got.Content != "answer" || got.Kind != eval.KindQuestion {
		t.Fatalf("get: %v %+v", err, got)
	}
	latest, err := repo.LatestForStep(ctx, ref)
	if err != nil || latest == nil || latest.ID != id {
		t.Fatalf("latest: %v %v", latest, err)
	}
}

func TestEvaluationRoundTrip(t *testing.T) {
	repo := subRepo(t)
	ctx := context.Background()
	id, err := repo.InsertSubmission(ctx, eval.Submission{
		Ref: course.StepRef{Module: "m", Step: "s"}, Kind: eval.KindLab,
		Content: "{}", Status: eval.StatusPending, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e, err := repo.EvaluationForSubmission(ctx, id); err != nil || e != nil {
		t.Fatalf("expected none: %v %v", e, err)
	}
	verdict := eval.Verdict{
		Criteria: []eval.Criterion{{Name: "Correctness", Score: 4, Justification: "ok"}},
		Summary:  "fine", NextSteps: []string{"more tests"},
	}
	if _, err := repo.InsertEvaluation(ctx, eval.Evaluation{
		SubmissionID: id, Model: "m/x", RubricVersion: "1",
		Verdict: verdict, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	e, err := repo.EvaluationForSubmission(ctx, id)
	if err != nil || e == nil || e.Verdict.Criteria[0].Score != 4 || e.RubricVersion != "1" {
		t.Fatalf("eval: %v %+v", err, e)
	}
}
