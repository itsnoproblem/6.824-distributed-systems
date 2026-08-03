# Interactive Coding Exercises Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In-browser coding exercises for the course tour — CodeMirror editor with live gofmt/vet diagnostics, server-side `go test` runs through the existing submission pipeline, complete-on-pass, plus inline video embeds and CC BY attribution machinery.

**Architecture:** New `internal/exercise` feature package (transport/endpoint/service). Student drafts live in SQLite; every run materializes a throwaway temp workspace and executes via a shared hardened-exec helper extracted from the v1 lab runner. Runs are submissions of a new kind `exercise` flowing through the existing status/polling machinery. The editor is vendored CodeMirror 6 plus ~130 lines of bespoke JS — everything else stays server-rendered HTMX.

**Tech Stack:** Existing stack (Go ≥1.25, templ, HTMX, modernc SQLite) + vendored CodeMirror 6 bundle (built once with esbuild via npx; no node in the normal build).

**Spec:** `docs/superpowers/specs/2026-08-03-interactive-exercises-design.md` — binding.

## Global Constraints

- All v1 global constraints hold (stdlib router, allowed deps unchanged, transport→endpoint→service layering, services define consumed interfaces, `templates` imports nothing from `internal/`, generated `*_templ.go` committed, `course.StepRef` identity, TDD, `make test` before every commit).
- No new Go dependencies. New client assets are exactly: `static/codemirror/codemirror.js` (vendored build artifact) and `static/exercise.js`.
- Exercise workspaces are throwaway temp dirs; student state lives only in SQLite (`drafts` + `submissions`).
- Completion is sticky: a passing run sets progress complete; later failing runs never revoke it.
- Exercise scaffold **code** may be adapted from MIT's CC BY–licensed course materials with an `attribution:` frontmatter line AND an entry in `content/ATTRIBUTION.md`. Prose stays original.
- The check endpoint returns diagnostics for broken code — that is its purpose, never an error status.
- Exercise packages are single-package, stdlib-only; the materializer generates `go.mod` (scaffolds never include one).

## File Structure

```
internal/execx/execx.go            # shared hardened subprocess runner (extracted from labrepo)
internal/execx/execx_test.go
internal/sqlite/migrations/002_exercises.sql
internal/sqlite/drafts.go          # DraftsRepo
internal/sqlite/submissions.go     # Modify: passed column, SetPassed, kind 'exercise'
internal/course/course.go          # Modify: CodeMeta, Step.Code/Video/Attribution, StepCode
internal/coursefs/coursefs.go      # Modify: parse code/video/attribution, load scaffold files
internal/exercise/{models,service,endpoint,transport}.go
internal/exercise/workspace.go     # Runner impl: materialize + execx + diagnostics
internal/exercise/service_test.go, workspace_test.go
internal/eval/labrepo.go           # Modify: delegate to execx
internal/eval/models.go            # Modify: KindExercise, Submission.Passed *bool
scripts/build-editor/{package.json,entry.js}   # one-shot bundle build (artifact committed)
static/codemirror/codemirror.js    # vendored build artifact
static/exercise.js                 # tabs, autosave, check, lint gutter, run
static/static.go                   # Modify: embed codemirror/ subdir
templates/exercise.templ           # editor section + status partials
templates/step.templ               # Modify: code-step container + video embed
templates/viewmodels.go            # Modify: exercise + video VMs
content/modules/<m>/exercises/<step-slug>/*.go   # scaffolds
content/ATTRIBUTION.md
content/rubric/exercise.md         # LLM feedback rubric (final task)
e2e/exercise_test.go + e2e/testdata additions
```

---

### Task 1: Extract shared hardened exec into internal/execx

**Files:**
- Create: `internal/execx/execx.go`, `internal/execx/execx_test.go`
- Modify: `internal/eval/labrepo.go` (delegate), `internal/eval/labrepo_test.go` (keep — still passes via delegation)

**Interfaces:**
- Produces: `execx.Run(ctx context.Context, dir string, cmd []string, timeout time.Duration) (output string, exitCode int, err error)`. Contract: combined output tail-truncated to 64KB; command exits non-zero → `exitCode` set, `err == nil` (findings belong to callers); timeout or failure to start → `err != nil`, `exitCode == -1`; child runs in its own process group and the whole group is SIGKILLed on cancel.
- Consumes (later): Task 4's workspace runner. `FSLabRepo.RunTests` keeps its exact v1 signature `(string, error)` by discarding the exit code.

- [ ] **Step 1: Write the failing execx tests** — `internal/execx/execx_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/execx/`
Expected: FAIL (package does not exist)

- [ ] **Step 3: Implement** — `internal/execx/execx.go` (the body is the v1 `FSLabRepo.RunTests` logic, generalized):

```go
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
```

Run: `go test ./internal/execx/` — Expected: PASS

- [ ] **Step 4: Delegate the lab runner** — in `internal/eval/labrepo.go`, replace the whole `RunTests` body and delete `truncateTail` plus the now-unused imports (`errors`, `os/exec`, `syscall`; keep `context`, `fmt`, `os`, `path/filepath`, `time`):

```go
// RunTests executes the step's test command in the lab repo via execx: test
// failures are findings (non-zero exit ⇒ nil error); timeouts and failures
// to execute return err.
func (l FSLabRepo) RunTests(ctx context.Context, workdir string, cmd []string, timeout time.Duration) (string, error) {
	out, _, err := execx.Run(ctx, filepath.Join(l.Dir, workdir), cmd, timeout)
	return out, err
}
```

(add import `github.com/itsnoproblem/mit-distributed-systems/internal/execx`). The existing `labrepo_test.go` tests (`TestRunTestsPasses`, `TestRunTestsTimeout`, `TestRunTestsKillsHungGrandchild`) stay as-is — they now guard the delegation. One expected message change: execx says "run timed out", labrepo tests assert `"timed out"` substring, which still matches.

- [ ] **Step 5: Full suite + commit**

Run: `make test` — Expected: PASS

```bash
git add -A && git commit -m "refactor: extract hardened subprocess runner into internal/execx"
```

---

### Task 2: Migration 002 — drafts table, exercise submission kind, passed flag

**Files:**
- Create: `internal/sqlite/migrations/002_exercises.sql`, `internal/sqlite/drafts.go`, `internal/sqlite/drafts_test.go`
- Modify: `internal/eval/models.go` (KindExercise + `Passed *bool`), `internal/sqlite/submissions.go` (passed column + `SetPassed`), `internal/sqlite/submissions_test.go` (+upgrade + passed tests)

**Interfaces:**
- Produces: `eval.KindExercise Kind = "exercise"`; `eval.Submission.Passed *bool` (nil for lab/question); `sqlite.NewDraftsRepo(db) *DraftsRepo` with `Upsert(ctx, ref course.StepRef, files map[string]string) error`, `Get(ctx, ref) (map[string]string, bool, error)`, `Delete(ctx, ref) error`; `(*SubmissionRepo).SetPassed(ctx, id int64, passed bool) error`.

- [ ] **Step 1: Write the migration** — `internal/sqlite/migrations/002_exercises.sql`:

```sql
CREATE TABLE drafts (
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    files_json  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (module_slug, step_slug)
);

CREATE TABLE submissions_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('lab', 'question', 'exercise')),
    content     TEXT NOT NULL,
    test_output TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('pending', 'running', 'complete', 'failed')),
    passed      INTEGER,
    created_at  TEXT NOT NULL
);
INSERT INTO submissions_new (id, module_slug, step_slug, kind, content, test_output, status, created_at)
    SELECT id, module_slug, step_slug, kind, content, test_output, status, created_at FROM submissions;
DROP TABLE submissions;
ALTER TABLE submissions_new RENAME TO submissions;
```

- [ ] **Step 2: Write failing upgrade + drafts tests**

Append to `internal/sqlite/submissions_test.go`:

```go
// TestMigration002PreservesData simulates a v1 database: apply only 001,
// insert a lab submission, then run the full Migrate and verify the row
// survived and the new kind + passed flag work.
func TestMigration002PreservesData(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.MigrateUpTo(db, "001_init.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO submissions
		(module_slug, step_slug, kind, content, test_output, status, created_at)
		VALUES ('m', 's', 'lab', 'code', 'out', 'complete', '2026-08-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewSubmissionRepo(db)
	ctx := context.Background()
	old, err := repo.GetSubmission(ctx, 1)
	if err != nil || old.Content != "code" || old.Status != eval.StatusComplete || old.Passed != nil {
		t.Fatalf("v1 row mangled: %+v err=%v", old, err)
	}
	id, err := repo.InsertSubmission(ctx, eval.Submission{
		Ref: course.StepRef{Module: "m", Step: "x"}, Kind: eval.KindExercise,
		Content: "{}", Status: eval.StatusPending, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("exercise kind rejected: %v", err)
	}
	if err := repo.SetPassed(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetSubmission(ctx, id)
	if got.Passed == nil || !*got.Passed {
		t.Fatalf("passed not persisted: %+v", got)
	}
}
```

Create `internal/sqlite/drafts_test.go`:

```go
package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func draftsRepo(t *testing.T) *sqlite.DraftsRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewDraftsRepo(db)
}

func TestDraftLifecycle(t *testing.T) {
	repo := draftsRepo(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "c1"}

	if _, ok, err := repo.Get(ctx, ref); err != nil || ok {
		t.Fatalf("expected no draft: ok=%v err=%v", ok, err)
	}
	if err := repo.Upsert(ctx, ref, map[string]string{"a.go": "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, ref, map[string]string{"a.go": "v2"}); err != nil {
		t.Fatal(err)
	}
	files, ok, err := repo.Get(ctx, ref)
	if err != nil || !ok || files["a.go"] != "v2" {
		t.Fatalf("get: %v %v %v", files, ok, err)
	}
	if err := repo.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ = repo.Get(ctx, ref); ok {
		t.Fatal("expected draft gone after delete")
	}
}
```

Run: `go test ./internal/sqlite/` — Expected: FAIL (migration, MigrateUpTo, DraftsRepo, KindExercise, SetPassed all missing)

- [ ] **Step 3: Implement the model + sqlite changes**

