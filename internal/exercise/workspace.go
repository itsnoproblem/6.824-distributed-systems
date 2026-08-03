package exercise

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/execx"
)

const checkTimeout = 30 * time.Second

// Workspace materializes throwaway exercise dirs and runs the Go toolchain
// in them. Stateless; every call builds a fresh temp dir.
type Workspace struct{}

// toSet converts a file-name slice into a membership set.
func toSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// buildFiles computes the full materialized file set for a step: a
// generated go.mod plus every scaffold file, with editable scaffold files
// overlaid by the matching entries in overlay. Overlay keys outside
// meta.Editable are ignored — the client can never rewrite the test
// harness. The result is the complete workspace contents, suitable for
// writing to disk (materialize) or for snapshotting into
// submissions.content (Materialize) without touching disk.
func buildFiles(meta *course.CodeMeta, overlay map[string]string) map[string]string {
	editableSet := toSet(meta.Editable)
	files := map[string]string{"go.mod": "module exercise\n\ngo 1.25\n"}
	for name, src := range meta.Files {
		files[name] = src
	}
	for name, src := range overlay {
		if editableSet[name] {
			files[name] = src
		}
	}
	return files
}

// materialize writes the full materialized file set (see buildFiles) to a
// fresh temp dir.
func (Workspace) materialize(meta *course.CodeMeta, editable map[string]string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "exercise-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	for name, src := range buildFiles(meta, editable) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("materialize %s: %w", name, err)
		}
	}
	return dir, cleanup, nil
}

// Materialize returns the full materialized file set — generated go.mod
// plus every scaffold file, with editable files overlaid — without writing
// anything to disk. Service.Run uses it to snapshot exactly what a run will
// execute into submissions.content (reproducibility, matching v1 labs).
func (Workspace) Materialize(meta *course.CodeMeta, editable map[string]string) map[string]string {
	return buildFiles(meta, editable)
}

func (w Workspace) RunExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string) (string, int, error) {
	dir, cleanup, err := w.materialize(meta, editable)
	if err != nil {
		return "", -1, err
	}
	defer cleanup()
	return execx.Run(ctx, dir, meta.Run, meta.Timeout)
}

// diagRe matches gofmt -e and go vet lines: "file.go:12:3: message"
// (optionally prefixed with "vet: " or "./").
var diagRe = regexp.MustCompile(`(?m)^(?:vet: )?\.?/?([\w.-]+\.go):(\d+):(\d+): (.+)$`)

// checkFailureDiagnostics turns an execx.Run failure (a check timeout, or a
// spawn failure for the check tool itself) into a synthetic diagnostic
// instead of a Go error. The check path must never surface an error status
// for anything arising from running tools on student code — only a genuine
// infrastructure failure before the tools run (materialize) may error.
func checkFailureDiagnostics(meta *course.CodeMeta, err error) []Diagnostic {
	msg := "check failed: " + err.Error()
	if strings.Contains(err.Error(), "timed out") {
		msg = "check timed out"
	}
	var file string
	if len(meta.Editable) > 0 {
		file = meta.Editable[0]
	}
	return []Diagnostic{{File: file, Line: 1, Col: 1, Message: msg}}
}

func (w Workspace) CheckExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string) ([]Diagnostic, error) {
	dir, cleanup, err := w.materialize(meta, editable)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	editableSet := toSet(meta.Editable)
	var combined strings.Builder
	var diags []Diagnostic

	gofmtArgs := append([]string{"-e", "-l"}, meta.Editable...)
	gofmtOut, _, err := execx.Run(ctx, dir, append([]string{"gofmt"}, gofmtArgs...), checkTimeout)
	if err != nil {
		return checkFailureDiagnostics(meta, err), nil
	}
	combined.WriteString(gofmtOut)
	// gofmt -l reports a mis-formatted-but-parseable file as a bare
	// filename on its own line — no "file:line:col:" prefix, so diagRe
	// below never matches it. Surface it as a file-level diagnostic here
	// so a valid-but-unformatted file isn't silently dropped.
	for _, line := range strings.Split(gofmtOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || diagRe.MatchString(line) {
			continue
		}
		if editableSet[line] {
			diags = append(diags, Diagnostic{File: line, Line: 1, Col: 1, Message: "file is not gofmt-formatted"})
		}
	}

	vetOut, _, err := execx.Run(ctx, dir, []string{"go", "vet", "."}, checkTimeout)
	if err != nil {
		return checkFailureDiagnostics(meta, err), nil
	}
	combined.WriteString("\n" + vetOut)

	seen := map[string]bool{}
	for _, m := range diagRe.FindAllStringSubmatch(combined.String(), -1) {
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		key := m[1] + ":" + m[2] + ":" + m[3] + ":" + m[4]
		if seen[key] {
			continue
		}
		seen[key] = true
		diags = append(diags, Diagnostic{File: m[1], Line: line, Col: col, Message: m[4]})
	}
	sort.Slice(diags, func(i, j int) bool {
		if diags[i].File != diags[j].File {
			return diags[i].File < diags[j].File
		}
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Col < diags[j].Col
	})
	return diags, nil
}
