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