`internal/eval/models.go`: add `KindExercise Kind = "exercise"` to the Kind const block, and add field `Passed *bool` to `Submission` (after `Status`), with comment `// exercise runs only: tests passed; nil for lab/question`.

`internal/sqlite/db.go`: add a test-only-visible seam used by the upgrade test — split `Migrate` so both entry points share one loop:

```go
// Migrate applies every embedded migration not yet recorded.
func Migrate(db *sql.DB) error { return MigrateUpTo(db, "") }

// MigrateUpTo applies migrations in filename order, stopping after `last`
// when it is non-empty. Exists so tests can construct historical schemas.
func MigrateUpTo(db *sql.DB, last string) error {
	// identical body to v1 Migrate, plus after each applied migration:
	//   if name == last { return nil }
}
```

`internal/sqlite/submissions.go`:
- `InsertSubmission`: column list gains `passed`; value is `s.Passed` marshaled as `any` (`nil` stays NULL): pass `passedVal(s.Passed)` where `func passedVal(p *bool) any { if p == nil { return nil }; if *p { return 1 }; return 0 }`.
- `subCols` gains `passed`; `scanSubmission` scans into `var passed sql.NullInt64` and sets `s.Passed` when valid.
- Add:

```go
func (r *SubmissionRepo) SetPassed(ctx context.Context, id int64, passed bool) error {
	v := 0
	if passed {
		v = 1
	}
	_, err := r.db.ExecContext(ctx, "UPDATE submissions SET passed = ? WHERE id = ?", v, id)
	return err
}
```

`internal/sqlite/drafts.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type DraftsRepo struct{ db *sql.DB }

func NewDraftsRepo(db *sql.DB) *DraftsRepo { return &DraftsRepo{db} }

func (r *DraftsRepo) Upsert(ctx context.Context, ref course.StepRef, files map[string]string) error {
	raw, err := json.Marshal(files)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO drafts (module_slug, step_slug, files_json, updated_at)
		 VALUES (?, ?, ?, ?)`,
		ref.Module, ref.Step, string(raw), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (r *DraftsRepo) Get(ctx context.Context, ref course.StepRef) (map[string]string, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx,
		"SELECT files_json FROM drafts WHERE module_slug = ? AND step_slug = ?",
		ref.Module, ref.Step).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var files map[string]string
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return nil, false, err
	}
	return files, true, nil
}

func (r *DraftsRepo) Delete(ctx context.Context, ref course.StepRef) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM drafts WHERE module_slug = ? AND step_slug = ?", ref.Module, ref.Step)
	return err
}
```

- [ ] **Step 4: Run tests, full suite, commit**

Run: `go test ./internal/sqlite/ ./internal/eval/` then `make test` — Expected: PASS

```bash
git add -A && git commit -m "feat: migration 002 - drafts table, exercise kind, passed flag"
```

---

### Task 3: Content model — `code` step type, videos, attribution

**Files:**
- Modify: `internal/course/course.go` (+CodeMeta, Step fields, StepCode), `internal/coursefs/coursefs.go` (parse + validate + scaffold loading), `internal/coursefs/coursefs_test.go` (+cases)
- Create: `internal/coursefs/testdata/valid/01-alpha/steps/03-code.md`, `internal/coursefs/testdata/valid/01-alpha/exercises/03-code/{adder.go,adder_test.go}`

**Interfaces:**
- Produces (consumed by Tasks 4–7):

```go
// internal/course additions
const StepCode StepType = "code"
type CodeMeta struct {
	Mode     string            // "fix" | "create"
	Editable []string
	Readonly []string
	Run      []string
	Timeout  time.Duration
	Files    map[string]string // scaffold filename -> source
}
// Step gains: Code *CodeMeta; Video string; Attribution string
```

**Validation rules (loader, boot-fails with path):** `code` steps require a `code:` block; `mode` ∈ {fix, create}; ≥1 editable file; editable/readonly disjoint; every listed file exists in `<moduleDir>/exercises/<step-slug>/`; no unlisted files in that dir; non-empty `run`; parseable `timeout`; scaffold files must not include `go.mod`. `video:` and `attribution:` are optional free-text on any step type.

- [ ] **Step 1: Add the domain types** — in `internal/course/course.go`, add `StepCode StepType = "code"` to the const block, the `CodeMeta` struct above (after `EvalMeta`), and the three new `Step` fields (`Code *CodeMeta`, `Video string`, `Attribution string`). Pure data — no test change needed beyond compilation.

- [ ] **Step 2: Write testdata + failing loader tests**

`internal/coursefs/testdata/valid/01-alpha/steps/03-code.md`:
```markdown
---
title: Fix the adder
type: code
video: "https://www.youtube.com/watch?v=abc123xyz"
attribution: "Adapted from example materials (CC BY)"
code:
  mode: fix
  editable: ["adder.go"]
  readonly: ["adder_test.go"]
  run: ["go", "test", "."]
  timeout: 1m
---

Make the test pass.
```

`internal/coursefs/testdata/valid/01-alpha/exercises/03-code/adder.go`:
```go
package adder

func Add(a, b int) int { return a - b }
```

`internal/coursefs/testdata/valid/01-alpha/exercises/03-code/adder_test.go`:
```go
package adder

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("Add(2,3) = %d", Add(2, 3))
	}
}
```

Append to `internal/coursefs/coursefs_test.go`:

```go
func TestLoadCodeStep(t *testing.T) {
	c, err := coursefs.Load("testdata/valid")
	if err != nil {
		t.Fatal(err)
	}
	_, step, ok := c.Step(course.StepRef{Module: "01-alpha", Step: "03-code"})
	if !ok || step.Type != course.StepCode {
		t.Fatalf("code step missing: %v %v", step, ok)
	}
	if step.Video == "" || step.Attribution == "" {
		t.Errorf("video/attribution not parsed: %+v", step)
	}
	m := step.Code
	if m == nil || m.Mode != "fix" || len(m.Editable) != 1 || len(m.Readonly) != 1 ||
		m.Timeout != time.Minute || len(m.Run) != 3 {
		t.Fatalf("code meta: %+v", m)
	}
	if !strings.Contains(m.Files["adder.go"], "a - b") ||
		!strings.Contains(m.Files["adder_test.go"], "TestAdd") {
		t.Fatalf("scaffold files not loaded: %v", mapsKeys(m.Files))
	}
}

func mapsKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

And add these rows to the existing `TestLoadErrors` table (each fails validation before any scaffold file is read, so `writeModule` alone suffices):

```go
{"code without block", "title: X\nkind: lecture\norder: 1",
	map[string]string{"01-a.md": "---\ntitle: T\ntype: code\n---\nb"}, "code"},
{"code bad mode", "title: X\nkind: lecture\norder: 1",
	map[string]string{"01-a.md": "---\ntitle: T\ntype: code\ncode:\n  mode: guess\n  editable: [\"a.go\"]\n  run: [\"go\", \"test\"]\n  timeout: 1m\n---\nb"}, "mode"},
{"code no editable", "title: X\nkind: lecture\norder: 1",
	map[string]string{"01-a.md": "---\ntitle: T\ntype: code\ncode:\n  mode: fix\n  editable: []\n  run: [\"go\", \"test\"]\n  timeout: 1m\n---\nb"}, "editable"},
```

Plus one standalone error test for a listed-but-missing scaffold file:

```go
func TestCodeStepMissingScaffoldFile(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "01-x", "title: X\nkind: lecture\norder: 1", map[string]string{
		"01-a.md": "---\ntitle: T\ntype: code\ncode:\n  mode: fix\n  editable: [\"gone.go\"]\n  run: [\"go\", \"test\"]\n  timeout: 1m\n---\nb",
	})
	if _, err := coursefs.Load(root); err == nil || !strings.Contains(err.Error(), "gone.go") {
		t.Fatalf("err = %v", err)
	}
}
```

Also update the existing `TestLoadValid` assertion — module `01-alpha` now has
three steps, so change `if len(alpha.Steps) != 2 || alpha.Steps[0].Slug != "01-read" {`
to expect `3` (the slug check is unchanged).

Run: `go test ./internal/coursefs/` — Expected: FAIL

- [ ] **Step 3: Implement the loader changes** — in `internal/coursefs/coursefs.go`:

1. `stepYAML` gains:

```go
	Video       string `yaml:"video"`
	Attribution string `yaml:"attribution"`
	Code        *struct {
		Mode     string   `yaml:"mode"`
		Editable []string `yaml:"editable"`
		Readonly []string `yaml:"readonly"`
		Run      []string `yaml:"run"`
		Timeout  string   `yaml:"timeout"`
	} `yaml:"code"`
```

2. `validTypes` gains `"code": course.StepCode`.
3. `loadStep` signature becomes `loadStep(moduleDir, path string)` (update the call site to pass `dir`), sets `step.Video = sy.Video` and `step.Attribution = strings.TrimSpace(sy.Attribution)`, and for `typ == course.StepCode` appends:

```go
	if typ == course.StepCode {
		if sy.Code == nil {
			return course.Step{}, fmt.Errorf("%s: code steps require a code block", path)
		}
		if sy.Code.Mode != "fix" && sy.Code.Mode != "create" {
			return course.Step{}, fmt.Errorf("%s: code.mode must be fix or create, got %q", path, sy.Code.Mode)
		}
		if len(sy.Code.Editable) == 0 {
			return course.Step{}, fmt.Errorf("%s: code.editable requires at least one file", path)
		}
		if len(sy.Code.Run) == 0 {
			return course.Step{}, fmt.Errorf("%s: code.run is required", path)
		}
		timeout, err := time.ParseDuration(sy.Code.Timeout)
		if err != nil {
			return course.Step{}, fmt.Errorf("%s: code.timeout: %w", path, err)
		}
		listed := map[string]bool{}
		for _, f := range sy.Code.Editable {
			listed[f] = true
		}
		for _, f := range sy.Code.Readonly {
			if listed[f] {
				return course.Step{}, fmt.Errorf("%s: %q is both editable and readonly", path, f)
			}
			listed[f] = true
		}
		exDir := filepath.Join(moduleDir, "exercises", step.Slug)
		files := map[string]string{}
		for f := range listed {
			raw, err := os.ReadFile(filepath.Join(exDir, f))
			if err != nil {
				return course.Step{}, fmt.Errorf("%s: scaffold file %s: %w", path, f, err)
			}
			files[f] = string(raw)
		}
		entries, err := os.ReadDir(exDir)
		if err != nil {
			return course.Step{}, fmt.Errorf("%s: %w", path, err)
		}
		for _, e := range entries {
			if e.Name() == "go.mod" {
				return course.Step{}, fmt.Errorf("%s: scaffolds must not include go.mod", path)
			}
			if !listed[e.Name()] {
				return course.Step{}, fmt.Errorf("%s: unlisted file %s in exercises dir", path, e.Name())
			}
		}
		step.Code = &course.CodeMeta{
			Mode: sy.Code.Mode, Editable: sy.Code.Editable, Readonly: sy.Code.Readonly,
			Run: sy.Code.Run, Timeout: timeout, Files: files,
		}
	}
```

