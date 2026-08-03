package execx_test

import (
	"context"
	"strings"
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
