# MIT 6.824 Course Tour Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go-tour-style local web app that walks a student through MIT 6.824 module by module, with per-step progress, mid-flight notes, and an optional OpenRouter-backed evaluation mode for lab code and reading-question answers.

**Architecture:** Single Go binary, server-rendered with templ + HTMX. Course content is Markdown/YAML files parsed at boot into an immutable in-memory course; SQLite stores only mutable user state (progress, notes, submissions, evaluations). Each feature package (`tour`, `notes`, `eval`) has three layers: `transport.go` (routes/decoders/rendering), `endpoint.go` (contract: request models, validation, service interface, endpoint factories), `service.go` (business logic; defines the repo/provider interfaces it consumes).

**Tech Stack:** Go ≥1.23 (stdlib `net/http` routing), `github.com/a-h/templ`, `github.com/yuin/goldmark`, `gopkg.in/yaml.v3`, `modernc.org/sqlite` (pure Go, no CGO), HTMX (vendored static file). Docker + docker compose.

## Global Constraints

- Go ≥ 1.23; router is stdlib `net/http` method patterns (`GET /modules/{module}/steps/{step}`). No web framework.
- Allowed dependencies: `github.com/a-h/templ`, `github.com/yuin/goldmark`, `gopkg.in/yaml.v3`, `modernc.org/sqlite`. Nothing else without explicit approval.
- Layering per feature package: `transport.go` → `endpoint.go` → `service.go`. Services define the interfaces of what they consume; implementations (`coursefs`, `sqlite`, `openrouter`) are injected in `cmd/tour/main.go`.
- Generated `*_templ.go` files are committed. Regenerate with `make generate` after editing any `.templ` file, before build/test/commit.
- TDD: write the failing test first. Run `make test` (which regenerates templ) before every commit.
- Never copy MIT course prose into `content/` — original guidance only; link out to papers/lab pages.
- No personal names and no conversation-scoped labels in committed docs.
- All mutable state lives in SQLite at `$DATA_DIR/tour.db`. Content and the lab repo are read-only at runtime.
- Canonical step identity everywhere is `course.StepRef{Module, Step string}` (module slug + step slug); never a bare step slug.

## File Structure

```
cmd/tour/main.go                  # wiring only: config → adapters → services → routes
pkg/api/api.go                    # Endpoint type, ErrNotFound/ErrInvalid, RenderHTML/RenderError
internal/config/config.go         # env-var config with defaults
internal/course/course.go         # pure domain types + lookup/navigation (no I/O)
internal/coursefs/coursefs.go     # file-backed course loader + Repo wrapper
internal/coursefs/frontmatter.go  # ----delimited YAML frontmatter splitter
internal/sqlite/db.go             # Open (pragmas) + Migrate (embedded SQL)
internal/sqlite/migrations/001_init.sql
internal/sqlite/progress.go       # ProgressRepo
internal/sqlite/notes.go          # NotesRepo
internal/sqlite/submissions.go    # SubmissionRepo (submissions + evaluations)
internal/tour/{service,endpoint,transport}.go
internal/notes/{service,endpoint,transport}.go
internal/eval/{models,service,endpoint,transport}.go
internal/eval/prompts.go          # rubric/guidance loading + prompt assembly
internal/eval/verdict.go          # LLM output → Verdict parsing
internal/eval/labrepo.go          # FSLabRepo: Snapshot + RunTests against mounted repo
internal/openrouter/client.go     # eval.LLM implementation
e2e/                              # dedicated integration-test package: harness + per-feature suites + testdata
templates/viewmodels.go           # plain-Go view models (templates never import features)
templates/*.templ                 # document, coursemap, step, notes, eval
static/{static.go,styles.css,htmx.min.js}
content/modules/<slug>/{module.yaml,steps/*.md}
content/rubric/{question.md,lab.md}
content/guidance/<module-slug>.md
scripts/gen-skeleton/main.go      # one-shot full-course skeleton generator
docker/{Dockerfile,docker-compose.yml,.env.example}
Makefile, README.md, .gitignore
```

Dependency direction: features import `templates` (leaf with its own VMs), `course`, `pkg/api`. `templates` imports nothing from `internal/` except nothing at all — only its own VMs. `sqlite` imports `course`, `notes`, `eval` (for row types); never the reverse.

---

### Task 1: Scaffold — module, config, api helpers, static assets, healthz server

**Files:**
- Create: `go.mod` (via commands), `.gitignore`, `Makefile`
- Create: `internal/config/config.go`, `internal/config/config_test.go`
- Create: `pkg/api/api.go`
- Create: `static/static.go`, `static/styles.css`, `static/htmx.min.js` (downloaded)
- Create: `cmd/tour/main.go`, `cmd/tour/main_test.go`

**Interfaces:**
- Produces: `config.Config{Port, DataDir, ContentDir, LabRepoDir, OpenRouterKey, OpenRouterModel string}`, `config.FromEnv(getenv func(string) string) Config`; `api.Endpoint`, `api.ErrNotFound`, `api.ErrInvalid`, `api.RenderHTML(w http.ResponseWriter, r *http.Request, status int, c templ.Component)`, `api.RenderError(w http.ResponseWriter, r *http.Request, err error)`; `static.FS embed.FS`.

- [ ] **Step 1: Initialize module and dependencies**

```bash
cd /Users/martymulligan/go-code/src/github.com/itsnoproblem/mit-distributed-systems
go mod init github.com/itsnoproblem/mit-distributed-systems
go get github.com/a-h/templ gopkg.in/yaml.v3 github.com/yuin/goldmark modernc.org/sqlite
```

- [ ] **Step 2: Write `.gitignore` and `Makefile`**

`.gitignore`:
```
/bin/
/data/
```

`Makefile`:
```makefile
.PHONY: generate build test run
generate:
	go run github.com/a-h/templ/cmd/templ generate
build: generate
	go build -o bin/tour ./cmd/tour
test: generate
	go test ./...
run: build
	./bin/tour
```

- [ ] **Step 3: Write the failing config test** — `internal/config/config_test.go`:

```go
package config_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/config"
)

func TestFromEnvDefaults(t *testing.T) {
	cfg := config.FromEnv(func(string) string { return "" })
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if cfg.ContentDir != "./content" {
		t.Errorf("ContentDir = %q, want ./content", cfg.ContentDir)
	}
	if cfg.OpenRouterModel != "anthropic/claude-sonnet-4" {
		t.Errorf("OpenRouterModel = %q", cfg.OpenRouterModel)
	}
	if cfg.OpenRouterKey != "" || cfg.LabRepoDir != "" {
		t.Errorf("key/labrepo should default empty, got %q %q", cfg.OpenRouterKey, cfg.LabRepoDir)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	env := map[string]string{
		"PORT": "9999", "DATA_DIR": "/d", "CONTENT_DIR": "/c",
		"LAB_REPO_DIR": "/lab", "OPENROUTER_API_KEY": "sk", "OPENROUTER_MODEL": "x/y",
	}
	cfg := config.FromEnv(func(k string) string { return env[k] })
	if cfg.Port != "9999" || cfg.DataDir != "/d" || cfg.ContentDir != "/c" ||
		cfg.LabRepoDir != "/lab" || cfg.OpenRouterKey != "sk" || cfg.OpenRouterModel != "x/y" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/config/`
Expected: FAIL (package does not exist)

- [ ] **Step 5: Implement config** — `internal/config/config.go`:

```go
// Package config resolves runtime configuration from environment variables.
package config

type Config struct {
	Port            string
	DataDir         string
	ContentDir      string
	LabRepoDir      string
	OpenRouterKey   string
	OpenRouterModel string
}

// FromEnv reads configuration via getenv, applying defaults for unset values.
func FromEnv(getenv func(string) string) Config {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	return Config{
		Port:            get("PORT", "8080"),
		DataDir:         get("DATA_DIR", "./data"),
		ContentDir:      get("CONTENT_DIR", "./content"),
		LabRepoDir:      getenv("LAB_REPO_DIR"),
		OpenRouterKey:   getenv("OPENROUTER_API_KEY"),
		OpenRouterModel: get("OPENROUTER_MODEL", "anthropic/claude-sonnet-4"),
	}
}
```

Run: `go test ./internal/config/` — Expected: PASS

- [ ] **Step 6: Write `pkg/api/api.go`**

```go
// Package api holds the shared endpoint contract and HTTP rendering helpers.
package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/a-h/templ"
)

// Endpoint is the transport-agnostic unit of work: decoded request in, response model out.
type Endpoint func(ctx context.Context, request any) (any, error)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid request")
)

func RenderHTML(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = c.Render(r.Context(), w)
}

func RenderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 7: Vendor HTMX and write static package + stylesheet**

```bash
mkdir -p static && curl -fsSL -o static/htmx.min.js https://unpkg.com/htmx.org@1.9.12/dist/htmx.min.js
```

`static/static.go`:
```go
// Package static embeds the app's client-side assets.
package static

import "embed"

