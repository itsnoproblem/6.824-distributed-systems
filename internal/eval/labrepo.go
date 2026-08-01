package eval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// FSLabRepo reads and tests the student's lab repository on local disk
// (the Docker-mounted clone of the course lab skeleton).
type FSLabRepo struct{ Dir string }

func (l FSLabRepo) Snapshot(workdir string, globs []string) (map[string]string, error) {
	if l.Dir == "" {
		return nil, fmt.Errorf("lab repository is not configured (set LAB_REPO_DIR)")
	}
	out := map[string]string{}
	base := filepath.Join(l.Dir, workdir)
	for _, g := range globs {
		matches, err := filepath.Glob(filepath.Join(base, g))
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", g, err)
		}
		for _, m := range matches {
			raw, err := os.ReadFile(m)
			if err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(l.Dir, m)
			if err != nil {
				rel = m
			}
			out[rel] = string(raw)
		}
	}
	return out, nil
}

// RunTests executes the step's test command in the lab repo. A non-zero exit
// from the tests themselves is a finding, not an error; only timeouts and
// failures to execute at all return err.
func (l FSLabRepo) RunTests(ctx context.Context, workdir string, cmd []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Dir = filepath.Join(l.Dir, workdir)
	// go test spawns the compiled test binary as a child process; a hung or
	// deadlocked test (e.g. a stuck Raft goroutine) can outlive `go test`
	// itself once that parent is killed, holding the stdout/stderr pipes
	// open so CombinedOutput blocks past the timeout. Run the whole tree in
	// its own process group so cancellation can kill it all at once, and cap
	// how long we wait for the pipes to close after that kill.
	c.WaitDelay = 5 * time.Second
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	raw, err := c.CombinedOutput()
	out := truncateTail(string(raw), 64*1024)
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("test run timed out after %s", timeout)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, nil
	}
	return out, err
}

// truncateTail keeps the last max bytes — the end of test output carries the failures.
func truncateTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-max:]
}
