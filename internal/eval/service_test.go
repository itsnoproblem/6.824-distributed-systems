package eval_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/runstream"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// labRef is the fixture course's submittable lab step.
var labRef = course.StepRef{Module: "m2", Step: "lab1"}

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

// stubLabRepo is eval.LabRepo for tests. Snapshot returns files, or a
// default single-file map when files is unset. RunTests either runs the
// legacy fail-once-then-succeed script (legacyFailOnce, used by the
// retry-flow test below) or emits chunks through sink and returns out/err —
// blocking on release if set, until release closes or ctx is canceled — and
// signaling started (if set) once it begins. This lets streaming/cancel
// tests drive a real goroutine while the retry test keeps its simple flow.
type stubLabRepo struct {
	legacyFailOnce bool
	runCalls       int

	files   map[string]string
	chunks  []string
	out     string
	err     error
	release chan struct{}
	started chan struct{}
}

func (s *stubLabRepo) Snapshot(_ string, _ []string) (map[string]string, error) {
	if s.files != nil {
		return s.files, nil
	}
	return map[string]string{"src/x/x.go": "package x"}, nil
}

func (s *stubLabRepo) RunTests(ctx context.Context, _ string, _ []string,
	_ time.Duration, sink func(string)) (string, error) {
	s.runCalls++
	if s.legacyFailOnce {
		if s.runCalls == 1 {
			return "", errors.New("boom")
		}
		return "PASS ok", nil
	}
	if s.started != nil {
		close(s.started)
	}
	for _, c := range s.chunks {
		if sink != nil {
			sink(c)
		}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return s.out, fmt.Errorf("run canceled: %w", ctx.Err())
		}
	}
	return s.out, s.err
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

// newEnvWithLab builds a Service backed by real (temp-file) sqlite repos.
// By default the async lab pipeline runs synchronously (inline); pass
// eval.WithRunAsync(func(f func()) { go f() }) to run it on a real
// goroutine, as the streaming/cancel tests need.
func newEnvWithLab(t *testing.T, llm eval.LLM, lab eval.LabRepo, opts ...eval.Option) testEnv {
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
	allOpts := append([]eval.Option{eval.WithRunAsync(func(f func()) { f() })}, opts...)
	svc, err := eval.NewService(coursefs.NewRepo(fixtureCourse()), subs, progress, llm, lab,
		"../../content", allOpts...)
	if err != nil {
		t.Fatal(err)
	}
	return testEnv{svc: svc, progress: progress, subs: subs}
}

// latestSubmissionID reads back the id of the most recent submission for ref.
func latestSubmissionID(t *testing.T, env testEnv, ref course.StepRef) int64 {
	t.Helper()
	sub, err := env.subs.LatestForStep(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if sub == nil {
		t.Fatal("latestSubmissionID: no submission found")
	}
	return sub.ID
}

// getSubmission reads back a submission row by id.
func getSubmission(t *testing.T, env testEnv, id int64) eval.Submission {
	t.Helper()
	sub, err := env.subs.GetSubmission(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return sub
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
	lab := &stubLabRepo{legacyFailOnce: true}
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

// TestSubmitLabStreamsChunksToWatcher verifies startLabRun registers with
// the broker before the pipeline goroutine runs: Watch, called right after
// SubmitLab returns, must find the run live and see its chunks.
func TestSubmitLabStreamsChunksToWatcher(t *testing.T) {
	lab := &stubLabRepo{
		chunks:  []string{"chunk-1\n", "chunk-2\n"},
		out:     "chunk-1\nchunk-2\n",
		release: make(chan struct{}),
		started: make(chan struct{}),
	}
	env := newEnvWithLab(t, nil, lab, eval.WithRunAsync(func(f func()) { go f() }))
	ctx := context.Background()

	if err := env.svc.SubmitLab(ctx, labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, env, labRef)

	events, err := env.svc.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	<-lab.started
	close(lab.release)

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

// TestCancelLabRunRecordsCanceledOutcome: a canceled run must record
// StatusFailed with output ending in the "canceled by user" marker, not a
// new status value.
func TestCancelLabRunRecordsCanceledOutcome(t *testing.T) {
	lab := &stubLabRepo{
		chunks:  []string{"partial\n"},
		out:     "partial\n",
		release: make(chan struct{}), // never closed: only cancel ends the run
		started: make(chan struct{}),
	}
	env := newEnvWithLab(t, nil, lab, eval.WithRunAsync(func(f func()) { go f() }))
	ctx := context.Background()

	if err := env.svc.SubmitLab(ctx, labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, env, labRef)
	<-lab.started

	events, err := env.svc.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.svc.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	for range events { // drain to KindDone/close: the run has fully finished
	}
	sub := getSubmission(t, env, id)
	if sub.Status != eval.StatusFailed {
		t.Fatalf("status = %s, want failed", sub.Status)
	}
	if !strings.Contains(sub.TestOutput, "canceled by user") {
		t.Fatalf("output %q missing canceled marker", sub.TestOutput)
	}
	if !strings.Contains(sub.TestOutput, "partial\n") {
		t.Fatalf("output %q lost the runner output captured before cancel", sub.TestOutput)
	}
}

// TestCancelWithoutLiveRunIsInvalid: once a submission has finished (the
// normal synchronous path here), it's no longer live and Cancel rejects.
func TestCancelWithoutLiveRunIsInvalid(t *testing.T) {
	env := newEnvWithLab(t, nil, &stubLabRepo{out: "ok"})
	ctx := context.Background()

	if err := env.svc.SubmitLab(ctx, labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, env, labRef)

	if err := env.svc.Cancel(ctx, id); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

// TestWatchFinishedRunSynthesizesDone: a submission with no live run (e.g.
// already complete) must degrade to an immediate KindDone rather than error.
func TestWatchFinishedRunSynthesizesDone(t *testing.T) {
	env := newEnvWithLab(t, nil, &stubLabRepo{out: "ok"})
	ctx := context.Background()

	if err := env.svc.SubmitLab(ctx, labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, env, labRef)

	events, err := env.svc.Watch(ctx, id)
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
	env := newEnvWithLab(t, nil, &stubLabRepo{})
	if _, err := env.svc.Watch(context.Background(), 9999); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestStepStateLiveFlag: Live must be true only while the latest
// submission's run is actually registered in the broker.
func TestStepStateLiveFlag(t *testing.T) {
	lab := &stubLabRepo{
		out:     "ok",
		release: make(chan struct{}),
		started: make(chan struct{}),
	}
	env := newEnvWithLab(t, nil, lab, eval.WithRunAsync(func(f func()) { go f() }))
	ctx := context.Background()

	if err := env.svc.SubmitLab(ctx, labRef); err != nil {
		t.Fatal(err)
	}
	<-lab.started

	view, err := env.svc.StepState(ctx, labRef)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Live {
		t.Fatal("running registered lab should be Live")
	}

	close(lab.release)
	id := latestSubmissionID(t, env, labRef)
	events, err := env.svc.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	view, err = env.svc.StepState(ctx, labRef)
	if err != nil {
		t.Fatal(err)
	}
	if view.Live {
		t.Fatal("finished run must not be Live")
	}
}
