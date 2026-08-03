# Interactive Coding Exercises — Design

**Date:** 2026-08-03
**Status:** Approved
**Builds on:** the v1 course tour (see `2026-08-01-course-tour-design.md`)

## What this is

A "light IDE" layer for the course tour: at chosen points in the course,
the student is prompted with an in-browser coding exercise — fix broken
code, or implement a package against provided tests — edited in a real
code editor, checked live for syntax/vet errors, executed server-side with
`go test`, with results in a log pane. Passing tests auto-completes the
step. Deliberately a facsimile, not a full IDE: no LSP/autocomplete, no
debugger, no terminal, no file management.

## Decisions made

| Question | Decision |
|---|---|
| Editor operates on | Standalone exercise packages authored in `content/`. Real labs keep the v1 mounted-repo submit flow. (Alternatives considered: editing the mounted lab repo in-browser — rejected for v2 as it risks the student's real work tree and needs file-tree UX; the workspace model here does not preclude adding it later.) |
| Progress relation | Complete-on-pass, no gating. Navigation stays free; the course map shows honest gaps. |
| Editor file surface | Tab bar with editable file(s) plus **read-only** tabs for tests/harness — reading the test is part of the pedagogy. No create/rename/delete. |
| Student-work persistence | Drafts in SQLite (continuing the "all mutable state in SQLite" rule); every run materializes a fresh throwaway workspace. (Alternative considered: durable on-disk workspaces per exercise — rejected: splits state across disk and DB, messier reset/backup semantics.) |
| Videos | Steps may embed lecture videos inline (privacy-enhanced YouTube embed) with a plain link fallback beneath. |
| Licensing | MIT 6.824/6.5840 materials are CC BY–licensed. Exercise **code** may be adapted from course materials with attribution. Prose remains original. |

## Content model

### New step type: `code`

```yaml
---
title: "Fix: the racy counter"
type: code
code:
  mode: fix                    # fix | create — display badge only
  editable: ["counter.go"]
  readonly: ["counter_test.go"]
  run: ["go", "test", "-race", "."]
  timeout: 2m
attribution: "Adapted from MIT 6.5840 course materials (CC BY 3.0 US)"  # optional
---
(markdown body = the exercise prompt)
```

- Scaffold files live in `content/modules/<module>/exercises/<step-slug>/`.
- Loader validation (boot-fails loudly, as v1): all listed files exist in
  the scaffold dir; `editable` and `readonly` are disjoint; at least one
  editable file; non-empty `run`; parseable `timeout`.
- Workspaces are single-package, stdlib-only; the materializer generates a
  minimal `go.mod` — scaffolds never carry one.

### Attribution

- Optional `attribution:` frontmatter renders beneath the exercise.
- `content/ATTRIBUTION.md` lists every adapted source; served at
  `/attribution`, linked from the course-map footer.
- This narrows the v1 "never copy MIT course material" rule deliberately:
  CC BY–licensed *code* may be adapted with attribution; prose guidance
  stays original.

### Videos

- Any step may carry `video: <url>` frontmatter. Step pages render a
  `youtube-nocookie.com` iframe above the body with the plain link below it
  (offline/blocked-embed fallback). Lecture 1–2 video URLs authored now;
  the rest progressively.

### Exemplar exercises (authored in this phase)

1. **Lecture 2 — Fix: the racy counter** (`fix`): data race + mutex
   discipline; `go test -race` exposes it.
2. **Lecture 2 — Build: a concurrent-safe KV store** (`create`): skeleton +
   TODO, tests provided in a read-only tab.
3. **Lab 1 — Warm-up: sequential word count** (`create`): adapted from the
   course's sequential MapReduce starter, with attribution.

## Architecture

New feature package `internal/exercise` with the standard
transport/endpoint/service layering. The service defines the interfaces it
consumes:

- `CourseRepo` (as elsewhere)
- `DraftRepo` — SQLite-backed draft storage
- `SubmissionRepo` — **reused from eval's contract**; runs are submissions
  of new kind `exercise`
- `ProgressMarker` — same seam eval uses; passing runs auto-complete
- `Workspace` — materialize scaffold + draft overlay + generated `go.mod`
  into a fresh temp dir; execute the step's `run` command; report output
  and pass/fail

### Shared exec hardening

The hardened subprocess logic in the v1 lab runner (context timeout,
`WaitDelay`, process-group SIGKILL, 64KB tail truncation) moves to a shared
`internal/execx` package consumed by both the lab runner and the exercise
workspace. Behavior-preserving refactor with tests.

### Routes

| Route | Behavior |
|---|---|
| `GET /exercises/{module}/{step}` | Editor section partial (lazy-loaded from the step page, like the eval section) |
| `PUT /exercises/{module}/{step}/draft` | Autosave editable files (client debounce ~800ms) |
| `POST /exercises/{module}/{step}/check` | `gofmt -e` + `go vet` on a materialized draft; returns line-mapped diagnostics (client debounce ~1.5s) |
| `POST /exercises/{module}/{step}/run` | Async run through the existing submission/polling machinery (1s poll). Exit-0 marks the step complete. Completion is sticky: a later failing run never revokes it (matching how answered questions behave). |

### Editor (client)

- CodeMirror 6 with Go language support, vendored as a prebuilt ESM bundle
  committed under `static/codemirror/` (built once with esbuild; the
  regeneration recipe documented in the README — the normal build has no
  node dependency).
- `static/exercise.js` (~100 lines, the only bespoke client JS): tab bar
  (read-only tabs locked), autosave + check debouncing, lint-gutter
  markers from check diagnostics, Run via htmx, Cmd+Enter to run.
- Everything else remains server-rendered HTMX.

### LLM feedback (final, cuttable slice)

When evaluation mode is unlocked, completed runs show a "Get feedback"
button: prompt assembled from the exercise prompt + draft files + test
output against a new `content/rubric/exercise.md`; stored as an evaluation
on the run's submission via the existing pipeline.

## Data model

Migration `002`:

- New table `drafts (module_slug, step_slug, files_json, updated_at,
  PRIMARY KEY (module_slug, step_slug))` — editable files only. Reset =
  delete row.
- `submissions.kind` CHECK gains `'exercise'` — in SQLite this recreates
  the table and copies rows; covered by an upgrade test (v1 data intact,
  new kind accepted).

Runs keep the full materialized file set in `submissions.content`
(reproducibility, as v1). Interrupted-run recovery at boot is inherited.

## UX flow

Code step page: prompt → tab bar → editor → Run / Reset / status →
collapsible log pane. No draft ⇒ scaffold shown. Pass ⇒ green banner,
auto-complete, optional feedback button. Fail ⇒ log pane opens on the
failure output. Mobile: read/run/log work; editing is desktop-first by
intent.

## Error handling

- Check endpoint returns diagnostics for unparseable code — that is its
  purpose, never a 500.
- Run failures land on the submission row with captured output (v1
  invariant), retry included.
- Draft-save failures surface an error banner; the client retries on next
  change — keystrokes are never silently dropped.
- Malformed exercise content fails boot with the offending path.

## Testing

- TDD throughout. Unit: draft repo, workspace materializer (golden
  temp-dir layout), diagnostics parser against real `gofmt -e`/`go vet`
  output, `execx` behavior tests (timeout, group-kill, truncation).
- Migration upgrade test: populated v1 schema → migrate → intact + new
  kind usable.
- e2e (dedicated `e2e/` package, real Go toolchain, trivial fixture
  exercise): load editor section → save draft → run → poll to pass →
  step auto-completed; a failing-run case; a diagnostics case; video-embed
  render check.
- Exemplar exercises guarded by the real-content parse test.

## Out of scope

- Editing the mounted lab repo in the browser (workspace model leaves room).
- LSP/gopls, autocomplete, debugger, terminal, file create/delete.
- Multi-user; hosting. Running student code stays acceptable because the
  tool is single-user and local — revisit before any hosted deployment.
- Docker image changes (Go toolchain already present).
