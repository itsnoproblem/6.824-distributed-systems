# Code review tool evaluation

This repo doubles as an evaluation ground for automated code-review tools.
Every PR is an eval sample. This scorecard records each finding, whether it
held up under verification, and qualitative notes on reviewer behavior, so a
keep/drop decision can be made from evidence rather than impressions.

## Tools under evaluation

| Tool | Connected | Surfaces |
|---|---|---|
| CodeAnt AI | 2026-08-11 | Repo-scan SAST (files issues), PR review (inline comments) |
| CodeRabbit | 2026-08-12 | PR review (inline comments + summary), thread follow-up, per-repo learnings |

## Scoring

Each finding gets a verdict after manual verification against the code:

- **TP** — real defect; fixing it improved the codebase
- **TP-minor** — technically correct but low impact in this deployment context
- **FP** — incorrect for this codebase's actual data flow / threat model

Precision = (TP + TP-minor) / (TP + TP-minor + FP): a TP-minor is still a
correct finding, so it counts in the numerator.

## Findings log

| Date | Sample | Tool | Finding | Verdict | Notes |
|---|---|---|---|---|---|
| 2026-08-11 | repo scan → issue #1 | CodeAnt | Code injection via `exec.Command` in `internal/execx` (CWE-94) | FP | Command args come from repo-authored YAML, never HTTP input; running learner code locally is the app's core feature. Tool self-labeled LOW confidence. Closed with threat-model comment. |
| 2026-08-11 | repo scan → issue #2 | CodeAnt | No read timeout on HTTP server, Slowloris (CWE-400) | TP-minor | Real gap, low exposure for a local app. Fixed in PR #3 with `ReadHeaderTimeout`; the same PR also hardened the default bind to loopback. |
| 2026-08-12 | PR #3 round 1 | CodeAnt | Loopback bind default breaks documented `docker compose up` (Dockerfile sets `PORT` but not `HOST`) | TP | Genuine regression introduced by the PR; missed by author and test suite. Severity "Major" was fair. |
| 2026-08-12 | PR #3 round 1 | CodeRabbit | Same Docker/`HOST` regression, found independently | TP | Convergence with CodeAnt on the highest-impact finding. |
| 2026-08-12 | PR #3 round 1 | CodeRabbit | `ReadHeaderTimeout` doesn't bound request-body reads; handlers consume bodies via `FormValue`/`json.NewDecoder` | TP | Traced actual body-reading call sites with its own repo scripts before flagging. Fixed with `ReadTimeout` (bounds body reads); `IdleTimeout` added alongside for idle keep-alive connections. `WriteTimeout` deliberately omitted (long-running submission responses) and the pushback was accepted. |

| 2026-08-12 | PR #4 (docs-only) | CodeAnt | No findings | — | Stayed silent on a diff with nothing to find — the desired noise behavior. |
| 2026-08-12 | PR #4 (docs-only) | CodeRabbit | Precision formula doesn't state how TP-minor counts | TP-minor | Fair reproducibility point for an eval doc; formula added. |
| 2026-08-12 | PR #4 (docs-only) | CodeRabbit | Findings log conflated which timeout fixed what (`IdleTimeout` isn't body-read protection) | TP-minor | Correct nit; attributions tightened. Ran repo scripts to cross-check the doc's claims against `cmd/tour/main.go`. |

## Running tally

| Tool | TP | TP-minor | FP | Precision |
|---|---|---|---|---|
| CodeAnt | 1 | 1 | 1 | 2/3 |
| CodeRabbit | 2 | 2 | 0 | 4/4 |

Sample size is small; treat the tally as directional until ~10+ findings per tool.

## Qualitative notes

**Latency (PR #3):** both tools started within ~15s of the PR opening.
CodeRabbit posted its verdict in ~5 min, CodeAnt in ~7 min. Re-review after a
push: both re-ran automatically.

**Review depth:** CodeRabbit executes its own analysis scripts (`rg` sweeps of
the repo) and shows the chain, which caught the body-read gap a pattern match
alone would miss. CodeAnt's PR review produced a single targeted finding —
less depth, but it was first to the highest-impact one.

**Interaction loop:** CodeRabbit re-verifies fixes against the new commit,
replies on threads, explicitly resolves them, and persists per-repo
"learnings" (e.g. the `WriteTimeout` rationale) so accepted pushback isn't
re-litigated on future PRs. CodeAnt re-reviews silently — clean result, but no
thread closure or acknowledgment.

**Repo-scan SAST (CodeAnt only):** pattern-match based, not taint analysis;
the FP was honestly labeled LOW confidence. HIGH-severity/LOW-confidence
issues on a codebase whose core feature is subprocess execution suggest a
scoped rule suppression (`dangerous-exec-command` for `internal/execx/`) to
keep the signal usable.

## Open questions

- Whether CodeAnt supports scoped rule suppression via repo config, and
  whether its repo-scan issues stay in sync as findings are fixed.
- Behavior on a larger feature PR (multi-file, new endpoints) — depth vs.
  noise at scale.
