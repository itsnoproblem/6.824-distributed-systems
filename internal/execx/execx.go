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
	"syscall"
	"time"
)

const maxOutput = 64 * 1024

// Run executes cmd in dir. A non-zero exit from the command itself is a
// finding, not an error: exitCode carries it and err stays nil. Timeouts
// and failures to start return err with exitCode -1.
func Run(ctx context.Context, dir string, cmd []string, timeout time.Duration) (string, int, error) {
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
	raw, err := c.CombinedOutput()
	out := truncateTail(string(raw), maxOutput)
	if ctx.Err() == context.DeadlineExceeded {
		return out, -1, fmt.Errorf("run timed out after %s", timeout)
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

// truncateTail keeps the last max bytes — the end of output carries the failures.
func truncateTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-max:]
}
