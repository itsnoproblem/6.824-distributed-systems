package exercise_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
	"github.com/itsnoproblem/mit-distributed-systems/internal/runstream"
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
	return newEnvWithLLM(t, nil)
}

func newEnvWithLLM(t *testing.T, llm eval.LLM) env {
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
	svc, err := exercise.NewService(coursefs.NewRepo(fixtureCourse()), sqlite.NewDraftsRepo(db),
		subs, progress, exercise.Workspace{}, llm, "../../content",
		exercise.WithRunAsync(func(f func()) { f() }))
	if err != nil {
		t.Fatal(err)
	}
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

// TestSaveDraftRejectsEmptyFiles covers the empty-files guard in SaveDraft:
// an empty map is invalid input, not a no-op.
func TestSaveDraftRejectsEmptyFiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.SaveDraft(ctx, ref, map[string]string{}); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("empty files map: %v", err)
	}
	if err := e.svc.SaveDraft(ctx, ref, nil); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("nil files map: %v", err)
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

// TestRunSnapshotsFullMaterializedFileSet covers the binding spec's
// reproducibility requirement (docs/superpowers/specs/2026-08-03-interactive-exercises-design.md:139):
// "Runs keep the full materialized file set in submissions.content
// (reproducibility, as v1)". A run's stored content must be the complete
// workspace — generated go.mod plus every scaffold file, editable AND
// readonly, with the draft overlay applied to the editable one — not just
// the editable map.
func TestRunSnapshotsFullMaterializedFileSet(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.SaveDraft(ctx, ref, map[string]string{
		"adder.go": "package adder // edited\n\nfunc Add(a, b int) int { return a + b }\n"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	sub, err := e.subs.LatestForStep(ctx, ref)
	if err != nil || sub == nil {
		t.Fatalf("sub=%+v err=%v", sub, err)
	}
	var files map[string]string
	if err := json.Unmarshal([]byte(sub.Content), &files); err != nil {
		t.Fatalf("content is not a JSON file map: %v (content=%q)", err, sub.Content)
	}
	meta := adderMeta()
	if !strings.Contains(files["go.mod"], "module exercise") {
		t.Fatalf("content missing generated go.mod: %+v", files)
	}
	if files["adder_test.go"] != meta.Files["adder_test.go"] {
		t.Fatalf("content missing/altered readonly file: %+v", files)
	}
	if !strings.Contains(files["adder.go"], "edited") {
		t.Fatalf("content missing draft-overlaid editable file: %+v", files)
	}
}

func TestRefForSubmission(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	sub, err := e.subs.LatestForStep(ctx, ref)
	if err != nil || sub == nil {
		t.Fatalf("sub=%+v err=%v", sub, err)
	}
	got, err := e.svc.RefForSubmission(ctx, sub.ID)
	if err != nil || got != ref {
		t.Fatalf("RefForSubmission(%d) = %+v, %v; want %+v, nil", sub.ID, got, err, ref)
	}
	if _, err := e.svc.RefForSubmission(ctx, sub.ID+9999); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("unknown submission id: %v", err)
	}
}

type fakeLLM struct{ resp string }

func (f fakeLLM) Complete(context.Context, string, string) (string, error) { return f.resp, nil }
func (f fakeLLM) Model() string                                            { return "fake/model" }

const exerciseVerdict = `{"criteria":[{"name":"Correctness","score":5,"justification":"clean"}],` +
	`"summary":"Nice work.","next_steps":["try the KV exercise"]}`

func TestFeedbackStoresEvaluation(t *testing.T) {
	e := newEnvWithLLM(t, fakeLLM{resp: exerciseVerdict}) // variant of newEnv passing the llm
	ctx := context.Background()
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Feedback(ctx, ref); err != nil {
		t.Fatal(err)
	}
	v, err := e.svc.State(ctx, ref)
	if err != nil || v.Evaluation == nil || v.Evaluation.Verdict.Summary != "Nice work." {
		t.Fatalf("evaluation: %+v err=%v", v.Evaluation, err)
	}
}

func TestFeedbackLockedMode(t *testing.T) {
	e := newEnv(t) // nil llm
	ctx := context.Background()
	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Feedback(ctx, ref); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("locked feedback err = %v", err)
	}
}

// fakeRunner is exercise.Runner for streaming/cancel tests. RunExercise
// emits chunks through sink, then blocks on release (if set) until it
// closes or ctx is canceled — honoring cancellation the way the real
// execx.Stream-backed Workspace does — and signals started (if set) once it
// begins. CheckExercise and Materialize delegate to a real exercise.Workspace
// so streaming tests don't also need to stub those.
type fakeRunner struct {
	chunks   []string
	out      string
	code     int
	err      error
	release  chan struct{}
	started  chan struct{}
	runCalls int
}

func (f *fakeRunner) RunExercise(ctx context.Context, _ *course.CodeMeta, _ map[string]string,
	sink func(string)) (string, int, error) {
	f.runCalls++
	if f.started != nil {
		close(f.started)
	}
	for _, c := range f.chunks {
		if sink != nil {
			sink(c)
		}
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return "partial output", -1, fmt.Errorf("run canceled: %w", ctx.Err())
		}
	}
	return f.out, f.code, f.err
}

func (f *fakeRunner) CheckExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string) ([]exercise.Diagnostic, error) {
	return exercise.Workspace{}.CheckExercise(ctx, meta, editable)
}

func (f *fakeRunner) Materialize(meta *course.CodeMeta, editable map[string]string) map[string]string {
	return exercise.Workspace{}.Materialize(meta, editable)
}