//go:embed *.css *.js
var FS embed.FS
```

`static/styles.css`:
```css
* { box-sizing: border-box; }
body { margin: 0; font: 16px/1.6 -apple-system, "Segoe UI", Helvetica, Arial, sans-serif; color: #1a1a1a; }
a { color: #007d9c; }
.topbar { display: flex; align-items: center; gap: 1rem; padding: .6rem 1rem;
  background: #007d9c; color: #fff; position: sticky; top: 0; }
.topbar a { color: #fff; text-decoration: none; }
.topbar h1 { font-size: 1.1rem; margin: 0; }
.topbar-title { font-weight: 600; flex: 1; }
.progress-label { font-size: .85rem; opacity: .9; }
.content { max-width: 46rem; margin: 0 auto; padding: 1.5rem 1rem 4rem; }
.content pre { background: #f5f5f5; padding: .8rem; overflow-x: auto; border-radius: 4px; }
.content code { background: #f5f5f5; padding: .1rem .3rem; border-radius: 3px; }
.module-list { list-style: none; padding: 0; }
.module-list li { display: flex; align-items: center; gap: 1rem; padding: .45rem 0;
  border-bottom: 1px solid #eee; }
.module-list a { flex: 1; text-decoration: none; }
.module-progress { font-size: .8rem; color: #666; display: flex; align-items: center; gap: .5rem; }
progress { accent-color: #007d9c; }
.step-nav { display: flex; justify-content: space-between; margin-top: 2.5rem; }
.step-actions { margin-top: 2rem; }
.btn { background: #007d9c; color: #fff; border: 0; padding: .5rem 1rem; border-radius: 4px;
  cursor: pointer; font-size: .95rem; }
.btn.done { background: #2e7d32; }
.link { background: none; border: 0; color: #007d9c; cursor: pointer; padding: 0; font-size: .85rem; }
.link.danger { color: #b00020; }
#notes-drawer { position: fixed; top: 0; right: 0; width: 22rem; max-width: 90vw; height: 100vh;
  background: #fafafa; border-left: 1px solid #ddd; padding: 3.2rem 1rem 1rem; overflow-y: auto;
  transform: translateX(100%); transition: transform .15s ease; }
body.drawer-open #notes-drawer { transform: none; }
.drawer-toggle { background: rgba(255,255,255,.15); color: #fff; border: 1px solid rgba(255,255,255,.4);
  border-radius: 4px; padding: .25rem .7rem; cursor: pointer; }
.note-list { list-style: none; padding: 0; }
.note { border: 1px solid #e5e5e5; background: #fff; border-radius: 4px; padding: .6rem; margin: .5rem 0; }
.note-body { white-space: pre-wrap; margin: 0 0 .3rem; }
.note-meta { font-size: .75rem; color: #888; margin-right: .8rem; }
textarea { width: 100%; font: inherit; padding: .5rem; border: 1px solid #ccc; border-radius: 4px; }
.question { border-left: 3px solid #007d9c; margin: .8rem 0; padding: .3rem .8rem; background: #f0f7f9; }
.locked { font-size: .85rem; color: #8a6d00; background: #fff8e1; padding: .5rem; border-radius: 4px; }
.saved { color: #2e7d32; }
.eval { border-top: 1px solid #eee; margin-top: 2rem; padding-top: 1rem; }
.eval-failed { background: #fdecea; padding: .8rem; border-radius: 4px; }
.test-output { max-height: 20rem; overflow-y: auto; font-size: .8rem; }
.report table { border-collapse: collapse; width: 100%; font-size: .9rem; }
.report th, .report td { border: 1px solid #ddd; padding: .4rem .6rem; text-align: left; vertical-align: top; }
.rubric-v { font-size: .75rem; color: #888; font-weight: normal; }
.empty { color: #777; font-style: italic; }
```

- [ ] **Step 8: Write failing healthz test** — `cmd/tour/main_test.go`:

```go
package main

import (
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	mux := newMux()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}
```

Run: `go test ./cmd/tour/` — Expected: FAIL (`newMux` undefined)

- [ ] **Step 9: Implement `cmd/tour/main.go`** (base mux grows in later tasks):

```go
// Command tour serves the guided MIT 6.824 course UI.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/itsnoproblem/mit-distributed-systems/internal/config"
	"github.com/itsnoproblem/mit-distributed-systems/static"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static.FS)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func main() {
	cfg := config.FromEnv(os.Getenv)
	mux := newMux()
	log.Printf("tour listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
```

Run: `go test ./...` — Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add -A && git commit -m "feat: scaffold module, config, api helpers, static assets"
```

---

### Task 2: Course domain types and navigation

**Files:**
- Create: `internal/course/course.go`, `internal/course/course_test.go`

**Interfaces:**
- Produces (consumed by every later task):

```go
type Kind string     // KindLecture "lecture" | KindLab "lab" | KindProject "project"
type StepType string // StepReading "reading" | StepQuestion "question" | StepExercise "exercise" | StepSubmit "submit"
type StepRef struct{ Module, Step string }
func (r StepRef) String() string // "module/step"
type Links struct{ Paper, Lab, Video string }
type EvalMeta struct{ Workdir string; Globs []string; TestCmd []string; Timeout time.Duration }
type Step struct{ Slug, Title string; Type StepType; BodyHTML string; Question string; Eval *EvalMeta }
type Module struct{ Slug, Title string; Kind Kind; Order int; Links Links; Steps []Step }
type Course struct{ Modules []Module } // Modules sorted by Order ascending
func (c *Course) Module(slug string) (*Module, bool)
func (c *Course) Step(ref StepRef) (*Module, *Step, bool)
func (c *Course) Prev(ref StepRef) (StepRef, bool) // crosses module boundaries
func (c *Course) Next(ref StepRef) (StepRef, bool)
func (c *Course) TotalSteps() int
```

- [ ] **Step 1: Write the failing tests** — `internal/course/course_test.go`:

```go
package course_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

func fixture() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Module One", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "s1", Title: "Step 1", Type: course.StepReading},
			{Slug: "s2", Title: "Step 2", Type: course.StepQuestion, Question: "Why?"},
		}},
		{Slug: "m2", Title: "Module Two", Kind: course.KindLab, Order: 2, Steps: []course.Step{
			{Slug: "s1", Title: "Lab step", Type: course.StepSubmit},
		}},
	}}
}

func TestStepLookup(t *testing.T) {
	c := fixture()
	mod, step, ok := c.Step(course.StepRef{Module: "m1", Step: "s2"})
	if !ok || mod.Slug != "m1" || step.Question != "Why?" {
		t.Fatalf("lookup failed: %v %v %v", mod, step, ok)
	}
	if _, _, ok := c.Step(course.StepRef{Module: "m1", Step: "nope"}); ok {
		t.Fatal("expected miss")
	}
}

func TestNextCrossesModules(t *testing.T) {
	c := fixture()
	next, ok := c.Next(course.StepRef{Module: "m1", Step: "s2"})
	if !ok || next != (course.StepRef{Module: "m2", Step: "s1"}) {
		t.Fatalf("next = %v %v", next, ok)
	}
	if _, ok := c.Next(course.StepRef{Module: "m2", Step: "s1"}); ok {
		t.Fatal("expected no next at course end")
	}
}

func TestPrevCrossesModules(t *testing.T) {
	c := fixture()
	prev, ok := c.Prev(course.StepRef{Module: "m2", Step: "s1"})
	if !ok || prev != (course.StepRef{Module: "m1", Step: "s2"}) {
		t.Fatalf("prev = %v %v", prev, ok)
	}
	if _, ok := c.Prev(course.StepRef{Module: "m1", Step: "s1"}); ok {
		t.Fatal("expected no prev at course start")
	}
}

func TestTotalSteps(t *testing.T) {
	if got := fixture().TotalSteps(); got != 3 {
		t.Fatalf("TotalSteps = %d, want 3", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/course/`
Expected: FAIL (package does not exist)

- [ ] **Step 3: Implement** — `internal/course/course.go`:

```go
// Package course defines the immutable domain model of the guided course.
// It has no I/O; internal/coursefs builds these values from content files.
package course

import "time"

type Kind string

const (
	KindLecture Kind = "lecture"
	KindLab     Kind = "lab"
	KindProject Kind = "project"
)

type StepType string

const (
	StepReading  StepType = "reading"
	StepQuestion StepType = "question"
	StepExercise StepType = "exercise"
	StepSubmit   StepType = "submit"
)

// StepRef is the canonical identity of a step across the whole app.
type StepRef struct{ Module, Step string }

func (r StepRef) String() string { return r.Module + "/" + r.Step }

type Links struct{ Paper, Lab, Video string }

// EvalMeta configures lab evaluation for a submit step, relative to the lab repo root.
type EvalMeta struct {
	Workdir string
	Globs   []string
	TestCmd []string
	Timeout time.Duration
}

type Step struct {
	Slug, Title string
	Type        StepType
	BodyHTML    string
	Question    string
	Eval        *EvalMeta
}

type Module struct {
	Slug, Title string
	Kind        Kind
	Order       int
	Links       Links
	Steps       []Step
}

// Course holds modules sorted by Order ascending.
type Course struct{ Modules []Module }

func (c *Course) Module(slug string) (*Module, bool) {
	for i := range c.Modules {
		if c.Modules[i].Slug == slug {
			return &c.Modules[i], true
		}
	}
	return nil, false
}

func (c *Course) Step(ref StepRef) (*Module, *Step, bool) {
	mod, ok := c.Module(ref.Module)
	if !ok {
		return nil, nil, false
	}
	for i := range mod.Steps {
		if mod.Steps[i].Slug == ref.Step {
			return mod, &mod.Steps[i], true
		}
	}
	return nil, nil, false
}

// flat returns every step in course order.
func (c *Course) flat() []StepRef {
	var refs []StepRef
	for _, m := range c.Modules {
		for _, s := range m.Steps {
			refs = append(refs, StepRef{Module: m.Slug, Step: s.Slug})
		}
	}
	return refs
}

func (c *Course) Next(ref StepRef) (StepRef, bool) {
	refs := c.flat()
	for i, r := range refs {
		if r == ref && i+1 < len(refs) {
			return refs[i+1], true
		}
	}
	return StepRef{}, false
}

func (c *Course) Prev(ref StepRef) (StepRef, bool) {
	refs := c.flat()
	for i, r := range refs {
		if r == ref && i > 0 {
			return refs[i-1], true
		}
	}
	return StepRef{}, false
}

func (c *Course) TotalSteps() int { return len(c.flat()) }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/course/` — Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/course && git commit -m "feat: course domain types with cross-module navigation"
```

---

### Task 3: coursefs — file-backed course loader

**Files:**
- Create: `internal/coursefs/frontmatter.go`, `internal/coursefs/coursefs.go`, `internal/coursefs/coursefs_test.go`
- Create: `internal/coursefs/testdata/valid/01-alpha/module.yaml`, `internal/coursefs/testdata/valid/01-alpha/steps/01-read.md`, `internal/coursefs/testdata/valid/01-alpha/steps/02-question.md`, `internal/coursefs/testdata/valid/02-beta-lab/module.yaml`, `internal/coursefs/testdata/valid/02-beta-lab/steps/01-submit.md`

**Interfaces:**
- Consumes: `course.*` types from Task 2.
- Produces: `coursefs.Load(dir string) (*course.Course, error)` (dir = `content/modules`); `coursefs.NewRepo(c *course.Course) *Repo` with `(*Repo).Course() *course.Course`.

**Loader rules (implement exactly):** module slug = directory name; steps sorted by filename; step slug = filename without `.md`; `module.yaml` requires non-empty `title`, valid `kind`, `order > 0`; every step requires non-empty `title` and valid `type`; `question` steps require non-empty `question`; `submit` steps require an `eval` block with non-empty `workdir`, `globs`, `test_cmd` and a parseable `timeout` duration; duplicate module or step slugs are errors; Markdown body renders to `Step.BodyHTML` via goldmark. Every error message must include the offending path.

- [ ] **Step 1: Write valid testdata**

`internal/coursefs/testdata/valid/01-alpha/module.yaml`:
```yaml
title: "Alpha Lecture"
kind: lecture
order: 1
links:
  paper: "https://example.com/paper.pdf"
```

`internal/coursefs/testdata/valid/01-alpha/steps/01-read.md`:
```markdown
---
title: Read the paper
type: reading
---

Read the **paper** carefully.
```

`internal/coursefs/testdata/valid/01-alpha/steps/02-question.md`:
```markdown
---
title: Reading question
type: question
question: |
  Why is the sky blue?
---

Answer in your own words.
```

`internal/coursefs/testdata/valid/02-beta-lab/module.yaml`:
```yaml
title: "Beta Lab"
kind: lab
order: 2
links:
  lab: "https://example.com/lab.html"
```

`internal/coursefs/testdata/valid/02-beta-lab/steps/01-submit.md`:
```markdown
---
title: Submit Beta
type: submit
eval:
  workdir: src/beta
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race"]
  timeout: 5m
---

Submit your work.
```

- [ ] **Step 2: Write the failing tests** — `internal/coursefs/coursefs_test.go`:

```go
package coursefs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
)

func TestLoadValid(t *testing.T) {
	c, err := coursefs.Load("testdata/valid")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Modules) != 2 {
		t.Fatalf("modules = %d, want 2", len(c.Modules))
	}
	alpha := c.Modules[0]
	if alpha.Slug != "01-alpha" || alpha.Kind != course.KindLecture || alpha.Links.Paper == "" {
		t.Fatalf("alpha parsed wrong: %+v", alpha)
	}
	if len(alpha.Steps) != 2 || alpha.Steps[0].Slug != "01-read" {
		t.Fatalf("alpha steps: %+v", alpha.Steps)
	}
	if !strings.Contains(alpha.Steps[0].BodyHTML, "<strong>paper</strong>") {
		t.Errorf("markdown not rendered: %q", alpha.Steps[0].BodyHTML)
	}
	if q := alpha.Steps[1].Question; !strings.Contains(q, "sky blue") {
		t.Errorf("question = %q", q)
	}
	sub := c.Modules[1].Steps[0]
	if sub.Eval == nil || sub.Eval.Workdir != "src/beta" || sub.Eval.Timeout != 5*time.Minute ||
		len(sub.Eval.TestCmd) != 3 {
		t.Fatalf("eval meta: %+v", sub.Eval)
	}
}

// writeModule builds a minimal module tree for error-case tests.
func writeModule(t *testing.T, root, slug, moduleYAML string, steps map[string]string) {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(filepath.Join(dir, "steps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.yaml"), []byte(moduleYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range steps {
		if err := os.WriteFile(filepath.Join(dir, "steps", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	goodStep := "---\ntitle: T\ntype: reading\n---\nbody"
	cases := []struct {
		name, moduleYAML string
		steps            map[string]string
		wantErr          string
	}{
		{"bad kind", "title: X\nkind: seminar\norder: 1", map[string]string{"01-a.md": goodStep}, "kind"},
		{"missing title", "kind: lecture\norder: 1", map[string]string{"01-a.md": goodStep}, "title"},
		{"bad step type", "title: X\nkind: lecture\norder: 1",
			map[string]string{"01-a.md": "---\ntitle: T\ntype: quiz\n---\nb"}, "type"},
		{"question without question", "title: X\nkind: lecture\norder: 1",
			map[string]string{"01-a.md": "---\ntitle: T\ntype: question\n---\nb"}, "question"},
		{"submit without eval", "title: X\nkind: lab\norder: 1",
			map[string]string{"01-a.md": "---\ntitle: T\ntype: submit\n---\nb"}, "eval"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeModule(t, root, "01-x", tc.moduleYAML, tc.steps)
			_, err := coursefs.Load(root)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRepo(t *testing.T) {
	c, err := coursefs.Load("testdata/valid")
	if err != nil {
		t.Fatal(err)
	}
	if coursefs.NewRepo(c).Course() != c {
		t.Fatal("repo should hand back the loaded course")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/coursefs/`
Expected: FAIL (package does not exist)

- [ ] **Step 4: Implement frontmatter splitter** — `internal/coursefs/frontmatter.go`:

```go
package coursefs

import (
	"bytes"
	"fmt"
)

var fmDelim = []byte("---\n")

// splitFrontmatter splits "---\n<yaml>\n---\n<body>" into its two parts.
func splitFrontmatter(b []byte) (fm, body []byte, err error) {
	if !bytes.HasPrefix(b, fmDelim) {
		return nil, nil, fmt.Errorf("missing frontmatter delimiter")
	}
	rest := b[len(fmDelim):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, nil, fmt.Errorf("unterminated frontmatter")
	}
	fm = rest[:end+1]
	body = rest[end+len("\n---"):]
	body = bytes.TrimPrefix(body, []byte("\n"))
	return fm, body, nil
}
```

- [ ] **Step 5: Implement loader** — `internal/coursefs/coursefs.go`:

```go
// Package coursefs loads the course from a content directory tree:
// <dir>/<module-slug>/{module.yaml,steps/*.md}. It is the file-backed
// implementation of every feature's CourseRepo interface.
package coursefs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type moduleYAML struct {
	Title string `yaml:"title"`
	Kind  string `yaml:"kind"`
	Order int    `yaml:"order"`
	Links struct {
		Paper string `yaml:"paper"`
		Lab   string `yaml:"lab"`
		Video string `yaml:"video"`
	} `yaml:"links"`
}

type stepYAML struct {
	Title    string `yaml:"title"`
	Type     string `yaml:"type"`
	Question string `yaml:"question"`
	Eval     *struct {
		Workdir string   `yaml:"workdir"`
		Globs   []string `yaml:"globs"`
		TestCmd []string `yaml:"test_cmd"`
		Timeout string   `yaml:"timeout"`
	} `yaml:"eval"`
}

var validKinds = map[string]course.Kind{
	"lecture": course.KindLecture, "lab": course.KindLab, "project": course.KindProject,
}

var validTypes = map[string]course.StepType{
	"reading": course.StepReading, "question": course.StepQuestion,
	"exercise": course.StepExercise, "submit": course.StepSubmit,
}

// Load parses every module under dir and returns the assembled course.
func Load(dir string) (*course.Course, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read content dir %s: %w", dir, err)
	}
	var modules []course.Module
	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if seen[e.Name()] {
			return nil, fmt.Errorf("duplicate module slug %q", e.Name())
		}
		seen[e.Name()] = true
		m, err := loadModule(filepath.Join(dir, e.Name()), e.Name())
		if err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no modules found under %s", dir)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Order < modules[j].Order })
	return &course.Course{Modules: modules}, nil
}

func loadModule(dir, slug string) (course.Module, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "module.yaml"))
	if err != nil {
		return course.Module{}, fmt.Errorf("%s: %w", dir, err)
	}
	var my moduleYAML
	if err := yaml.Unmarshal(raw, &my); err != nil {
		return course.Module{}, fmt.Errorf("%s/module.yaml: %w", dir, err)
	}
	kind, ok := validKinds[my.Kind]
	if !ok {
		return course.Module{}, fmt.Errorf("%s/module.yaml: invalid kind %q", dir, my.Kind)
	}
	if my.Title == "" {
		return course.Module{}, fmt.Errorf("%s/module.yaml: title is required", dir)
	}
	if my.Order <= 0 {
		return course.Module{}, fmt.Errorf("%s/module.yaml: order must be > 0", dir)
	}
	stepFiles, err := filepath.Glob(filepath.Join(dir, "steps", "*.md"))
	if err != nil || len(stepFiles) == 0 {
		return course.Module{}, fmt.Errorf("%s: no step files (%v)", dir, err)
	}
	sort.Strings(stepFiles)
	var steps []course.Step
	seenSteps := map[string]bool{}
	for _, f := range stepFiles {
		s, err := loadStep(f)
		if err != nil {
			return course.Module{}, err
		}
		if seenSteps[s.Slug] {
			return course.Module{}, fmt.Errorf("%s: duplicate step slug %q", dir, s.Slug)
		}
		seenSteps[s.Slug] = true
		steps = append(steps, s)
	}
	return course.Module{
		Slug: slug, Title: my.Title, Kind: kind, Order: my.Order,
		Links: course.Links{Paper: my.Links.Paper, Lab: my.Links.Lab, Video: my.Links.Video},
		Steps: steps,
	}, nil
}

func loadStep(path string) (course.Step, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return course.Step{}, fmt.Errorf("%s: %w", path, err)
	}
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return course.Step{}, fmt.Errorf("%s: %w", path, err)
	}
	var sy stepYAML
	if err := yaml.Unmarshal(fm, &sy); err != nil {
		return course.Step{}, fmt.Errorf("%s: %w", path, err)
	}
	typ, ok := validTypes[sy.Type]
	if !ok {
		return course.Step{}, fmt.Errorf("%s: invalid step type %q", path, sy.Type)
	}
	if sy.Title == "" {
		return course.Step{}, fmt.Errorf("%s: title is required", path)
	}
	if typ == course.StepQuestion && strings.TrimSpace(sy.Question) == "" {
		return course.Step{}, fmt.Errorf("%s: question steps require a question", path)
	}
	step := course.Step{
		Slug:     strings.TrimSuffix(filepath.Base(path), ".md"),
		Title:    sy.Title,
		Type:     typ,
		Question: strings.TrimSpace(sy.Question),
	}
	if typ == course.StepSubmit {
		if sy.Eval == nil {
			return course.Step{}, fmt.Errorf("%s: submit steps require an eval block", path)
		}
		if sy.Eval.Workdir == "" || len(sy.Eval.Globs) == 0 || len(sy.Eval.TestCmd) == 0 {
			return course.Step{}, fmt.Errorf("%s: eval requires workdir, globs, test_cmd", path)
		}
		timeout, err := time.ParseDuration(sy.Eval.Timeout)
		if err != nil {
			return course.Step{}, fmt.Errorf("%s: eval.timeout: %w", path, err)
		}
		step.Eval = &course.EvalMeta{
			Workdir: sy.Eval.Workdir, Globs: sy.Eval.Globs,
			TestCmd: sy.Eval.TestCmd, Timeout: timeout,
		}
	}
	var buf bytes.Buffer
	if err := goldmark.New().Convert(body, &buf); err != nil {
		return course.Step{}, fmt.Errorf("%s: render markdown: %w", path, err)
	}
	step.BodyHTML = buf.String()
	return step, nil
}

// Repo is the injectable wrapper satisfying each feature's CourseRepo interface.
type Repo struct{ c *course.Course }

func NewRepo(c *course.Course) *Repo     { return &Repo{c} }
func (r *Repo) Course() *course.Course   { return r.c }
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/coursefs/` — Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/coursefs && git commit -m "feat: file-backed course loader with validation"
```

---

### Task 4: Exemplar content — first lecture and Lab 1

**Files:**
- Create: `content/modules/01-introduction/module.yaml` + `steps/{01-read-the-paper,02-reading-question,03-wrap-up}.md`
- Create: `content/modules/lab-01-mapreduce/module.yaml` + `steps/{01-overview,02-build-mapreduce,03-submit}.md`
- Create: `internal/coursefs/realcontent_test.go`

**Interfaces:**
- Consumes: `coursefs.Load`. Produces: the real content tree later tasks' UI shows. All prose below is original guidance — none of it is copied from MIT materials.

- [ ] **Step 1: Write the failing real-content test** — `internal/coursefs/realcontent_test.go`:

```go
package coursefs_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
)

// TestRealContentParses guards the actual content tree: any authoring error
// that would crash boot fails this test first.
func TestRealContentParses(t *testing.T) {
	c, err := coursefs.Load("../../content/modules")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Modules) < 2 {
		t.Fatalf("expected at least 2 modules, got %d", len(c.Modules))
	}
	if _, ok := c.Module("01-introduction"); !ok {
		t.Error("missing module 01-introduction")
	}
	if _, ok := c.Module("lab-01-mapreduce"); !ok {
		t.Error("missing module lab-01-mapreduce")
	}
}
```

Run: `go test ./internal/coursefs/ -run RealContent` — Expected: FAIL (no content dir)

- [ ] **Step 2: Author the lecture module**

`content/modules/01-introduction/module.yaml`:
```yaml
title: "Lecture 1: Introduction & MapReduce"
kind: lecture
order: 10
links:
  paper: "https://pdos.csail.mit.edu/6.824/papers/mapreduce.pdf"
  video: "https://pdos.csail.mit.edu/6.824/schedule.html"
```

`content/modules/01-introduction/steps/01-read-the-paper.md`:
```markdown
---
title: Read the MapReduce paper
type: reading
---

Read *MapReduce: Simplified Data Processing on Large Clusters* (Dean &
Ghemawat, 2004) — linked above as the paper for this module.

Focus while you read:

- **The programming model (§2)** — why do `map` and `reduce` as pure
  functions make distribution possible at all?
- **Execution overview (§3.1)** — trace one job end to end: input splits,
  the master, intermediate files, the shuffle to reducers.
- **Fault tolerance (§3.3)** — what exactly happens when a worker dies
  mid-task, and why is simply re-running it safe?
- **Stragglers (§3.6)** — backup tasks are a blunt instrument; why do they
  work so well anyway?

Skim the refinements (§4); read the sort benchmark story (§5) for a feel of
the scale this was built for.
```

`content/modules/01-introduction/steps/02-reading-question.md`:
```markdown
---
title: Reading question
type: question
question: |
  A MapReduce worker crashes after completing two map tasks and while running
  a third. Explain what the master must do about each of the three tasks, and
  why completed map tasks are treated differently from completed reduce tasks.
---

Answer from the paper's fault-tolerance model. Aim for the *why*, not just
the mechanics.
```

`content/modules/01-introduction/steps/03-wrap-up.md`:
```markdown
---
title: Wrap-up
type: reading
---

Before moving on, check you can answer these from memory:

- Why does MapReduce *restrict* the programming model instead of exposing
  general distributed programming?
- Where is the single point of failure, and why was that acceptable?
- What limits job completion time, and how do backup tasks attack it?

Anything still fuzzy? Open the **Notes** drawer and write it down — you will
want the list when Lab 1 makes these questions concrete.
```

- [ ] **Step 3: Author the Lab 1 module**

`content/modules/lab-01-mapreduce/module.yaml`:
```yaml
title: "Lab 1: MapReduce"
kind: lab
order: 15
links:
  lab: "https://pdos.csail.mit.edu/6.824/labs/lab-mr.html"
```

`content/modules/lab-01-mapreduce/steps/01-overview.md`:
```markdown
---
title: Lab 1 overview
type: reading
---

You will build a working MapReduce system: a coordinator process that hands
out tasks, and worker processes that execute map and reduce functions, talk
to the coordinator over RPC, and survive worker crashes.

Setup:

1. Clone the course lab repo (instructions on the lab page linked above).
2. Point this app's `LAB_REPO_DIR` at your clone so the submit step can
   snapshot your code and run the lab's tests.

Everything you write lives in `src/mr/`; the test harness is
`src/main/test-mr.sh`.
```

`content/modules/lab-01-mapreduce/steps/02-build-mapreduce.md`:
```markdown
---
title: Build it
type: exercise
---

A working order of attack:

1. **Task handout** — coordinator RPC that gives an idle worker a map task;
   worker runs the map function and writes intermediate files partitioned by
   `ihash(key) % nReduce`.
2. **Reduce path** — once all map tasks finish, hand out reduce tasks;
   reducers read their partition from every map output, sort, and write
   `mr-out-*`.
3. **Completion** — coordinator learns tasks finished; `Done()` returns true
   only when all reduce output exists.
4. **Crash tolerance** — re-issue tasks not completed within a timeout;
  make output writes atomic (write temp file, then rename) so duplicated
  tasks are harmless.

Run `bash test-mr.sh` in `src/main` until every test passes — the crash test
is the one that finds real design flaws.
```

`content/modules/lab-01-mapreduce/steps/03-submit.md`:
```markdown
---
title: Submit Lab 1
type: submit
eval:
  workdir: src/main
  globs: ["../mr/*.go"]
  test_cmd: ["bash", "test-mr.sh"]
  timeout: 20m
---

When `test-mr.sh` passes locally, snapshot and evaluate your implementation
here. The evaluation runs the lab's own test harness and then reviews your
`src/mr` code against the rubric.
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/coursefs/ -run RealContent` — Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add content internal/coursefs/realcontent_test.go
git commit -m "feat: exemplar content for lecture 1 and lab 1"
```

---

### Task 5: SQLite — open, migrate, progress repository

**Files:**
- Create: `internal/sqlite/db.go`, `internal/sqlite/migrations/001_init.sql`, `internal/sqlite/db_test.go`, `internal/sqlite/progress.go`, `internal/sqlite/progress_test.go`

**Interfaces:**
- Consumes: `course.StepRef`.
- Produces: `sqlite.Open(path string) (*sql.DB, error)`; `sqlite.Migrate(db *sql.DB) error` (idempotent); `sqlite.NewProgressRepo(db *sql.DB) *ProgressRepo` with methods `SetComplete(ctx context.Context, ref course.StepRef, done bool) error` and `Completed(ctx context.Context) (map[course.StepRef]time.Time, error)`. Note: migrations are embedded, so they live under `internal/sqlite/migrations/` (embed requires the package subtree) rather than a top-level `migrations/` dir — a deliberate refinement of the spec layout.

- [ ] **Step 1: Write the migration** — `internal/sqlite/migrations/001_init.sql`:

```sql
CREATE TABLE progress (
    module_slug  TEXT NOT NULL,
    step_slug    TEXT NOT NULL,
    completed_at TEXT NOT NULL,
    PRIMARY KEY (module_slug, step_slug)
);

CREATE TABLE notes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    body_md     TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE submissions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('lab', 'question')),
    content     TEXT NOT NULL,
    test_output TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('pending', 'running', 'complete', 'failed')),
    created_at  TEXT NOT NULL
);

CREATE TABLE evaluations (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    submission_id  INTEGER NOT NULL REFERENCES submissions (id),
    model          TEXT NOT NULL,
    rubric_version TEXT NOT NULL,
    verdict_json   TEXT NOT NULL,
    created_at     TEXT NOT NULL
);
```

- [ ] **Step 2: Write the failing db tests** — `internal/sqlite/db_test.go`:

```go
package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func TestOpenAndMigrate(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	// idempotent
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	for _, table := range []string{"progress", "notes", "submissions", "evaluations"} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}
```

Run: `go test ./internal/sqlite/` — Expected: FAIL (package does not exist)

- [ ] **Step 3: Implement** — `internal/sqlite/db.go`:

```go
// Package sqlite implements the app's repositories on a local SQLite file.
package sqlite

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite",
		path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer; a single conn avoids SQLITE_BUSY entirely.
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

func Migrate(db *sql.DB) error {
	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)"); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := db.QueryRow(
			"SELECT count(*) FROM schema_migrations WHERE version = ?", name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(raw)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := db.Exec(
			"INSERT INTO schema_migrations (version) VALUES (?)", name); err != nil {
			return err
		}
	}
	return nil
}
```

Run: `go test ./internal/sqlite/` — Expected: PASS

- [ ] **Step 4: Write the failing progress repo test** — `internal/sqlite/progress_test.go`:

```go
package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func testDB(t *testing.T) *sqlite.ProgressRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewProgressRepo(db)
}

func TestProgressRoundTrip(t *testing.T) {
	repo := testDB(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "s1"}

	if err := repo.SetComplete(ctx, ref, true); err != nil {
		t.Fatal(err)
	}
	// marking twice is fine
	if err := repo.SetComplete(ctx, ref, true); err != nil {
		t.Fatal(err)
	}
	done, err := repo.Completed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := done[ref]; !ok || len(done) != 1 {
		t.Fatalf("completed = %v", done)
	}
	if err := repo.SetComplete(ctx, ref, false); err != nil {
		t.Fatal(err)
	}
	done, _ = repo.Completed(ctx)
	if len(done) != 0 {
		t.Fatalf("expected empty after unmark, got %v", done)
	}
}
```

Run: `go test ./internal/sqlite/ -run Progress` — Expected: FAIL (`NewProgressRepo` undefined)

- [ ] **Step 5: Implement** — `internal/sqlite/progress.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type ProgressRepo struct{ db *sql.DB }

func NewProgressRepo(db *sql.DB) *ProgressRepo { return &ProgressRepo{db} }

func (p *ProgressRepo) SetComplete(ctx context.Context, ref course.StepRef, done bool) error {
	if !done {
		_, err := p.db.ExecContext(ctx,
			"DELETE FROM progress WHERE module_slug = ? AND step_slug = ?", ref.Module, ref.Step)
		return err
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO progress (module_slug, step_slug, completed_at) VALUES (?, ?, ?)`,
		ref.Module, ref.Step, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (p *ProgressRepo) Completed(ctx context.Context) (map[course.StepRef]time.Time, error) {
	rows, err := p.db.QueryContext(ctx,
		"SELECT module_slug, step_slug, completed_at FROM progress")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[course.StepRef]time.Time{}
	for rows.Next() {
		var ref course.StepRef
		var at string
		if err := rows.Scan(&ref.Module, &ref.Step, &at); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return nil, err
		}
		out[ref] = ts
	}
	return out, rows.Err()
}
```

Run: `go test ./internal/sqlite/` — Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/sqlite && git commit -m "feat: sqlite open/migrate and progress repository"
```

---

### Task 6: Tour feature — course map, step pages, progress toggle

**Files:**
- Create: `templates/viewmodels.go`, `templates/document.templ`, `templates/coursemap.templ`, `templates/step.templ`
- Create: `internal/tour/service.go`, `internal/tour/service_test.go`, `internal/tour/endpoint.go`, `internal/tour/transport.go`, `e2e/tour_test.go`
- Create: `e2e/harness_test.go`, `e2e/tour_test.go`, `e2e/testdata/content/modules/…` (below)
- Modify: `cmd/tour/main.go` (wire course + db + tour routes)

**Interfaces:**
- Consumes: `course.*`, `coursefs.Load/NewRepo`, `sqlite.Open/Migrate/NewProgressRepo`, `api.*`.
- Produces:
  - `tour.NewService(c CourseRepo, p ProgressRepo) *Service` where `CourseRepo interface{ Course() *course.Course }` and `ProgressRepo interface{ SetComplete(ctx, course.StepRef, bool) error; Completed(ctx) (map[course.StepRef]time.Time, error) }` (both defined in `tour`).
  - Service methods: `CourseMap(ctx) (CourseMapView, error)`, `StepPage(ctx, course.StepRef) (StepView, error)`, `SetComplete(ctx, course.StepRef, bool) error`.
  - `tour.RegisterRoutes(mux *http.ServeMux, svc TourService)`.
  - Templates: `Document(title string, body templ.Component)`, `CourseMap(CourseMapVM)`, `StepPage(StepVM)`, `CompleteToggle(moduleSlug, stepSlug string, completed bool)`, `StepURL(module, step string) string`.
  - e2e harness (test-only, package `e2e`): `newApp(t *testing.T, o options) *app` with `app{TS *httptest.Server; DB *sql.DB}` and `options{ContentDir string}` — later tasks extend `options`.

- [ ] **Step 1: Write template view models** — `templates/viewmodels.go`:

```go
// Package templates renders every page and partial. It depends on nothing in
// internal/ — feature transports map their view types into these VMs.
package templates

type CourseMapVM struct {
	Groups      []KindGroupVM
	Done, Total int
}

type KindGroupVM struct {
	Label   string
	Modules []ModuleCardVM
}

type ModuleCardVM struct {
	Slug, Title  string
	Done, Total  int
	FirstStepURL string
}

type StepVM struct {
	ModuleSlug, StepSlug       string
	ModuleTitle, Title         string
	Type                       string
	BodyHTML                   string
	Completed                  bool
	PrevURL, NextURL           string
	Index, Total               int
	PaperURL, LabURL, VideoURL string
}

func StepURL(module, step string) string { return "/modules/" + module + "/steps/" + step }
```

- [ ] **Step 2: Write the templ files**

`templates/document.templ`:
```templ
package templates

templ Document(title string, body templ.Component) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>{ title }</title>
			<link rel="stylesheet" href="/static/styles.css"/>
			<script src="/static/htmx.min.js"></script>
		</head>
		<body>
			@body
		</body>
	</html>
}
```

`templates/coursemap.templ`:
```templ
package templates

import "fmt"

templ CourseMap(v CourseMapVM) {
	<header class="topbar">
		<h1>MIT 6.824 — Guided Tour</h1>
		<span class="topbar-title"></span>
		<span class="progress-label">{ fmt.Sprintf("%d/%d steps", v.Done, v.Total) }</span>
	</header>
	<main class="content">
		<p><a href="/notes">All notes →</a></p>
		for _, g := range v.Groups {
			<section>
				<h2>{ g.Label }</h2>
				<ul class="module-list">
					for _, m := range g.Modules {
						<li>
							<a href={ templ.SafeURL(m.FirstStepURL) }>{ m.Title }</a>
							<span class="module-progress">
								<progress max={ fmt.Sprint(m.Total) } value={ fmt.Sprint(m.Done) }></progress>
								{ fmt.Sprintf("%d/%d", m.Done, m.Total) }
							</span>
						</li>
					}
				</ul>
			</section>
		}
	</main>
}
```

`templates/step.templ` (the notes-drawer and eval containers lazy-load via
HTMX from endpoints that arrive in Tasks 7–8; until then the load requests
404 and htmx leaves the empty container alone — harmless):
```templ
package templates

import "fmt"

templ StepPage(v StepVM) {
	<header class="topbar">
		<a href="/">⌂ Map</a>
		<span class="topbar-title">{ v.ModuleTitle }</span>
		<span class="progress-label">{ fmt.Sprintf("step %d of %d", v.Index, v.Total) }</span>
		<button class="drawer-toggle" onclick="document.body.classList.toggle('drawer-open')">Notes</button>
	</header>
	<main class="content step">
		<h2>{ v.Title }</h2>
		if v.PaperURL != "" || v.LabURL != "" || v.VideoURL != "" {
			<p class="module-links">
				if v.PaperURL != "" {
					<a href={ templ.SafeURL(v.PaperURL) } target="_blank">Paper</a>
				}
				if v.LabURL != "" {
					<a href={ templ.SafeURL(v.LabURL) } target="_blank">Lab page</a>
				}
				if v.VideoURL != "" {
					<a href={ templ.SafeURL(v.VideoURL) } target="_blank">Lecture</a>
				}
			</p>
		}
		@templ.Raw(v.BodyHTML)
		if v.Type == "question" || v.Type == "submit" {
			<div id="eval-section"
				hx-get={ "/eval/section?module=" + v.ModuleSlug + "&step=" + v.StepSlug }
				hx-trigger="load" hx-swap="innerHTML"></div>
		}
		<div class="step-actions">
			@CompleteToggle(v.ModuleSlug, v.StepSlug, v.Completed)
		</div>
		<nav class="step-nav">
			<span>
				if v.PrevURL != "" {
					<a href={ templ.SafeURL(v.PrevURL) }>← Previous</a>
				}
			</span>
			<span>
				if v.NextURL != "" {
					<a href={ templ.SafeURL(v.NextURL) }>Next →</a>
				}
			</span>
		</nav>
	</main>
	<aside id="notes-drawer"
		hx-get={ "/notes/drawer?module=" + v.ModuleSlug + "&step=" + v.StepSlug }
		hx-trigger="load" hx-swap="innerHTML"></aside>
}

templ CompleteToggle(moduleSlug, stepSlug string, completed bool) {
	<form hx-post={ "/modules/" + moduleSlug + "/steps/" + stepSlug + "/complete" } hx-swap="outerHTML">
		if completed {
			<input type="hidden" name="done" value="false"/>
			<button class="btn done">✓ Completed — click to undo</button>
		} else {
			<input type="hidden" name="done" value="true"/>
			<button class="btn">Mark step complete</button>
		}
	</form>
}
```

Run: `make generate` — Expected: creates `templates/*_templ.go` without errors.

- [ ] **Step 3: Write the failing service test** — `internal/tour/service_test.go`:

```go
package tour_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/internal/tour"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

func fixtureCourse() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Lecture M1", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "s1", Title: "One", Type: course.StepReading},
			{Slug: "s2", Title: "Two", Type: course.StepReading},
		}},
		{Slug: "m2", Title: "Lab M2", Kind: course.KindLab, Order: 2, Steps: []course.Step{
			{Slug: "s1", Title: "Three", Type: course.StepReading},
		}},
	}}
}

func newSvc(t *testing.T) *tour.Service {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return tour.NewService(coursefs.NewRepo(fixtureCourse()), sqlite.NewProgressRepo(db))
}

func TestCourseMapAndProgress(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.SetComplete(ctx, course.StepRef{Module: "m1", Step: "s1"}, true); err != nil {
		t.Fatal(err)
	}
	v, err := svc.CourseMap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Total != 3 || v.Done != 1 {
		t.Fatalf("overall = %d/%d, want 1/3", v.Done, v.Total)
	}
	if len(v.Groups) != 2 || v.Groups[0].Kind != course.KindLecture {
		t.Fatalf("groups: %+v", v.Groups)
	}
	if mp := v.Groups[0].Modules[0]; mp.Done != 1 || mp.Total != 2 {
		t.Fatalf("m1 progress = %d/%d", mp.Done, mp.Total)
	}
}

func TestStepPage(t *testing.T) {
	svc := newSvc(t)
	v, err := svc.StepPage(context.Background(), course.StepRef{Module: "m1", Step: "s2"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Index != 2 || v.Total != 2 || v.Completed {
		t.Fatalf("view: %+v", v)
	}
	if v.Prev == nil || v.Prev.Step != "s1" || v.Next == nil || v.Next.Module != "m2" {
		t.Fatalf("nav: prev=%v next=%v", v.Prev, v.Next)
	}
}

func TestUnknownStepIsNotFound(t *testing.T) {
	svc := newSvc(t)
	if _, err := svc.StepPage(context.Background(), course.StepRef{Module: "x", Step: "y"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := svc.SetComplete(context.Background(), course.StepRef{Module: "x", Step: "y"}, true); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

Run: `go test ./internal/tour/` — Expected: FAIL (package does not exist)

- [ ] **Step 4: Implement the service** — `internal/tour/service.go`:

```go
// Package tour is the course-browsing feature: course map, step pages,
// and per-step completion.
package tour

import (
	"context"
	"fmt"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type CourseRepo interface{ Course() *course.Course }

type ProgressRepo interface {
	SetComplete(ctx context.Context, ref course.StepRef, done bool) error
	Completed(ctx context.Context) (map[course.StepRef]time.Time, error)
}

type ModuleProgress struct {
	Module      course.Module
	Done, Total int
}

type KindGroup struct {
	Kind    course.Kind
	Modules []ModuleProgress
}

type CourseMapView struct {
	Groups      []KindGroup
	Done, Total int
}

type StepView struct {
	Module       course.Module
	Step         course.Step
	Ref          course.StepRef
	Completed    bool
	Prev, Next   *course.StepRef
	Index, Total int
}

type Service struct {
	course   CourseRepo
	progress ProgressRepo
}

func NewService(c CourseRepo, p ProgressRepo) *Service { return &Service{c, p} }

func (s *Service) CourseMap(ctx context.Context) (CourseMapView, error) {
	done, err := s.progress.Completed(ctx)
	if err != nil {
		return CourseMapView{}, err
	}
	crs := s.course.Course()
	view := CourseMapView{Total: crs.TotalSteps()}
	for _, kind := range []course.Kind{course.KindLecture, course.KindLab, course.KindProject} {
		group := KindGroup{Kind: kind}
		for _, m := range crs.Modules {
			if m.Kind != kind {
				continue
			}
			mp := ModuleProgress{Module: m, Total: len(m.Steps)}
			for _, st := range m.Steps {
				if _, ok := done[course.StepRef{Module: m.Slug, Step: st.Slug}]; ok {
					mp.Done++
				}
			}
			view.Done += mp.Done
			group.Modules = append(group.Modules, mp)
		}
		if len(group.Modules) > 0 {
			view.Groups = append(view.Groups, group)
		}
	}
	return view, nil
}

func (s *Service) StepPage(ctx context.Context, ref course.StepRef) (StepView, error) {
	crs := s.course.Course()
	mod, step, ok := crs.Step(ref)
	if !ok {
		return StepView{}, fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	done, err := s.progress.Completed(ctx)
	if err != nil {
		return StepView{}, err
	}
	view := StepView{Module: *mod, Step: *step, Ref: ref, Total: len(mod.Steps)}
	_, view.Completed = done[ref]
	for i, st := range mod.Steps {
		if st.Slug == ref.Step {
			view.Index = i + 1
		}
	}
	if prev, ok := crs.Prev(ref); ok {
		view.Prev = &prev
	}
	if next, ok := crs.Next(ref); ok {
		view.Next = &next
	}
	return view, nil
}

func (s *Service) SetComplete(ctx context.Context, ref course.StepRef, done bool) error {
	if _, _, ok := s.course.Course().Step(ref); !ok {
		return fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	return s.progress.SetComplete(ctx, ref, done)
}
```

Run: `go test ./internal/tour/` — Expected: PASS

- [ ] **Step 5: Write the endpoints** — `internal/tour/endpoint.go`:

```go
package tour

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// TourService is the contract this feature's endpoints require; *Service satisfies it.
type TourService interface {
	CourseMap(ctx context.Context) (CourseMapView, error)
	StepPage(ctx context.Context, ref course.StepRef) (StepView, error)
	SetComplete(ctx context.Context, ref course.StepRef, done bool) error
}

type StepPageRequest struct{ Module, Step string }

func (r StepPageRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type SetCompleteRequest struct {
	Module, Step string
	Done         bool
}

func (r SetCompleteRequest) Validate() error {
	return StepPageRequest{Module: r.Module, Step: r.Step}.Validate()
}

type SetCompleteResponse struct {
	Module, Step string
	Done         bool
}

func makeCourseMapEndpoint(svc TourService) api.Endpoint {
	return func(ctx context.Context, _ any) (any, error) {
		return svc.CourseMap(ctx)
	}
}

func makeStepPageEndpoint(svc TourService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(StepPageRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		return svc.StepPage(ctx, course.StepRef{Module: req.Module, Step: req.Step})
	}
}

func makeSetCompleteEndpoint(svc TourService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SetCompleteRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.SetComplete(ctx, ref, req.Done); err != nil {
			return nil, err
		}
		return SetCompleteResponse{Module: req.Module, Step: req.Step, Done: req.Done}, nil
	}
}
```

- [ ] **Step 6: Write the transport** — `internal/tour/transport.go`:

```go
package tour

import (
	"net/http"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func RegisterRoutes(mux *http.ServeMux, svc TourService) {
	courseMap := makeCourseMapEndpoint(svc)
	stepPage := makeStepPageEndpoint(svc)
	setComplete := makeSetCompleteEndpoint(svc)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		resp, err := courseMap(r.Context(), nil)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		vm := courseMapVM(resp.(CourseMapView))
		api.RenderHTML(w, r, http.StatusOK,
			templates.Document("MIT 6.824 — Guided Tour", templates.CourseMap(vm)))
	})

	mux.HandleFunc("GET /modules/{module}/steps/{step}", func(w http.ResponseWriter, r *http.Request) {
		req := StepPageRequest{Module: r.PathValue("module"), Step: r.PathValue("step")}
		resp, err := stepPage(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		vm := stepVM(resp.(StepView))
		api.RenderHTML(w, r, http.StatusOK, templates.Document(vm.Title, templates.StepPage(vm)))
	})

	mux.HandleFunc("POST /modules/{module}/steps/{step}/complete", func(w http.ResponseWriter, r *http.Request) {
		req := SetCompleteRequest{
			Module: r.PathValue("module"), Step: r.PathValue("step"),
			Done: r.FormValue("done") == "true",
		}
		resp, err := setComplete(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(SetCompleteResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.CompleteToggle(res.Module, res.Step, res.Done))
	})
}

func courseMapVM(v CourseMapView) templates.CourseMapVM {
	out := templates.CourseMapVM{Done: v.Done, Total: v.Total}
	for _, g := range v.Groups {
		gv := templates.KindGroupVM{Label: kindLabel(g.Kind)}
		for _, mp := range g.Modules {
			first := ""
			if len(mp.Module.Steps) > 0 {
				first = templates.StepURL(mp.Module.Slug, mp.Module.Steps[0].Slug)
			}
			gv.Modules = append(gv.Modules, templates.ModuleCardVM{
				Slug: mp.Module.Slug, Title: mp.Module.Title,
				Done: mp.Done, Total: mp.Total, FirstStepURL: first,
			})
		}
		out.Groups = append(out.Groups, gv)
	}
	return out
}

func kindLabel(k course.Kind) string {
	switch k {
	case course.KindLecture:
		return "Lectures"
	case course.KindLab:
		return "Labs"
	default:
		return "Final project"
	}
}

func stepVM(v StepView) templates.StepVM {
	vm := templates.StepVM{
		ModuleSlug: v.Ref.Module, StepSlug: v.Ref.Step,
		ModuleTitle: v.Module.Title, Title: v.Step.Title,
		Type: string(v.Step.Type), BodyHTML: v.Step.BodyHTML,
		Completed: v.Completed, Index: v.Index, Total: v.Total,
		PaperURL: v.Module.Links.Paper, LabURL: v.Module.Links.Lab, VideoURL: v.Module.Links.Video,
	}
	if v.Prev != nil {
		vm.PrevURL = templates.StepURL(v.Prev.Module, v.Prev.Step)
	}
	if v.Next != nil {
		vm.NextURL = templates.StepURL(v.Next.Module, v.Next.Step)
	}
	return vm
}
```

- [ ] **Step 7: Write the e2e harness and its testdata**

`e2e/testdata/content/modules/01-test-lecture/module.yaml`:
```yaml
title: "Test Lecture"
kind: lecture
order: 1
```

`e2e/testdata/content/modules/01-test-lecture/steps/01-read.md`:
```markdown
---
title: Read something
type: reading
---

Test reading body.
```

`e2e/testdata/content/modules/01-test-lecture/steps/02-question.md`:
```markdown
---
title: Test question
type: question
question: |
  What is a distributed system?
---

Answer below.
```

`e2e/testdata/content/modules/02-test-lab/module.yaml`:
```yaml
title: "Test Lab"
kind: lab
order: 2
```

`e2e/testdata/content/modules/02-test-lab/steps/01-submit.md`:
```markdown
---
title: Submit test lab
type: submit
eval:
  workdir: src/x
  globs: ["*.go"]
  test_cmd: ["go", "test"]
  timeout: 1m
---

Submit it.
```

`e2e/harness_test.go`:
```go
// Package e2e wires a full application over a temp database and testdata
// content, and drives it over HTTP the way a browser would. Dedicated
// integration-test package; contains only _test.go files.
package e2e

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/internal/tour"
)

type options struct {
	ContentDir string // defaults to e2e/testdata/content
}

type app struct {
	TS *httptest.Server
	DB *sql.DB
}

func newApp(t *testing.T, o options) *app {
	t.Helper()
	if o.ContentDir == "" {
		o.ContentDir = "testdata/content" // go test runs with the package dir as cwd
	}
	crs, err := coursefs.Load(filepath.Join(o.ContentDir, "modules"))
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "tour.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	courseRepo := coursefs.NewRepo(crs)
	mux := http.NewServeMux()
	tour.RegisterRoutes(mux, tour.NewService(courseRepo, sqlite.NewProgressRepo(db)))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &app{TS: ts, DB: db}
}
```

- [ ] **Step 8: Write the failing integration test** — `e2e/tour_test.go`:

```go
package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestBrowseAndComplete(t *testing.T) {
	app := newApp(t, options{})

	code, body := get(t, app.TS.URL+"/")
	if code != 200 || !strings.Contains(body, "Test Lecture") || !strings.Contains(body, "Test Lab") {
		t.Fatalf("course map: %d %q", code, body)
	}

	code, body = get(t, app.TS.URL+"/modules/01-test-lecture/steps/01-read")
	if code != 200 || !strings.Contains(body, "Test reading body") {
		t.Fatalf("step page: %d", code)
	}
	if !strings.Contains(body, "/notes/drawer?module=01-test-lecture") {
		t.Error("notes drawer container missing")
	}
	if !strings.Contains(body, "Next →") {
		t.Error("next nav missing")
	}

	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/01-read/complete",
		url.Values{"done": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Completed") {
		t.Fatalf("toggle response: %q", string(b))
	}

	_, body = get(t, app.TS.URL+"/")
	if !strings.Contains(body, "1/2") {
		t.Fatalf("expected 1/2 module progress on map, got: %q", body)
	}

	code, _ = get(t, app.TS.URL+"/modules/nope/steps/nah")
	if code != 404 {
		t.Fatalf("unknown step = %d, want 404", code)
	}
}
```

Run: `make test` — Expected: PASS (service already implemented; if templ compile errors surface, fix the `.templ` files and regenerate)

- [ ] **Step 9: Wire into main** — replace `main()` in `cmd/tour/main.go` (keep `newMux` as-is):

```go
func main() {
	cfg := config.FromEnv(os.Getenv)
	crs, err := coursefs.Load(filepath.Join(cfg.ContentDir, "modules"))
	if err != nil {
		log.Fatalf("load content: %v", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := sqlite.Open(filepath.Join(cfg.DataDir, "tour.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	courseRepo := coursefs.NewRepo(crs)

	mux := newMux()
	tour.RegisterRoutes(mux, tour.NewService(courseRepo, sqlite.NewProgressRepo(db)))

	log.Printf("tour listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
```

Add imports: `"path/filepath"`, `internal/coursefs`, `internal/sqlite`, `internal/tour`.

Run: `make test` then `make run` and open http://localhost:8080 — course map shows Lecture 1 and Lab 1; stepping through works; Mark complete persists across restart.

- [ ] **Step 10: Commit**

```bash
git add -A && git commit -m "feat: tour feature - course map, step pages, progress"
```

---

### Task 7: Notes — drawer, index, CRUD

**Files:**
- Create: `internal/notes/service.go`, `internal/notes/service_test.go`, `internal/notes/endpoint.go`, `internal/notes/transport.go`, `e2e/notes_test.go`
- Create: `internal/sqlite/notes.go`, `internal/sqlite/notes_test.go`
- Create: `templates/notes.templ`
- Modify: `templates/viewmodels.go` (add note VMs), `e2e/harness_test.go` (wire notes), `cmd/tour/main.go` (wire notes)

**Interfaces:**
- Consumes: `course.StepRef`, `coursefs.Repo`, `sqlite` db.
- Produces:
  - `notes.Note{ID int64; Ref course.StepRef; Body string; CreatedAt, UpdatedAt time.Time}`; `notes.ModuleNotes{ModuleSlug, ModuleTitle string; Notes []Note}`.
  - `notes.Repo` interface (defined in `notes`, implemented by `sqlite.NotesRepo`): `Insert(ctx, Note) (int64, error)`, `Update(ctx, id int64, body string, updatedAt time.Time) error`, `Delete(ctx, id int64) error`, `Get(ctx, id int64) (Note, error)`, `ForStep(ctx, course.StepRef) ([]Note, error)`, `All(ctx) ([]Note, error)`.
  - `notes.NewService(c CourseRepo, r Repo) *Service` with `Add(ctx, ref, body) (Note, error)`, `Edit(ctx, id, body) (Note, error)`, `Remove(ctx, id) error`, `ForStep(ctx, ref) ([]Note, error)`, `GroupedByModule(ctx) ([]ModuleNotes, error)`.
  - `notes.RegisterRoutes(mux, svc NotesService)`; templates `NotesDrawer(NotesDrawerVM)`, `NotesIndex(NotesIndexVM)`, `NoteItem(NoteVM)`, `NoteEditForm(NoteVM)`.

- [ ] **Step 1: Write the failing sqlite notes repo test** — `internal/sqlite/notes_test.go`:

```go
package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func notesRepo(t *testing.T) *sqlite.NotesRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewNotesRepo(db)
}

func TestNotesCRUD(t *testing.T) {
	repo := notesRepo(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "s1"}
	now := time.Now().UTC().Truncate(time.Second)

	id, err := repo.Insert(ctx, notes.Note{Ref: ref, Body: "first", CreatedAt: now, UpdatedAt: now})
	if err != nil || id == 0 {
		t.Fatalf("insert: %v id=%d", err, id)
	}
	got, err := repo.Get(ctx, id)
	if err != nil || got.Body != "first" || got.Ref != ref {
		t.Fatalf("get: %v %+v", err, got)
	}
	if err := repo.Update(ctx, id, "edited", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	forStep, err := repo.ForStep(ctx, ref)
	if err != nil || len(forStep) != 1 || forStep[0].Body != "edited" {
		t.Fatalf("forStep: %v %+v", err, forStep)
	}
	if _, err := repo.Insert(ctx, notes.Note{Ref: course.StepRef{Module: "m2", Step: "s1"},
		Body: "other", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	all, err := repo.All(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("all: %v %d", err, len(all))
	}
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if all, _ = repo.All(ctx); len(all) != 1 {
		t.Fatalf("after delete: %d", len(all))
	}
}
```

Run: `go test ./internal/sqlite/ -run Notes` — Expected: FAIL (notes package / NewNotesRepo missing)

- [ ] **Step 2: Define notes domain + service (failing service test first)** — `internal/notes/service_test.go`:

```go
package notes_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

func fixtureCourse() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Module One", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "s1", Title: "One", Type: course.StepReading},
		}},
		{Slug: "m2", Title: "Module Two", Kind: course.KindLab, Order: 2, Steps: []course.Step{
			{Slug: "s1", Title: "Two", Type: course.StepReading},
		}},
	}}
}

func newSvc(t *testing.T) *notes.Service {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return notes.NewService(coursefs.NewRepo(fixtureCourse()), sqlite.NewNotesRepo(db))
}

func TestAddAndGroup(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if _, err := svc.Add(ctx, course.StepRef{Module: "m2", Step: "s1"}, "lab note"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, course.StepRef{Module: "m1", Step: "s1"}, "lecture note"); err != nil {
		t.Fatal(err)
	}
	groups, err := svc.GroupedByModule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// grouped in course order, not insertion order
	if len(groups) != 2 || groups[0].ModuleTitle != "Module One" || groups[1].ModuleTitle != "Module Two" {
		t.Fatalf("groups: %+v", groups)
	}
}

func TestAddValidates(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if _, err := svc.Add(ctx, course.StepRef{Module: "nope", Step: "s1"}, "x"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("unknown step err = %v", err)
	}
	if _, err := svc.Add(ctx, course.StepRef{Module: "m1", Step: "s1"}, "   "); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("empty body err = %v", err)
	}
}

func TestEditAndRemove(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	n, err := svc.Add(ctx, course.StepRef{Module: "m1", Step: "s1"}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	edited, err := svc.Edit(ctx, n.ID, "v2")
	if err != nil || edited.Body != "v2" {
		t.Fatalf("edit: %v %+v", err, edited)
	}
	if err := svc.Remove(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ForStep(ctx, course.StepRef{Module: "m1", Step: "s1"})
	if err != nil || len(got) != 0 {
		t.Fatalf("after remove: %v %d", err, len(got))
	}
}
```

Run: `go test ./internal/notes/` — Expected: FAIL

- [ ] **Step 3: Implement service** — `internal/notes/service.go`:

```go
// Package notes is the note-taking feature: notes attach to the step where
// they were taken and are browsed grouped by module.
package notes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type Note struct {
	ID        int64
	Ref       course.StepRef
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ModuleNotes struct {
	ModuleSlug, ModuleTitle string
	Notes                   []Note
}

type CourseRepo interface{ Course() *course.Course }

type Repo interface {
	Insert(ctx context.Context, n Note) (int64, error)
	Update(ctx context.Context, id int64, body string, updatedAt time.Time) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (Note, error)
	ForStep(ctx context.Context, ref course.StepRef) ([]Note, error)
	All(ctx context.Context) ([]Note, error)
}

type Service struct {
	course CourseRepo
	repo   Repo
	now    func() time.Time
}

func NewService(c CourseRepo, r Repo) *Service { return &Service{c, r, time.Now} }

func (s *Service) Add(ctx context.Context, ref course.StepRef, body string) (Note, error) {
	if _, _, ok := s.course.Course().Step(ref); !ok {
		return Note{}, fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	if strings.TrimSpace(body) == "" {
		return Note{}, fmt.Errorf("%w: note body is empty", api.ErrInvalid)
	}
	now := s.now().UTC()
	n := Note{Ref: ref, Body: body, CreatedAt: now, UpdatedAt: now}
	id, err := s.repo.Insert(ctx, n)
	if err != nil {
		return Note{}, err
	}
	n.ID = id
	return n, nil
}

func (s *Service) Edit(ctx context.Context, id int64, body string) (Note, error) {
	if strings.TrimSpace(body) == "" {
		return Note{}, fmt.Errorf("%w: note body is empty", api.ErrInvalid)
	}
	if err := s.repo.Update(ctx, id, body, s.now().UTC()); err != nil {
		return Note{}, err
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Remove(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }

func (s *Service) ForStep(ctx context.Context, ref course.StepRef) ([]Note, error) {
	return s.repo.ForStep(ctx, ref)
}

// GroupedByModule returns notes bucketed by module in course order; notes
// whose module no longer exists in the content are appended at the end.
func (s *Service) GroupedByModule(ctx context.Context) ([]ModuleNotes, error) {
	all, err := s.repo.All(ctx)
	if err != nil {
		return nil, err
	}
	byModule := map[string][]Note{}
	for _, n := range all {
		byModule[n.Ref.Module] = append(byModule[n.Ref.Module], n)
	}
	var out []ModuleNotes
	for _, m := range s.course.Course().Modules {
		if ns, ok := byModule[m.Slug]; ok {
			out = append(out, ModuleNotes{ModuleSlug: m.Slug, ModuleTitle: m.Title, Notes: ns})
			delete(byModule, m.Slug)
		}
	}
	for slug, ns := range byModule {
		out = append(out, ModuleNotes{ModuleSlug: slug, ModuleTitle: slug, Notes: ns})
	}
	return out, nil
}
```

- [ ] **Step 4: Implement sqlite notes repo** — `internal/sqlite/notes.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
)

type NotesRepo struct{ db *sql.DB }

func NewNotesRepo(db *sql.DB) *NotesRepo { return &NotesRepo{db} }

const noteCols = "id, module_slug, step_slug, body_md, created_at, updated_at"

func scanNote(row interface{ Scan(...any) error }) (notes.Note, error) {
	var n notes.Note
	var created, updated string
	if err := row.Scan(&n.ID, &n.Ref.Module, &n.Ref.Step, &n.Body, &created, &updated); err != nil {
		return notes.Note{}, err
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339, created)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return n, nil
}

func (r *NotesRepo) Insert(ctx context.Context, n notes.Note) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO notes (module_slug, step_slug, body_md, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		n.Ref.Module, n.Ref.Step, n.Body,
		n.CreatedAt.Format(time.RFC3339), n.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *NotesRepo) Update(ctx context.Context, id int64, body string, updatedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE notes SET body_md = ?, updated_at = ? WHERE id = ?",
		body, updatedAt.Format(time.RFC3339), id)
	return err
}

func (r *NotesRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM notes WHERE id = ?", id)
	return err
}

func (r *NotesRepo) Get(ctx context.Context, id int64) (notes.Note, error) {
	return scanNote(r.db.QueryRowContext(ctx,
		"SELECT "+noteCols+" FROM notes WHERE id = ?", id))
}

func (r *NotesRepo) ForStep(ctx context.Context, ref course.StepRef) ([]notes.Note, error) {
	return r.query(ctx,
		"SELECT "+noteCols+" FROM notes WHERE module_slug = ? AND step_slug = ? ORDER BY created_at DESC",
		ref.Module, ref.Step)
}

func (r *NotesRepo) All(ctx context.Context) ([]notes.Note, error) {
	return r.query(ctx,
		"SELECT "+noteCols+" FROM notes ORDER BY module_slug, created_at DESC")
}

func (r *NotesRepo) query(ctx context.Context, q string, args ...any) ([]notes.Note, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []notes.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
```

Run: `go test ./internal/sqlite/ ./internal/notes/` — Expected: PASS

- [ ] **Step 5: Endpoints** — `internal/notes/endpoint.go`:

```go
package notes

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type NotesService interface {
	Add(ctx context.Context, ref course.StepRef, body string) (Note, error)
	Edit(ctx context.Context, id int64, body string) (Note, error)
	Remove(ctx context.Context, id int64) error
	ForStep(ctx context.Context, ref course.StepRef) ([]Note, error)
	GroupedByModule(ctx context.Context) ([]ModuleNotes, error)
}

type DrawerRequest struct{ Module, Step string }

func (r DrawerRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type AddNoteRequest struct{ Module, Step, Body string }

type DrawerResponse struct {
	Ref   course.StepRef
	Notes []Note
}

func makeDrawerEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(DrawerRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		ns, err := svc.ForStep(ctx, ref)
		if err != nil {
			return nil, err
		}
		return DrawerResponse{Ref: ref, Notes: ns}, nil
	}
}

func makeAddEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(AddNoteRequest)
		if err := (DrawerRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if _, err := svc.Add(ctx, ref, req.Body); err != nil {
			return nil, err
		}
		ns, err := svc.ForStep(ctx, ref)
		if err != nil {
			return nil, err
		}
		return DrawerResponse{Ref: ref, Notes: ns}, nil
	}
}

type EditNoteRequest struct {
	ID   int64
	Body string
}

func makeEditEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(EditNoteRequest)
		return svc.Edit(ctx, req.ID, req.Body)
	}
}

func makeRemoveEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		return nil, svc.Remove(ctx, request.(int64))
	}
}

func makeIndexEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, _ any) (any, error) {
		return svc.GroupedByModule(ctx)
	}
}
```

- [ ] **Step 6: Templates** — append to `templates/viewmodels.go`:

```go
type NoteVM struct {
	ID                   int64
	ModuleSlug, StepSlug string
	Body                 string
	CreatedAt            string
}

type NotesDrawerVM struct {
	ModuleSlug, StepSlug string
	Notes                []NoteVM
}

type ModuleNotesVM struct {
	ModuleTitle string
	Notes       []NoteVM
}

type NotesIndexVM struct{ Groups []ModuleNotesVM }
```

`templates/notes.templ`:
```templ
package templates

import "fmt"

templ NotesDrawer(v NotesDrawerVM) {
	<div class="drawer-inner">
		<h3>Notes</h3>
		<form hx-post="/notes" hx-target="#notes-drawer" hx-swap="innerHTML">
			<input type="hidden" name="module" value={ v.ModuleSlug }/>
			<input type="hidden" name="step" value={ v.StepSlug }/>
			<textarea name="body" rows="4" placeholder="Jot a note for this step…" required></textarea>
			<button class="btn">Save note</button>
		</form>
		<ul class="note-list">
			for _, n := range v.Notes {
				@NoteItem(n)
			}
		</ul>
	</div>
}

templ NoteItem(n NoteVM) {
	<li class="note" id={ fmt.Sprintf("note-%d", n.ID) }>
		<p class="note-body">{ n.Body }</p>
		<span class="note-meta">{ n.CreatedAt }</span>
		<button class="link" hx-get={ fmt.Sprintf("/notes/%d/edit", n.ID) }
			hx-target={ fmt.Sprintf("#note-%d", n.ID) } hx-swap="outerHTML">edit</button>
		<button class="link danger" hx-delete={ fmt.Sprintf("/notes/%d", n.ID) }
			hx-target={ fmt.Sprintf("#note-%d", n.ID) } hx-swap="outerHTML">delete</button>
	</li>
}

templ NoteEditForm(n NoteVM) {
	<li class="note" id={ fmt.Sprintf("note-%d", n.ID) }>
		<form hx-put={ fmt.Sprintf("/notes/%d", n.ID) }
			hx-target={ fmt.Sprintf("#note-%d", n.ID) } hx-swap="outerHTML">
			<textarea name="body" rows="4" required>{ n.Body }</textarea>
			<button class="btn">Save</button>
		</form>
	</li>
}

templ NotesIndex(v NotesIndexVM) {
	<header class="topbar">
		<a href="/">⌂ Map</a>
		<span class="topbar-title">All notes</span>
	</header>
	<main class="content">
		if len(v.Groups) == 0 {
			<p class="empty">No notes yet. Open any step and hit “Notes”.</p>
		}
		for _, g := range v.Groups {
			<section>
				<h2>{ g.ModuleTitle }</h2>
				<ul class="note-list">
					for _, n := range g.Notes {
						@NoteItem(n)
					}
				</ul>
			</section>
		}
	</main>
}
```

- [ ] **Step 7: Transport** — `internal/notes/transport.go`:

The edit form needs a `Get` on the service: add `Get(ctx context.Context, id int64) (Note, error)` to the `NotesService` interface in `endpoint.go` and to `Service` in `service.go` (delegating to `s.repo.Get`), then write the transport:

```go
package notes

import (
	"net/http"
	"strconv"

	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func RegisterRoutes(mux *http.ServeMux, svc NotesService) {
	drawer := makeDrawerEndpoint(svc)
	add := makeAddEndpoint(svc)
	edit := makeEditEndpoint(svc)
	remove := makeRemoveEndpoint(svc)
	index := makeIndexEndpoint(svc)

	mux.HandleFunc("GET /notes", func(w http.ResponseWriter, r *http.Request) {
		resp, err := index(r.Context(), nil)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK,
			templates.Document("All notes", templates.NotesIndex(indexVM(resp.([]ModuleNotes)))))
	})

	mux.HandleFunc("GET /notes/drawer", func(w http.ResponseWriter, r *http.Request) {
		req := DrawerRequest{Module: r.URL.Query().Get("module"), Step: r.URL.Query().Get("step")}
		resp, err := drawer(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, drawerComponent(resp.(DrawerResponse)))
	})

	mux.HandleFunc("POST /notes", func(w http.ResponseWriter, r *http.Request) {
		req := AddNoteRequest{
			Module: r.FormValue("module"), Step: r.FormValue("step"), Body: r.FormValue("body"),
		}
		resp, err := add(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, drawerComponent(resp.(DrawerResponse)))
	})

	mux.HandleFunc("GET /notes/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		n, err := svc.Get(r.Context(), id)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, templates.NoteEditForm(noteVM(n)))
	})

	mux.HandleFunc("PUT /notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		resp, err := edit(r.Context(), EditNoteRequest{ID: id, Body: r.FormValue("body")})
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, templates.NoteItem(noteVM(resp.(Note))))
	})

	mux.HandleFunc("DELETE /notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		if _, err := remove(r.Context(), id); err != nil {
			api.RenderError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK) // empty body: htmx outerHTML swap removes the node
	})
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		api.RenderError(w, r, api.ErrInvalid)
		return 0, false
	}
	return id, true
}

func drawerComponent(resp DrawerResponse) templ.Component {
	vm := templates.NotesDrawerVM{ModuleSlug: resp.Ref.Module, StepSlug: resp.Ref.Step}
	for _, n := range resp.Notes {
		vm.Notes = append(vm.Notes, noteVM(n))
	}
	return templates.NotesDrawer(vm)
}

func noteVM(n Note) templates.NoteVM {
	return templates.NoteVM{
		ID: n.ID, ModuleSlug: n.Ref.Module, StepSlug: n.Ref.Step,
		Body: n.Body, CreatedAt: n.CreatedAt.Local().Format("Jan 2 15:04"),
	}
}

func indexVM(groups []ModuleNotes) templates.NotesIndexVM {
	var vm templates.NotesIndexVM
	for _, g := range groups {
		gv := templates.ModuleNotesVM{ModuleTitle: g.ModuleTitle}
		for _, n := range g.Notes {
			gv.Notes = append(gv.Notes, noteVM(n))
		}
		vm.Groups = append(vm.Groups, gv)
	}
	return vm
}
```

Add the import `"github.com/a-h/templ"` for the `templ.Component` return type, add `Get` to the `NotesService` interface in `endpoint.go`, and add to `service.go`:

```go
func (s *Service) Get(ctx context.Context, id int64) (Note, error) { return s.repo.Get(ctx, id) }
```

- [ ] **Step 8: Wire notes into the e2e harness and main**

In `e2e/harness_test.go` add after the tour registration:
```go
	notes.RegisterRoutes(mux, notes.NewService(courseRepo, sqlite.NewNotesRepo(db)))
```
(import `internal/notes`). In `cmd/tour/main.go` add the same line after `tour.RegisterRoutes`.

- [ ] **Step 9: Write the failing integration test** — `e2e/notes_test.go`:

```go
package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNotesFlow(t *testing.T) {
	app := newApp(t, options{})

	// add a note from a step
	resp, err := http.PostForm(app.TS.URL+"/notes", url.Values{
		"module": {"01-test-lecture"}, "step": {"01-read"}, "body": {"remember the shuffle"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "remember the shuffle") {
		t.Fatalf("drawer after add: %q", string(b))
	}

	// drawer partial serves it back
	resp, _ = http.Get(app.TS.URL + "/notes/drawer?module=01-test-lecture&step=01-read")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "remember the shuffle") {
		t.Fatal("drawer GET missing note")
	}

	// index groups under the module title
	resp, _ = http.Get(app.TS.URL + "/notes")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	if !strings.Contains(body, "Test Lecture") || !strings.Contains(body, "remember the shuffle") {
		t.Fatalf("index: %q", body)
	}

	// validation surfaces as 400
	resp, _ = http.PostForm(app.TS.URL+"/notes", url.Values{
		"module": {"01-test-lecture"}, "step": {"01-read"}, "body": {"  "},
	})
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("empty body = %d, want 400", resp.StatusCode)
	}
}
```

Run: `make test` — Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add -A && git commit -m "feat: notes - drawer, index, CRUD"
```

---

### Task 8: Evaluation foundation — submissions, locked-mode question answers

**Files:**
- Create: `internal/eval/models.go`, `internal/eval/prompts.go` (rubric loading only — prompt builders arrive in Task 9), `internal/eval/service.go`, `internal/eval/service_test.go`, `internal/eval/endpoint.go`, `internal/eval/transport.go`, `e2e/eval_test.go`
- Create: `internal/sqlite/submissions.go`, `internal/sqlite/submissions_test.go`
- Create: `templates/eval.templ`; `content/rubric/question.md`, `content/rubric/lab.md`
- Create: `e2e/testdata/content/rubric/{question.md,lab.md}`
- Modify: `templates/viewmodels.go` (eval VMs), `e2e/harness_test.go` (options.LLM/options.Lab + eval wiring), `cmd/tour/main.go` (wire eval)

**Interfaces:**
- Consumes: `course.*`, `sqlite`, `api.*`, templates.
- Produces:
  - `eval` types: `Kind` (`KindLab`/`KindQuestion`), `Status` (`StatusPending/Running/Complete/Failed`), `Submission{ID int64; Ref course.StepRef; Kind Kind; Content, TestOutput string; Status Status; CreatedAt time.Time}`, `Criterion{Name string; Score int; Justification string}` (JSON tags `name/score/justification`), `Verdict{Criteria []Criterion; Summary string; NextSteps []string}` (tags `criteria/summary/next_steps`), `Evaluation{ID, SubmissionID int64; Model, RubricVersion string; Verdict Verdict; CreatedAt time.Time}`.
  - Interfaces defined in `eval`: `LLM{Complete(ctx, system, user string) (string, error); Model() string}`, `LabRepo{Snapshot(workdir string, globs []string) (map[string]string, error); RunTests(ctx, workdir string, cmd []string, timeout time.Duration) (string, error)}`, `SubmissionRepo` (6 methods, below), `CourseRepo`, `ProgressMarker{SetComplete(ctx, course.StepRef, bool) error}`.
  - `eval.NewService(c CourseRepo, subs SubmissionRepo, p ProgressMarker, llm LLM, lab LabRepo, contentDir string, opts ...Option) (*Service, error)` — loads rubrics from `<contentDir>/rubric/{question,lab}.md` at construction; `eval.WithRunAsync(func(func())) Option`; methods this task: `Enabled() bool`, `SubmitAnswer(ctx, ref, answer) error`, `StepState(ctx, ref) (StepEvalView, error)`.
  - `eval.LoadRubric(path string) (Rubric, error)` with `Rubric{Version, Body string}`.
  - `eval.RegisterRoutes(mux, svc EvalService)` — this task: `GET /eval/section`, `POST /modules/{module}/steps/{step}/answer`.
  - `sqlite.NewSubmissionRepo(db) *SubmissionRepo` implementing `eval.SubmissionRepo`.

- [ ] **Step 1: Write eval models** — `internal/eval/models.go`:

```go
// Package eval is the evaluation feature: submissions of lab code and
// reading-question answers, optionally reviewed by an LLM against a rubric.
package eval

import (
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type Kind string

const (
	KindLab      Kind = "lab"
	KindQuestion Kind = "question"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
)

type Submission struct {
	ID         int64
	Ref        course.StepRef
	Kind       Kind
	Content    string // answer text, or JSON map[path]source for labs
	TestOutput string
	Status     Status
	CreatedAt  time.Time
}

type Criterion struct {
	Name          string `json:"name"`
	Score         int    `json:"score"`
	Justification string `json:"justification"`
}

type Verdict struct {
	Criteria  []Criterion `json:"criteria"`
	Summary   string      `json:"summary"`
	NextSteps []string    `json:"next_steps"`
}

type Evaluation struct {
	ID            int64
	SubmissionID  int64
	Model         string
	RubricVersion string
	Verdict       Verdict
	CreatedAt     time.Time
}
```

- [ ] **Step 2: Write the failing submissions repo test** — `internal/sqlite/submissions_test.go`:

```go
package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func subRepo(t *testing.T) *sqlite.SubmissionRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewSubmissionRepo(db)
}

func TestSubmissionLifecycle(t *testing.T) {
	repo := subRepo(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "q1"}

	if latest, err := repo.LatestForStep(ctx, ref); err != nil || latest != nil {
		t.Fatalf("expected no submission yet: %v %v", latest, err)
	}
	id, err := repo.InsertSubmission(ctx, eval.Submission{
		Ref: ref, Kind: eval.KindQuestion, Content: "answer",
		Status: eval.StatusPending, CreatedAt: time.Now(),
	})
	if err != nil || id == 0 {
		t.Fatalf("insert: %v", err)
	}
	if err := repo.UpdateSubmission(ctx, id, eval.StatusComplete, "out"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetSubmission(ctx, id)
	if err != nil || got.Status != eval.StatusComplete || got.TestOutput != "out" ||
		got.Content != "answer" || got.Kind != eval.KindQuestion {
		t.Fatalf("get: %v %+v", err, got)
	}
	latest, err := repo.LatestForStep(ctx, ref)
	if err != nil || latest == nil || latest.ID != id {
		t.Fatalf("latest: %v %v", latest, err)
	}
}

func TestEvaluationRoundTrip(t *testing.T) {
	repo := subRepo(t)
	ctx := context.Background()
	id, err := repo.InsertSubmission(ctx, eval.Submission{
		Ref: course.StepRef{Module: "m", Step: "s"}, Kind: eval.KindLab,
		Content: "{}", Status: eval.StatusPending, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e, err := repo.EvaluationForSubmission(ctx, id); err != nil || e != nil {
		t.Fatalf("expected none: %v %v", e, err)
	}
	verdict := eval.Verdict{
		Criteria: []eval.Criterion{{Name: "Correctness", Score: 4, Justification: "ok"}},
		Summary:  "fine", NextSteps: []string{"more tests"},
	}
	if _, err := repo.InsertEvaluation(ctx, eval.Evaluation{
		SubmissionID: id, Model: "m/x", RubricVersion: "1",
		Verdict: verdict, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	e, err := repo.EvaluationForSubmission(ctx, id)
	if err != nil || e == nil || e.Verdict.Criteria[0].Score != 4 || e.RubricVersion != "1" {
		t.Fatalf("eval: %v %+v", err, e)
	}
}
```

Run: `go test ./internal/sqlite/ -run 'Submission|Evaluation'` — Expected: FAIL

- [ ] **Step 3: Implement submissions repo** — `internal/sqlite/submissions.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

type SubmissionRepo struct{ db *sql.DB }

func NewSubmissionRepo(db *sql.DB) *SubmissionRepo { return &SubmissionRepo{db} }

func (r *SubmissionRepo) InsertSubmission(ctx context.Context, s eval.Submission) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO submissions (module_slug, step_slug, kind, content, test_output, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.Ref.Module, s.Ref.Step, string(s.Kind), s.Content, s.TestOutput, string(s.Status),
		s.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *SubmissionRepo) UpdateSubmission(ctx context.Context, id int64, status eval.Status, testOutput string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE submissions SET status = ?, test_output = ? WHERE id = ?",
		string(status), testOutput, id)
	return err
}

const subCols = "id, module_slug, step_slug, kind, content, test_output, status, created_at"

func scanSubmission(row interface{ Scan(...any) error }) (eval.Submission, error) {
	var s eval.Submission
	var kind, status, created string
	if err := row.Scan(&s.ID, &s.Ref.Module, &s.Ref.Step, &kind, &s.Content,
		&s.TestOutput, &status, &created); err != nil {
		return eval.Submission{}, err
	}
	s.Kind, s.Status = eval.Kind(kind), eval.Status(status)
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return s, nil
}

func (r *SubmissionRepo) GetSubmission(ctx context.Context, id int64) (eval.Submission, error) {
	return scanSubmission(r.db.QueryRowContext(ctx,
		"SELECT "+subCols+" FROM submissions WHERE id = ?", id))
}

func (r *SubmissionRepo) LatestForStep(ctx context.Context, ref course.StepRef) (*eval.Submission, error) {
	s, err := scanSubmission(r.db.QueryRowContext(ctx,
		"SELECT "+subCols+` FROM submissions
		 WHERE module_slug = ? AND step_slug = ? ORDER BY id DESC LIMIT 1`,
		ref.Module, ref.Step))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SubmissionRepo) InsertEvaluation(ctx context.Context, e eval.Evaluation) (int64, error) {
	verdict, err := json.Marshal(e.Verdict)
	if err != nil {
		return 0, err
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO evaluations (submission_id, model, rubric_version, verdict_json, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		e.SubmissionID, e.Model, e.RubricVersion, string(verdict),
		e.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *SubmissionRepo) EvaluationForSubmission(ctx context.Context, submissionID int64) (*eval.Evaluation, error) {
	var e eval.Evaluation
	var verdict, created string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, submission_id, model, rubric_version, verdict_json, created_at
		 FROM evaluations WHERE submission_id = ? ORDER BY id DESC LIMIT 1`, submissionID).
		Scan(&e.ID, &e.SubmissionID, &e.Model, &e.RubricVersion, &verdict, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(verdict), &e.Verdict); err != nil {
		return nil, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &e, nil
}
```

Run: `go test ./internal/sqlite/` — Expected: PASS

- [ ] **Step 4: Author the rubrics** (both real content and e2e testdata)

`content/rubric/question.md`:
```markdown
---
version: "1"
---

Score each criterion 1–5 (1 = missing, 3 = partially there, 5 = complete and precise):

1. **Correctness** — Are the claims factually right per the paper?
2. **Completeness** — Does the answer address every part of the question?
3. **Depth** — Does it explain *why*, not just restate mechanics?

Be strict; praise nothing the answer did not earn. In `next_steps`, point at
the specific paper sections to re-read for any criterion scoring below 4.
```

`content/rubric/lab.md`:
```markdown
---
version: "1"
---

Score each criterion 1–5:

1. **Correctness vs. tests** — Weigh the provided test output heavily;
   failing tests cap this criterion at 2.
2. **Concurrency discipline** — Consistent locking, no invited data races,
   no sleeps standing in for synchronization.
3. **Protocol fidelity** — The implementation follows the protocol the paper
   and lab specify; cite specifics when it deviates.
4. **Clarity** — A course TA could follow the code; names and structure
   carry meaning.

Justify every score with concrete references to files and functions. In
`next_steps`, give the 1–3 most valuable concrete improvements, most
important first.
```

Copy the same two files to `e2e/testdata/content/rubric/question.md` and `e2e/testdata/content/rubric/lab.md` (identical bodies are fine — the loader only needs valid files).

- [ ] **Step 5: Write failing rubric-loader + service tests** — add to `internal/eval/service_test.go`:

```go
package eval_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

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
	}}
}

type testEnv struct {
	svc      *eval.Service
	progress *sqlite.ProgressRepo
	subs     *sqlite.SubmissionRepo
}

func newEnv(t *testing.T, llm eval.LLM) testEnv {
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
	svc, err := eval.NewService(coursefs.NewRepo(fixtureCourse()), subs, progress, llm, nil,
		"../../content", eval.WithRunAsync(func(f func()) { f() }))
	if err != nil {
		t.Fatal(err)
	}
	return testEnv{svc: svc, progress: progress, subs: subs}
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
```

Run: `go test ./internal/eval/` — Expected: FAIL

- [ ] **Step 6: Implement rubric loading** — `internal/eval/prompts.go`:

```go
package eval

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rubric is versioned evaluation criteria loaded from content/rubric/*.md.
type Rubric struct {
	Version string
	Body    string
}

func LoadRubric(path string) (Rubric, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Rubric{}, err
	}
	s := string(raw)
	if !strings.HasPrefix(s, "---\n") {
		return Rubric{}, fmt.Errorf("%s: missing frontmatter", path)
	}
	rest := s[len("---\n"):]
	i := strings.Index(rest, "\n---")
	if i < 0 {
		return Rubric{}, fmt.Errorf("%s: unterminated frontmatter", path)
	}
	var fm struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(rest[:i+1]), &fm); err != nil {
		return Rubric{}, fmt.Errorf("%s: %w", path, err)
	}
	if fm.Version == "" {
		return Rubric{}, fmt.Errorf("%s: version is required", path)
	}
	return Rubric{
		Version: fm.Version,
		Body:    strings.TrimSpace(strings.TrimPrefix(rest[i+len("\n---"):], "\n")),
	}, nil
}
```

- [ ] **Step 7: Implement the service (locked mode)** — `internal/eval/service.go`:

```go
package eval

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type CourseRepo interface{ Course() *course.Course }

type ProgressMarker interface {
	SetComplete(ctx context.Context, ref course.StepRef, done bool) error
}

// LLM is the evaluation model provider; nil unlocks nothing — evaluation
// mode is locked and submissions are stored without review.
type LLM interface {
	Complete(ctx context.Context, system, user string) (string, error)
	Model() string
}

// LabRepo abstracts the student's mounted lab repository (implemented by
// FSLabRepo in the lab-evaluation task; nil until then).
type LabRepo interface {
	Snapshot(workdir string, globs []string) (map[string]string, error)
	RunTests(ctx context.Context, workdir string, cmd []string, timeout time.Duration) (string, error)
}

type SubmissionRepo interface {
	InsertSubmission(ctx context.Context, s Submission) (int64, error)
	UpdateSubmission(ctx context.Context, id int64, status Status, testOutput string) error
	GetSubmission(ctx context.Context, id int64) (Submission, error)
	LatestForStep(ctx context.Context, ref course.StepRef) (*Submission, error)
	InsertEvaluation(ctx context.Context, e Evaluation) (int64, error)
	EvaluationForSubmission(ctx context.Context, submissionID int64) (*Evaluation, error)
}

type StepEvalView struct {
	Enabled    bool
	Step       course.Step
	Submission *Submission
	Evaluation *Evaluation
}

type Service struct {
	course      CourseRepo
	subs        SubmissionRepo
	progress    ProgressMarker
	llm         LLM
	lab         LabRepo
	rubrics     map[string]Rubric
	guidanceDir string
	runAsync    func(func())
	now         func() time.Time
}

type Option func(*Service)

// WithRunAsync overrides how lab evaluations are scheduled; tests run them inline.
func WithRunAsync(f func(func())) Option { return func(s *Service) { s.runAsync = f } }

func NewService(c CourseRepo, subs SubmissionRepo, p ProgressMarker, llm LLM, lab LabRepo,
	contentDir string, opts ...Option) (*Service, error) {
	rubrics := map[string]Rubric{}
	for _, name := range []string{"question", "lab"} {
		r, err := LoadRubric(filepath.Join(contentDir, "rubric", name+".md"))
		if err != nil {
			return nil, fmt.Errorf("load rubric: %w", err)
		}
		rubrics[name] = r
	}
	s := &Service{
		course: c, subs: subs, progress: p, llm: llm, lab: lab,
		rubrics: rubrics, guidanceDir: filepath.Join(contentDir, "guidance"),
		runAsync: func(f func()) { go f() }, now: time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

func (s *Service) Enabled() bool { return s.llm != nil }

// SubmitAnswer stores a reading-question answer and marks the step complete.
// (The LLM evaluation branch is added with the OpenRouter provider task.)
func (s *Service) SubmitAnswer(ctx context.Context, ref course.StepRef, answer string) error {
	_, step, ok := s.course.Course().Step(ref)
	if !ok || step.Type != course.StepQuestion {
		return fmt.Errorf("%w: question step %s", api.ErrNotFound, ref)
	}
	if strings.TrimSpace(answer) == "" {
		return fmt.Errorf("%w: answer is empty", api.ErrInvalid)
	}
	id, err := s.subs.InsertSubmission(ctx, Submission{
		Ref: ref, Kind: KindQuestion, Content: answer,
		Status: StatusPending, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}
	if err := s.progress.SetComplete(ctx, ref, true); err != nil {
		return err
	}
	return s.subs.UpdateSubmission(ctx, id, StatusComplete, "")
}

func (s *Service) StepState(ctx context.Context, ref course.StepRef) (StepEvalView, error) {
	_, step, ok := s.course.Course().Step(ref)
	if !ok {
		return StepEvalView{}, fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	sub, err := s.subs.LatestForStep(ctx, ref)
	if err != nil {
		return StepEvalView{}, err
	}
	view := StepEvalView{Enabled: s.Enabled(), Step: *step, Submission: sub}
	if sub != nil {
		if view.Evaluation, err = s.subs.EvaluationForSubmission(ctx, sub.ID); err != nil {
			return StepEvalView{}, err
		}
	}
	return view, nil
}
```

Run: `go test ./internal/eval/` — Expected: PASS

- [ ] **Step 8: Endpoints** — `internal/eval/endpoint.go`:

```go
package eval

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// EvalService is the contract this feature's endpoints require; *Service
// satisfies it. Later tasks extend it with lab submission and retry.
type EvalService interface {
	StepState(ctx context.Context, ref course.StepRef) (StepEvalView, error)
	SubmitAnswer(ctx context.Context, ref course.StepRef, answer string) error
}

type SectionRequest struct{ Module, Step string }

func (r SectionRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type AnswerRequest struct{ Module, Step, Answer string }

type SectionResponse struct {
	Ref  course.StepRef
	View StepEvalView
}

func makeSectionEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}

func makeAnswerEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(AnswerRequest)
		if err := (SectionRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.SubmitAnswer(ctx, ref, req.Answer); err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}
```

- [ ] **Step 9: Templates** — append to `templates/viewmodels.go`:

```go
type CriterionVM struct {
	Name          string
	Score         int
	Justification string
}

type ReportVM struct {
	Model, RubricVersion, Summary string
	Criteria                      []CriterionVM
	NextSteps                     []string
}

type EvalSectionVM struct {
	ModuleSlug, StepSlug string
	Type                 string
	Enabled              bool
	Question             string
	Answer               string
	Status               string
	SubmissionID         int64
	TestOutput           string
	Report               *ReportVM
}
```

`templates/eval.templ` (the retry button targets a route that lands with the
OpenRouter task; `LabSection` is replaced wholesale by the lab-evaluation
task):
```templ
package templates

import "fmt"

templ EvalSection(v EvalSectionVM) {
	if v.Type == "question" {
		@QuestionForm(v)
	} else {
		@LabSection(v)
	}
}

templ QuestionForm(v EvalSectionVM) {
	<div class="eval">
		<h3>Reading question</h3>
		<blockquote class="question">{ v.Question }</blockquote>
		<form hx-post={ "/modules/" + v.ModuleSlug + "/steps/" + v.StepSlug + "/answer" }
			hx-target="#eval-section" hx-swap="innerHTML">
			<textarea name="answer" rows="8" required>{ v.Answer }</textarea>
			if v.Enabled {
				<button class="btn">Submit for evaluation</button>
			} else {
				<button class="btn">Save answer</button>
			}
		</form>
		if !v.Enabled {
			<p class="locked">
				Evaluation mode is locked — set OPENROUTER_API_KEY to get LLM feedback.
				Answers are still saved.
			</p>
		}
		@SubmissionResult(v)
	</div>
}

templ SubmissionResult(v EvalSectionVM) {
	if v.Status == "failed" {
		<div class="eval-failed">
			<p>Evaluation failed.</p>
			if v.TestOutput != "" {
				<pre class="test-output">{ v.TestOutput }</pre>
			}
			<button class="btn" hx-post={ fmt.Sprintf("/submissions/%d/retry", v.SubmissionID) }
				hx-target="#eval-section" hx-swap="innerHTML">Retry evaluation</button>
		</div>
	}
	if v.Status == "complete" && v.Report == nil && v.SubmissionID != 0 && v.Type == "question" {
		<p class="saved">✓ Answer saved.</p>
	}
	if v.Report != nil {
		@Report(*v.Report)
	}
}

templ Report(r ReportVM) {
	<div class="report">
		<h4>Evaluation — { r.Model } <span class="rubric-v">rubric v{ r.RubricVersion }</span></h4>
		<table>
			<tr><th>Criterion</th><th>Score</th><th>Justification</th></tr>
			for _, c := range r.Criteria {
				<tr>
					<td>{ c.Name }</td>
					<td>{ fmt.Sprintf("%d/5", c.Score) }</td>
					<td>{ c.Justification }</td>
				</tr>
			}
		</table>
		<p>{ r.Summary }</p>
		if len(r.NextSteps) > 0 {
			<h5>Suggested next steps</h5>
			<ul>
				for _, s := range r.NextSteps {
					<li>{ s }</li>
				}
			</ul>
		}
	</div>
}

// LabSection is a stub until the lab-evaluation pipeline task replaces it.
templ LabSection(v EvalSectionVM) {
	<div class="eval">
		<h3>Submit this lab part</h3>
		<p class="locked">Lab submission arrives with the evaluation pipeline.</p>
	</div>
}
```

- [ ] **Step 10: Transport** — `internal/eval/transport.go`:

```go
package eval

import (
	"net/http"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func RegisterRoutes(mux *http.ServeMux, svc EvalService) {
	section := makeSectionEndpoint(svc)
	answer := makeAnswerEndpoint(svc)

	renderSection := func(w http.ResponseWriter, r *http.Request, resp any, err error) {
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(SectionResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.EvalSection(sectionVM(res.Ref, res.View)))
	}

	mux.HandleFunc("GET /eval/section", func(w http.ResponseWriter, r *http.Request) {
		req := SectionRequest{Module: r.URL.Query().Get("module"), Step: r.URL.Query().Get("step")}
		resp, err := section(r.Context(), req)
		renderSection(w, r, resp, err)
	})

	mux.HandleFunc("POST /modules/{module}/steps/{step}/answer", func(w http.ResponseWriter, r *http.Request) {
		req := AnswerRequest{
			Module: r.PathValue("module"), Step: r.PathValue("step"),
			Answer: r.FormValue("answer"),
		}
		resp, err := answer(r.Context(), req)
		renderSection(w, r, resp, err)
	})
}

func sectionVM(ref course.StepRef, v StepEvalView) templates.EvalSectionVM {
	vm := templates.EvalSectionVM{
		ModuleSlug: ref.Module, StepSlug: ref.Step,
		Type: string(v.Step.Type), Enabled: v.Enabled, Question: v.Step.Question,
	}
	if v.Submission != nil {
		vm.SubmissionID = v.Submission.ID
		vm.Status = string(v.Submission.Status)
		vm.TestOutput = v.Submission.TestOutput
		if v.Submission.Kind == KindQuestion {
			vm.Answer = v.Submission.Content
		}
	}
	if v.Evaluation != nil {
		r := templates.ReportVM{
			Model: v.Evaluation.Model, RubricVersion: v.Evaluation.RubricVersion,
			Summary: v.Evaluation.Verdict.Summary, NextSteps: v.Evaluation.Verdict.NextSteps,
		}
		for _, c := range v.Evaluation.Verdict.Criteria {
			r.Criteria = append(r.Criteria, templates.CriterionVM{
				Name: c.Name, Score: c.Score, Justification: c.Justification,
			})
		}
		vm.Report = &r
	}
	return vm
}
```

- [ ] **Step 11: Wire eval into the e2e harness and main**

`e2e/harness_test.go` — extend `options` and the wiring (full updated file
sections):
```go
type options struct {
	ContentDir string
	LLM        eval.LLM     // nil = locked mode
	Lab        eval.LabRepo // nil until a test needs lab submission
}
```
after the notes registration:
```go
	evalSvc, err := eval.NewService(courseRepo, sqlite.NewSubmissionRepo(db),
		sqlite.NewProgressRepo(db), o.LLM, o.Lab, o.ContentDir,
		eval.WithRunAsync(func(f func()) { f() })) // synchronous: tests see final state
	if err != nil {
		t.Fatalf("eval service: %v", err)
	}
	eval.RegisterRoutes(mux, evalSvc)
```

`cmd/tour/main.go` — extract `progressRepo := sqlite.NewProgressRepo(db)`
(use it in the tour wiring too) and add after the notes registration:
```go
	evalSvc, err := eval.NewService(courseRepo, sqlite.NewSubmissionRepo(db),
		progressRepo, nil, nil, cfg.ContentDir)
	if err != nil {
		log.Fatalf("eval service: %v", err)
	}
	eval.RegisterRoutes(mux, evalSvc)
	log.Printf("evaluation mode enabled: %v", evalSvc.Enabled())
```

- [ ] **Step 12: Write the failing integration test** — `e2e/eval_test.go`:

```go
package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func fetch(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestQuestionLockedMode(t *testing.T) {
	app := newApp(t, options{})

	body := fetch(t, app.TS.URL+"/eval/section?module=01-test-lecture&step=02-question")
	if !strings.Contains(body, "locked") || !strings.Contains(body, "What is a distributed system?") {
		t.Fatalf("section: %q", body)
	}

	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/02-question/answer",
		url.Values{"answer": {"machines that fail independently"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Answer saved") {
		t.Fatalf("after answer: %q", string(b))
	}

	// answer survives reload, prefilled in the form
	body = fetch(t, app.TS.URL+"/eval/section?module=01-test-lecture&step=02-question")
	if !strings.Contains(body, "machines that fail independently") {
		t.Fatal("answer not prefilled")
	}

	// answering auto-completed the step
	body = fetch(t, app.TS.URL+"/modules/01-test-lecture/steps/02-question")
	if !strings.Contains(body, "Completed") {
		t.Fatal("step not auto-completed")
	}
}
```

Run: `make test` — Expected: PASS

- [ ] **Step 13: Commit**

```bash
git add -A && git commit -m "feat: eval foundation - submissions, locked-mode answers"
```

---

### Task 9: OpenRouter provider and question evaluation

**Files:**
- Create: `internal/openrouter/client.go`, `internal/openrouter/client_test.go`, `internal/openrouter/live_test.go`
- Create: `internal/eval/verdict.go`, `internal/eval/verdict_test.go`, `internal/eval/prompts_test.go`
- Modify: `internal/eval/prompts.go` (prompt builders), `internal/eval/service.go` (LLM branch + retry), `internal/eval/endpoint.go` (+Retry/RefForSubmission), `internal/eval/transport.go` (+retry route), `e2e/eval_test.go` (+fake LLM tests), `cmd/tour/main.go` (construct client from config)

**Interfaces:**
- Consumes: `eval.LLM` (implements it), `eval.Rubric`, `course.*`.
- Produces: `openrouter.New(apiKey, model string) *Client` (exported field `BaseURL` for tests; implements `eval.LLM`); `eval.ParseVerdict(raw string) (Verdict, error)`; `eval.BuildQuestionPrompt(r Rubric, mod course.Module, step course.Step, answer string) (system, user string)`; `eval.BuildLabPrompt(r Rubric, guidance string, mod course.Module, step course.Step, files map[string]string, testOutput string) (system, user string)`; service methods `Retry(ctx, id int64) error`, `RefForSubmission(ctx, id int64) (course.StepRef, error)`; route `POST /submissions/{id}/retry`.

- [ ] **Step 1: Write failing verdict tests** — `internal/eval/verdict_test.go`:

```go
package eval_test

import (
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

func TestParseVerdict(t *testing.T) {
	plain := `{"criteria":[{"name":"Correctness","score":4,"justification":"good"}],` +
		`"summary":"Nice.","next_steps":["reread"]}`
	cases := []struct {
		name, in string
		wantErr  string
	}{
		{"plain json", plain, ""},
		{"fenced json", "```json\n" + plain + "\n```", ""},
		{"prose wrapped", "Here you go:\n" + plain + "\nHope that helps!", ""},
		{"garbage", "I cannot evaluate this.", "no JSON object"},
		{"empty criteria", `{"criteria":[],"summary":"x","next_steps":[]}`, "no criteria"},
		{"score out of range", `{"criteria":[{"name":"C","score":9,"justification":"j"}],` +
			`"summary":"x","next_steps":[]}`, "out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := eval.ParseVerdict(tc.in)
			if tc.wantErr == "" {
				if err != nil || v.Criteria[0].Score != 4 || v.Summary != "Nice." {
					t.Fatalf("got %v %+v", err, v)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
```

Run: `go test ./internal/eval/ -run Verdict` — Expected: FAIL

- [ ] **Step 2: Implement** — `internal/eval/verdict.go`:

```go
package eval

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseVerdict extracts the JSON verdict from raw LLM output, tolerating
// markdown fences and surrounding prose.
func ParseVerdict(raw string) (Verdict, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return Verdict{}, fmt.Errorf("no JSON object in model output")
	}
	var v Verdict
	if err := json.Unmarshal([]byte(raw[start:end+1]), &v); err != nil {
		return Verdict{}, fmt.Errorf("decode verdict: %w", err)
	}
	if len(v.Criteria) == 0 {
		return Verdict{}, fmt.Errorf("verdict has no criteria")
	}
	for _, c := range v.Criteria {
		if c.Score < 1 || c.Score > 5 {
			return Verdict{}, fmt.Errorf("criterion %q score %d out of range 1-5", c.Name, c.Score)
		}
	}
	return v, nil
}
```

Run: `go test ./internal/eval/ -run Verdict` — Expected: PASS

- [ ] **Step 3: Write failing prompt tests** — `internal/eval/prompts_test.go`:

```go
package eval_test

import (
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

func TestBuildQuestionPrompt(t *testing.T) {
	r := eval.Rubric{Version: "1", Body: "RUBRIC-BODY"}
	mod := course.Module{Title: "Lecture X"}
	step := course.Step{Question: "Why is the sky blue?"}
	system, user := eval.BuildQuestionPrompt(r, mod, step, "scattering")
	if !strings.Contains(system, "RUBRIC-BODY") || !strings.Contains(system, `"criteria"`) {
		t.Fatalf("system: %q", system)
	}
	for _, want := range []string{"Lecture X", "Why is the sky blue?", "scattering"} {
		if !strings.Contains(user, want) {
			t.Fatalf("user missing %q: %q", want, user)
		}
	}
}

func TestBuildLabPrompt(t *testing.T) {
	r := eval.Rubric{Version: "1", Body: "LAB-RUBRIC"}
	system, user := eval.BuildLabPrompt(r, "GUIDANCE-TEXT",
		course.Module{Title: "Lab X"}, course.Step{Title: "Submit"},
		map[string]string{"b.go": "package b", "a.go": "package a"}, "TEST-OUT")
	if !strings.Contains(system, "LAB-RUBRIC") || !strings.Contains(system, "GUIDANCE-TEXT") {
		t.Fatalf("system: %q", system)
	}
	// files appear sorted with path headers
	ai := strings.Index(user, "--- a.go ---")
	bi := strings.Index(user, "--- b.go ---")
	if ai < 0 || bi < 0 || ai > bi || !strings.Contains(user, "TEST-OUT") {
		t.Fatalf("user: %q", user)
	}
}
```

Run: `go test ./internal/eval/ -run Prompt` — Expected: FAIL

- [ ] **Step 4: Implement prompt builders** — append to `internal/eval/prompts.go`:

```go
const verdictShape = `{"criteria":[{"name":"<criterion>","score":<1-5>,"justification":"<why>"}],` +
	`"summary":"<2-4 sentences>","next_steps":["<concrete action>"]}`

const taRole = "You are a strict but constructive teaching assistant for MIT 6.824 " +
	"(Distributed Systems). Respond with ONLY a JSON object in exactly this shape:\n"

func BuildQuestionPrompt(r Rubric, mod course.Module, step course.Step, answer string) (system, user string) {
	system = taRole + verdictShape +
		"\n\nEvaluate the student's answer to a reading question.\n\nRubric (version " +
		r.Version + "):\n" + r.Body
	user = fmt.Sprintf("Module: %s\n\nQuestion:\n%s\n\nStudent answer:\n%s",
		mod.Title, step.Question, answer)
	return system, user
}

func BuildLabPrompt(r Rubric, guidance string, mod course.Module, step course.Step,
	files map[string]string, testOutput string) (system, user string) {
	system = taRole + verdictShape +
		"\n\nReview the student's lab code and its test output.\n\nRubric (version " +
		r.Version + "):\n" + r.Body
	if guidance != "" {
		system += "\n\nLab-specific guidance:\n" + guidance
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Module: %s — %s\n\nTest output:\n%s\n\nCode:\n", mod.Title, step.Title, testOutput)
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

Add imports `"sort"` and `"github.com/itsnoproblem/mit-distributed-systems/internal/course"` to `prompts.go`.

Run: `go test ./internal/eval/` — Expected: PASS

- [ ] **Step 5: Write failing OpenRouter client test** — `internal/openrouter/client_test.go`:

```go
package openrouter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/openrouter"
)

func TestComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth = %q", got)
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "test/model" {
			t.Errorf("model = %v", req["model"])
		}
		msgs := req["messages"].([]any)
		if len(msgs) != 2 {
			t.Errorf("messages = %d", len(msgs))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "the verdict"}}},
		})
	}))
	defer srv.Close()

	c := openrouter.New("test-key", "test/model")
	c.BaseURL = srv.URL
	out, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil || out != "the verdict" {
		t.Fatalf("got %q %v", out, err)
	}
	if c.Model() != "test/model" {
		t.Fatalf("Model() = %q", c.Model())
	}
}

func TestCompleteErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := openrouter.New("k", "m")
	c.BaseURL = srv.URL
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil ||
		!strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v", err)
	}
}
```

Run: `go test ./internal/openrouter/` — Expected: FAIL

- [ ] **Step 6: Implement client** — `internal/openrouter/client.go`:

```go
// Package openrouter implements eval.LLM against the OpenRouter chat API.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string // overridable in tests
	apiKey  string
	model   string
	hc      *http.Client
}

func New(apiKey, model string) *Client {
	return &Client{
		BaseURL: "https://openrouter.ai/api/v1",
		apiKey:  apiKey,
		model:   model,
		hc:      &http.Client{Timeout: 180 * time.Second},
	}
}

func (c *Client) Model() string { return c.model }

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Temperature float64   `json:"temperature"`
	Messages    []message `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model, Temperature: 0.2,
		Messages: []message{{Role: "system", Content: system}, {Role: "user", Content: user}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter: status %d: %.200s", resp.StatusCode, raw)
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("openrouter: decode response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("openrouter: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openrouter: response has no choices")
	}
	return out.Choices[0].Message.Content, nil
}
```

Run: `go test ./internal/openrouter/` — Expected: PASS

- [ ] **Step 7: Env-gated live smoke test** — `internal/openrouter/live_test.go`:

```go
package openrouter_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/openrouter"
)

// TestLiveComplete hits the real API. Run manually:
//
//	OPENROUTER_LIVE=1 OPENROUTER_API_KEY=... go test ./internal/openrouter/ -run Live -v
func TestLiveComplete(t *testing.T) {
	if os.Getenv("OPENROUTER_LIVE") != "1" {
		t.Skip("set OPENROUTER_LIVE=1 and OPENROUTER_API_KEY to run")
	}
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Fatal("OPENROUTER_API_KEY is required with OPENROUTER_LIVE=1")
	}
	c := openrouter.New(key, "anthropic/claude-sonnet-4")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := c.Complete(ctx, "Reply with exactly the word: pong", "ping")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("live response: %q", out)
	if out == "" {
		t.Fatal("empty response")
	}
}
```

- [ ] **Step 8: Add the LLM branch and retry to the service** — in `internal/eval/service.go`, replace `SubmitAnswer`'s final `return s.subs.UpdateSubmission(ctx, id, StatusComplete, "")` with:

```go
	if s.llm == nil {
		return s.subs.UpdateSubmission(ctx, id, StatusComplete, "")
	}
	return s.evaluateQuestion(ctx, id)
```

and add:

```go
// evaluateQuestion runs the synchronous question-evaluation pipeline; LLM
// failures land on the submission as StatusFailed, never as an HTTP error.
func (s *Service) evaluateQuestion(ctx context.Context, id int64) error {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return err
	}
	mod, step, ok := s.course.Course().Step(sub.Ref)
	if !ok {
		return fmt.Errorf("%w: step %s", api.ErrNotFound, sub.Ref)
	}
	if err := s.subs.UpdateSubmission(ctx, id, StatusRunning, ""); err != nil {
		return err
	}
	rubric := s.rubrics["question"]
	system, user := BuildQuestionPrompt(rubric, *mod, *step, sub.Content)
	raw, err := s.llm.Complete(ctx, system, user)
	if err != nil {
		return s.subs.UpdateSubmission(ctx, id, StatusFailed, "LLM error: "+err.Error())
	}
	verdict, err := ParseVerdict(raw)
	if err != nil {
		return s.subs.UpdateSubmission(ctx, id, StatusFailed, "verdict parse error: "+err.Error())
	}
	if _, err := s.subs.InsertEvaluation(ctx, Evaluation{
		SubmissionID: id, Model: s.llm.Model(), RubricVersion: rubric.Version,
		Verdict: verdict, CreatedAt: s.now().UTC(),
	}); err != nil {
		return err
	}
	return s.subs.UpdateSubmission(ctx, id, StatusComplete, "")
}

func (s *Service) RefForSubmission(ctx context.Context, id int64) (course.StepRef, error) {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return course.StepRef{}, fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	return sub.Ref, nil
}

// Retry re-evaluates a failed submission. (Lab retries arrive with the lab
// pipeline task; until then only questions are retryable.)
func (s *Service) Retry(ctx context.Context, id int64) error {
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	if sub.Status != StatusFailed {
		return fmt.Errorf("%w: submission %d is %s, not failed", api.ErrInvalid, id, sub.Status)
	}
	if s.llm == nil {
		return fmt.Errorf("%w: evaluation mode is locked", api.ErrInvalid)
	}
	if sub.Kind != KindQuestion {
		return fmt.Errorf("%w: lab retry requires the lab pipeline", api.ErrInvalid)
	}
	return s.evaluateQuestion(ctx, id)
}
```

- [ ] **Step 9: Extend endpoints + routes** — in `internal/eval/endpoint.go` add to the `EvalService` interface:

```go
	Retry(ctx context.Context, id int64) error
	RefForSubmission(ctx context.Context, id int64) (course.StepRef, error)
```

and add:

```go
func makeRetryEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		id := request.(int64)
		if err := svc.Retry(ctx, id); err != nil {
			return nil, err
		}
		ref, err := svc.RefForSubmission(ctx, id)
		if err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}
```

In `internal/eval/transport.go` `RegisterRoutes`, add:

```go
	retry := makeRetryEndpoint(svc)
	mux.HandleFunc("POST /submissions/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		resp, err := retry(r.Context(), id)
		renderSection(w, r, resp, err)
	})
```

(add import `"strconv"`).

- [ ] **Step 10: Integration tests with a fake LLM** — append to `e2e/eval_test.go`:

```go
type fakeLLM struct {
	resp string
	err  error
}

func (f fakeLLM) Complete(_ context.Context, _, _ string) (string, error) { return f.resp, f.err }
func (f fakeLLM) Model() string                                           { return "fake/model" }

const goodVerdict = "```json\n" +
	`{"criteria":[{"name":"Correctness","score":4,"justification":"solid"}],` +
	`"summary":"Good answer.","next_steps":["reread the failure model section"]}` + "\n```"

func TestQuestionEvaluated(t *testing.T) {
	app := newApp(t, options{LLM: fakeLLM{resp: goodVerdict}})
	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/02-question/answer",
		url.Values{"answer": {"my considered answer"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	for _, want := range []string{"Correctness", "4/5", "Good answer.", "fake/model", "rubric v1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("report missing %q: %q", want, body)
		}
	}
}

func TestLLMFailureIsRecordedAndRetryable(t *testing.T) {
	app := newApp(t, options{LLM: fakeLLM{err: errors.New("boom")}})
	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/02-question/answer",
		url.Values{"answer": {"attempt"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Evaluation failed") || !strings.Contains(string(b), "/retry") {
		t.Fatalf("failure UI: %q", string(b))
	}
	// retry with a still-failing LLM stays failed but responds 200
	resp, err = http.Post(app.TS.URL+"/submissions/1/retry", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "Evaluation failed") {
		t.Fatalf("retry: %d %q", resp.StatusCode, string(b))
	}
}
```

(add imports `"context"`, `"errors"` to the test file).

Run: `make test` — Expected: PASS

- [ ] **Step 11: Wire the client in main** — in `cmd/tour/main.go` replace the eval wiring's `nil` LLM with:

```go
	var llm eval.LLM
	if cfg.OpenRouterKey != "" {
		llm = openrouter.New(cfg.OpenRouterKey, cfg.OpenRouterModel)
	}
```

and pass `llm` to `eval.NewService` (import `internal/openrouter`).

Run: `make test` — Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add -A && git commit -m "feat: openrouter provider and question evaluation with retry"
```

---

### Task 10: Lab evaluation pipeline — snapshot, test run, async evaluate, polling UI

**Files:**
- Create: `internal/eval/labrepo.go`, `internal/eval/labrepo_test.go`
- Create: `internal/eval/testdata/fakerepo/go.mod`, `internal/eval/testdata/fakerepo/src/hello/hello_test.go`
- Create: `content/guidance/lab-01-mapreduce.md`
- Modify: `internal/eval/service.go` (SubmitLab + evaluateLab + full Retry), `internal/eval/endpoint.go` (+SubmitLab), `internal/eval/transport.go` (+submit-lab, +section-by-submission routes), `templates/eval.templ` (real LabSection), `e2e/eval_test.go` (+lab tests), `cmd/tour/main.go` (FSLabRepo)

**Interfaces:**
- Consumes: `eval.LabRepo` (implements as `FSLabRepo`), `course.EvalMeta`, fakeLLM/goodVerdict from Task 9's integration test file.
- Produces: `eval.FSLabRepo{Dir string}` implementing `LabRepo`; service methods `SubmitLab(ctx, ref) error` and full `Retry`; routes `POST /modules/{module}/steps/{step}/submit-lab`, `GET /submissions/{id}/section`.

- [ ] **Step 1: Write the test fixture repo**

`internal/eval/testdata/fakerepo/go.mod`:
```
module fakerepo

go 1.23
```

`internal/eval/testdata/fakerepo/src/hello/hello_test.go`:
```go
package hello

import "testing"

func TestAlwaysPasses(t *testing.T) {}
```

(Directories named `testdata` are ignored by the go tool, so this nested
module never interferes with `go test ./...`.)

- [ ] **Step 2: Write failing FSLabRepo tests** — `internal/eval/labrepo_test.go`:

```go
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
		[]string{"go", "test"}, time.Minute)
	if err != nil {
		t.Fatalf("err = %v, out = %q", err, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("out = %q", out)
	}
}

func TestRunTestsTimeout(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	_, err := repo.RunTests(context.Background(), "src/hello",
		[]string{"sleep", "5"}, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}
```

Run: `go test ./internal/eval/ -run 'Snapshot|RunTests'` — Expected: FAIL

- [ ] **Step 3: Implement** — `internal/eval/labrepo.go`:

```go
package eval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
```

Run: `go test ./internal/eval/ -run 'Snapshot|RunTests'` — Expected: PASS

- [ ] **Step 4: Add SubmitLab and evaluateLab to the service** — append to `internal/eval/service.go` (add import `"encoding/json"`, `"os"`):

```go
// SubmitLab snapshots the lab code, records the submission, marks the step
// complete, and schedules the async run-tests-then-evaluate pipeline.
func (s *Service) SubmitLab(ctx context.Context, ref course.StepRef) error {
	_, step, ok := s.course.Course().Step(ref)
	if !ok || step.Type != course.StepSubmit || step.Eval == nil {
		return fmt.Errorf("%w: submit step %s", api.ErrNotFound, ref)
	}
	if s.lab == nil {
		return fmt.Errorf("%w: no lab repository configured (set LAB_REPO_DIR)", api.ErrInvalid)
	}
	files, err := s.lab.Snapshot(step.Eval.Workdir, step.Eval.Globs)
	if err != nil {
		return fmt.Errorf("%w: snapshot: %v", api.ErrInvalid, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("%w: no files matched %v under %s",
			api.ErrInvalid, step.Eval.Globs, step.Eval.Workdir)
	}
	content, err := json.Marshal(files)
	if err != nil {
		return err
	}
	id, err := s.subs.InsertSubmission(ctx, Submission{
		Ref: ref, Kind: KindLab, Content: string(content),
		Status: StatusPending, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}
	if err := s.progress.SetComplete(ctx, ref, true); err != nil {
		return err
	}
	s.runAsync(func() { s.evaluateLab(id) })
	return nil
}

// evaluateLab runs in the background with its own context; every failure
// lands on the submission row, never in a log the user can't see.
func (s *Service) evaluateLab(id int64) {
	ctx := context.Background()
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		return
	}
	mod, step, ok := s.course.Course().Step(sub.Ref)
	if !ok || step.Eval == nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, "step no longer exists in content")
		return
	}
	_ = s.subs.UpdateSubmission(ctx, id, StatusRunning, "")
	out, err := s.lab.RunTests(ctx, step.Eval.Workdir, step.Eval.TestCmd, step.Eval.Timeout)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nRUNNER ERROR: "+err.Error())
		return
	}
	if s.llm == nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusComplete, out)
		return
	}
	var files map[string]string
	if err := json.Unmarshal([]byte(sub.Content), &files); err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nSNAPSHOT DECODE ERROR: "+err.Error())
		return
	}
	rubric := s.rubrics["lab"]
	system, user := BuildLabPrompt(rubric, s.loadGuidance(sub.Ref.Module), *mod, *step, files, out)
	raw, err := s.llm.Complete(ctx, system, user)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nLLM ERROR: "+err.Error())
		return
	}
	verdict, err := ParseVerdict(raw)
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nVERDICT PARSE ERROR: "+err.Error())
		return
	}
	if _, err := s.subs.InsertEvaluation(ctx, Evaluation{
		SubmissionID: id, Model: s.llm.Model(), RubricVersion: rubric.Version,
		Verdict: verdict, CreatedAt: s.now().UTC(),
	}); err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nSTORE ERROR: "+err.Error())
		return
	}
	_ = s.subs.UpdateSubmission(ctx, id, StatusComplete, out)
}

// loadGuidance returns per-module evaluator guidance, or "" when none is authored.
func (s *Service) loadGuidance(moduleSlug string) string {
	raw, err := os.ReadFile(filepath.Join(s.guidanceDir, moduleSlug+".md"))
	if err != nil {
		return ""
	}
	return string(raw)
}
```

Replace `Retry`'s question-only guard (`if sub.Kind != KindQuestion { ... }` and the final return) with:

```go
	switch sub.Kind {
	case KindQuestion:
		return s.evaluateQuestion(ctx, id)
	default:
		s.runAsync(func() { s.evaluateLab(id) })
		return nil
	}
```

- [ ] **Step 5: Extend endpoints + routes** — in `internal/eval/endpoint.go` add `SubmitLab(ctx context.Context, ref course.StepRef) error` to the `EvalService` interface, plus:

```go
type SubmitLabRequest struct{ Module, Step string }

func makeSubmitLabEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SubmitLabRequest)
		if err := (SectionRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.SubmitLab(ctx, ref); err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}

func makeSectionBySubmissionEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		ref, err := svc.RefForSubmission(ctx, request.(int64))
		if err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}
```

In `internal/eval/transport.go` `RegisterRoutes` add:

```go
	submitLab := makeSubmitLabEndpoint(svc)
	bySubmission := makeSectionBySubmissionEndpoint(svc)

	mux.HandleFunc("POST /modules/{module}/steps/{step}/submit-lab", func(w http.ResponseWriter, r *http.Request) {
		req := SubmitLabRequest{Module: r.PathValue("module"), Step: r.PathValue("step")}
		resp, err := submitLab(r.Context(), req)
		renderSection(w, r, resp, err)
	})

	mux.HandleFunc("GET /submissions/{id}/section", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		resp, err := bySubmission(r.Context(), id)
		renderSection(w, r, resp, err)
	})
```

- [ ] **Step 6: Replace the LabSection stub** — in `templates/eval.templ` replace the whole `templ LabSection` block with:

```templ
templ LabSection(v EvalSectionVM) {
	<div class="eval">
		<h3>Submit this lab part</h3>
		if !v.Enabled {
			<p class="locked">
				Evaluation mode is locked — set OPENROUTER_API_KEY for LLM review.
				Submitting still snapshots your code and runs the lab tests.
			</p>
		}
		if v.Status == "pending" || v.Status == "running" {
			<div hx-get={ fmt.Sprintf("/submissions/%d/section", v.SubmissionID) }
				hx-trigger="every 3s" hx-target="#eval-section" hx-swap="innerHTML">
				<p>⏳ Evaluation running — lab tests can take several minutes.</p>
			</div>
		} else {
			<form hx-post={ "/modules/" + v.ModuleSlug + "/steps/" + v.StepSlug + "/submit-lab" }
				hx-target="#eval-section" hx-swap="innerHTML">
				<button class="btn">Snapshot code & run evaluation</button>
			</form>
			@SubmissionResult(v)
			if v.Status == "complete" && v.TestOutput != "" {
				<details>
					<summary>Test output</summary>
					<pre class="test-output">{ v.TestOutput }</pre>
				</details>
			}
		}
	</div>
}
```

Run: `make generate` — Expected: regenerates cleanly.

- [ ] **Step 7: Author Lab 1 evaluator guidance** — `content/guidance/lab-01-mapreduce.md`:

```markdown
Lab-specific pitfalls to check:

- The coordinator must tolerate workers that crash mid-task: tasks should be
  re-issued after a timeout, and duplicate completions must be harmless.
- Output must be atomic — look for write-temp-then-rename; direct writes to
  `mr-out-*` are a correctness bug under crashes.
- Intermediate files must be partitioned with `ihash(key) % nReduce`.
- RPC handlers must not hold the coordinator's mutex across blocking calls.
- Crash-test failures in `test-mr.sh` almost always mean missing task
  re-issue or non-atomic output — say so explicitly if the output shows it.
```

- [ ] **Step 8: Integration tests** — append to `e2e/eval_test.go` (reuses `fakeLLM`/`goodVerdict` from Task 9; add imports `"time"`):

```go
type stubLab struct {
	files map[string]string
	out   string
	err   error
}

func (s stubLab) Snapshot(string, []string) (map[string]string, error) { return s.files, nil }
func (s stubLab) RunTests(context.Context, string, []string, time.Duration) (string, error) {
	return s.out, s.err
}

func TestLabSubmitEvaluated(t *testing.T) {
	app := newApp(t, options{
		LLM: fakeLLM{resp: goodVerdict},
		Lab: stubLab{files: map[string]string{"src/x/x.go": "package x"}, out: "PASS\nok"},
	})
	// the e2e harness uses a synchronous runner, so the pipeline finishes before the response
	resp, err := http.Post(app.TS.URL+"/modules/02-test-lab/steps/01-submit/submit-lab", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	for _, want := range []string{"Correctness", "Good answer.", "Test output"} {
		if !strings.Contains(body, want) {
			t.Fatalf("lab report missing %q: %q", want, body)
		}
	}
	// submitting auto-completed the step
	if !strings.Contains(fetch(t, app.TS.URL+"/modules/02-test-lab/steps/01-submit"), "Completed") {
		t.Fatal("submit step not auto-completed")
	}
}

func TestLabRunnerFailure(t *testing.T) {
	app := newApp(t, options{
		LLM: fakeLLM{resp: goodVerdict},
		Lab: stubLab{files: map[string]string{"a.go": "package a"}, out: "partial output",
			err: errors.New("test run timed out after 1m0s")},
	})
	resp, err := http.Post(app.TS.URL+"/modules/02-test-lab/steps/01-submit/submit-lab", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	if !strings.Contains(body, "Evaluation failed") || !strings.Contains(body, "RUNNER ERROR") {
		t.Fatalf("failure UI: %q", body)
	}
}
```

Run: `make test` — Expected: PASS

- [ ] **Step 9: Wire FSLabRepo in main** — in `cmd/tour/main.go` replace the `nil` lab argument with `eval.FSLabRepo{Dir: cfg.LabRepoDir}`.

Run: `make test` — Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add -A && git commit -m "feat: lab evaluation - snapshot, test runner, async pipeline, polling UI"
```

---

### Task 11: Full course skeleton — generator + generated content

**Files:**
- Create: `scripts/gen-skeleton/main.go`
- Create (generated): `content/modules/*` for every remaining lecture, lab, and the project; `content/guidance/lab-0{2..5}-*.md`
- Modify: `internal/coursefs/realcontent_test.go` (expect the full course)

**Interfaces:**
- Consumes: the content conventions from Tasks 3–4. Produces: the complete navigable course. The generator skips any module directory that already exists, so re-running never clobbers hand-authored content. Paper links in stubs point at the course schedule page; refining them (and the guidance prose) module-by-module is the ongoing authoring work the design doc anticipates.

- [ ] **Step 1: Update the real-content test to expect the full course** — in `internal/coursefs/realcontent_test.go` change the module-count assertion to:

```go
	// 22 lectures + 5 labs + 1 project
	if len(c.Modules) != 28 {
		t.Fatalf("expected 28 modules, got %d", len(c.Modules))
	}
```

Run: `go test ./internal/coursefs/ -run RealContent` — Expected: FAIL (only 2 modules exist)

- [ ] **Step 2: Write the generator** — `scripts/gen-skeleton/main.go`:

```go
// Command gen-skeleton writes module.yaml + step stubs for every course unit
// that does not already have a module directory. Existing directories are
// never touched, so hand-authored content survives re-runs.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const schedule = "https://pdos.csail.mit.edu/6.824/schedule.html"

type lecture struct {
	slug, title string
	order       int
}

var lectures = []lecture{
	{"01-introduction", "Lecture 1: Introduction & MapReduce", 10},
	{"02-rpc-and-threads", "Lecture 2: RPC and Threads", 20},
	{"03-gfs", "Lecture 3: GFS", 30},
	{"04-paxos", "Lecture 4: Paxos", 40},
	{"05-go-patterns", "Lecture 5: Go Patterns", 50},
	{"06-raft-1", "Lecture 6: Fault Tolerance — Raft (1)", 60},
	{"07-raft-2", "Lecture 7: Fault Tolerance — Raft (2)", 70},
	{"08-linearizability", "Lecture 8: Consistency & Linearizability", 80},
	{"09-zookeeper", "Lecture 9: ZooKeeper", 90},
	{"10-lab-qa", "Lecture 10: Q&A — Raft Labs", 100},
	{"11-distributed-transactions", "Lecture 11: Distributed Transactions", 110},
	{"12-spanner", "Lecture 12: Spanner", 120},
	{"13-chain-replication", "Lecture 13: Chain Replication", 140},
	{"14-occ-farm", "Lecture 14: Optimistic Concurrency Control — FaRM", 150},
	{"15-verification", "Lecture 15: Verification of Distributed Systems", 160},
	{"16-memcached", "Lecture 16: Cache Consistency — Memcached at Facebook", 170},
	{"17-aws-lambda", "Lecture 17: AWS Lambda", 180},
	{"18-ray", "Lecture 18: Ray", 200},
	{"19-sundr", "Lecture 19: Fork Consistency — SUNDR", 210},
	{"20-bitcoin", "Lecture 20: Peer-to-peer — Bitcoin", 220},
	{"21-byzantine-ft", "Lecture 21: Byzantine Fault Tolerance", 230},
	{"22-project-demos", "Lecture 22: Project Demos", 240},
}

type labPart struct {
	name, workdir, runFilter, timeout string
}

type lab struct {
	slug, title, page string
	order             int
	parts             []labPart
}

var labs = []lab{
	{"lab-01-mapreduce", "Lab 1: MapReduce", "https://pdos.csail.mit.edu/6.824/labs/lab-mr.html", 15, nil},
	{"lab-02-kvserver", "Lab 2: Key/Value Server", "https://pdos.csail.mit.edu/6.824/labs/lab-kvsrv.html", 45,
		[]labPart{{"2", "src/kvsrv", "", "10m"}}},
	{"lab-03-raft", "Lab 3: Raft", "https://pdos.csail.mit.edu/6.824/labs/lab-raft.html", 65,
		[]labPart{
			{"3A", "src/raft", "3A", "10m"}, {"3B", "src/raft", "3B", "10m"},
			{"3C", "src/raft", "3C", "10m"}, {"3D", "src/raft", "3D", "15m"},
		}},
	{"lab-04-kvraft", "Lab 4: Fault-tolerant Key/Value Service", "https://pdos.csail.mit.edu/6.824/labs/lab-kvraft.html", 130,
		[]labPart{{"4A", "src/kvraft", "4A", "10m"}, {"4B", "src/kvraft", "4B", "15m"}}},
	{"lab-05-sharded", "Lab 5: Sharded Key/Value Service", "https://pdos.csail.mit.edu/6.824/labs/lab-shard.html", 190,
		[]labPart{{"5A", "src/shardctrler", "", "10m"}, {"5B", "src/shardkv", "", "20m"}}},
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func skipOrMkdir(dir string) (bool, error) {
	if _, err := os.Stat(dir); err == nil {
		fmt.Printf("skip %s (exists)\n", dir)
		return true, nil
	}
	return false, os.MkdirAll(filepath.Join(dir, "steps"), 0o755)
}

func write(path, content string) error { return os.WriteFile(path, []byte(content), 0o644) }

func writeLecture(root string, l lecture) error {
	dir := filepath.Join(root, l.slug)
	if skip, err := skipOrMkdir(dir); skip || err != nil {
		return err
	}
	must(write(filepath.Join(dir, "module.yaml"), fmt.Sprintf(
		"title: %q\nkind: lecture\norder: %d\nlinks:\n  paper: %q\n", l.title, l.order, schedule)))
	must(write(filepath.Join(dir, "steps", "01-read-the-paper.md"),
		`---
title: Read the paper
type: reading
---

Read this lecture's paper (find it on the schedule page linked above).
Full guidance for this module is not yet authored — while reading, capture
in the Notes drawer: the system's goal, its core mechanism, and one design
decision you would question.
`))
	must(write(filepath.Join(dir, "steps", "02-reading-question.md"),
		`---
title: Reading question
type: question
question: |
  Summarize the core idea of this lecture's paper in your own words, then
  name one design decision you find questionable and explain why.
---

Answer from the paper, not from summaries of it.
`))
	must(write(filepath.Join(dir, "steps", "03-wrap-up.md"),
		`---
title: Wrap-up
type: reading
---

Check you can restate the paper's main contribution from memory, then move on.
`))
	return nil
}

func writeLab(root string, lb lab) error {
	dir := filepath.Join(root, lb.slug)
	if skip, err := skipOrMkdir(dir); skip || err != nil {
		return err
	}
	must(write(filepath.Join(dir, "module.yaml"), fmt.Sprintf(
		"title: %q\nkind: lab\norder: %d\nlinks:\n  lab: %q\n", lb.title, lb.order, lb.page)))
	must(write(filepath.Join(dir, "steps", "01-overview.md"), fmt.Sprintf(
		`---
title: Overview
type: reading
---

Work through %s per the lab page linked above. Detailed guidance for this
lab is not yet authored; the submit step(s) below still snapshot your code
and run the lab tests.
`, lb.title)))
	for i, p := range lb.parts {
		runFilter := ""
		if p.runFilter != "" {
			runFilter = fmt.Sprintf(", \"-run\", %q", p.runFilter)
		}
		must(write(filepath.Join(dir, "steps", fmt.Sprintf("%02d-submit-%s.md", i+2, p.name)), fmt.Sprintf(
			`---
title: Submit part %s
type: submit
eval:
  workdir: %s
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race"%s]
  timeout: %s
---

When the part %s tests pass locally, snapshot and evaluate here.
`, p.name, p.workdir, runFilter, p.timeout, p.name)))
	}
	return nil
}

func writeProject(root string) error {
	dir := filepath.Join(root, "project")
	if skip, err := skipOrMkdir(dir); skip || err != nil {
		return err
	}
	must(write(filepath.Join(dir, "module.yaml"),
		fmt.Sprintf("title: %q\nkind: project\norder: 125\nlinks:\n  lab: %q\n",
			"Final Project", schedule)))
	must(write(filepath.Join(dir, "steps", "01-proposal.md"),
		`---
title: Proposal
type: reading
---

Pick a distributed-systems idea you can build and evaluate in the remaining
weeks. Write a one-page proposal: the problem, the design sketch, and how
you will know it works. Keep the scope honest — a small system measured well
beats a big system that almost runs.
`))
	must(write(filepath.Join(dir, "steps", "02-report.md"),
		`---
title: Build, measure, report
type: reading
---

Build it, measure it, and write the report: design, what worked, what
surprised you, and what you would do differently. Capture running notes in
the Notes drawer as you go — they become the report outline.
`))
	return nil
}

var guidanceStubs = map[string]string{
	"lab-02-kvserver": `Pitfalls to check:

- Duplicate RPC detection: retried Puts/Appends must not apply twice.
- The reply for a duplicate must match the original reply, not recompute.
- Memory: completed request records must eventually be freed.
`,
	"lab-03-raft": `Pitfalls to check:

- Figure 2 of the Raft paper is a specification, not a suggestion — check
  every rule it states, especially commitIndex advancement and log matching.
- Election timers must be reset only on: granting a vote, receiving
  AppendEntries from the current leader, or starting an election.
- Locks must not be held across RPC calls or channel sends.
- Persistent state (currentTerm, votedFor, log) must be persisted before
  replying to any RPC that changed it.
`,
	"lab-04-kvraft": `Pitfalls to check:

- Client operations must be deduplicated across leader changes.
- Reads must go through the log (or leases) — serving stale reads from a
  deposed leader is a linearizability bug.
- Snapshot installation must discard conflicting log state atomically.
`,
	"lab-05-sharded": `Pitfalls to check:

- Configuration changes must be serialized through the log; two groups
  disagreeing on config ownership loses keys.
- Shard migration must carry duplicate-detection state with the shard data.
- A group must reject keys for shards it no longer owns, even mid-migration.
`,
}

func writeGuidanceStubs(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for slug, body := range guidanceStubs {
		path := filepath.Join(dir, slug+".md")
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("skip %s (exists)\n", path)
			continue
		}
		if err := write(path, body); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	root := "content/modules"
	if _, err := os.Stat(root); err != nil {
		log.Fatalf("run from the repo root: %v", err)
	}
	for _, l := range lectures {
		must(writeLecture(root, l))
	}
	for _, lb := range labs {
		must(writeLab(root, lb))
	}
	must(writeProject(root))
	must(writeGuidanceStubs("content/guidance"))
	fmt.Println("done")
}
```

- [ ] **Step 3: Run the generator**

```bash
go run ./scripts/gen-skeleton
```

Expected: `skip content/modules/01-introduction (exists)`, `skip content/modules/lab-01-mapreduce (exists)`, `skip content/guidance/lab-01-mapreduce.md (exists)`, then `done`; 26 new module directories appear.

- [ ] **Step 4: Run the full suite**

Run: `make test` — Expected: PASS, including `TestRealContentParses` now seeing 28 modules.

- [ ] **Step 5: Sanity-check in the browser** — `make run`, open http://localhost:8080: the course map lists 22 lectures, 5 labs, and the project, ordered sensibly, with lab parts as submit steps.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: full course skeleton via generator, lab guidance stubs"
```

---

### Task 12: Docker, compose, README, final verification

**Files:**
- Create: `docker/Dockerfile`, `docker/docker-compose.yml`, `docker/.env.example`, `README.md`

**Interfaces:**
- Consumes: everything. Produces: `docker compose up` as the one-command way to run the stack, with SQLite state on a named volume that survives rebuilds.

- [ ] **Step 1: Write the Dockerfile** — `docker/Dockerfile`:

```dockerfile
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/a-h/templ/cmd/templ generate && go build -o /out/tour ./cmd/tour

# The runtime keeps the Go toolchain: the eval runner executes `go test`
# inside the mounted lab repo.
FROM golang:1.23
WORKDIR /app
COPY --from=build /out/tour /app/tour
COPY content /app/content
ENV PORT=8080 DATA_DIR=/data CONTENT_DIR=/app/content LAB_REPO_DIR=/lab \
    GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomod
EXPOSE 8080
ENTRYPOINT ["/app/tour"]
```

- [ ] **Step 2: Write compose + env example**

`docker/docker-compose.yml`:
```yaml
services:
  tour:
    build:
      context: ..
      dockerfile: docker/Dockerfile
    ports:
      - "8080:8080"
    environment:
      OPENROUTER_API_KEY: ${OPENROUTER_API_KEY:-}
      OPENROUTER_MODEL: ${OPENROUTER_MODEL:-anthropic/claude-sonnet-4}
    volumes:
      - tour-data:/data
      - ${LAB_REPO_DIR:?point LAB_REPO_DIR at your 6.824 lab repo clone}:/lab:ro

volumes:
  tour-data:
```

`docker/.env.example`:
```
# Path to your clone of the MIT 6.824 lab skeleton repo (mounted read-only at /lab)
LAB_REPO_DIR=/absolute/path/to/your/6.5840
# Optional: unlocks evaluation mode when set
OPENROUTER_API_KEY=
# Optional: override the evaluation model
OPENROUTER_MODEL=anthropic/claude-sonnet-4
```

- [ ] **Step 3: Write the README** — `README.md`:

````markdown
# MIT 6.824 Course Tour

A Go-tour-style guided UI for working through
[MIT 6.824 / 6.5840 (Distributed Systems)](https://pdos.csail.mit.edu/6.824/)
module by module — with per-step progress tracking, mid-flight note taking,
and an optional LLM evaluation mode for lab code and reading-question
answers.

This app is a companion, not a mirror: it owns the structure and original
guidance and links out to the course's papers and lab pages. No MIT course
prose is reproduced here.

## Quick start (Docker)

```bash
cd docker
cp .env.example .env    # edit: set LAB_REPO_DIR, optionally OPENROUTER_API_KEY
docker compose up --build
```

Open http://localhost:8080. Progress, notes, and submissions persist in the
`tour-data` volume across rebuilds.

## Quick start (local)

```bash
make run    # serves on :8080; state in ./data, content from ./content
```

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `DATA_DIR` | `./data` | SQLite location (`tour.db`) |
| `CONTENT_DIR` | `./content` | Course content tree |
| `LAB_REPO_DIR` | _(unset)_ | Your clone of the course lab repo; required for lab submission |
| `OPENROUTER_API_KEY` | _(unset)_ | Unlocks evaluation mode when set |
| `OPENROUTER_MODEL` | `anthropic/claude-sonnet-4` | Evaluation model |

## Evaluation mode

With an OpenRouter key set, submitting work gets you rubric-based LLM
feedback:

- **Reading questions** — your answer is scored against
  `content/rubric/question.md`.
- **Labs** — the app snapshots the relevant files from your mounted lab
  repo, runs the lab's own test command, and reviews code + test output
  against `content/rubric/lab.md` plus the per-lab guidance in
  `content/guidance/`.

Without a key everything still works — answers and code snapshots are
saved; only the LLM review is locked. Rubrics are versioned; each stored
evaluation records the rubric version it used.

## Content authoring

Each module is a directory under `content/modules/<slug>/` with a
`module.yaml` and ordered `steps/*.md` (frontmatter: `title`, `type`
`reading|question|exercise|submit`; `question:` text for question steps; an
`eval:` block with `workdir`/`globs`/`test_cmd`/`timeout` for submit steps).
Malformed content fails boot with a precise message; `make test` catches it
first. `scripts/gen-skeleton` stubs any module that doesn't exist yet and
never touches existing directories.

## Development

```bash
make test   # regenerates templ files, runs the full suite
make run    # build + serve
```

Architecture: Go + HTMX + [templ](https://templ.guide), one binary. Feature
packages (`internal/tour`, `internal/notes`, `internal/eval`) each split
into transport / endpoint / service layers; services define the interfaces
they consume, `internal/coursefs` (file-backed course), `internal/sqlite`
(user state), and `internal/openrouter` (LLM) implement them. The design
spec lives in `docs/superpowers/specs/`.
````

- [ ] **Step 4: Validate compose config**

```bash
cd docker && LAB_REPO_DIR=/tmp docker compose config -q && cd ..
```

Expected: exits 0, no output.

- [ ] **Step 5: Build the image and smoke-test it**

```bash
docker build -f docker/Dockerfile -t course-tour .
mkdir -p /tmp/fake-lab
docker run -d --name tour-smoke -p 18080:8080 -v /tmp/fake-lab:/lab:ro course-tour
sleep 2 && curl -fsS http://localhost:18080/healthz && curl -fsS http://localhost:18080/ | head -c 200
docker rm -f tour-smoke
```

Expected: `ok` from healthz and HTML containing the course map.

- [ ] **Step 6: Full suite + commit**

```bash
make test
git add -A && git commit -m "feat: docker compose stack, README"
```

---

## Done — definition of v1 complete

Every box above checked; `make test` green; `docker compose up` from
`docker/` serves the full 28-module course with progress, notes, and (with a
key) evaluation of reading questions and labs. Follow-on authoring work
(richer per-module guidance, refined paper links, soliciting course-staff
feedback on the rubric) continues in content files without code changes.
