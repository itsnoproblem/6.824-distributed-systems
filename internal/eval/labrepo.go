package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/execx"
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

// RunTests executes the step's test command in the lab repo via execx,
// forwarding output chunks to sink as they arrive (nil sink allowed): test
// failures are findings (non-zero exit ⇒ nil error); timeouts, cancelation,
// and failures to execute return err.
func (l FSLabRepo) RunTests(ctx context.Context, workdir string, cmd []string,
	timeout time.Duration, sink func(string)) (string, error) {
	out, _, err := execx.Stream(ctx, filepath.Join(l.Dir, workdir), cmd, timeout, sink)
	return out, err
}
