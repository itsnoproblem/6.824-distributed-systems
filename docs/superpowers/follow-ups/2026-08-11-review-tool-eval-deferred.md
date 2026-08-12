# 2026-08-11 — Review-tool evaluation kickoff: retro & deferred

## What shipped

- **PR #3** (merged) — resolution of both CodeAnt repo-scan issues:
  issue #1 (exec.Command "injection") closed as by-design with a
  threat-model comment; issue #2 (Slowloris) fixed with
  `ReadHeaderTimeout`, plus `ReadTimeout`/`IdleTimeout` from review
  round 2, and a loopback bind default with a new `HOST` env var
  (`0.0.0.0` in the Docker image; Cloud Run would set it the same way).
- **PR #4** (merged) — `docs/tool-eval.md`, the review-tool scorecard,
  seeded with four samples: two repo-scan issues and two PR review
  rounds. Running tally at close: CodeRabbit 4/4 precision, CodeAnt 2/3.

## What surprised

- **The loopback-bind hardening broke `docker compose up`** — the
  Dockerfile sets `PORT` but didn't set `HOST`, so the published port
  would have forwarded to a listener that wasn't there. The Go test
  suite cannot see this class of regression; both review bots caught it
  independently within minutes. Fixed and proven end-to-end (container
  boot + `/healthz` through the published port). Logged as
  Observation 4.
- **CodeRabbit reviews docs prose, correctly.** On the docs-only PR it
  cross-checked the scorecard's claims against `cmd/tour/main.go` with
  its own scripts and caught a wrong timeout attribution. Not noise —
  but worth knowing it will never be fully silent.
- **Observation 4 rode along inside the PR #3 fix commit** — `git add
  -A` swept the observation log into a security-fix commit. Harmless
  here, but it's scope smudge; stage deliberately when unrelated tracked
  files are dirty.

## What's deferred

- **CodeAnt scoped rule suppression** — confirm whether repo config
  (`.codeant.yml` or dashboard) can suppress `dangerous-exec-command`
  for `internal/execx/` only; without it, every future PR touching that
  package may re-trigger the known FP.
- **Eval round on a real feature PR** — the scorecard's biggest open
  question is depth-vs-noise on a multi-file feature diff. The next
  feature branch should ship as a normal PR and get logged as samples.
- **Hosted deployment** — `HOST=0.0.0.0` leaves the door open for Cloud
  Run, but no deploy config exists yet. When it lands, it needs the
  same deployment-surface sweep that bit PR #3.
- **Remote branch cleanup** — `claude/codeant-review-issues-60d566` and
  `docs/tool-eval-scorecard` are merged but not deleted on origin.

## What we'd do differently

- Sweep every deployment surface (docker/, compose, Makefile, README
  quick-start, CI) before pushing any config-default change — the
  test suite only covers what runs in-process.
- Stage files explicitly instead of `git add -A` when the tree carries
  unrelated tracked edits (observation log, docs).
