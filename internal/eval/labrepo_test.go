package eval_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

func TestSnapshot(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	files, err := repo.Snapshot("src/hello", []string{"*.go"})
	if err != nil {
		t.Fatal(err)
	}
	src, ok := files["src/hello/hello_test.go"]
	if !ok || !strings.Contains(src, "TestAlwaysPasses") {
		t.Fatalf("files: %v", files)
	}
}

func TestRunTestsPasses(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	out, err := repo.RunTests(context.Background(), "src/hello",
		[]string{"go", "test"}, time.Minute, nil)
	if err != nil {
		t.Fatalf("err = %v, out = %q", err, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("out = %q", out)
	}
}

func TestRunTestsStreamsToSink(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	var chunks []string
	out, err := repo.RunTests(context.Background(), "src/hello",
		[]string{"go", "test"}, time.Minute, func(s string) { chunks = append(chunks, s) })
	if err != nil {
		t.Fatalf("err = %v, out = %q", err, out)
	}
	if len(chunks) == 0 {
		t.Fatal("expected sink to receive at least one chunk")
	}
	if !strings.Contains(strings.Join(chunks, ""), "ok") {
		t.Fatalf("sink chunks = %v", chunks)
	}
}

func TestRunTestsTimeout(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	_, err := repo.RunTests(context.Background(), "src/hello",
		[]string{"sleep", "5"}, 100*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}

// TestRunTestsKillsHungGrandchild reproduces a hung lab test binary: `go
// test` forks a child that outlives it. Here `sh -c "sleep 60 & wait"`
// backgrounds a grandchild and waits on it, so killing only the `sh`
// process (the old behavior) leaves the sleep running and holding the
// output pipe open, blocking CombinedOutput well past the timeout. With
// process-group kill + WaitDelay, RunTests must return promptly.
func TestRunTestsKillsHungGrandchild(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	start := time.Now()
	_, err := repo.RunTests(context.Background(), "src/hello",
		[]string{"sh", "-c", "sleep 60 & wait"}, 300*time.Millisecond, nil)
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if elapsed > 15*time.Second {
		t.Fatalf("RunTests took %s, want well under 15s", elapsed)
	}
}
