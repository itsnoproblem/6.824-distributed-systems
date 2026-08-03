# Kickoff: implement interactive coding exercises (v2)

Implement the interactive coding exercises feature (v2) for the MIT 6.824
course tour.

Repo: /Users/martymulligan/go-code/src/github.com/itsnoproblem/mit-distributed-systems (branch: main, all tests green)
Approved spec: docs/superpowers/specs/2026-08-03-interactive-exercises-design.md
Complete plan (8 tasks, full code in every step): docs/superpowers/plans/2026-08-03-interactive-exercises.md

😎 Execute the plan with superpowers:subagent-driven-development:

- Work on branch `feature/interactive-exercises` off main. The branch may
  already exist from an earlier kickoff (a worktree was spawned at plan
  commit 82f61b8) — reuse it if so; never run two sessions in one worktree.
- Fresh implementer subagent per task; task-scoped review (spec compliance +
  quality) after each; fix + re-review loop on Critical/Important findings;
  keep the ledger in .superpowers/sdd/progress.md; final whole-branch review
  on the most capable model when all tasks pass.
- TDD per the plan's steps; run `make test` before every commit; commits end
  with the trailer: Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
- Plan-mandated findings from reviewers: fix them when the fix serves the
  spec's stated intent; batch genuine design contradictions for the user
  instead of interrupting per-task. Lesson from v1 (retro
  2026-08-03-course-tour-v1-deferred.md): every Important finding came from
  the plan's own example code — review failure paths skeptically.
- Environment notes: the Docker container owns port 8080 (leave it running);
  the dev server uses autoPort via .claude/launch.json; integration tests
  live in the e2e/ package; Task 5 needs npm/npx once to build the vendored
  CodeMirror bundle.
- When the final review is clean, STOP and present the merge options — do
  not merge without the user.
