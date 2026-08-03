# Skill Observation Log

Observations captured during task-oriented work. Each entry identifies a
potential improvement to a skill, rule, or workflow.

**Status key:** OPEN = not yet actioned | ACTIONED = applied | DECLINED = not pursued

---

### Observation 1: Committed docs must be self-contained and name-free

**Status:** ACTIONED
**Date:** 2026-08-01
**Session context:** Writing the course-tour design spec during project brainstorming
**Target:** auto-memory (written: doc-hygiene-no-names-no-orphan-labels.md)

**Issue:** The first version of the design spec included the user's personal
name ("brainstorming session with…", audience row) and a conversation-scoped
label ("Approach B") whose alternatives were never described in the doc
itself. The user corrected both in one message.
**Suggested improvement:** Before committing any doc, sweep for personal
names and for labels/references that only made sense in the conversation
that produced the doc; either describe choices self-containedly or summarize
the rejected alternatives inline.
**Principle:** Docs outlive the session that wrote them — a reader has the
repo, not the chat.

### Observation 2: Plan example code ships real bugs — treat it as draft, not gospel

**Status:** OPEN
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

**Status:** OPEN
**Date:** 2026-08-03
**Session context:** interactive exercises v2, subagent-driven development; final-review fix wave (trivial README/gofmt/frontmatter edits) dispatched to a cheap-tier subagent
**Target:** auto-memory (dispatch-prompt hygiene for subagent-driven development)

**Issue:** Despite the dispatch prompt stating "Work from: <worktree path>", the fix subagent ended up operating in the repo's primary worktree (where main is checked out), committed a bogus partial replay of an earlier task to main, ran `git reset --hard` against a dirty tree to undo itself (flagged by the harness security layer), then committed the fix wave onto main instead of the feature branch. Controller had to reconstruct state from reflogs and redo the fixes by hand; main was left with a stray commit pending user-approved restore.
**Suggested improvement:** Dispatch prompts for any subagent that commits should include a hard guard: "Before ANY git commit, run `git branch --show-current` and `pwd`; if the branch is not <expected branch> or the cwd is not <expected worktree>, STOP and report BLOCKED — never checkout, reset, or cd to another worktree." Cheap-tier models especially need the mechanical guard, not just a stated working directory.
**Principle:** A stated working directory is context, not a constraint — agents that mutate git history need an explicit verify-before-commit invariant, because recovery from a wrong-branch commit costs far more than the guard.
