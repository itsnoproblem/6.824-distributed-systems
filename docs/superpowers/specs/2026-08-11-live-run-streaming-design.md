# Live run streaming + cancellation — design

Date: 2026-08-11
Status: approved

## Problem

Lab submissions and interactive-exercise runs already execute asynchronously
(background goroutine, status polling), but their test output is buffered by
`execx` via `CombinedOutput` and becomes visible only when the run finishes.
MIT lab test suites can run for minutes; the user stares at a spinner, then
receives a wall of text. There is also no way to cancel an in-flight run —
a mistaken submission occupies the runner until its timeout expires.

## Goals

- Stream test output to the browser live, line by line, for both lab
  submissions and exercise runs.
- Allow canceling an in-flight run from the UI.
- Preserve every existing downstream behavior: final output persisted to
  SQLite, LLM evaluation pipeline, retry semantics, `RecoverInterrupted`.

## Non-goals

- Persisting stream chunks to SQLite. Runs do not survive a server restart
  (boot-time `RecoverInterrupted` already fails orphans); chunk persistence
  would add write churn for no recovery benefit in a single-user local app.
- WebSockets. Output flows one direction; SSE is sufficient and simpler.
- Streaming the LLM evaluation phase. Only the test-execution phase streams.

## Architecture

A new `internal/runstream` package owns live runs. `internal/execx` gains a
streaming execution path. The eval and exercise services write output chunks
through the broker as tests execute; final output still lands in SQLite
exactly as today.

### `internal/runstream`

- `Broker` — registry of active runs, keyed by run ID (the submission ID
  namespaced by kind, e.g. `lab/42`, `exercise/17`, so the two services'
  ID sequences cannot collide).
  - `Register(id, cancel func()) *Run` — creates and tracks a live run;
    `cancel` is what `Run.Cancel` invokes to kill the underlying process.
  - `Get(id) (*Run, bool)` — lookup for the SSE/cancel endpoints.
- `Run`
  - `Append(chunk)` — called by the runner goroutine; appends to an
    in-memory buffer and fans out to subscribers.
  - `Subscribe(ctx) <-chan Event` — replays the buffer accrued so far,
    then tails live until the run finishes or ctx is canceled.
  - `Finish(result)` — marks the run done, emits a terminal event to all
    subscribers, and deregisters it from the broker.
  - `Cancel()` — invokes the cancel function supplied at registration
    (kills the test process via context cancellation).
- Concurrency contract: one writer goroutine, N concurrent subscribers,
  mutex-guarded. Subscriber channels are buffered with drop-oldest
  overflow so a stalled client can never block the runner goroutine; a
  dropped-chunk marker event tells the client to re-sync on completion.

Implementation notes (two deviations): (1) each service constructs its own
Broker rather than sharing one instance — the namespaced key scheme is kept
so brokers could be shared later without collisions, but per-service
ownership avoids any wiring changes in `cmd/tour` and the e2e harness.
(2) `Finish()` takes no result and the terminal SSE event carries no
payload — on `done` the client refetches the server-rendered section, which
is the single source of truth for outcome display, so duplicating outcome
data in the event bought nothing.

### `internal/execx`

New streaming variant alongside the existing buffered API: starts the
command with a combined output pipe, scans and forwards chunks to a
callback, honors context cancellation by killing the process group, and
returns the full accumulated output and error with the same semantics the
buffered path has today (timeout formatting included).

### Services (`internal/eval`, `internal/exercise`)

- Run pipelines register with the broker before executing tests, forward
  chunks, and `Finish` when the test phase ends (the LLM phase, when
  enabled, proceeds as today after the stream closes).
- New `Cancel(ctx, id)` on each service: valid only for a run currently
  live in the broker; a canceled run records status `failed` with a
  trailing `canceled by user` marker in its stored output. No schema
  change — status is already a text column, and failed-status retry
  continues to work unchanged.

### Transport

Per surface (eval, exercise), two new endpoints following the package's
existing endpoint/transport structure:

- `GET .../runs/{id}/stream` — SSE. Replays accrued output, tails live,
  sends periodic heartbeat comments to defeat idle timeouts, and ends with
  a terminal event carrying the run's outcome. If the run is not live
  (already finished or unknown), responds with a single terminal event
  built from the stored submission, so a late-connecting client degrades
  gracefully.
- `POST .../runs/{id}/cancel` — cancels a live run; 404-equivalent error
  for runs that are not live.

Note: the server intentionally sets no `WriteTimeout` (long-running
responses); SSE makes that choice permanent rather than temporary.

### Frontend

- Step (lab submit) and exercise templates gain a terminal-style output
  pane. An `EventSource` connects on run start (and on page load when a
  run is in flight), appends chunks, and renders the terminal event as
  the final status.
- A cancel button appears while a run is live.
- If `EventSource` errors, the client falls back to the existing
  poll/refresh behavior — streaming is progressive enhancement, not a
  new hard dependency.

## Error handling

- Client disconnect mid-stream: subscriber context cancels, unsubscribes;
  the run itself is unaffected.
- Server restart mid-run: unchanged — `RecoverInterrupted` fails the
  orphaned submission at next boot.
- Slow subscriber: drop-oldest buffer plus re-sync marker (above).
- Cancel racing natural completion: `Cancel` after `Finish` is a no-op;
  the terminal event reflects whichever happened first.

## Testing (TDD throughout)

- `runstream` unit tests: replay-then-tail ordering, multiple concurrent
  subscribers, drop-oldest overflow, cancel/finish races, subscriber
  context cancellation. Run with `-race`.
- `execx` streaming tests: chunk delivery order, context cancellation
  kills the process, timeout semantics match the buffered path.
- Service tests: pipeline forwards chunks, cancel marks status correctly,
  LLM phase still runs after stream close (existing fakes extended).
- e2e: SSE endpoint delivers chunks incrementally during a slow scripted
  run; cancel mid-run ends the stream and records the canceled outcome;
  late connect to a finished run yields the terminal event.

## Context: code-review tool evaluation

This feature doubles as the next evaluation sample for `docs/tool-eval.md`,
answering its open question about reviewer behavior on a larger multi-file
feature PR (new endpoints, goroutine lifecycles, SSE edge cases). The PR is
written honestly — no seeded defects; findings score precision and depth on
realistic code.
