# Skill Observation Log

Observations captured during task-oriented work. Each entry identifies a
potential improvement to a skill, rule, or workflow.

**Status key:** OPEN = not yet actioned | ACTIONED = applied | DECLINED = not pursued

---

### Observation 2: Plan example code ships real bugs — treat it as draft, not gospel

**Status:** ACTIONED (2026-08-11 — feedback memory `plan-example-code-is-draft.md` written to auto-memory)
**Date:** 2026-08-03
**Session context:** v1 course-tour build via subagent-driven development
**Target:** auto-memory (feedback memory on plan authoring), pending user decision

**Issue:** Five reviewer-confirmed defects in v1 came verbatim from the
implementation plan's own example code, not from implementer drift:
non-transactional migrations (Task 5), 500-instead-of-404 for missing notes
(Task 7), request-context cancelation stranding submissions (Task 9),
locked-mode lab retries blocked (Task 10), and a Docker base image that
could not build the module (Task 12). Implementers transcribed faithfully;
task reviewers caught every one — the "plan-mandated findings are still
findings" review rule earned its keep.
**Suggested improvement:** When writing plans with complete code, schedule
skepticism: state in the plan header that example code is a draft subject to
review, and during plan self-review specifically probe failure paths
(transactions, cancelation, restarts, guard order) rather than only type
consistency and spec coverage.
**Principle:** Complete code in a plan moves the bug-introduction site from
the implementer to the planner; review pressure has to move with it.

### Observation 3: Fix-wave subagent escaped its worktree and committed to main

**Status:** ACTIONED (2026-08-11 — covered by auto-memory `subagent-git-commit-guard.md`)
**Date:** 2026-08-03
**Session context:** interactive exercises v2, subagent-driven development; final-review fix wave (trivial README/gofmt/frontmatter edits) dispatched to a cheap-tier subagent
**Target:** auto-memory (dispatch-prompt hygiene for subagent-driven development)

**Issue:** Despite the dispatch prompt stating "Work from: <worktree path>", the fix subagent ended up operating in the repo's primary worktree (where main is checked out), committed a bogus partial replay of an earlier task to main, ran `git reset --hard` against a dirty tree to undo itself (flagged by the harness security layer), then committed the fix wave onto main instead of the feature branch. Controller had to reconstruct state from reflogs and redo the fixes by hand; main was left with a stray commit pending user-approved restore.
**Suggested improvement:** Dispatch prompts for any subagent that commits should include a hard guard: "Before ANY git commit, run `git branch --show-current` and `pwd`; if the branch is not <expected branch> or the cwd is not <expected worktree>, STOP and report BLOCKED — never checkout, reset, or cd to another worktree." Cheap-tier models especially need the mechanical guard, not just a stated working directory.
**Principle:** A stated working directory is context, not a constraint — agents that mutate git history need an explicit verify-before-commit invariant, because recovery from a wrong-branch commit costs far more than the guard.

### Observation 4: Default-change PRs must sweep every deployment surface

**Status:** ACTIONED (2026-08-11 — rule added to project CLAUDE.md)
**Date:** 2026-08-11
**Session context:** CodeAnt/CodeRabbit tool evaluation; PR #3 changed the server bind default to 127.0.0.1
**Target:** project CLAUDE.md

**Issue:** Changing the HOST default to loopback broke the documented `docker compose up` flow (Dockerfile sets PORT but not HOST; compose publishes 8080:8080 which can't reach a loopback listener). The regression shipped in the PR and was caught by CodeAnt's PR review, not by the pre-push checklist — the test suite can't see deployment configs.
**Suggested improvement:** Add a rule: when changing any config default or env-var behavior, grep all deployment surfaces (docker/, compose files, Makefile, README quick-start, CI workflows) for dependence on the old default before pushing.
**Principle:** Test suites only cover what runs in-process; defaults are a contract with every deployment artifact in the repo, and those artifacts need a manual sweep when the contract changes.

### Observation 5: Piped test commands mask exit codes and break verify-before-push

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** Fix round for external review findings on the live-run-streaming PR
**Target:** superpowers:verification-before-completion (personal skill)

**Issue:** A chained command `go test ./... 2>&1 | tail -3 && git commit && git push` pushed a commit with a failing test: the pipeline's exit status is tail's, not go test's, so the && chain proceeded past a FAIL. The failure was visible in the printed output but the automation keyed off the wrong exit code.
**Suggested improvement:** When gating commit/push on a test run, never pipe the test command in the same chain — capture with `go test ... > out 2>&1; RC=$?` and gate on $RC (or use pipefail). The skill's checklist should name exit-code masking via pipes as a known way "evidence before assertions" silently breaks.
**Principle:** Verification gates must consume the verified command's own exit status; any pipe or postprocessing between the command and the gate can convert failure into success.
