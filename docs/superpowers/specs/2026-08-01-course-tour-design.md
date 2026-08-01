# MIT 6.824 Course Tour — Design

**Date:** 2026-08-01
**Status:** Approved

## What this is

A guided, Go-tour-style web UI that walks a student through MIT 6.824 /
6.5840 (Distributed Systems) module by module, with mid-flight note taking,
per-step progress tracking, and an optional LLM "evaluation mode" for lab
code and reading-question answers.

## Decisions made

| Question | Decision |
|---|---|
| Audience | Single user, running locally. No auth. |
| Content | Companion frame: the app owns structure and original guidance text; papers/lab pages open via links. MIT prose is never copied. |
| Lab submissions | The student's 6.824 lab repo is Docker-mounted read-only; the app snapshots code and runs the lab's own `go test`. |
| Evaluation scope | Labs and reading questions. Final project excluded (v1). |
| Architecture | Content as files in the repo, SQLite for user state only, single-shot rubric evaluation via OpenRouter. (Chosen over two alternatives: syncing content into the DB at boot, and an agentic tool-calling evaluator — the latter remains a possible later upgrade.) |
| Database | SQLite on a named Docker volume. |

## Architecture

Single Go module, single binary, single container. Go + HTMX, server-rendered
with [templ](https://templ.guide), client JS only where HTMX can't reach.

### Layering (per feature package)

- **transport.go** — routes, request decoders, HTMX/HTML response shaping.
- **endpoint.go** — request/response models, validation,
  `makeXEndpoint(svc XService) api.Endpoint` factories.
- **service.go** — business logic. Defines the repository/provider
  interfaces it consumes; implementations are injected in `cmd/tour/main.go`.

Dependencies are defined where they are consumed, not where they are
implemented.

### Layout

```
mit-distributed-systems/
├── cmd/tour/main.go        # wiring only: config, db, repos, services, routes
├── pkg/api/                # shared Endpoint type, decode/render helpers
├── internal/
│   ├── tour/               # course browsing + progress
│   ├── notes/              # note CRUD + grouped views
│   ├── eval/               # submissions, test runner, rubric evaluation
│   ├── coursefs/           # file-backed CourseRepository (parses content/ at boot)
│   └── sqlite/             # SQLite implementations of Progress/Notes/Submission repos
├── content/
│   ├── modules/            # e.g. 06-raft-1/module.yaml + steps/*.md
│   ├── guidance/           # per-lab evaluator guidance ("skills")
│   └── rubric/             # versioned evaluation rubric
├── templates/              # templ components
├── migrations/             # embedded SQL migrations
└── docker/                 # Dockerfile + compose
```

### Configuration (env)

`PORT`, `DATA_DIR`, `LAB_REPO_DIR`, `OPENROUTER_API_KEY` (optional),
`OPENROUTER_MODEL` (optional). Presence of the key unlocks evaluation mode.

### Docker

The stack is defined in `docker/docker-compose.yml`; `docker compose up`
is the one command to bring it up.

- Image is Go-based (not scratch/distroless): the eval runner executes the
  lab's own `go test` inside the container.
- The SQLite database lives on a **named volume declared in the compose
  file**, so state persists across image rebuilds and container recreation.
- The lab repo is bind-mounted read-only; its host path is set via env
  (`LAB_REPO_DIR`) or a compose `.env` file.

## Course content model

One module per course unit — 22 lectures, 5 labs, final project — each a
directory under `content/modules/`:

- `module.yaml` — slug, title, kind (`lecture|lab|project`), order, external
  links (paper PDF, lab page, lecture video).
- `steps/*.md` — ordered Markdown files with frontmatter (`slug`, `title`,
  `type`).

### Step types

| Type | Behavior |
|---|---|
| `reading` | Original guidance on the paper (what to focus on) + link out. |
| `question` | The lecture's reading question + answer box. Answers always saved; evaluated when eval mode is on. |
| `exercise` | Lab-part instructions summary + checklist. Frontmatter carries eval metadata: file globs to snapshot from the mounted repo, `go test` command, timeout. |
| `submit` | The "run evaluation" step for a lab part. |

Typical lecture module: reading → question → wrap-up. Typical lab module:
overview → (exercise + submit) per part (e.g. Lab 3 has 3A–3D).

**Authoring expectation:** v1 ships the complete module/step skeleton for the
whole schedule (every lecture, lab part, and the project map), but rich
guidance text is authored progressively — early modules (MapReduce through
Raft) get full treatment first; later modules may initially carry only the
question, links, and a brief focus note.

Content is parsed by `coursefs` at boot into an immutable in-memory course
behind the `CourseRepository` interface. Malformed content (broken
frontmatter, duplicate slugs, gaps in ordering) fails boot loudly — content
bugs must never 500 at browse time.

## Tour UX

- **Shell** (Go-tour skeleton): persistent top bar with module title, step
  position, progress; content pane; prev/next navigation at the bottom.
- **Notes drawer** toggleable on every step: open, jot, close, keep moving.
  A note automatically records the module/step it was taken on.
- **Course map** landing page: modules grouped lecture/lab/project, per-module
  progress bars, overall %.
- **Notes page**: all notes grouped by module, filterable by module.
  (Full-text search deliberately out of v1.)

## Progress

- Per-step completion: manual "mark complete" on reading steps; automatic on
  saving a question answer or running a lab evaluation.
- Module and course progress are derived, never stored.

## Data model (SQLite)

```
progress     (step_slug PK, completed_at)
notes        (id, module_slug, step_slug, body_md, created_at, updated_at)
submissions  (id, step_slug, kind lab|question, content, test_output,
              status pending|running|complete|failed, created_at)
evaluations  (id, submission_id, model, rubric_version, verdict_json, created_at)
```

Course content never enters the DB; step/module slugs are the foreign keys
into the file-backed course. Embedded SQL migrations run at boot.

## Evaluation mode

**Invariant: the submission is stored before evaluation runs.** A failed
evaluation never loses work; retry re-evaluates an existing submission.

### Lab flow (async — Raft tests run minutes)

1. Snapshot files matching the step's globs from the mounted repo into the
   submission record.
2. Run the step's `go test` command (with `-race`) under a timeout; capture
   output.
3. Compose one structured OpenRouter call:
   rubric + per-lab guidance + code + test summary.
4. Store the verdict; the step page polls via HTMX while status is `running`.

### Reading-question flow

Same pipeline minus the test run; fast enough to feel synchronous.

### Prompt assembly (the "agent guidance" layer)

- `content/rubric/` — generic criteria: correctness vs. tests, concurrency
  discipline, protocol fidelity to the paper, code clarity. Each criterion is
  scored with required written justification.
- `content/guidance/<lab>.md` — per-lab pitfalls (e.g. Raft Figure 2
  compliance, election-timer reset bugs).
- Verdict is a JSON schema: per-criterion score, feedback, suggested next
  steps. Rendered as a report on the step.

Rubric files carry a version string stored with each evaluation, so rubric
edits never rewrite history.

**Follow-up (out of app):** solicit 6.824 course staff feedback on the
rubric once drafted.

### Locked mode

No `OPENROUTER_API_KEY` ⇒ submit UI still saves answers/snapshots but shows
evaluation as locked.

## Error handling

- Test runner failures (timeout, compile error) mark the submission `failed`
  with captured output shown in the UI — findings, not crashes.
- OpenRouter errors keep the submission and offer a retry button.
- Content parse errors fail boot with a precise message.

## Testing (TDD throughout)

- Unit tests per service, with fakes for the interfaces each service defines.
- OpenRouter provider: contract test against recorded responses + an
  env-gated live smoke test.
- `coursefs`: golden-file parse tests over the real `content/` tree.
- Integration: a dedicated `e2e/` package (mirroring the convention of the
  reference codebase) runs httptest servers against a real temp-file SQLite
  database, exercising browse → note → complete → submit → evaluate end to
  end with a fake LLM provider.

## Out of scope (v1)

- Multi-user, auth, hosting.
- Final-project evaluation.
- Exam-prep modules (mid-term/final practice materials).
- Full-text note search.
- Agentic (tool-calling) evaluator — the provider interface is the seam for
  upgrading to this later.
- Scraping/mirroring MIT content.