Run: `go test ./internal/coursefs/` then `make test` — Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat: code step type with scaffold loading, video and attribution fields"
```

---

### Task 4: Exercise engine — workspace runner, diagnostics, service

**Files:**
- Create: `internal/exercise/models.go`, `internal/exercise/workspace.go`, `internal/exercise/workspace_test.go`, `internal/exercise/service.go`, `internal/exercise/service_test.go`

**Interfaces:**
- Consumes: `execx.Run`, `course.CodeMeta`, `eval.Submission/Status/KindExercise`, sqlite repos.
- Produces (consumed by Tasks 5–6):

```go
// models.go
type Diagnostic struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Message string `json:"message"`
}
type FileView struct{ Name, Content string; Readonly bool }
type View struct {
	Meta       *course.CodeMeta
	Step       course.Step
	Files      []FileView // editable first, then readonly, each group in meta order
	HasDraft   bool
	Submission *eval.Submission // latest exercise run, nil if none
}

// service.go — interfaces defined here, sqlite implements
type CourseRepo interface{ Course() *course.Course }
type ProgressMarker interface{ SetComplete(ctx context.Context, ref course.StepRef, done bool) error }
type DraftRepo interface {
	Upsert(ctx context.Context, ref course.StepRef, files map[string]string) error
	Get(ctx context.Context, ref course.StepRef) (map[string]string, bool, error)
	Delete(ctx context.Context, ref course.StepRef) error
}
type SubmissionRepo interface {
	InsertSubmission(ctx context.Context, s eval.Submission) (int64, error)
	UpdateSubmission(ctx context.Context, id int64, status eval.Status, testOutput string) error
	SetPassed(ctx context.Context, id int64, passed bool) error
	GetSubmission(ctx context.Context, id int64) (eval.Submission, error)
	LatestForStep(ctx context.Context, ref course.StepRef) (*eval.Submission, error)
}
type Runner interface {
	RunExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string) (output string, exitCode int, err error)
	CheckExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string) ([]Diagnostic, error)
}

func NewService(c CourseRepo, d DraftRepo, s SubmissionRepo, p ProgressMarker, r Runner, opts ...Option) *Service
func WithRunAsync(f func(func())) Option
// methods: State(ctx, ref) (View, error); SaveDraft(ctx, ref, files map[string]string) error;
// ResetDraft(ctx, ref) error; Check(ctx, ref) ([]Diagnostic, error); Run(ctx, ref) error;
// RefForSubmission(ctx, id int64) (course.StepRef, error)
```

Reusing `eval`'s model types across features is deliberate: `eval` owns the submission vocabulary the way `course` owns course vocabulary; `exercise` imports `eval` models only (no service/transport coupling, no cycle).

- [ ] **Step 1: Write failing workspace tests** — `internal/exercise/workspace_test.go`:

```go
package exercise_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
)

func adderMeta() *course.CodeMeta {
	return &course.CodeMeta{
		Mode:     "fix",
		Editable: []string{"adder.go"},
		Readonly: []string{"adder_test.go"},
		Run:      []string{"go", "test", "."},
		Timeout:  time.Minute,
		Files: map[string]string{
			"adder.go":      "package adder\n\nfunc Add(a, b int) int { return a - b }\n",
			"adder_test.go": "package adder\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(Add(2, 3))\n\t}\n}\n",
		},
	}
}

