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
| 2026-08-12 | PR #6 (feature: live run streaming) | CodeAnt | e2e SSE reader has no deadline despite its comment claiming one; a streaming regression hangs the suite instead of failing it | TP | Internal review had noted the missing deadline as minor convention; the comment/behavior mismatch tipped it. Fixed with request-context deadlines. |
| 2026-08-12 | PR #6 | CodeAnt | Cancel/completion race: cancel handler can retain the run pointer past `Finish` and rewrite a successful run as canceled | TP | Converged with CodeRabbit on the PR's highest-impact defect. Fixed: `Cancel` rejects after finish. |
| 2026-08-12 | PR #6 | CodeRabbit | Same cancel-after-finish TOCTOU, surfaced at two layers (service + `runstream.Run`) with a concrete `Cancel() bool` fix proposal | TP | The proposed fix was adopted nearly verbatim. Internal review had found this race and triaged it accept as a "few-instruction window"; the tools' pointer-retention framing showed the window was wider than assessed. |
| 2026-08-12 | PR #6 | CodeRabbit | `Finish` emitted the `done` event before terminal status persisted — a watcher's refetch could read stale `running` | TP | Distinct from the TOCTOU and missed by all internal reviews. Fixed: persist-before-done, asserted by e2e. |
| 2026-08-12 | PR #6 | CodeRabbit | Spec doc's transport section drifted from implementation (route shapes, done payload, 404 behavior) | TP-minor | Only reviewer (internal or external) to cross-read the spec against the code. Spec corrected. |
| 2026-08-12 | PR #6 | CodeRabbit | e2e cancel test could fire before the process started, weakening what it proved | TP-minor | Fixed: cancel now triggers only after observed output. |
| 2026-08-12 | PR #6 | CodeRabbit | SSE handler ignored response write errors and kept heartbeating a broken connection | TP-minor | Fixed; low impact for a local app but correct. |
| 2026-08-12 | PR #6 | CodeRabbit | JS cancel button stayed disabled on HTTP 4xx/5xx (only transport failure was handled) | TP-minor | Fixed. Caught the gap left by a same-day polish fix that only covered rejections. |
| 2026-08-12 | PR #6 | CodeRabbit | errcheck: unchecked `resp.Body.Close()` in new test files | FP | Linter-sourced. Repo runs no errcheck; every existing test file closes bodies bare; a Close error on a read body carries no actionable signal. Declined for convention consistency — defensible as TP-minor under a stricter lint posture. |

## Running tally

| Tool | TP | TP-minor | FP | Precision |
|---|---|---|---|---|
| CodeAnt | 3 | 1 | 1 | 4/5 |
| CodeRabbit | 4 | 6 | 1 | 10/11 |

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

**Scale (PR #6, ~15 commits / 40+ files incl. a new concurrency package):**
this answers the depth-vs-noise-at-scale question. Both tools completed
within ~4 minutes of the PR opening. CodeAnt stayed low-volume and
high-severity: two findings, both real, no noise — but also no coverage of
docs, tests-as-spec, or frontend. CodeRabbit went broad (7 actionable): it
converged with CodeAnt on the highest-impact defect and supplied the
adoptable fix, was the only reviewer — human, agent, or tool — to cross-read
the design doc against the code, and its one noise finding was
linter-sourced rather than model judgment. Notably it surfaced (but did not
raise as a finding) an ast-grep SSRF warning on test code — visible tool
output, suppressed as a non-issue, which is exactly the desired triage
behavior.

**External vs. internal review (PR #6):** the branch was built with a
per-task internal review gate plus a final whole-branch review, which had
already found the cancel/completion race and triaged it accept as a
few-instruction window. Both external tools independently rated it Major,
and the pointer-retention (TOCTOU) framing showed the accepted window was
wider than assessed — the verdict flipped to fix. Two further defects
(done-before-persist ordering, spec/code drift) were missed by every
internal pass. Lesson: internal review triage decisions on concurrency
windows deserve adversarial re-derivation, not acceptance by consensus.

## Open questions

- Whether CodeAnt supports scoped rule suppression via repo config, and
  whether its repo-scan issues stay in sync as findings are fixed.
- CodeRabbit thread behavior on a reasoned decline (the errcheck finding):
  does it resolve, re-litigate, or persist a learning?
