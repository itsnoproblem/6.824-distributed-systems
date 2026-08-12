// Package execx runs untrusted-ish subprocesses (lab tests, exercise runs)
// with a hard timeout that survives hung children: the child gets its own
// process group and the whole group is SIGKILLed on cancel, with WaitDelay
// closing the pipe-wait afterwards. Deadlocked test binaries are the
// expected failure mode, not an edge case.
package execx

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxOutput = 64 * 1024

// Run executes cmd in dir. A non-zero exit from the command itself is a
// finding, not an error: exitCode carries it and err stays nil. Timeouts
// and failures to start return err with exitCode -1.
func Run(ctx context.Context, dir string, cmd []string, timeout time.Duration) (string, int, error) {
	return Stream(ctx, dir, cmd, timeout, nil)
}

// Stream executes like Run and additionally forwards each output chunk to
// sink as it arrives (nil sink is allowed). os/exec serializes writes when
// stdout and stderr share one writer, so sink is never called concurrently.
// A parent-context cancelation (as opposed to the timeout elapsing) returns
// an error wrapping context.Canceled.
func Stream(ctx context.Context, dir string, cmd []string, timeout time.Duration, sink func(string)) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Dir = dir
	c.WaitDelay = 5 * time.Second
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	w := &sinkWriter{sink: sink}
	c.Stdout = w
	c.Stderr = w
	err := c.Run()
	out := truncateTail(w.String(), maxOutput)
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return out, -1, fmt.Errorf("run timed out after %s", timeout)
	case context.Canceled:
		return out, -1, fmt.Errorf("run canceled: %w", context.Canceled)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, exitErr.ExitCode(), nil
	}
	if err != nil {
		return out, -1, err
	}
	return out, 0, nil
}

// sinkWriter accumulates all output and forwards each write to sink. The
// mutex guards the builder against the WaitDelay edge where exec's copier
// may still be writing as Run returns.
type sinkWriter struct {
	mu   sync.Mutex
	buf  strings.Builder
	sink func(string)
}

func (w *sinkWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	w.mu.Unlock()
	if w.sink != nil {
		w.sink(string(p))
	}
	return len(p), nil
}

func (w *sinkWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// truncateTail keeps the last max bytes — the end of output carries the failures.
func truncateTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-max:]
}
