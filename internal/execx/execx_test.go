package execx_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/execx"
)

func TestRunSuccess(t *testing.T) {
	out, code, err := execx.Run(context.Background(), t.TempDir(),
		[]string{"sh", "-c", "echo hello"}, time.Minute)
	if err != nil || code != 0 || !strings.Contains(out, "hello") {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
}

func TestRunNonZeroExitIsNotError(t *testing.T) {
	out, code, err := execx.Run(context.Background(), t.TempDir(),
		[]string{"sh", "-c", "echo failing; exit 2"}, time.Minute)
	if err != nil || code != 2 || !strings.Contains(out, "failing") {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
}

func TestRunTimeout(t *testing.T) {
	_, code, err := execx.Run(context.Background(), t.TempDir(),
		[]string{"sleep", "5"}, 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") || code != -1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestRunKillsHungGrandchild(t *testing.T) {
	start := time.Now()
	_, _, err := execx.Run(context.Background(), t.TempDir(),
		[]string{"sh", "-c", "sleep 60 & wait"}, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("hung for %s — group kill / WaitDelay not working", elapsed)
	}
}

func TestRunTruncatesTail(t *testing.T) {
	out, code, err := execx.Run(context.Background(), t.TempDir(),
		[]string{"sh", "-c", "yes x | head -c 100000; echo END"}, time.Minute)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if len(out) > 70*1024 || !strings.Contains(out, "END") ||
		!strings.Contains(out, "truncated") {
		t.Fatalf("len=%d — want ≤64KB tail keeping the end", len(out))
	}
}

func TestStreamForwardsChunksIncrementally(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	var got []string
	sink := func(chunk string) {
		mu.Lock()
		got = append(got, chunk)
		mu.Unlock()
	}
	// Two echoes separated by a short sleep force at least two distinct writes.
	out, code, err := execx.Stream(context.Background(), dir,
		[]string{"sh", "-c", "echo first; sleep 0.2; echo second"}, 10*time.Second, sink)
	if err != nil || code != 0 {
		t.Fatalf("Stream: code=%d err=%v", code, err)
	}
	if out != "first\nsecond\n" {
		t.Fatalf("accumulated output = %q", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 {
		t.Fatalf("sink saw %d chunk(s), want at least 2 (incremental delivery)", len(got))
	}
	if joined := strings.Join(got, ""); joined != out {
		t.Fatalf("sink chunks %q != returned output %q", joined, out)
	}
}

func TestStreamNilSink(t *testing.T) {
	out, code, err := execx.Stream(context.Background(), t.TempDir(),
		[]string{"echo", "ok"}, 10*time.Second, nil)
	if err != nil || code != 0 || out != "ok\n" {
		t.Fatalf("got out=%q code=%d err=%v", out, code, err)
	}
}

func TestStreamContextCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, code, err := execx.Stream(ctx, t.TempDir(),
		[]string{"sh", "-c", "echo started; sleep 60"}, time.Minute, nil)
	if err == nil {
		t.Fatal("want error for canceled run")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled in chain", err)
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1", code)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("kill took %s — process group not killed", elapsed)
	}
}

func TestStreamTimeoutMatchesRunSemantics(t *testing.T) {
	out, code, err := execx.Stream(context.Background(), t.TempDir(),
		[]string{"sh", "-c", "echo before; sleep 60"}, 500*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout error", err)
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1", code)
	}
	if !strings.Contains(out, "before") {
		t.Fatalf("output before the hang lost: %q", out)
	}
}