// newEnvWithRunner builds a Service backed by real (temp-file) sqlite repos
// and a caller-supplied Runner. By default the async run pipeline executes
// synchronously (inline); pass exercise.WithRunAsync(func(f func()) { go f() })
// to run it on a real goroutine, as the streaming/cancel tests need.
func newEnvWithRunner(t *testing.T, runner exercise.Runner, opts ...exercise.Option) env {
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
	allOpts := append([]exercise.Option{exercise.WithRunAsync(func(f func()) { f() })}, opts...)
	svc, err := exercise.NewService(coursefs.NewRepo(fixtureCourse()), sqlite.NewDraftsRepo(db),
		subs, progress, runner, nil, "../../content", allOpts...)
	if err != nil {
		t.Fatal(err)
	}
	return env{svc: svc, progress: progress, subs: subs}
}

// latestSubmissionID reads back the id of the most recent submission for ref.
func latestSubmissionID(t *testing.T, e env, ref course.StepRef) int64 {
	t.Helper()
	sub, err := e.subs.LatestForStep(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if sub == nil {
		t.Fatal("latestSubmissionID: no submission found")
	}
	return sub.ID
}

// getSubmission reads back a submission row by id.
func getSubmission(t *testing.T, e env, id int64) eval.Submission {
	t.Helper()
	sub, err := e.subs.GetSubmission(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

// TestRunStreamsChunksToWatcher verifies startRun registers with the broker
// before the pipeline goroutine runs: Watch, called right after Run returns,
// must find the run live and see its chunks.
func TestRunStreamsChunksToWatcher(t *testing.T) {
	runner := &fakeRunner{
		chunks:  []string{"chunk-1\n", "chunk-2\n"},
		out:     "chunk-1\nchunk-2\n",
		release: make(chan struct{}),
		started: make(chan struct{}),
	}
	e := newEnvWithRunner(t, runner, exercise.WithRunAsync(func(f func()) { go f() }))
	ctx := context.Background()

	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, e, ref)

	events, err := e.svc.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	<-runner.started
	close(runner.release)

	var got []string
	for ev := range events {
		if ev.Kind == runstream.KindChunk {
			got = append(got, ev.Data)
		}
	}
	if strings.Join(got, "") != "chunk-1\nchunk-2\n" {
		t.Fatalf("watched chunks = %q", got)
	}
}

// TestCancelExerciseRunRecordsCanceledOutcome: a canceled run must record
// StatusFailed with output ending in the "canceled by user" marker, and
// Passed must never be set on the canceled path.
func TestCancelExerciseRunRecordsCanceledOutcome(t *testing.T) {
	runner := &fakeRunner{
		chunks:  []string{"partial\n"},
		out:     "partial\n",
		release: make(chan struct{}), // never closed: only cancel ends the run
		started: make(chan struct{}),
	}
	e := newEnvWithRunner(t, runner, exercise.WithRunAsync(func(f func()) { go f() }))
	ctx := context.Background()

	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, e, ref)
	<-runner.started

	events, err := e.svc.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	for range events { // drain to KindDone/close: the run has fully finished
	}
	sub := getSubmission(t, e, id)
	if sub.Status != eval.StatusFailed {
		t.Fatalf("status = %s, want failed", sub.Status)
	}
	if !strings.Contains(sub.TestOutput, "canceled by user") {
		t.Fatalf("output %q missing canceled marker", sub.TestOutput)
	}
	if !strings.Contains(sub.TestOutput, "partial\n") {
		t.Fatalf("output %q lost the runner output captured before cancel", sub.TestOutput)
	}
	if sub.Passed != nil {
		t.Fatalf("Passed = %v, want nil (never set on the canceled path)", *sub.Passed)
	}
}

// TestCancelWithoutLiveRunIsInvalid: once a run has finished (the normal
// synchronous path here), it's no longer live and Cancel rejects.
func TestCancelWithoutLiveRunIsInvalid(t *testing.T) {
	e := newEnvWithRunner(t, &fakeRunner{out: "ok", code: 0})
	ctx := context.Background()

	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, e, ref)

	if err := e.svc.Cancel(ctx, id); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

// TestWatchFinishedRunSynthesizesDone: a submission with no live run (e.g.
// already complete) must degrade to an immediate KindDone rather than error.
func TestWatchFinishedRunSynthesizesDone(t *testing.T) {
	e := newEnvWithRunner(t, &fakeRunner{out: "ok", code: 0})
	ctx := context.Background()

	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, e, ref)

	events, err := e.svc.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := <-events
	if !ok || ev.Kind != runstream.KindDone {
		t.Fatalf("got %+v ok=%v, want immediate KindDone", ev, ok)
	}
}

// TestWatchUnknownSubmissionNotFound: Watch on an id with no submission row
// at all must be ErrNotFound, distinct from the "finished" synthesized case.
func TestWatchUnknownSubmissionNotFound(t *testing.T) {
	e := newEnvWithRunner(t, &fakeRunner{})
	if _, err := e.svc.Watch(context.Background(), 9999); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestStateLiveFlag: View.Live must be true only while the latest
// submission's run is actually registered in the broker.
func TestStateLiveFlag(t *testing.T) {
	runner := &fakeRunner{
		out:     "ok",
		release: make(chan struct{}),
		started: make(chan struct{}),
	}
	e := newEnvWithRunner(t, runner, exercise.WithRunAsync(func(f func()) { go f() }))
	ctx := context.Background()

	if err := e.svc.Run(ctx, ref); err != nil {
		t.Fatal(err)
	}
	<-runner.started

	v, err := e.svc.State(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Live {
		t.Fatal("running exercise should be Live")
	}

	close(runner.release)
	id := latestSubmissionID(t, e, ref)
	events, err := e.svc.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	v, err = e.svc.State(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if v.Live {
		t.Fatal("finished run must not be Live")
	}
}
