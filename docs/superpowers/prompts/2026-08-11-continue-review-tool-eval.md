# Kickoff: continue the review-tool evaluation (round 3)

The repo is an evaluation ground for automated code-review tools —
scorecard and method live in `docs/tool-eval.md`. Rounds 1–2 (CodeAnt
repo-scan issues, PR #3 security fix, PR #4 docs-only noise test) are
logged there; the running tally (CodeRabbit 4/4, CodeAnt 2/3) was
measured as of main @ 24316d0, when both tools were connected and PR #4
merged. Retro: `docs/superpowers/follow-ups/2026-08-11-review-tool-eval-deferred.md`.

Repo: /Users/martymulligan/go-code/src/github.com/itsnoproblem/mit-distributed-systems (branch: main, all tests green)

Next round needs a real multi-file feature PR — the scorecard's biggest
open question is depth-vs-noise at scale. Pick the next feature with the
operator, build it normally (TDD, ship-pr), and treat the PR's review
rounds as eval samples: verify every bot finding against the code before
fixing, reply on each thread, and append verdict rows + tally updates to
`docs/tool-eval.md` in the same PR.

Also close out when convenient:

- CodeAnt scoped rule suppression: can repo config suppress
  `dangerous-exec-command` for `internal/execx/` only? Without it the
  known FP may re-fire on any PR touching that package.
- Merged-but-undeleted remote branches:
  `claude/codeant-review-issues-60d566`, `docs/tool-eval-scorecard`.

Lessons that must carry forward (earned in round 2):

- Any config-default change gets a deployment-surface sweep before push
  (docker/, compose, Makefile, README quick-start, CI) — the test suite
  only sees in-process behavior; both bots caught a `HOST` regression
  the suite could not.
- Stage files explicitly; `git add -A` swept the observation log into a
  security-fix commit last time.
- CodeRabbit cross-checks docs claims against code and persists per-repo
  learnings from accepted pushback; CodeAnt re-reviews silently and its
  repo-scan issues carry honest confidence labels — weight LOW
  confidence accordingly before acting.
