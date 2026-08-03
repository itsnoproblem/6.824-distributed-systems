package exercise_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

func fixtureCourse() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Module One", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "r1", Title: "Read", Type: course.StepReading},
			{Slug: "c1", Title: "Fix adder", Type: course.StepCode, Code: adderMeta()},
		}},
	}}
}

type env struct {
	svc      *exercise.Service
	progress *sqlite.ProgressRepo
	subs     *sqlite.SubmissionRepo
}

func newEnv(t *testing.T) env {
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
	svc := exercise.NewService(coursefs.NewRepo(fixtureCourse()), sqlite.NewDraftsRepo(db),
		subs, progress, exercise.Workspace{},
		exercise.WithRunAsync(func(f func()) { f() }))
	return env{svc: svc, progress: progress, subs: subs}
}

var ref = course.StepRef{Module: "m1", Step: "c1"}

func TestStateScaffoldThenDraft(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	v, err := e.svc.State(ctx, ref)
	if err != nil || v.HasDraft || len(v.Files) != 2 {
		t.Fatalf("state: %+v err=%v", v, err)
	}
	if v.Files[0].Name != "adder.go" || v.Files[0].Readonly ||
		v.Files[1].Name != "adder_test.go" || !v.Files[1].Readonly {
		t.Fatalf("file order/flags: %+v", v.Files)
	}
	if err := e.svc.SaveDraft(ctx, ref, map[string]string{"adder.go": "package adder // edited"}); err != nil {
		t.Fatal(err)
	}
	v, _ = e.svc.State(ctx, ref)
	if !v.HasDraft || !strings.Contains(v.Files[0].Content, "edited") {
		t.Fatalf("draft not reflected: %+v", v.Files[0])
	}
	if err := e.svc.ResetDraft(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if v, _ = e.svc.State(ctx, ref); v.HasDraft {
		t.Fatal("reset should drop the draft")
	}
}

func TestSaveDraftValidates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.SaveDraft(ctx, course.StepRef{Module: "m1", Step: "r1"}, map[string]string{"a": "b"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("non-code step: %v", err)
	}
	if err := e.svc.SaveDraft(ctx, ref, map[string]string{"adder_test.go": "hax"}); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("read-only file must be rejected: %v", err)
	}
}

func TestRunFailThenPassMarksComplete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// scaffold is buggy: run completes with passed=false, no progress
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	sub, _ := e.subs.LatestForStep(ctx, ref)
	if sub == nil || sub.Status != eval.StatusComplete || sub.Passed == nil || *sub.Passed {
		t.Fatalf("buggy run: %+v", sub)
	}
	if done, _ := e.progress.Completed(ctx); len(done) != 0 {
		t.Fatal("failing run must not complete the step")
	}
	// fix it: passed=true, step complete
	if err := e.svc.SaveDraft(ctx, ref, map[string]string{
		"adder.go": "package adder\n\nfunc Add(a, b int) int { return a + b }\n"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	sub, _ = e.subs.LatestForStep(ctx, ref)
	if sub.Passed == nil || !*sub.Passed {
		t.Fatalf("fixed run: %+v", sub)
	}
	if done, _ := e.progress.Completed(ctx); len(done) != 1 {
		t.Fatal("passing run must complete the step")
	}
}

// TestRunPassThenFailStaysComplete covers sticky completion: a later
// failing run on a step that has already passed once must never revoke
// progress. SetComplete is only ever called with true from evaluate.
func TestRunPassThenFailStaysComplete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.SaveDraft(ctx, ref, map[string]string{
		"adder.go": "package adder\n\nfunc Add(a, b int) int { return a + b }\n"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if done, _ := e.progress.Completed(ctx); len(done) != 1 {
		t.Fatal("passing run must complete the step")
	}
	// break it again and re-run
	if err := e.svc.SaveDraft(ctx, ref, map[string]string{
		"adder.go": "package adder\n\nfunc Add(a, b int) int { return a - b }\n"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	sub, _ := e.subs.LatestForStep(ctx, ref)
	if sub.Passed == nil || *sub.Passed {
		t.Fatalf("second run should fail: %+v", sub)
	}
	if done, _ := e.progress.Completed(ctx); len(done) != 1 {
		t.Fatal("a later failing run must never revoke completion")
	}
}

// TestStateEmptyDraftIsScaffoldOnly covers the empty-draft edge case: with
// no draft saved, State must serve the scaffold files verbatim and readonly
// files must always come from the scaffold, never from any draft entry
// (drafts can only ever contain editable-file keys, enforced by SaveDraft).
func TestStateEmptyDraftIsScaffoldOnly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	v, err := e.svc.State(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if v.HasDraft {
		t.Fatal("no draft saved: HasDraft must be false")
	}
	meta := adderMeta()
	if v.Files[0].Content != meta.Files["adder.go"] {
		t.Fatalf("editable file must be scaffold verbatim: %q", v.Files[0].Content)
	}
	if v.Files[1].Content != meta.Files["adder_test.go"] {
		t.Fatalf("readonly file must be scaffold verbatim: %q", v.Files[1].Content)
	}
}
