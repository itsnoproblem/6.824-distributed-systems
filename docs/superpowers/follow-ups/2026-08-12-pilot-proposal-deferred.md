# 2026-08-12 — AI code-review pilot proposal: retro & deferred

## What shipped

- An exec-facing proposal memo for a two-quarter pilot of AI code review
  (CodeRabbit + CodeAnt side by side, 2-3 repos, ~10 developers), built
  directly from the evidence in `docs/tool-eval.md`: the precision tally,
  the Docker/`HOST` regression both bots caught, and CodeRabbit's
  persisted-learnings behavior. Delivered off-repo and shared at the
  workplace; intentionally deleted from the repo afterward, so no copy
  is committed here. This retro is the record that it existed.

## What surprised

- Nothing structural. One friction point: current per-seat pricing for
  both vendors was only findable via third-party aggregators (CodeRabbit
  Pro $24-30/dev/mo, CodeAnt $12-25/dev/mo depending on tier); the memo
  flags that vendor quotes must confirm the numbers.
- The eval scorecard's discipline (verdict per finding, verified before
  fixing, qualitative loop notes) converted into pitch evidence with no
  rework. The logging habit paid for itself in a context it wasn't
  designed for.

## What's deferred

- Round-3 eval work is unchanged and already teed up in
  `docs/superpowers/prompts/2026-08-11-continue-review-tool-eval.md`;
  the multi-file feature PR it calls for is now in flight as PR #6
  (`claude/code-review-tool-feature-85e7e6`, worktree
  `youtube-video-download-275e24`). Its bot review rounds are the next
  eval samples.
- CodeAnt scoped rule suppression question (from the same prompt) is
  still open.
- Tools worth a phase-2 look if the pilot advances, noted during
  research: Greptile (org-wide codebase context, closest to the
  long-term vision), GitHub Copilot code review (bundled baseline any
  paid tool must beat), Qodo Merge (self-hostable), Amazon Q Developer
  (possibly already bundled for an AWS shop).

## What we'd do differently

- Nothing significant for a session this size. Pricing research should
  start at vendor pages and fall back to aggregators, not the reverse.