func TestRunExerciseScaffoldFails(t *testing.T) {
	out, code, err := exercise.Workspace{}.RunExercise(context.Background(), adderMeta(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 || !strings.Contains(out, "FAIL") {
		t.Fatalf("buggy scaffold should fail: code=%d out=%q", code, out)
	}
}

func TestRunExerciseDraftOverlayPasses(t *testing.T) {
	draft := map[string]string{"adder.go": "package adder\n\nfunc Add(a, b int) int { return a + b }\n"}
	out, code, err := exercise.Workspace{}.RunExercise(context.Background(), adderMeta(), draft)
	if err != nil || code != 0 {
		t.Fatalf("fixed draft should pass: code=%d err=%v out=%q", code, err, out)
	}
}

func TestRunExerciseIgnoresNonEditableOverlay(t *testing.T) {
	// overlaying the read-only test file must not take effect
	draft := map[string]string{
		"adder_test.go": "package adder\n", // would delete the test
	}
	_, code, err := exercise.Workspace{}.RunExercise(context.Background(), adderMeta(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Fatal("read-only overlay must be ignored; scaffold bug should still fail")
	}
}

func TestCheckExerciseReportsSyntaxAndVet(t *testing.T) {
	broken := map[string]string{"adder.go": "package adder\n\nfunc Add(a, b int) int { return a +\n"}
	diags, err := exercise.Workspace{}.CheckExercise(context.Background(), adderMeta(), broken)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 || diags[0].File != "adder.go" || diags[0].Line == 0 {
		t.Fatalf("diags = %+v", diags)
	}
	clean := map[string]string{"adder.go": "package adder\n\nfunc Add(a, b int) int { return a + b }\n"}
	diags, err = exercise.Workspace{}.CheckExercise(context.Background(), adderMeta(), clean)
	if err != nil || len(diags) != 0 {
		t.Fatalf("clean code: diags=%+v err=%v", diags, err)
	}
}
```

Run: `go test ./internal/exercise/` — Expected: FAIL (package missing)

- [ ] **Step 2: Implement models + workspace** — `internal/exercise/models.go` (types from the Interfaces block, with package doc `// Package exercise is the interactive coding-exercise feature: in-browser drafts, throwaway workspaces, go-test validation.`), and `internal/exercise/workspace.go`:

```go
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

// materialize writes go.mod + scaffold files, overlaying editable files
// from the draft. Overlay keys outside meta.Editable are ignored — the
// client can never rewrite the test harness.
func (Workspace) materialize(meta *course.CodeMeta, editable map[string]string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "exercise-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	editableSet := map[string]bool{}
	for _, f := range meta.Editable {
		editableSet[f] = true
	}
	files := map[string]string{"go.mod": "module exercise\n\ngo 1.25\n"}
	for name, src := range meta.Files {
		files[name] = src
	}
	for name, src := range editable {
		if editableSet[name] {
			files[name] = src
		}
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("materialize %s: %w", name, err)
		}
	}
	return dir, cleanup, nil
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

func (w Workspace) CheckExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string) ([]Diagnostic, error) {
	dir, cleanup, err := w.materialize(meta, editable)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	var combined strings.Builder
	gofmtArgs := append([]string{"-e", "-l"}, meta.Editable...)
	out, _, err := execx.Run(ctx, dir, append([]string{"gofmt"}, gofmtArgs...), checkTimeout)
	if err != nil {
		return nil, fmt.Errorf("gofmt: %w", err)
	}
	combined.WriteString(out)
	out, _, err = execx.Run(ctx, dir, []string{"go", "vet", "."}, checkTimeout)
	if err != nil {
		return nil, fmt.Errorf("go vet: %w", err)
	}
	combined.WriteString("\n" + out)
	seen := map[string]bool{}
	var diags []Diagnostic
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
		return diags[i].Line < diags[j].Line
	})
	return diags, nil
}
```

Run: `go test ./internal/exercise/ -run 'RunExercise|CheckExercise'` — Expected: PASS

- [ ] **Step 3: Write failing service tests** — `internal/exercise/service_test.go`:

```go
package exercise_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
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
	svc := exercise.NewService(coursefs.NewRepo(fixtureCourse()), sqlite.NewDraftsRepo(db),
		subs, progress, exercise.Workspace{},
		exercise.WithRunAsync(func(f func()) { f() }))
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
```

Run: `go test ./internal/exercise/` — Expected: FAIL (Service missing)

- [ ] **Step 4: Implement the service** — `internal/exercise/service.go`:

```go
package exercise

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type Service struct {
	course   CourseRepo
	drafts   DraftRepo
	subs     SubmissionRepo
	progress ProgressMarker
	runner   Runner
	runAsync func(func())
	now      func() time.Time
}

type Option func(*Service)

// WithRunAsync overrides how runs are scheduled; tests run them inline.
func WithRunAsync(f func(func())) Option { return func(s *Service) { s.runAsync = f } }

func NewService(c CourseRepo, d DraftRepo, subs SubmissionRepo, p ProgressMarker, r Runner, opts ...Option) *Service {
	s := &Service{
		course: c, drafts: d, subs: subs, progress: p, runner: r,
		runAsync: func(f func()) { go f() }, now: time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) codeStep(ref course.StepRef) (*course.Step, error) {
	_, step, ok := s.course.Course().Step(ref)
	if !ok || step.Type != course.StepCode || step.Code == nil {
		return nil, fmt.Errorf("%w: code step %s", api.ErrNotFound, ref)
	}
	return step, nil
}

// effective returns the editable file set: scaffold overlaid with any draft.
func (s *Service) effective(ctx context.Context, ref course.StepRef, meta *course.CodeMeta) (map[string]string, bool, error) {
	files := map[string]string{}
	for _, name := range meta.Editable {
		files[name] = meta.Files[name]
	}
	draft, ok, err := s.drafts.Get(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	if ok {
		for name, src := range draft {
			if _, editable := files[name]; editable {
				files[name] = src
			}
		}
	}
	return files, ok, nil
}

func (s *Service) State(ctx context.Context, ref course.StepRef) (View, error) {
	step, err := s.codeStep(ref)
	if err != nil {
		return View{}, err
	}
	editable, hasDraft, err := s.effective(ctx, ref, step.Code)
	if err != nil {
		return View{}, err
	}
	view := View{Meta: step.Code, Step: *step, HasDraft: hasDraft}
	for _, name := range step.Code.Editable {
		view.Files = append(view.Files, FileView{Name: name, Content: editable[name]})
	}
	for _, name := range step.Code.Readonly {
		view.Files = append(view.Files, FileView{Name: name, Content: step.Code.Files[name], Readonly: true})
	}
	if view.Submission, err = s.subs.LatestForStep(ctx, ref); err != nil {
		return View{}, err
	}
	return view, nil
}

func (s *Service) SaveDraft(ctx context.Context, ref course.StepRef, files map[string]string) error {
	step, err := s.codeStep(ref)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%w: no files in draft", api.ErrInvalid)
	}
	editable := map[string]bool{}
	for _, f := range step.Code.Editable {
		editable[f] = true
	}
	for name := range files {
		if !editable[name] {
			return fmt.Errorf("%w: %q is not an editable file", api.ErrInvalid, name)
		}
	}
	return s.drafts.Upsert(ctx, ref, files)
}

func (s *Service) ResetDraft(ctx context.Context, ref course.StepRef) error {
	if _, err := s.codeStep(ref); err != nil {
		return err
	}
	return s.drafts.Delete(ctx, ref)
}

func (s *Service) Check(ctx context.Context, ref course.StepRef) ([]Diagnostic, error) {
	step, err := s.codeStep(ref)
	if err != nil {
		return nil, err
	}
	editable, _, err := s.effective(ctx, ref, step.Code)
	if err != nil {
		return nil, err
	}
	return s.runner.CheckExercise(ctx, step.Code, editable)
}

// Run snapshots the effective file set into a submission and schedules the
// async test run. The snapshot is stored before any side effect.
func (s *Service) Run(ctx context.Context, ref course.StepRef) error {
	step, err := s.codeStep(ref)
	if err != nil {
		return err
	}
	editable, _, err := s.effective(ctx, ref, step.Code)
	if err != nil {
		return err
	}
	content, err := json.Marshal(editable)
	if err != nil {
		return err
	}
	id, err := s.subs.InsertSubmission(ctx, eval.Submission{
		Ref: ref, Kind: eval.KindExercise, Content: string(content),
		Status: eval.StatusPending, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}
	s.runAsync(func() { s.evaluate(id) })
	return nil
}

// evaluate runs in the background; every failure lands on the submission row.
func (s *Service) evaluate(id int64) {
	ctx := context.Background()
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		log.Printf("exercise evaluate: load submission %d: %v", id, err)
		return
	}
	step, err := s.codeStep(sub.Ref)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, "step no longer exists in content")
		return
	}
	_ = s.subs.UpdateSubmission(ctx, id, eval.StatusRunning, "")
	var editable map[string]string
	if err := json.Unmarshal([]byte(sub.Content), &editable); err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, "snapshot decode error: "+err.Error())
		return
	}
	out, code, err := s.runner.RunExercise(ctx, step.Code, editable)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, out+"\n\nRUNNER ERROR: "+err.Error())
		return
	}
	passed := code == 0
	_ = s.subs.SetPassed(ctx, id, passed)
	_ = s.subs.UpdateSubmission(ctx, id, eval.StatusComplete, out)
	if passed {
		// sticky completion: only ever set true
		_ = s.progress.SetComplete(ctx, sub.Ref, true)
	}
}

func (s *Service) RefForSubmission(ctx context.Context, id int64) (course.StepRef, error) {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return course.StepRef{}, fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	return sub.Ref, nil
}
```

Run: `go test ./internal/exercise/` then `make test` — Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: exercise engine - workspace runner, diagnostics, service"
```

---

### Task 5: Editor assets — vendored CodeMirror, exercise.js, templates

**Files:**
- Create: `scripts/build-editor/package.json`, `scripts/build-editor/entry.js`, `static/codemirror/codemirror.js` (build artifact), `static/exercise.js`, `templates/exercise.templ`, `templates/viewmodels_test.go`
- Modify: `static/static.go` (embed subdir), `static/styles.css` (+editor styles), `templates/viewmodels.go` (+exercise/video VMs + `YouTubeEmbedURL`), `templates/step.templ` (+code container, +video embed), `.gitignore` (+node_modules)

**Interfaces:**
- Produces: `templates.ExerciseSection(ExerciseVM)`, `templates.ExerciseStatus(ExerciseVM)`; VMs below; `templates.YouTubeEmbedURL(watchURL string) string` (`""` when unrecognized); `window.CM` bundle + auto-initializing `static/exercise.js` reading `#exercise-root[data-config]` with config JSON `{files: [{name, content, readonly}], saveUrl, checkUrl, runUrl}`.

```go
type ExerciseFileVM struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	Readonly bool   `json:"readonly"`
}

type ExerciseVM struct {
	ModuleSlug, StepSlug string
	Mode, ModeLabel      string // "fix"/"create", "Fix the bug"/"Build it"
	Attribution          string
	ConfigJSON           string
	Status               string
	Passed               bool
	Output               string
	SubmissionID         int64
}
// StepVM gains: VideoWatchURL, VideoEmbedURL string
```

- [ ] **Step 1: Build and vendor the CodeMirror bundle**

`scripts/build-editor/package.json`:
```json
{
  "name": "build-editor",
  "private": true,
  "description": "One-shot build of the vendored CodeMirror bundle. Artifact is committed at static/codemirror/codemirror.js; the normal app build never needs node.",
  "dependencies": {
    "@codemirror/lang-go": "^6.0.1",
    "@codemirror/lint": "^6.8.4",
    "codemirror": "^6.0.1",
    "esbuild": "^0.24.0"
  }
}
```

`scripts/build-editor/entry.js`:
```js
import { EditorView, basicSetup } from "codemirror";
import { EditorState, Compartment } from "@codemirror/state";
import { go } from "@codemirror/lang-go";
import { lintGutter, setDiagnostics } from "@codemirror/lint";

window.CM = { EditorView, EditorState, Compartment, basicSetup, go, lintGutter, setDiagnostics };
```

```bash
echo "scripts/build-editor/node_modules/" >> .gitignore
cd scripts/build-editor && npm install && npx esbuild entry.js --bundle --minify --format=iife --outfile=../../static/codemirror/codemirror.js && cd ../..
ls -la static/codemirror/codemirror.js   # expect roughly 400KB-1MB, non-empty
```

Commit `package-lock.json` (reproducibility) but not `node_modules/`.

- [ ] **Step 2: Embed the new assets** — `static/static.go` embed directive becomes:

```go
//go:embed *.css *.js codemirror
var FS embed.FS
```

Append to `static/styles.css`:
```css
.tabbar { display: flex; gap: .25rem; margin: .8rem 0 0; }
.tab { border: 1px solid #ccc; border-bottom: 0; background: #f5f5f5; padding: .3rem .8rem;
  border-radius: 4px 4px 0 0; cursor: pointer; font-size: .85rem; }
.tab.active { background: #fff; font-weight: 600; }
.tab.readonly { color: #888; }
#exercise-editor { border: 1px solid #ccc; border-radius: 0 4px 4px 4px; }
#exercise-editor .cm-editor { max-height: 30rem; }
#exercise-editor .cm-scroller { overflow: auto; font-size: .9rem; }
.exercise-actions { display: flex; align-items: center; gap: 1rem; margin: .8rem 0; }
.attribution { font-size: .78rem; color: #888; font-style: italic; }
.badge { display: inline-block; font-size: .72rem; text-transform: uppercase; letter-spacing: .04em;
  padding: .15rem .5rem; border-radius: 3px; background: #eef4f6; color: #007d9c; }
.test-failed { color: #b00020; }
.video { aspect-ratio: 16 / 9; margin: .8rem 0 .2rem; }
.video iframe { width: 100%; height: 100%; border: 0; border-radius: 4px; }
.video-fallback { font-size: .8rem; margin-top: 0; }
```

- [ ] **Step 3: TDD the embed-URL helper** — `templates/viewmodels_test.go`:

```go
package templates_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func TestYouTubeEmbedURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.youtube.com/watch?v=cQP8WApzIQQ", "https://www.youtube-nocookie.com/embed/cQP8WApzIQQ"},
		{"https://youtu.be/cQP8WApzIQQ", "https://www.youtube-nocookie.com/embed/cQP8WApzIQQ"},
		{"https://www.youtube.com/watch?v=abc123&t=42", "https://www.youtube-nocookie.com/embed/abc123"},
		{"https://vimeo.com/12345", ""},
		{"not a url", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := templates.YouTubeEmbedURL(tc.in); got != tc.want {
			t.Errorf("YouTubeEmbedURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
```

Run: `go test ./templates/` — Expected: FAIL. Then append to `templates/viewmodels.go`:

```go
// YouTubeEmbedURL converts a YouTube watch URL into a privacy-enhanced
// embed URL, or returns "" for anything it does not recognize (the caller
// then renders only the plain link).
func YouTubeEmbedURL(watch string) string {
	u, err := url.Parse(watch)
	if err != nil {
		return ""
	}
	var id string
	switch {
	case strings.HasSuffix(u.Host, "youtube.com"):
		id = u.Query().Get("v")
	case u.Host == "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	}
	if id == "" {
		return ""
	}
	return "https://www.youtube-nocookie.com/embed/" + id
}
```

(add imports `net/url`, `strings`, plus the VM structs from the Interfaces block). Run: `go test ./templates/` — Expected: PASS

- [ ] **Step 4: Write the client bootstrap** — `static/exercise.js`:

```js
// Exercise editor bootstrap. Depends on window.CM (vendored CodeMirror
// bundle) and htmx. Idempotent: re-scans after every htmx swap.
(function () {
  "use strict";

  function debounce(fn, ms) {
    var t;
    return function () { clearTimeout(t); t = setTimeout(fn, ms); };
  }

  function init(root) {
    root.dataset.initialized = "1";
    var cfg = JSON.parse(root.dataset.config);
    var tabbar = root.querySelector("#exercise-tabs");
    var host = root.querySelector("#exercise-editor");
    var CM = window.CM;
    var states = {};
    var current = null;
    var view = null;

    function mkState(file) {
      return CM.EditorState.create({
        doc: file.content,
        extensions: [
          CM.basicSetup,
          CM.go(),
          CM.lintGutter(),
          CM.EditorView.editable.of(!file.readonly),
          CM.EditorView.updateListener.of(function (u) {
            if (u.docChanged) { onChange(); }
          }),
        ],
      });
    }

    function editableFiles() {
      var out = {};
      cfg.files.forEach(function (f) {
        if (f.readonly) { return; }
        out[f.name] = f.name === current
          ? view.state.doc.toString()
          : states[f.name].doc.toString();
      });
      return out;
    }

    function save() {
      return fetch(cfg.saveUrl, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ files: editableFiles() }),
      }).catch(function () { /* transient; next change retries */ });
    }

    function check() {
      fetch(cfg.checkUrl, { method: "POST" })
        .then(function (r) { return r.json(); })
        .then(function (diags) {
          var forCurrent = diags
            .filter(function (d) { return d.file === current; })
            .map(function (d) {
              var line = view.state.doc.line(Math.min(d.line, view.state.doc.lines));
              var from = Math.min(line.from + Math.max(d.col - 1, 0), line.to);
              return { from: from, to: line.to, severity: "error", message: d.message };
            });
          view.dispatch(CM.setDiagnostics(view.state, forCurrent));
        })
        .catch(function () {});
    }

    var debouncedSave = debounce(save, 800);
    var debouncedCheck = debounce(function () { save().then(check); }, 1500);
    function onChange() { debouncedSave(); debouncedCheck(); }

    function show(name) {
      if (view && current) { states[current] = view.state; }
      current = name;
      if (!view) {
        view = new CM.EditorView({ state: states[name], parent: host });
      } else {
        view.setState(states[name]);
      }
      tabbar.querySelectorAll("button").forEach(function (b) {
        b.classList.toggle("active", b.dataset.file === name);
      });
    }

    cfg.files.forEach(function (f) {
      states[f.name] = mkState(f);
      var b = document.createElement("button");
      b.type = "button";
      b.className = "tab" + (f.readonly ? " readonly" : "");
      b.dataset.file = f.name;
      b.textContent = f.readonly ? f.name + " (read-only)" : f.name;
      b.addEventListener("click", function () { show(f.name); });
      tabbar.appendChild(b);
    });
    show(cfg.files[0].name);

    var runBtn = root.querySelector("#exercise-run");
    runBtn.addEventListener("click", function () {
      runBtn.disabled = true;
      save().then(function () {
        return window.htmx.ajax("POST", cfg.runUrl,
          { target: "#exercise-status", swap: "innerHTML" });
      }).finally(function () { runBtn.disabled = false; });
    });

    root.addEventListener("keydown", function (e) {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        runBtn.click();
      }
    });
  }

  function scan() {
    document.querySelectorAll("#exercise-root:not([data-initialized])")
      .forEach(function (root) { if (window.CM) { init(root); } });
  }
  scan();
  document.body.addEventListener("htmx:afterSettle", scan);
})();
```

- [ ] **Step 5: Templates** — `templates/exercise.templ`:

```templ
package templates

import "fmt"

templ ExerciseSection(v ExerciseVM) {
	<div class="exercise" id="exercise-root" data-config={ v.ConfigJSON }>
		<span class={ "badge badge-" + v.Mode }>{ v.ModeLabel }</span>
		if v.Attribution != "" {
			<p class="attribution">{ v.Attribution }</p>
		}
		<div class="tabbar" id="exercise-tabs"></div>
		<div id="exercise-editor"></div>
		<div class="exercise-actions">
			<button class="btn" id="exercise-run" type="button">Run tests</button>
			<button class="link danger" type="button"
				hx-post={ "/exercises/" + v.ModuleSlug + "/" + v.StepSlug + "/reset" }
				hx-confirm="Discard your changes and restore the original code?"
				hx-target="#exercise-section" hx-swap="innerHTML">Reset to original</button>
		</div>
		<div id="exercise-status">
			@ExerciseStatus(v)
		</div>
		<script src="/static/codemirror/codemirror.js"></script>
		<script src="/static/exercise.js"></script>
	</div>
}

templ ExerciseStatus(v ExerciseVM) {
	if v.Status == "pending" || v.Status == "running" {
		<div hx-get={ fmt.Sprintf("/exercises/submissions/%d/status", v.SubmissionID) }
			hx-trigger="every 1s" hx-target="#exercise-status" hx-swap="innerHTML">
			<p>⏳ Running tests…</p>
		</div>
	}
	if v.Status == "complete" && v.Passed {
		<p class="saved">✓ Tests pass — step complete.</p>
		<details>
			<summary>Test output</summary>
			<pre class="test-output">{ v.Output }</pre>
		</details>
	}
	if v.Status == "complete" && !v.Passed && v.SubmissionID != 0 {
		<p class="test-failed">✗ Tests failing — keep going.</p>
		<details open>
			<summary>Test output</summary>
			<pre class="test-output">{ v.Output }</pre>
		</details>
	}
	if v.Status == "failed" {
		<div class="eval-failed">
			<p>Run failed.</p>
			<pre class="test-output">{ v.Output }</pre>
		</div>
	}
}
```

In `templates/step.templ`, inside `StepPage` after the module-links block add:

```templ
		if v.VideoEmbedURL != "" {
			<div class="video">
				<iframe src={ templ.SafeURL(v.VideoEmbedURL) } title="Lecture video" allowfullscreen></iframe>
			</div>
			<p class="video-fallback">
				<a href={ templ.SafeURL(v.VideoWatchURL) } target="_blank">Watch on YouTube</a>
			</p>
		}
```

and after the eval-section container add:

```templ
		if v.Type == "code" {
			<div id="exercise-section"
				hx-get={ "/exercises/" + v.ModuleSlug + "/" + v.StepSlug }
				hx-trigger="load" hx-swap="innerHTML"></div>
		}
```

Run: `make generate` then `make test` — Expected: PASS (nothing consumes the new components yet)

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: vendored CodeMirror bundle, exercise editor client, templates"
```

---

### Task 6: Exercise endpoints, transport, wiring, e2e

**Files:**
- Create: `internal/exercise/endpoint.go`, `internal/exercise/transport.go`, `e2e/exercise_test.go`
- Create: `e2e/testdata/content/modules/03-test-code/module.yaml`, `e2e/testdata/content/modules/03-test-code/steps/01-fix.md`, `e2e/testdata/content/modules/03-test-code/exercises/01-fix/{adder.go,adder_test.go}`
- Modify: `cmd/tour/main.go` (wire), `e2e/harness_test.go` (wire with sync runner), `internal/tour/transport.go` (stepVM: video fields)

**Interfaces:**
- Produces: `exercise.RegisterRoutes(mux *http.ServeMux, svc ExerciseService)` with routes `GET /exercises/{module}/{step}`, `PUT /exercises/{module}/{step}/draft` (JSON `{"files":{...}}` → 204), `POST /exercises/{module}/{step}/check` (JSON `[]Diagnostic`, never-null array), `POST /exercises/{module}/{step}/run` (→ `ExerciseStatus` partial), `POST /exercises/{module}/{step}/reset` (→ `ExerciseSection`), `GET /exercises/submissions/{id}/status` (→ `ExerciseStatus`).

- [ ] **Step 1: Endpoints** — `internal/exercise/endpoint.go`:

```go
package exercise

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// ExerciseService is the contract the endpoints require; *Service satisfies it.
type ExerciseService interface {
	State(ctx context.Context, ref course.StepRef) (View, error)
	SaveDraft(ctx context.Context, ref course.StepRef, files map[string]string) error
	ResetDraft(ctx context.Context, ref course.StepRef) error
	Check(ctx context.Context, ref course.StepRef) ([]Diagnostic, error)
	Run(ctx context.Context, ref course.StepRef) error
	RefForSubmission(ctx context.Context, id int64) (course.StepRef, error)
}

type SectionRequest struct{ Module, Step string }

func (r SectionRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type SaveDraftRequest struct {
	Module, Step string
	Files        map[string]string
}

type StateResponse struct {
	Ref  course.StepRef
	View View
}

func makeSectionEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}

func makeSaveDraftEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SaveDraftRequest)
		if err := (SectionRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		return nil, svc.SaveDraft(ctx, ref, req.Files)
	}
}

func makeCheckEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		return svc.Check(ctx, course.StepRef{Module: req.Module, Step: req.Step})
	}
}

func makeRunEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.Run(ctx, ref); err != nil {
			return nil, err
		}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}

func makeResetEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.ResetDraft(ctx, ref); err != nil {
			return nil, err
		}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}

func makeStatusEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		ref, err := svc.RefForSubmission(ctx, request.(int64))
		if err != nil {
			return nil, err
		}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}
```

- [ ] **Step 2: Transport** — `internal/exercise/transport.go`:

```go
package exercise

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

const maxDraftBytes = 1 << 20

func RegisterRoutes(mux *http.ServeMux, svc ExerciseService) {
	section := makeSectionEndpoint(svc)
	saveDraft := makeSaveDraftEndpoint(svc)
	check := makeCheckEndpoint(svc)
	run := makeRunEndpoint(svc)
	reset := makeResetEndpoint(svc)
	status := makeStatusEndpoint(svc)

	pathReq := func(r *http.Request) SectionRequest {
		return SectionRequest{Module: r.PathValue("module"), Step: r.PathValue("step")}
	}
	renderSection := func(w http.ResponseWriter, r *http.Request, resp any, err error) {
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(StateResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.ExerciseSection(exerciseVM(res.Ref, res.View)))
	}
	renderStatus := func(w http.ResponseWriter, r *http.Request, resp any, err error) {
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(StateResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.ExerciseStatus(exerciseVM(res.Ref, res.View)))
	}

	mux.HandleFunc("GET /exercises/{module}/{step}", func(w http.ResponseWriter, r *http.Request) {
		resp, err := section(r.Context(), pathReq(r))
		renderSection(w, r, resp, err)
	})

	mux.HandleFunc("PUT /exercises/{module}/{step}/draft", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Files map[string]string `json:"files"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxDraftBytes)).Decode(&body); err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		req := SaveDraftRequest{Module: r.PathValue("module"), Step: r.PathValue("step"), Files: body.Files}
		if _, err := saveDraft(r.Context(), req); err != nil {
			api.RenderError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /exercises/{module}/{step}/check", func(w http.ResponseWriter, r *http.Request) {
		resp, err := check(r.Context(), pathReq(r))
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		diags := resp.([]Diagnostic)
		if diags == nil {
			diags = []Diagnostic{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(diags)
	})

	mux.HandleFunc("POST /exercises/{module}/{step}/run", func(w http.ResponseWriter, r *http.Request) {
		resp, err := run(r.Context(), pathReq(r))
		renderStatus(w, r, resp, err)
	})

	mux.HandleFunc("POST /exercises/{module}/{step}/reset", func(w http.ResponseWriter, r *http.Request) {
		resp, err := reset(r.Context(), pathReq(r))
		renderSection(w, r, resp, err)
	})

	mux.HandleFunc("GET /exercises/submissions/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		resp, err := status(r.Context(), id)
		renderStatus(w, r, resp, err)
	})
}

func exerciseVM(ref course.StepRef, v View) templates.ExerciseVM {
	files := make([]templates.ExerciseFileVM, 0, len(v.Files))
	for _, f := range v.Files {
		files = append(files, templates.ExerciseFileVM{Name: f.Name, Content: f.Content, Readonly: f.Readonly})
	}
	base := "/exercises/" + ref.Module + "/" + ref.Step
	cfg, _ := json.Marshal(struct {
		Files    []templates.ExerciseFileVM `json:"files"`
		SaveURL  string                     `json:"saveUrl"`
		CheckURL string                     `json:"checkUrl"`
		RunURL   string                     `json:"runUrl"`
	}{files, base + "/draft", base + "/check", base + "/run"})
	vm := templates.ExerciseVM{
		ModuleSlug: ref.Module, StepSlug: ref.Step,
		Mode: v.Meta.Mode, ModeLabel: modeLabel(v.Meta.Mode),
		Attribution: v.Step.Attribution, ConfigJSON: string(cfg),
		Files: files,
	}
	if v.Submission != nil {
		vm.Status = string(v.Submission.Status)
		vm.Output = v.Submission.TestOutput
		vm.SubmissionID = v.Submission.ID
		if v.Submission.Passed != nil {
			vm.Passed = *v.Submission.Passed
		}
	}
	return vm
}

func modeLabel(mode string) string {
	if mode == "create" {
		return "Build it"
	}
	return "Fix the bug"
}
```

Note: `ExerciseVM` needs the `Files []ExerciseFileVM` field added in `templates/viewmodels.go` (used only for VM construction symmetry; the template reads `ConfigJSON`). Add it.

- [ ] **Step 3: Wire main and the e2e harness**

`cmd/tour/main.go`: extract `subsRepo := sqlite.NewSubmissionRepo(db)` (reuse in the eval wiring) and add after `eval.RegisterRoutes`:

```go
	exercise.RegisterRoutes(mux, exercise.NewService(courseRepo, sqlite.NewDraftsRepo(db),
		subsRepo, progressRepo, exercise.Workspace{}))
```

`e2e/harness_test.go`: same wiring with `exercise.WithRunAsync(func(f func()) { f() })`.

- [ ] **Step 4: e2e fixture**

`e2e/testdata/content/modules/03-test-code/module.yaml`:
```yaml
title: "Test Code"
kind: lecture
order: 3
```

`e2e/testdata/content/modules/03-test-code/steps/01-fix.md`:
```markdown
---
title: Fix the adder
type: code
video: "https://www.youtube.com/watch?v=testvideo1"
code:
  mode: fix
  editable: ["adder.go"]
  readonly: ["adder_test.go"]
  run: ["go", "test", "."]
  timeout: 1m
---

Make the test pass.
```

`e2e/testdata/content/modules/03-test-code/exercises/01-fix/adder.go`:
```go
package adder

func Add(a, b int) int { return a - b }
```

`e2e/testdata/content/modules/03-test-code/exercises/01-fix/adder_test.go`:
```go
package adder

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("Add(2,3) = %d", Add(2, 3))
	}
}
```

- [ ] **Step 5: Write the e2e tests** — `e2e/exercise_test.go` (uses the real Go toolchain; the harness's sync runner makes runs complete before the response renders):

```go
package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func put(t *testing.T, url, body string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func post(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestExerciseFlow(t *testing.T) {
	app := newApp(t, options{})
	base := app.TS.URL + "/exercises/03-test-code/01-fix"

	// editor section renders scaffold, config, and both tabs' content
	body := fetch(t, base)
	for _, want := range []string{"exercise-root", "data-config", "saveUrl", "adder_test.go", "Fix the bug"} {
		if !strings.Contains(body, want) {
			t.Fatalf("section missing %q", want)
		}
	}

	// run the buggy scaffold: completes with failing tests
	if _, out := post(t, base+"/run"); !strings.Contains(out, "Tests failing") {
		t.Fatalf("buggy run: %q", out)
	}

	// fix it via a draft, then run again: passes and completes the step
	if code := put(t, base+"/draft",
		`{"files":{"adder.go":"package adder\n\nfunc Add(a, b int) int { return a + b }\n"}}`); code != 204 {
		t.Fatalf("draft save = %d", code)
	}
	if _, out := post(t, base+"/run"); !strings.Contains(out, "Tests pass") {
		t.Fatalf("fixed run: %q", out)
	}
	if !strings.Contains(fetch(t, app.TS.URL+"/modules/03-test-code/steps/01-fix"), "Completed") {
		t.Fatal("passing run should auto-complete the step")
	}

	// polling endpoint serves the latest state
	if code, out := func() (int, string) {
		resp, err := http.Get(app.TS.URL + "/exercises/submissions/2/status")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}(); code != 200 || !strings.Contains(out, "Tests pass") {
		t.Fatalf("status: %d %q", code, out)
	}
}

func TestExerciseCheckDiagnostics(t *testing.T) {
	app := newApp(t, options{})
	base := app.TS.URL + "/exercises/03-test-code/01-fix"

	if code := put(t, base+"/draft",
		`{"files":{"adder.go":"package adder\n\nfunc Add(a, b int) int { return a +\n"}}`); code != 204 {
		t.Fatalf("draft save = %d", code)
	}
	code, out := post(t, base+"/check")
	if code != 200 || !strings.Contains(out, `"adder.go"`) || !strings.Contains(out, `"line"`) {
		t.Fatalf("check: %d %q", code, out)
	}

	// draft touching a read-only file is rejected
	if code := put(t, base+"/draft", `{"files":{"adder_test.go":"package adder\n"}}`); code != 400 {
		t.Fatalf("read-only draft = %d, want 400", code)
	}
}

func TestVideoEmbedOnStepPage(t *testing.T) {
	app := newApp(t, options{})
	body := fetch(t, app.TS.URL+"/modules/03-test-code/steps/01-fix")
	if !strings.Contains(body, "youtube-nocookie.com/embed/testvideo1") ||
		!strings.Contains(body, "Watch on YouTube") {
		t.Fatal("video embed missing")
	}
}
```

For the video test to pass, `internal/tour/transport.go` `stepVM` must map the new fields:

```go
	vm.VideoWatchURL = v.Step.Video
	vm.VideoEmbedURL = templates.YouTubeEmbedURL(v.Step.Video)
```

(`tour.StepView` already carries the full `course.Step`.)

Run: `make test` — Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: exercise endpoints, transport, wiring, e2e coverage"
```

---

### Task 7: Exemplar content — three exercises, attribution page, lecture video

**Files:**
- Create: `content/modules/02-rpc-and-threads/steps/05-fix-racy-counter.md`, `content/modules/02-rpc-and-threads/exercises/05-fix-racy-counter/{counter.go,counter_test.go}`
- Create: `content/modules/02-rpc-and-threads/steps/06-build-kv-store.md`, `content/modules/02-rpc-and-threads/exercises/06-build-kv-store/{kv.go,kv_test.go}`
- Create: `content/modules/lab-01-mapreduce/steps/04-warm-up-wordcount.md`, `content/modules/lab-01-mapreduce/exercises/04-warm-up-wordcount/{wc.go,wc_test.go}`
- Create: `content/ATTRIBUTION.md`, `e2e/testdata/content/ATTRIBUTION.md`
- Modify: `content/modules/02-rpc-and-threads/steps/04-wrap-up.md` (point at the exercises), `content/modules/01-introduction/steps/01-read-the-paper.md` (+video), `internal/coursefs/coursefs.go` (+`RenderMarkdownFile`), `internal/coursefs/realcontent_test.go` (+assertions), `templates/coursemap.templ` (+footer link), `templates/document.templ` — no change; add `templates.SimplePage`, `internal/tour/transport.go` (+`RegisterAttribution`), `cmd/tour/main.go`, `e2e/harness_test.go` (+attribution wiring), `e2e/tour_test.go` (+attribution page test)

Existing steps are never renamed (student progress rows key on slugs); new exercise steps append after the wrap-up with prose framing.

- [ ] **Step 1: Update the real-content guard first** — append to `internal/coursefs/realcontent_test.go` `TestRealContentParses`:

```go
	mod, _ := c.Module("02-rpc-and-threads")
	var codeSteps int
	for _, s := range mod.Steps {
		if s.Type == course.StepCode {
			codeSteps++
		}
	}
	if codeSteps != 2 {
		t.Errorf("lecture 2 code steps = %d, want 2", codeSteps)
	}
	if lab, _ := c.Module("lab-01-mapreduce"); len(lab.Steps) != 4 {
		t.Errorf("lab 1 steps = %d, want 4 (incl. warm-up)", len(lab.Steps))
	}
	if _, step, ok := c.Step(course.StepRef{Module: "01-introduction", Step: "01-read-the-paper"}); !ok || step.Video == "" {
		t.Error("lecture 1 read step should carry a video URL")
	}
```

(add import `internal/course`). Run: `go test ./internal/coursefs/ -run RealContent` — Expected: FAIL

- [ ] **Step 2: Author the racy-counter exercise**

`content/modules/02-rpc-and-threads/steps/05-fix-racy-counter.md`:
```markdown
---
title: "Exercise — Fix: the racy counter"
type: code
code:
  mode: fix
  editable: ["counter.go"]
  readonly: ["counter_test.go"]
  run: ["go", "test", "-race", "."]
  timeout: 2m
---

This counter is incremented from fifty goroutines at once, and it loses
updates. The test runs with the race detector on, so it will tell you
*exactly* where the unsynchronized access happens — read its output before
you touch the code.

The mutex is already sitting in the struct, unused. Your job is to put it to
work so that `Inc` and `Value` are safe to call concurrently. When
`go test -race` passes, the step completes.
```

`content/modules/02-rpc-and-threads/exercises/05-fix-racy-counter/counter.go`:
```go
package counter

import "sync"

// Counter counts events from many goroutines. Nothing here is synchronized
// yet — the mutex is waiting for you.
type Counter struct {
	mu sync.Mutex
	n  int
}

func (c *Counter) Inc() { c.n++ }

func (c *Counter) Value() int { return c.n }
```

`content/modules/02-rpc-and-threads/exercises/05-fix-racy-counter/counter_test.go`:
```go
package counter

import (
	"sync"
	"testing"
)

func TestConcurrentIncrements(t *testing.T) {
	c := &Counter{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if got := c.Value(); got != 10000 {
		t.Fatalf("Value() = %d, want 10000 — increments were lost", got)
	}
}
```

- [ ] **Step 3: Author the KV-store exercise**

`content/modules/02-rpc-and-threads/steps/06-build-kv-store.md`:
```markdown
---
title: "Exercise — Build: a concurrent-safe KV store"
type: code
code:
  mode: create
  editable: ["kv.go"]
  readonly: ["kv_test.go"]
  run: ["go", "test", "-race", "."]
  timeout: 2m
---

Every lab from Lab 2 onward has a key/value store at its heart. Build the
smallest honest version now: `Get`, `Put`, and `Append`, all safe under
concurrent callers.

Read the test file first — it defines the contract, including what `Append`
returns. Run early and often; the race detector is on.
```

`content/modules/02-rpc-and-threads/exercises/06-build-kv-store/kv.go`:
```go
package kv

import "sync"

// Store is a concurrency-safe string key/value store. Implement NewStore,
// Get, Put, and Append so the tests pass — every method may be called from
// many goroutines at once.
type Store struct {
	mu   sync.Mutex
	data map[string]string
}

func NewStore() *Store {
	// TODO: construct the store
	return nil
}

// Get returns the value for key, or "" when absent.
func (s *Store) Get(key string) string {
	// TODO
	return ""
}

// Put stores value under key.
func (s *Store) Put(key, value string) {
	// TODO
}

// Append appends value to the current value for key and returns the value
// from before the append.
func (s *Store) Append(key, value string) string {
	// TODO
	return ""
}
```

`content/modules/02-rpc-and-threads/exercises/06-build-kv-store/kv_test.go`:
```go
package kv

import (
	"fmt"
	"sync"
	"testing"
)

func TestBasicOps(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
	if got := s.Get("missing"); got != "" {
		t.Fatalf("Get(missing) = %q, want empty", got)
	}
	s.Put("k", "v1")
	if got := s.Get("k"); got != "v1" {
		t.Fatalf("Get = %q", got)
	}
	if old := s.Append("k", "+v2"); old != "v1" {
		t.Fatalf("Append returned %q, want previous value v1", old)
	}
	if got := s.Get("k"); got != "v1+v2" {
		t.Fatalf("after append: %q", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", n%4)
			for j := 0; j < 100; j++ {
				s.Put(key, "v")
				s.Append(key, "+")
				_ = s.Get(key)
			}
		}(i)
	}
	wg.Wait()
}
```

Also edit `content/modules/02-rpc-and-threads/steps/04-wrap-up.md` — append to the body:

```markdown

Then cement it: the next two steps are hands-on exercises — fix a racy
counter, then build the tiny KV store you will meet again in every lab.
```

- [ ] **Step 4: Author the Lab 1 warm-up (with attribution)**

`content/modules/lab-01-mapreduce/steps/04-warm-up-wordcount.md`:
```markdown
---
title: "Exercise — Warm-up: sequential word count"
type: code
attribution: "Exercise concept adapted from the MIT 6.5840 Lab 1 (MapReduce) word-count application (CC BY 3.0 US)."
code:
  mode: create
  editable: ["wc.go"]
  readonly: ["wc_test.go"]
  run: ["go", "test", "."]
  timeout: 1m
---

Before (or while) building the real thing, get the kernel of Lab 1 working
sequentially: count words. This is exactly the map half of the lab's
word-count application, minus the distribution.

A word is a maximal run of letters — `strings.FieldsFunc` with
`unicode.IsLetter` does the splitting; the counting is yours.
```

`content/modules/lab-01-mapreduce/exercises/04-warm-up-wordcount/wc.go`:
```go
package wc

import (
	"strings"
	"unicode"
)

// WordCount returns how many times each word appears in contents, where a
// word is any maximal run of letters. This mirrors the map function you
// will implement for real in Lab 1's word-count application.
func WordCount(contents string) map[string]int {
	// TODO: split contents into letter-runs and count them.
	_ = strings.FieldsFunc
	_ = unicode.IsLetter
	return nil
}
```

`content/modules/lab-01-mapreduce/exercises/04-warm-up-wordcount/wc_test.go`:
```go
package wc

import "testing"

func TestWordCount(t *testing.T) {
	got := WordCount("the quick brown fox jumps over the lazy dog the end")
	if got["the"] != 3 || got["fox"] != 1 || got["end"] != 1 {
		t.Fatalf("counts = %v", got)
	}
	if len(got) != 9 {
		t.Fatalf("distinct words = %d, want 9", len(got))
	}
	if got2 := WordCount("one-two one two"); got2["one"] != 2 || got2["two"] != 2 {
		t.Fatalf("punctuation must split words: %v", got2)
	}
}
```

- [ ] **Step 5: Attribution page + footer + lecture video**

`content/ATTRIBUTION.md`:
```markdown
# Attribution

This project links to and builds on materials from MIT's 6.824 / 6.5840
Distributed Systems course, which are licensed under a Creative Commons
Attribution 3.0 United States (CC BY 3.0 US) license.

- Course site: <https://pdos.csail.mit.edu/6.824/>
- License: <https://creativecommons.org/licenses/by/3.0/us/>

Adapted material:

- **Lab 1 warm-up: sequential word count** — exercise concept adapted from
  the 6.5840 Lab 1 (MapReduce) word-count application.

All other guidance text and exercises are original to this project.
```

Copy a two-line variant to `e2e/testdata/content/ATTRIBUTION.md` (`# Attribution` + one body line).

Add to `internal/coursefs/coursefs.go`:

```go
// RenderMarkdownFile renders a standalone markdown file (e.g. the
// attribution page) to HTML with the same renderer the course content uses.
func RenderMarkdownFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := goldmark.New().Convert(raw, &buf); err != nil {
		return "", fmt.Errorf("%s: render markdown: %w", path, err)
	}
	return buf.String(), nil
}
```

Add to `templates/` (new file `templates/simple.templ`):

```templ
package templates

templ SimplePage(title string, bodyHTML string) {
	<header class="topbar">
		<a href="/">⌂ Map</a>
		<span class="topbar-title">{ title }</span>
	</header>
	<main class="content">
		@templ.Raw(bodyHTML)
	</main>
}
```

Add to `internal/tour/transport.go`:

```go
// RegisterAttribution serves the pre-rendered attribution page.
func RegisterAttribution(mux *http.ServeMux, bodyHTML string) {
	mux.HandleFunc("GET /attribution", func(w http.ResponseWriter, r *http.Request) {
		api.RenderHTML(w, r, http.StatusOK,
			templates.Document("Attribution", templates.SimplePage("Attribution", bodyHTML)))
	})
}
```

In `templates/coursemap.templ`, before `</main>` add:

```templ
		<footer class="footer">
			<a href="/attribution">Attribution &amp; licensing</a>
		</footer>
```

with CSS `.footer { margin-top: 3rem; font-size: .8rem; color: #888; }` appended to `static/styles.css`.

Wire in `cmd/tour/main.go` (and mirror in `e2e/harness_test.go`):

```go
	attributionHTML, err := coursefs.RenderMarkdownFile(filepath.Join(cfg.ContentDir, "ATTRIBUTION.md"))
	if err != nil {
		log.Fatalf("attribution page: %v", err)
	}
	tour.RegisterAttribution(mux, attributionHTML)
```

Lecture video — in `content/modules/01-introduction/steps/01-read-the-paper.md` frontmatter add:

```yaml
video: "https://www.youtube.com/watch?v=cQP8WApzIQQ"
```

(That is the well-known Lecture 1 recording from the course's public playlist. Do not add video URLs for other lectures in this task — each needs its ID verified against the official playlist first; that is ongoing content authoring.)

Append to `e2e/tour_test.go`:

```go
func TestAttributionPage(t *testing.T) {
	app := newApp(t, options{})
	code, body := get(t, app.TS.URL+"/attribution")
	if code != 200 || !strings.Contains(body, "Attribution") {
		t.Fatalf("attribution page: %d", code)
	}
}
```

- [ ] **Step 6: Full suite + commit**

Run: `make test` — Expected: PASS (realcontent guard from Step 1 now green; exercise scaffolds validate).

Sanity note for the racy-counter exercise: its *scaffold* must fail its own test under `-race` — verify manually once:
```bash
cd content/modules/02-rpc-and-threads/exercises/05-fix-racy-counter && go mod init tmp 2>/dev/null; go test -race . ; cd - && rm content/modules/02-rpc-and-threads/exercises/05-fix-racy-counter/go.mod
```
Expected: FAIL (race detected or lost updates). Remove the temporary go.mod afterwards as shown.

```bash
git add -A && git commit -m "feat: exemplar exercises, attribution page, lecture 1 video"
```

---

### Task 8: LLM feedback for exercises, README, final verification

**Files:**
- Create: `content/rubric/exercise.md`, `e2e/testdata/content/rubric/exercise.md`, `internal/exercise/prompts.go`, `internal/exercise/prompts_test.go`
- Modify: `internal/exercise/service.go` (+llm, +Feedback), `internal/exercise/models.go` (View.Evaluation), `internal/exercise/endpoint.go` (+Feedback), `internal/exercise/transport.go` (+route, VM report), `internal/exercise/service_test.go` (+feedback test, updated NewService call sites), `templates/exercise.templ` (+feedback button/report), `templates/viewmodels.go` (ExerciseVM.Report/FeedbackEnabled), `cmd/tour/main.go`, `e2e/harness_test.go`, `README.md`

**Interfaces:**
- `exercise.NewService` gains parameters and an error: `NewService(c CourseRepo, d DraftRepo, subs SubmissionRepo, p ProgressMarker, r Runner, llm eval.LLM, contentDir string, opts ...Option) (*Service, error)` — loads `content/rubric/exercise.md` at construction (same `eval.LoadRubric`); `llm` nil ⇒ feedback hidden. `SubmissionRepo` interface gains `InsertEvaluation(ctx, e eval.Evaluation) (int64, error)` and `EvaluationForSubmission(ctx, id int64) (*eval.Evaluation, error)` (sqlite already implements both). New service method `Feedback(ctx, ref) error`; `View` gains `Evaluation *eval.Evaluation`.

- [ ] **Step 1: Author the rubric** — `content/rubric/exercise.md` (copy identical file to `e2e/testdata/content/rubric/exercise.md`):

```markdown
---
version: "1"
---

Score each criterion 1–5:

1. **Correctness** — Weigh the test output heavily; failing tests cap this
   criterion at 2.
2. **Idiomatic Go** — Naming, zero-value use, error handling, no needless
   cleverness.
3. **Concurrency reasoning** — Where shared state is involved: is the
   synchronization necessary and sufficient, not superstitious?

Keep feedback proportionate to a short exercise: two or three concrete
observations beat an essay. In `next_steps`, give the single most valuable
improvement first.
```

- [ ] **Step 2: TDD the prompt builder** — `internal/exercise/prompts_test.go`:

```go
package exercise_test

import (
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
)

func TestBuildExercisePrompt(t *testing.T) {
	r := eval.Rubric{Version: "1", Body: "EX-RUBRIC"}
	step := course.Step{Title: "Fix adder", Attribution: "adapted"}
	system, user := exercise.BuildExercisePrompt(r, course.Module{Title: "Lecture X"}, step,
		map[string]string{"b.go": "package b", "a.go": "package a"}, "TEST-OUT", true)
	if !strings.Contains(system, "EX-RUBRIC") || !strings.Contains(system, `"criteria"`) {
		t.Fatalf("system: %q", system)
	}
	ai, bi := strings.Index(user, "--- a.go ---"), strings.Index(user, "--- b.go ---")
	if ai < 0 || bi < 0 || ai > bi || !strings.Contains(user, "TEST-OUT") ||
		!strings.Contains(user, "tests are passing") {
		t.Fatalf("user: %q", user)
	}
}
```

Run: `go test ./internal/exercise/ -run Prompt` — Expected: FAIL. Implement `internal/exercise/prompts.go`:

```go
package exercise

import (
	"fmt"
	"sort"
	"strings"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

const verdictShape = `{"criteria":[{"name":"<criterion>","score":<1-5>,"justification":"<why>"}],` +
	`"summary":"<2-3 sentences>","next_steps":["<concrete action>"]}`

// BuildExercisePrompt assembles the feedback prompt for a completed
// exercise run. passed reports whether the tests were green.
func BuildExercisePrompt(r eval.Rubric, mod course.Module, step course.Step,
	files map[string]string, testOutput string, passed bool) (system, user string) {
	system = "You are a strict but constructive teaching assistant for MIT 6.824 " +
		"(Distributed Systems). Review a student's short coding exercise. Respond with " +
		"ONLY a JSON object in exactly this shape:\n" + verdictShape +
		"\n\nRubric (version " + r.Version + "):\n" + r.Body
	state := "tests are failing"
	if passed {
		state = "tests are passing"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Module: %s — %s (%s)\n\nTest output:\n%s\n\nStudent code:\n",
		mod.Title, step.Title, state, testOutput)
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&b, "\n--- %s ---\n%s\n", p, files[p])
	}
	return system, b.String()
}
```

- [ ] **Step 3: Extend the service** — in `internal/exercise/service.go`:

- Struct gains `llm eval.LLM` and `rubric eval.Rubric`; `NewService` becomes:

```go
func NewService(c CourseRepo, d DraftRepo, subs SubmissionRepo, p ProgressMarker, r Runner,
	llm eval.LLM, contentDir string, opts ...Option) (*Service, error) {
	rubric, err := eval.LoadRubric(filepath.Join(contentDir, "rubric", "exercise.md"))
	if err != nil {
		return nil, fmt.Errorf("load exercise rubric: %w", err)
	}
	s := &Service{
		course: c, drafts: d, subs: subs, progress: p, runner: r,
		llm: llm, rubric: rubric,
		runAsync: func(f func()) { go f() }, now: time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}
```

- Add `FeedbackEnabled() bool { return s.llm != nil }`, and:

```go
// Feedback reviews the latest completed run with the LLM. Synchronous —
// exercise code is small and the wait is a click away from the result.
func (s *Service) Feedback(ctx context.Context, ref course.StepRef) error {
	step, err := s.codeStep(ref)
	if err != nil {
		return err
	}
	if s.llm == nil {
		return fmt.Errorf("%w: evaluation mode is locked", api.ErrInvalid)
	}
	sub, err := s.subs.LatestForStep(ctx, ref)
	if err != nil {
		return err
	}
	if sub == nil || sub.Status != eval.StatusComplete {
		return fmt.Errorf("%w: run the exercise before requesting feedback", api.ErrInvalid)
	}
	var files map[string]string
	if err := json.Unmarshal([]byte(sub.Content), &files); err != nil {
		return err
	}
	mod, _, _ := s.course.Course().Step(ref)
	passed := sub.Passed != nil && *sub.Passed
	system, user := BuildExercisePrompt(s.rubric, *mod, *step, files, sub.TestOutput, passed)
	raw, err := s.llm.Complete(ctx, system, user)
	if err != nil {
		return fmt.Errorf("feedback: %w", err)
	}
	verdict, err := eval.ParseVerdict(raw)
	if err != nil {
		return fmt.Errorf("feedback verdict: %w", err)
	}
	_, err = s.subs.InsertEvaluation(ctx, eval.Evaluation{
		SubmissionID: sub.ID, Model: s.llm.Model(), RubricVersion: s.rubric.Version,
		Verdict: verdict, CreatedAt: s.now().UTC(),
	})
	return err
}
```

- `State` additionally loads `view.Evaluation, err = s.subs.EvaluationForSubmission(ctx, view.Submission.ID)` when a submission exists (mirror eval's pattern); `View` gains `Evaluation *eval.Evaluation`.
- Update every `NewService` call site: service tests (pass `nil, "../../content"`), `cmd/tour/main.go` (pass the same `llm` variable eval uses, `cfg.ContentDir`, handle the error with `log.Fatalf`), `e2e/harness_test.go` (pass `o.LLM`, `o.ContentDir`).

Add a service test (fake LLM lives in this file):

```go
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
```

- [ ] **Step 4: Endpoint, route, templates**

`endpoint.go`: `ExerciseService` gains `Feedback(ctx, ref) error` and `FeedbackEnabled() bool`; add `makeFeedbackEndpoint` (Feedback then State → StateResponse). `transport.go`: route `POST /exercises/{module}/{step}/feedback` rendering via `renderStatus`; `exerciseVM` maps `FeedbackEnabled: svc.FeedbackEnabled()` — pass the flag into `exerciseVM` as a parameter — and when `v.Evaluation != nil` builds `vm.Report` exactly as eval's `sectionVM` does (same `templates.ReportVM`).

`templates/viewmodels.go`: `ExerciseVM` gains `FeedbackEnabled bool` and `Report *ReportVM`.

`templates/exercise.templ` — in `ExerciseStatus`, after the complete branches add:

```templ
	if v.Status == "complete" && v.FeedbackEnabled && v.Report == nil && v.SubmissionID != 0 {
		<button class="btn" type="button"
			hx-post={ "/exercises/" + v.ModuleSlug + "/" + v.StepSlug + "/feedback" }
			hx-target="#exercise-status" hx-swap="innerHTML">Get feedback</button>
	}
	if v.Report != nil {
		@Report(*v.Report)
	}
```

- [ ] **Step 5: README updates** — add to `README.md` after the "Content authoring" section:

```markdown
## Interactive exercises

Steps of type `code` open an in-browser editor (vendored CodeMirror). An
exercise lives in `content/modules/<module>/exercises/<step-slug>/` with
its scaffold files; the step's frontmatter `code:` block declares editable
vs. read-only files, the test command, and a timeout. Drafts autosave to
SQLite; every run executes in a throwaway workspace with the repo's Go
toolchain; passing tests complete the step. With evaluation mode unlocked,
completed runs offer rubric-based LLM feedback
(`content/rubric/exercise.md`).

Adapted CC BY course material is credited per-exercise via `attribution:`
frontmatter and globally at `/attribution` (`content/ATTRIBUTION.md`).

### Regenerating the editor bundle

`static/codemirror/codemirror.js` is a committed build artifact. To upgrade
CodeMirror: `cd scripts/build-editor && npm install && npx esbuild entry.js
--bundle --minify --format=iife --outfile=../../static/codemirror/codemirror.js`
```

- [ ] **Step 6: Full verification + commit**

Run: `make test` — Expected: PASS across all packages.
Then a manual smoke: `make run`, open a code step, verify editor loads, tabs switch, Run works, diagnostics appear for a syntax error.

```bash
git add -A && git commit -m "feat: LLM feedback for exercises, README docs"
```

---

## Done — definition of v2 complete

All eight tasks checked and `make test` green: the tour serves three
interactive exercises (editable in-browser with live diagnostics, validated
by `go test` in throwaway workspaces, complete-on-pass), Lecture 1 embeds
its video, `/attribution` credits adapted CC BY material, and unlocked
evaluation mode offers rubric feedback on completed runs. Rebuild the
Docker image (`docker compose up -d --build` from `docker/`) to ship it to
the running container.
