# Course Tour v1 — retro and deferred items

**Session span:** 2026-08-01 → 2026-08-03
**Merged:** `feature/course-tour-v1` → `main` (fast-forward, 23 commits)

## What shipped

- Full v1 of the guided course tour: 28-module content skeleton (Lecture 1
  and Lab 1 fully authored), Go-tour-style navigation with per-step
  progress, notes drawer + grouped index, evaluation mode (locked and
  unlocked) for reading questions and labs, async lab pipeline with polling.
- Deployment: docker compose with `restart: unless-stopped` and a persisted
  `tour-data` volume, exposed on the tailnet via `tailscale serve`
  (https://macbook-pro.tail05a42a.ts.net/).
- Post-merge additions on main: Beej's Guide reading step in Lecture 2, the
  v2 interactive-exercises spec and 8-task implementation plan.

## What surprised

- **The plan's own example code carried the bugs.** Every Important review
  finding across twelve tasks traced to plan-authored code, not implementer
  drift (see Observation 2 in the observations log). The task-review gate
  that refuses to grade plan-mandated code leniently is what caught them.
- **The fault-tolerance app needed fault tolerance first.** A dev server
  died twice (laptop sleep) behind a live tailscale proxy — 502s with a
  healthy network. The durable fixes (boot-time recovery of stranded
  `running` submissions, process-group SIGKILL + `WaitDelay` for hung test
  binaries) both came out of the final whole-branch review, not the plan.
- **Toolchain pinning bites quietly:** the plan's `golang:1.23` base image
  could not build a `go 1.25` module because official Go images set
  `GOTOOLCHAIN=local`. Caught at build time by the Task 12 implementer.
- **Dark-mode rendering:** the stylesheet set text color but no body
  background; dark-preferring browsers rendered dark-on-dark. Found only by
  looking at the actual page in a dark-scheme browser.

## Deferred (concrete, from the review ledger)

Fast-follows, none blocking:

1. e2e test exercising the real async polling path (all current tests force
   the synchronous runner; `GET /submissions/{id}/section` has no coverage).
2. `SubmitLab` double-submit guard (rapid double-POST races two test runs).
3. Lab snapshot globs include `*_test.go` (~70KB of course harness per Raft
   submission) — exclude test files or document the token cost.
4. `coursefs` module sort is unstable with duplicate `order` values; use
   `sort.SliceStable` or a slug tiebreaker, and consider enforcing
   order-uniqueness at load.
5. `RefForSubmission`/`Retry` map any repo error to 404, masking DB errors.
6. Note PUT/DELETE routes covered only at service level; note timestamps
   render UTC inside the container (`.Local()` no-op).
7. Eval section renders `LabSection` for any non-question step type reached
   by hand-crafted URL; add an explicit type check.
8. **User decision pending:** 500 bodies include `err.Error()` (deliberate
   for solo-local debugging) — genericize before any non-local deployment.
9. Lecture videos: only Lecture 1's ID is verified/embedded; remaining
   lectures need IDs checked against the official playlist (ongoing content
   authoring, applies after v2 lands).

## What we'd do differently

- Put restart/crash recovery in the spec whenever a design includes async
  background work — "what happens to in-flight state when the process
  dies" is a spec question, not a review discovery.
- Treat plan example code as a draft: self-review plans for failure paths
  (transactions, cancelation, guard ordering), not just type consistency.
