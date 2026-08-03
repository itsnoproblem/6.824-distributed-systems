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
