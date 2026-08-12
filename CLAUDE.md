# Project rules

## Config-default changes sweep every deployment surface

Before pushing any change to a config default or env-var behavior, grep
every deployment surface for dependence on the old value: `docker/`
(Dockerfile, compose), `Makefile`, README quick-start, and CI workflows.
The Go test suite only covers what runs in-process; deployment artifacts
are a contract with defaults and break silently. (A loopback-bind
default change broke `docker compose up` in PR #3 — caught by review
bots, not tests.)

## Review-tool evaluation

This repo doubles as an eval ground for automated code-review tools.
Every PR's bot findings are eval samples: verify each against the code
before fixing, reply on the thread with the resolution, and log verdict
rows + tally updates in `docs/tool-eval.md`.

## Observation log

The project keeps a task-observer log at
`docs/superpowers/observations/log.md`.
