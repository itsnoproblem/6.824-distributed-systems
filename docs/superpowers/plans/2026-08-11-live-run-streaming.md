# Live Run Streaming + Cancellation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stream lab/exercise test output to the browser live over SSE and let the user cancel an in-flight run.

**Architecture:** A new `internal/runstream` package holds live runs in memory (append-only chunk buffer with replay-then-tail subscription and a cancel hook). `internal/execx` gains a streaming execution path that both services use. Each service (eval, exercise) owns a broker instance, registers a run before scheduling its goroutine (no live-lookup race), and exposes `Watch`/`Cancel`. Transports add an SSE endpoint and a cancel endpoint per surface; the running-state templates render a live terminal pane driven by `static/runstream.js`. Final output still persists to SQLite exactly as before; the LLM phase is unchanged.

**Tech Stack:** Go stdlib (`net/http` SSE, `os/exec`), templ templates (regenerate with `make generate`), htmx + vanilla JS front-end, SQLite persistence (unchanged schema).

**Spec:** `docs/superpowers/specs/2026-08-11-live-run-streaming-design.md`

## Global Constraints

- Go module `github.com/itsnoproblem/mit-distributed-systems`; exercise workspaces generate `go 1.25` go.mod.
- Never edit `templates/*_templ.go` by hand — edit `templates/*.templ` and run `make generate`.
- `make test` runs `templ generate` then `go test ./...`; it must pass at every commit.
- Run `gofmt -l .` before every commit; it must print nothing (excluding vendored `static/codemirror`).
- `internal/runstream` tests must be run with `-race`.
- TDD: each behavior gets a failing test before implementation.
- No schema change: a canceled run is stored as status `failed` with output ending in `canceled by user`.
- Status strings are `pending` / `running` / `complete` / `failed` (existing `eval.Status` values) — do not invent new ones.
- SSE chunk payloads are JSON-encoded strings (exact byte fidelity across newlines); client decodes with `JSON.parse`.
- The HTTP server intentionally has no `WriteTimeout` (see `cmd/tour/main.go` comment) — do not add one; SSE depends on this.

---

### Task 1: `internal/runstream` — Broker and Run

**Files:**
- Create: `internal/runstream/runstream.go`
- Test: `internal/runstream/runstream_test.go`

**Interfaces:**
- Consumes: nothing (stdlib only).
- Produces (used by Tasks 2–6):
  - `type Broker struct{...}`; `func NewBroker() *Broker`
  - `func (b *Broker) Register(id string, cancel func()) *Run`
  - `func (b *Broker) Get(id string) (*Run, bool)`
  - `type Run struct{...}` with `Append(chunk string)`, `Subscribe(ctx context.Context) <-chan Event`, `Finish()`, `Cancel()`, `Canceled() bool`
  - `type Event struct { Kind EventKind; Data string }`
  - `type EventKind int` with constants `KindChunk`, `KindDropped`, `KindDone`

- [ ] **Step 1: Write the failing tests**

Create `internal/runstream/runstream_test.go`:

```go
package runstream

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// collect drains events until KindDone or timeout; returns chunks joined and
// whether a KindDropped marker was seen.
func collect(t *testing.T, ch <-chan Event) (string, bool) {
	t.Helper()
	var b strings.Builder
	dropped := false
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return b.String(), dropped
			}
			switch ev.Kind {
			case KindChunk:
				b.WriteString(ev.Data)
			case KindDropped:
				dropped = true
			case KindDone:
				return b.String(), dropped
			}
		case <-timeout:
			t.Fatal("timed out waiting for events")
		}
	}
}

func TestSubscribeReplaysThenTails(t *testing.T) {
	b := NewBroker()
	r := b.Register("lab/1", func() {})
	r.Append("one\n")
	r.Append("two\n")

	ch := r.Subscribe(context.Background())
	done := make(chan string)
	go func() {
		out, _ := collect(t, ch)
		done <- out
	}()

	r.Append("three\n")
	r.Finish()

	if out := <-done; out != "one\ntwo\nthree\n" {
		t.Fatalf("got %q, want replay then tail in order", out)
	}
}

func TestMultipleSubscribersSeeAllChunks(t *testing.T) {
	b := NewBroker()
	r := b.Register("lab/2", func() {})
	r.Append("a")

	const n = 5
	var wg sync.WaitGroup
	outs := make([]string, n)
	for i := 0; i < n; i++ {
		ch := r.Subscribe(context.Background())
		wg.Add(1)
		go func(i int, ch <-chan Event) {
			defer wg.Done()
			outs[i], _ = collect(t, ch)
		}(i, ch)
	}
	r.Append("b")
	r.Append("c")
	r.Finish()
	wg.Wait()
	for i, out := range outs {
		if out != "abc" {
			t.Fatalf("subscriber %d got %q, want \"abc\"", i, out)
		}
	}
}

func TestFinishDeregistersFromBroker(t *testing.T) {
	b := NewBroker()
	r := b.Register("lab/3", func() {})
	if _, ok := b.Get("lab/3"); !ok {
		t.Fatal("registered run not found")
	}
	r.Finish()
	if _, ok := b.Get("lab/3"); ok {
		t.Fatal("finished run still registered")
	}
}

func TestFinishIsIdempotentAndAppendAfterFinishIsIgnored(t *testing.T) {
	b := NewBroker()
	r := b.Register("lab/4", func() {})
	r.Append("kept")
	r.Finish()
	r.Finish() // second call must not panic (double close of wake channel)
	r.Append("ignored")
	out, _ := collect(t, r.Subscribe(context.Background()))
	if out != "kept" {
		t.Fatalf("got %q, want appends after Finish ignored", out)
	}
}

func TestCancelInvokesHookOnceAndSetsFlag(t *testing.T) {
	calls := 0
	b := NewBroker()
	r := b.Register("lab/5", func() { calls++ })
	if r.Canceled() {
		t.Fatal("fresh run reports canceled")
	}
	r.Cancel()
	r.Cancel()
	if calls != 1 {
		t.Fatalf("cancel hook ran %d times, want 1", calls)
	}
	if !r.Canceled() {
		t.Fatal("Canceled() false after Cancel()")
	}
}

func TestSubscriberContextCancelStopsDelivery(t *testing.T) {
	b := NewBroker()
	r := b.Register("lab/6", func() {})
	ctx, cancel := context.WithCancel(context.Background())
	ch := r.Subscribe(ctx)
	cancel()
	// Channel must close without Finish ever being called.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("subscriber channel never closed after ctx cancel")
		}
	}
}

func TestBufferCapDropsOldestAndMarksSubscriber(t *testing.T) {
	b := NewBroker()
	r := b.Register("lab/7", func() {})
	chunk := strings.Repeat("x", 1024)
	// Overflow the retained window (maxBuffered bytes) before subscribing.
	for i := 0; i < (maxBuffered/1024)+10; i++ {
		r.Append(chunk)
	}
	r.Finish()
	out, dropped := collect(t, r.Subscribe(context.Background()))
	if !dropped {
		t.Fatal("late subscriber saw no KindDropped marker after overflow")
	}
	if len(out) > maxBuffered {
		t.Fatalf("retained %d bytes, cap is %d", len(out), maxBuffered)
	}
	if len(out) == 0 {
		t.Fatal("retained window is empty")
	}
}

func TestConcurrentAppendAndSubscribe(t *testing.T) {
	b := NewBroker()
	r := b.Register("lab/8", func() {})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		ch := r.Subscribe(context.Background())
		wg.Add(1)
		go func(ch <-chan Event) {
			defer wg.Done()
			collect(t, ch)
		}(ch)
	}
	for i := 0; i < 200; i++ {
		r.Append("z")
	}
	r.Finish()
	wg.Wait() // -race is the real assertion here
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -race ./internal/runstream/`
Expected: FAIL — `undefined: NewBroker` (package doesn't compile yet; create `runstream.go` with just the package clause first if you want a cleaner failure, but a compile error naming the missing identifiers is an acceptable "failing" state).

- [ ] **Step 3: Implement**

Create `internal/runstream/runstream.go`:

```go
// Package runstream provides streaming access to in-flight test runs: each
// live run keeps an in-memory buffer of output chunks, fans them out to
// subscribers (replay what's accrued, then tail), and carries the hook that
// cancels the underlying process. Completed runs deregister; history lives
// in SQLite, not here.
package runstream

import (
	"context"
	"sync"
)

// maxBuffered caps the bytes of output retained per run. When exceeded the
// oldest chunks are dropped; a subscriber that lands behind the retained
// window receives a single KindDropped marker and resumes from what remains.
const maxBuffered = 256 * 1024

type EventKind int

const (
	// KindChunk carries a piece of run output in Data.
	KindChunk EventKind = iota
	// KindDropped tells the subscriber that earlier output was dropped
	// (buffer cap exceeded before it caught up).
	KindDropped
	// KindDone is the terminal event; the subscriber channel closes after it.
	KindDone
)

type Event struct {
	Kind EventKind
	Data string
}

// Broker is the registry of live runs, keyed by a caller-chosen id
// (namespaced by kind, e.g. "lab/42", so key schemes stay collision-proof
// even if a broker is ever shared).
type Broker struct {
	mu   sync.Mutex
	runs map[string]*Run
}

func NewBroker() *Broker { return &Broker{runs: map[string]*Run{}} }

// Register creates and tracks a live run. cancel is invoked (once) by
// Run.Cancel to kill the underlying process.
func (b *Broker) Register(id string, cancel func()) *Run {
	r := &Run{id: id, broker: b, cancel: cancel, wake: make(chan struct{})}
	b.mu.Lock()
	b.runs[id] = r
	b.mu.Unlock()
	return r
}

func (b *Broker) Get(id string) (*Run, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.runs[id]
	return r, ok
}

func (b *Broker) remove(id string) {
	b.mu.Lock()
	delete(b.runs, id)
	b.mu.Unlock()
}

// Run is one live test run. One goroutine appends; any number subscribe.
// Append never blocks on subscribers — a stalled reader only stalls its own
// delivery goroutine, and the retained window is capped at maxBuffered.
type Run struct {
	id         string
	broker     *Broker
	cancel     func()
	cancelOnce sync.Once

	mu       sync.Mutex
	chunks   []string
	start    int // absolute index of chunks[0] (grows as oldest are dropped)
	size     int // bytes currently retained
	done     bool
	canceled bool
	wake     chan struct{} // closed and replaced on every state change
}

func (r *Run) Append(chunk string) {
	if chunk == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	r.chunks = append(r.chunks, chunk)
	r.size += len(chunk)
	for r.size > maxBuffered && len(r.chunks) > 1 {
		r.size -= len(r.chunks[0])
		r.chunks = r.chunks[1:]
		r.start++
	}
	close(r.wake)
	r.wake = make(chan struct{})
}

// Finish marks the run complete, releases all subscribers with KindDone, and
// deregisters it from the broker. Idempotent.
func (r *Run) Finish() {
	r.mu.Lock()
	if r.done {
		r.mu.Unlock()
		return
	}
	r.done = true
	close(r.wake)
	r.wake = make(chan struct{})
	r.mu.Unlock()
	r.broker.remove(r.id)
}

// Cancel marks the run canceled and invokes the registration hook once.
func (r *Run) Cancel() {
	r.mu.Lock()
	r.canceled = true
	r.mu.Unlock()
	r.cancelOnce.Do(r.cancel)
}

func (r *Run) Canceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.canceled
}

// Subscribe streams the run to a new channel: retained chunks first, then
// live tail, then KindDone; the channel closes afterwards. Delivery runs in
// its own goroutine, so a slow consumer never blocks Append or other
// subscribers. Cancel ctx to unsubscribe early.
func (r *Run) Subscribe(ctx context.Context) <-chan Event {
	ch := make(chan Event)
	go func() {
		defer close(ch)
		next := 0 // absolute index of the next chunk to deliver
		for {
			r.mu.Lock()
			if next < r.start {
				next = r.start
				r.mu.Unlock()
				if !send(ctx, ch, Event{Kind: KindDropped}) {
					return
				}
				continue
			}
			if next < r.start+len(r.chunks) {
				ev := Event{Kind: KindChunk, Data: r.chunks[next-r.start]}
				next++
				r.mu.Unlock()
				if !send(ctx, ch, ev) {
					return
				}
				continue
			}
			if r.done {
				r.mu.Unlock()
				send(ctx, ch, Event{Kind: KindDone})
				return
			}
			wake := r.wake
			r.mu.Unlock()
			select {
			case <-wake:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func send(ctx context.Context, ch chan<- Event, ev Event) bool {
	select {
	case ch <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/runstream/`
Expected: PASS (all 8 tests)

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/runstream/
git add internal/runstream/
git commit -m "feat: runstream broker for live run output fan-out"
```

---

### Task 2: `runstream.ServeSSE` — SSE transport helper

**Files:**
- Create: `internal/runstream/sse.go`
- Test: `internal/runstream/sse_test.go`

**Interfaces:**
- Consumes: `Event`, `EventKind` from Task 1.
- Produces (used by Tasks 5–6): `func ServeSSE(w http.ResponseWriter, r *http.Request, events <-chan Event)` — writes `text/event-stream` with event names `chunk`/`dropped`/`done`, JSON-encoded string data, `: ping` heartbeat comments, returns when the channel closes / `done` is sent / the client disconnects.

- [ ] **Step 1: Write the failing tests**

Create `internal/runstream/sse_test.go`:

```go
package runstream

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, events []Event) string {
	t.Helper()
	ch := make(chan Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	ServeSSE(rec, req, ch)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	return rec.Body.String()
}

func TestServeSSEWritesChunkEventsAsJSONStrings(t *testing.T) {
	body := serve(t, []Event{
		{Kind: KindChunk, Data: "line one\nline two\n"},
		{Kind: KindDone},
	})
	// JSON encoding keeps the newline escaped, so the payload is one data line.
	if !strings.Contains(body, "event: chunk\ndata: \"line one\\nline two\\n\"\n\n") {
		t.Fatalf("chunk event malformed:\n%s", body)
	}
	if !strings.Contains(body, "event: done\n") {
		t.Fatalf("done event missing:\n%s", body)
	}
}

func TestServeSSEWritesDroppedEvent(t *testing.T) {
	body := serve(t, []Event{{Kind: KindDropped}, {Kind: KindDone}})
	if !strings.Contains(body, "event: dropped\n") {
		t.Fatalf("dropped event missing:\n%s", body)
	}
}

func TestServeSSEStopsWhenClientDisconnects(t *testing.T) {
	ch := make(chan Event) // never closed, nothing sent
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/stream", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		ServeSSE(rec, req, ch)
		close(done)
	}()
	cancel()
	<-done // must return; test hangs (and times out) if it doesn't
}

func TestServeSSEEndsAfterChannelCloseWithoutDone(t *testing.T) {
	body := serve(t, []Event{{Kind: KindChunk, Data: "x"}})
	if !strings.Contains(body, "event: chunk\n") {
		t.Fatalf("chunk missing:\n%s", body)
	}
	// No done event required — closing the channel alone must end the handler.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -race ./internal/runstream/`
Expected: FAIL — `undefined: ServeSSE`

- [ ] **Step 3: Implement**

Create `internal/runstream/sse.go`:

```go
package runstream

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// heartbeatInterval paces SSE comment lines that keep idle connections
// alive through proxies while a run produces no output.
const heartbeatInterval = 15 * time.Second

var eventNames = map[EventKind]string{
	KindChunk:   "chunk",
	KindDropped: "dropped",
	KindDone:    "done",
}

// ServeSSE writes events to w as Server-Sent Events until the channel
// closes, a KindDone event is written, or the client disconnects. Chunk
// data is JSON-encoded so newlines survive SSE line framing byte-for-byte.
func ServeSSE(w http.ResponseWriter, r *http.Request, events <-chan Event) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			writeEvent(w, ev)
			f.Flush()
			if ev.Kind == KindDone {
				return
			}
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeEvent(w io.Writer, ev Event) {
	data, _ := json.Marshal(ev.Data) // a string always marshals
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventNames[ev.Kind], data)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/runstream/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/runstream/
git add internal/runstream/
git commit -m "feat: SSE writer for runstream events"
```

---

### Task 3: `execx.Stream` — streaming subprocess execution

**Files:**
- Modify: `internal/execx/execx.go`
- Test: `internal/execx/execx_test.go` (append new tests; existing tests must keep passing)

**Interfaces:**
- Consumes: nothing new.
- Produces (used by Tasks 4–5): `func Stream(ctx context.Context, dir string, cmd []string, timeout time.Duration, sink func(string)) (string, int, error)` — semantics identical to `Run` (non-zero exit ⇒ finding with nil error; timeout ⇒ err with exit -1; canceled ctx ⇒ err wrapping `context.Canceled`), plus each output chunk is forwarded to `sink` as it arrives (nil sink allowed). `Run` becomes a thin wrapper over `Stream`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/execx/execx_test.go` (keep existing tests untouched):

```go
func TestStreamForwardsChunksIncrementally(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	var got []string
	sink := func(chunk string) {
		mu.Lock()
		got = append(got, chunk)
		mu.Unlock()
	}
	// Two echoes separated by a short sleep force at least two distinct writes.
	out, code, err := Stream(context.Background(), dir,
		[]string{"sh", "-c", "echo first; sleep 0.2; echo second"}, 10*time.Second, sink)
	if err != nil || code != 0 {
		t.Fatalf("Stream: code=%d err=%v", code, err)
	}
	if out != "first\nsecond\n" {
		t.Fatalf("accumulated output = %q", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 {
		t.Fatalf("sink saw %d chunk(s), want at least 2 (incremental delivery)", len(got))
	}
	if joined := strings.Join(got, ""); joined != out {
		t.Fatalf("sink chunks %q != returned output %q", joined, out)
	}
}

func TestStreamNilSink(t *testing.T) {
	out, code, err := Stream(context.Background(), t.TempDir(),
		[]string{"echo", "ok"}, 10*time.Second, nil)
	if err != nil || code != 0 || out != "ok\n" {
		t.Fatalf("got out=%q code=%d err=%v", out, code, err)
	}
}

func TestStreamContextCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, code, err := Stream(ctx, t.TempDir(),
		[]string{"sh", "-c", "echo started; sleep 60"}, time.Minute, nil)
	if err == nil {
		t.Fatal("want error for canceled run")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled in chain", err)
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1", code)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("kill took %s — process group not killed", elapsed)
	}
}

func TestStreamTimeoutMatchesRunSemantics(t *testing.T) {
	out, code, err := Stream(context.Background(), t.TempDir(),
		[]string{"sh", "-c", "echo before; sleep 60"}, 500*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout error", err)
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1", code)
	}
	if !strings.Contains(out, "before") {
		t.Fatalf("output before the hang lost: %q", out)
	}
}
```

Add any missing imports (`strings`, `sync`, `errors`) to the test file's import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/execx/`
Expected: FAIL — `undefined: Stream`

- [ ] **Step 3: Implement**

In `internal/execx/execx.go`, replace the body of `Run` and add `Stream` + `sinkWriter` (keep package comment, `maxOutput`, `truncateTail` as-is; add `"strings"` and `"sync"` imports):

```go
// Run executes cmd in dir. A non-zero exit from the command itself is a
// finding, not an error: exitCode carries it and err stays nil. Timeouts
// and failures to start return err with exitCode -1.
func Run(ctx context.Context, dir string, cmd []string, timeout time.Duration) (string, int, error) {
	return Stream(ctx, dir, cmd, timeout, nil)
}

// Stream executes like Run and additionally forwards each output chunk to
// sink as it arrives (nil sink is allowed). os/exec serializes writes when
// stdout and stderr share one writer, so sink is never called concurrently.
// A parent-context cancelation (as opposed to the timeout elapsing) returns
// an error wrapping context.Canceled.
func Stream(ctx context.Context, dir string, cmd []string, timeout time.Duration, sink func(string)) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Dir = dir
	c.WaitDelay = 5 * time.Second
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	w := &sinkWriter{sink: sink}
	c.Stdout = w
	c.Stderr = w
	err := c.Run()
	out := truncateTail(w.String(), maxOutput)
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return out, -1, fmt.Errorf("run timed out after %s", timeout)
	case context.Canceled:
		return out, -1, fmt.Errorf("run canceled: %w", context.Canceled)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, exitErr.ExitCode(), nil
	}
	if err != nil {
		return out, -1, err
	}
	return out, 0, nil
}

// sinkWriter accumulates all output and forwards each write to sink. The
// mutex guards the builder against the WaitDelay edge where exec's copier
// may still be writing as Run returns.
type sinkWriter struct {
	mu   sync.Mutex
	buf  strings.Builder
	sink func(string)
}

func (w *sinkWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	w.mu.Unlock()
	if w.sink != nil {
		w.sink(string(p))
	}
	return len(p), nil
}

func (w *sinkWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}
```

Note: the old `Run` used `CombinedOutput`; the new shared path uses one writer for both stdout and stderr, which os/exec documents as write-serialized when the writer compares equal. Behavior (interleaving, truncation, exit-code mapping) is unchanged.

- [ ] **Step 4: Run the whole package's tests (old + new) and the packages that depend on execx**

Run: `go test ./internal/execx/ ./internal/exercise/ ./internal/eval/`
Expected: PASS — existing `Run` behavior must be intact.

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/execx/
git add internal/execx/
git commit -m "feat: execx.Stream forwards subprocess output incrementally"
```

---

### Task 4: eval service — broker integration, Watch, Cancel

**Files:**
- Modify: `internal/eval/service.go` (LabRepo interface, Service fields, `SubmitLab`, `evaluateLab`, `Retry`, `StepState`; add `startLabRun`, `Watch`, `Cancel`)
- Modify: `internal/eval/labrepo.go` (`RunTests` gains sink parameter)
- Test: `internal/eval/service_test.go` (update fakes, add tests)
- Test: `internal/eval/labrepo_test.go` (update calls for new signature)

**Interfaces:**
- Consumes: `runstream.NewBroker/Register/Get`, `Run.Append/Finish/Cancel/Canceled/Subscribe`, `runstream.Event` (Task 1).
- Produces (used by Task 5):
  - `LabRepo.RunTests(ctx context.Context, workdir string, cmd []string, timeout time.Duration, sink func(string)) (string, error)` — changed signature; `FSLabRepo` delegates to `execx.Stream`.
  - `func (s *Service) Watch(ctx context.Context, id int64) (<-chan runstream.Event, error)` — live subscription, or a synthesized immediate `KindDone` for non-live submissions; `api.ErrNotFound` for unknown ids.
  - `func (s *Service) Cancel(ctx context.Context, id int64) error` — `api.ErrNotFound` unknown id, `api.ErrInvalid` no live run.
  - `StepEvalView.Live bool` — true only when the latest submission is a lab, pending/running, and registered in the broker.
  - Run key format: `"lab/" + strconv.FormatInt(id, 10)` (unexported helper `labRunKey`).

- [ ] **Step 1: Update the fake LabRepo and add failing tests**

In `internal/eval/service_test.go`, find the fake implementing `LabRepo` and change its `RunTests` to the new signature. Give the fake a controllable script so tests can emit chunks and block:

```go
// fakeLab emits scripted chunks through sink, then blocks until release is
// closed (or returns immediately when release is nil).
type fakeLab struct {
	files   map[string]string
	chunks  []string
	out     string
	err     error
	release chan struct{}
	started chan struct{} // closed once RunTests has begun (nil = untracked)
}

func (f *fakeLab) Snapshot(workdir string, globs []string) (map[string]string, error) {
	return f.files, nil
}

func (f *fakeLab) RunTests(ctx context.Context, workdir string, cmd []string,
	timeout time.Duration, sink func(string)) (string, error) {
	if f.started != nil {
		close(f.started)
	}
	for _, c := range f.chunks {
		if sink != nil {
			sink(c)
		}
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return f.out, fmt.Errorf("run canceled: %w", ctx.Err())
		}
	}
	return f.out, f.err
}
```

(Adapt names/fields to the existing fake if one exists — extend it rather than duplicating. All existing constructions of the fake must compile with the new method signature.)

Add tests (adjust construction helpers to whatever `service_test.go` already uses for building a `Service` with fakes; keep `WithRunAsync(func(f func()) { f() })` for the synchronous ones and use real goroutines where noted):

```go
func TestSubmitLabStreamsChunksToWatcher(t *testing.T) {
	// Arrange a service whose runAsync runs in a real goroutine, with a lab
	// fake that emits two chunks then blocks until released.
	lab := &fakeLab{
		files:   map[string]string{"a.go": "x"},
		chunks:  []string{"chunk-1\n", "chunk-2\n"},
		out:     "chunk-1\nchunk-2\n",
		release: make(chan struct{}),
		started: make(chan struct{}),
	}
	svc := newTestService(t, lab, WithRunAsync(func(f func()) { go f() }))

	if err := svc.SubmitLab(context.Background(), labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, svc) // helper: read back the inserted submission id

	// The run is registered before the goroutine is scheduled, so Watch must
	// find it live immediately.
	events, err := svc.Watch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	<-lab.started
	close(lab.release)

	var got []string
	for ev := range events {
		if ev.Kind == runstream.KindChunk {
			got = append(got, ev.Data)
		}
	}
	if strings.Join(got, "") != "chunk-1\nchunk-2\n" {
		t.Fatalf("watched chunks = %q", got)
	}
}

func TestCancelLabRunRecordsCanceledOutcome(t *testing.T) {
	lab := &fakeLab{
		files:   map[string]string{"a.go": "x"},
		chunks:  []string{"partial\n"},
		out:     "partial\n",
		release: make(chan struct{}), // never closed: only cancel ends the run
		started: make(chan struct{}),
	}
	svc := newTestService(t, lab, WithRunAsync(func(f func()) { go f() }))
	if err := svc.SubmitLab(context.Background(), labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, svc)
	<-lab.started

	events, err := svc.Watch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Cancel(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	for range events { // drain to KindDone/close: the run has fully finished
	}
	sub := getSubmission(t, svc, id) // helper: read back the row
	if sub.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", sub.Status)
	}
	if !strings.Contains(sub.TestOutput, "canceled by user") {
		t.Fatalf("output %q missing canceled marker", sub.TestOutput)
	}
}

func TestCancelWithoutLiveRunIsInvalid(t *testing.T) {
	// Insert a completed submission via the normal synchronous path, then Cancel.
	svc := newTestService(t, &fakeLab{files: map[string]string{"a.go": "x"}, out: "ok"},
		WithRunAsync(func(f func()) { f() }))
	if err := svc.SubmitLab(context.Background(), labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, svc)
	err := svc.Cancel(context.Background(), id)
	if !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestWatchFinishedRunSynthesizesDone(t *testing.T) {
	svc := newTestService(t, &fakeLab{files: map[string]string{"a.go": "x"}, out: "ok"},
		WithRunAsync(func(f func()) { f() }))
	if err := svc.SubmitLab(context.Background(), labRef); err != nil {
		t.Fatal(err)
	}
	id := latestSubmissionID(t, svc)
	events, err := svc.Watch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := <-events
	if !ok || ev.Kind != runstream.KindDone {
		t.Fatalf("got %+v ok=%v, want immediate KindDone", ev, ok)
	}
}

func TestWatchUnknownSubmissionNotFound(t *testing.T) {
	svc := newTestService(t, &fakeLab{}, WithRunAsync(func(f func()) { f() }))
	if _, err := svc.Watch(context.Background(), 9999); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestStepStateLiveFlag(t *testing.T) {
	lab := &fakeLab{
		files:   map[string]string{"a.go": "x"},
		out:     "ok",
		release: make(chan struct{}),
		started: make(chan struct{}),
	}
	svc := newTestService(t, lab, WithRunAsync(func(f func()) { go f() }))
	if err := svc.SubmitLab(context.Background(), labRef); err != nil {
		t.Fatal(err)
	}
	<-lab.started
	view, err := svc.StepState(context.Background(), labRef)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Live {
		t.Fatal("running registered lab should be Live")
	}
	close(lab.release)
	id := latestSubmissionID(t, svc)
	events, _ := svc.Watch(context.Background(), id)
	for range events {
	}
	view, err = svc.StepState(context.Background(), labRef)
	if err != nil {
		t.Fatal(err)
	}
	if view.Live {
		t.Fatal("finished run must not be Live")
	}
}
```

`newTestService`, `latestSubmissionID`, `getSubmission`, and `labRef` stand for this test file's existing arrangement helpers/fixtures — reuse or add small helpers consistent with what's already there (the file already builds Services with fake repos; follow its pattern).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/eval/`
Expected: FAIL — compile errors on the new `RunTests` signature and `undefined: Watch/Cancel/Live`.

- [ ] **Step 3: Implement**

`internal/eval/labrepo.go` — change `RunTests`:

```go
// RunTests executes the step's test command in the lab repo via execx,
// forwarding output chunks to sink as they arrive (nil sink allowed): test
// failures are findings (non-zero exit ⇒ nil error); timeouts, cancelation,
// and failures to execute return err.
func (l FSLabRepo) RunTests(ctx context.Context, workdir string, cmd []string,
	timeout time.Duration, sink func(string)) (string, error) {
	out, _, err := execx.Stream(ctx, filepath.Join(l.Dir, workdir), cmd, timeout, sink)
	return out, err
}
```

`internal/eval/service.go` changes:

1. Interface:

```go
// LabRepo abstracts the student's mounted lab repository (implemented by
// FSLabRepo; may be nil in tests that don't exercise labs).
type LabRepo interface {
	Snapshot(workdir string, globs []string) (map[string]string, error)
	RunTests(ctx context.Context, workdir string, cmd []string,
		timeout time.Duration, sink func(string)) (string, error)
}
```

2. Service gains a broker (import `"strconv"` and `"github.com/itsnoproblem/mit-distributed-systems/internal/runstream"`):

```go
type Service struct {
	// ... existing fields ...
	broker *runstream.Broker
}
```

In `NewService`, add `broker: runstream.NewBroker(),` to the literal.

3. Add `Live` to the view:

```go
type StepEvalView struct {
	Enabled    bool
	Live       bool // a run for the latest submission is streaming right now
	Step       course.Step
	Submission *Submission
	Evaluation *Evaluation
}
```

4. Run-key helper and run scheduling. `SubmitLab` replaces `s.runAsync(func() { s.evaluateLab(id) })` with `s.startLabRun(id)`; `Retry`'s lab branch replaces `s.runAsync(func() { s.evaluateLab(id) })` with `s.startLabRun(id)`:

```go
func labRunKey(id int64) string { return "lab/" + strconv.FormatInt(id, 10) }

// startLabRun registers the live run BEFORE scheduling the pipeline
// goroutine, so a Watch/Cancel arriving right after the submit response
// always finds it.
func (s *Service) startLabRun(id int64) {
	runCtx, cancel := context.WithCancel(context.Background())
	live := s.broker.Register(labRunKey(id), cancel)
	s.runAsync(func() {
		defer cancel()
		s.evaluateLab(runCtx, id, live)
	})
}
```

5. `evaluateLab` becomes `func (s *Service) evaluateLab(runCtx context.Context, id int64, live *runstream.Run)`. Changes inside, keeping everything else as-is:

```go
func (s *Service) evaluateLab(runCtx context.Context, id int64, live *runstream.Run) {
	defer live.Finish() // idempotent; releases subscribers on every exit path
	ctx := context.Background()
	sub, err := s.subs.GetSubmission(ctx, id)
	if err != nil {
		log.Printf("evaluateLab: load submission %d: %v", id, err)
		return
	}
	mod, step, ok := s.course.Course().Step(sub.Ref)
	if !ok || step.Eval == nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, "step no longer exists in content")
		return
	}
	_ = s.subs.UpdateSubmission(ctx, id, StatusRunning, "")
	out, err := s.lab.RunTests(runCtx, step.Eval.Workdir, step.Eval.TestCmd, step.Eval.Timeout, live.Append)
	live.Finish() // test phase over: release stream subscribers before the LLM phase
	if live.Canceled() {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\ncanceled by user")
		return
	}
	if err != nil {
		_ = s.subs.UpdateSubmission(ctx, id, StatusFailed, out+"\n\nRUNNER ERROR: "+err.Error())
		return
	}
	// ... remainder (nil-LLM early return, snapshot decode, prompt, verdict,
	// evaluation insert, StatusComplete) unchanged, still using ctx ...
}
```

6. `Watch` and `Cancel`:

```go
// Watch subscribes to the live output of a submission's run. For a
// submission with no live run (finished, interrupted, or in its LLM phase)
// it synthesizes an immediate done event so late connections degrade
// gracefully instead of erroring.
func (s *Service) Watch(ctx context.Context, id int64) (<-chan runstream.Event, error) {
	if _, err := s.subs.GetSubmission(ctx, id); err != nil {
		return nil, fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	if run, ok := s.broker.Get(labRunKey(id)); ok {
		return run.Subscribe(ctx), nil
	}
	ch := make(chan runstream.Event, 1)
	ch <- runstream.Event{Kind: runstream.KindDone}
	close(ch)
	return ch, nil
}

// Cancel kills the live run for a submission. Only the test-execution phase
// is cancelable; afterwards the run is no longer live and this rejects.
func (s *Service) Cancel(ctx context.Context, id int64) error {
	if _, err := s.subs.GetSubmission(ctx, id); err != nil {
		return fmt.Errorf("%w: submission %d", api.ErrNotFound, id)
	}
	run, ok := s.broker.Get(labRunKey(id))
	if !ok {
		return fmt.Errorf("%w: submission %d has no live run", api.ErrInvalid, id)
	}
	run.Cancel()
	return nil
}
```

7. `StepState` sets `Live` after loading the submission:

```go
	view := StepEvalView{Enabled: s.Enabled(), Step: *step, Submission: sub}
	if sub != nil && sub.Kind == KindLab &&
		(sub.Status == StatusPending || sub.Status == StatusRunning) {
		_, view.Live = s.broker.Get(labRunKey(sub.ID))
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/eval/`
Expected: PASS (new tests and all pre-existing ones)

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/eval/
git add internal/eval/
git commit -m "feat: eval service streams lab runs and supports cancel"
```

---

### Task 5: exercise service — broker integration, Watch, Cancel

**Files:**
- Modify: `internal/exercise/service.go` (Runner interface, Service fields, `Run`, `evaluate`, `State`; add `startRun`, `Watch`, `Cancel`)
- Modify: `internal/exercise/workspace.go` (`RunExercise` gains sink parameter)
- Test: `internal/exercise/service_test.go` (update fakes, add tests)
- Test: `internal/exercise/workspace_test.go` (update calls for new signature)

**Interfaces:**
- Consumes: `runstream` (Task 1), `execx.Stream` (Task 3).
- Produces (used by Task 6):
  - `Runner.RunExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string, sink func(string)) (string, int, error)` — changed signature.
  - `func (s *Service) Watch(ctx context.Context, id int64) (<-chan runstream.Event, error)` — same contract as eval's.
  - `func (s *Service) Cancel(ctx context.Context, id int64) error` — same contract as eval's.
  - `View.Live bool`.
  - Run key: `"exercise/" + strconv.FormatInt(id, 10)` (unexported helper `runKey`).

- [ ] **Step 1: Update fake runner and add failing tests**

In `internal/exercise/service_test.go`, extend the existing fake `Runner` with the sink parameter and a scripted streaming mode mirroring Task 4's `fakeLab` (`chunks []string`, `release chan struct{}`, `started chan struct{}`; emit chunks through sink, then block on release unless nil, honoring ctx cancellation by returning `("partial output", -1, fmt.Errorf("run canceled: %w", ctx.Err()))`).

Add tests, mirroring Task 4 exactly but through the exercise API (`svc.Run(ctx, ref)` to start, submissions read back the same way this file already does):

- `TestRunStreamsChunksToWatcher` — start with `WithRunAsync(func(f func()) { go f() })`, `svc.Run`, then `Watch` immediately returns a live subscription; collected chunks equal the scripted output.
- `TestCancelExerciseRunRecordsCanceledOutcome` — cancel mid-run; drain events; stored status is `eval.StatusFailed` and output contains `canceled by user`; `Passed` was never set.
- `TestCancelWithoutLiveRunIsInvalid` — synchronous run to completion, then `Cancel` → `api.ErrInvalid`.
- `TestWatchFinishedRunSynthesizesDone` — synchronous run, `Watch` → immediate `KindDone`.
- `TestStateLiveFlag` — `View.Live` true while the scripted run blocks, false after it finishes.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/exercise/`
Expected: FAIL — compile errors on `RunExercise` signature; `undefined: Watch/Cancel`.

- [ ] **Step 3: Implement**

`internal/exercise/workspace.go`:

```go
// RunExercise materializes the workspace and runs the step's test command,
// forwarding output chunks to sink as they arrive (nil sink allowed).
func (w Workspace) RunExercise(ctx context.Context, meta *course.CodeMeta,
	editable map[string]string, sink func(string)) (string, int, error) {
	dir, cleanup, err := w.materialize(meta, editable)
	if err != nil {
		return "", -1, err
	}
	defer cleanup()
	return execx.Stream(ctx, dir, meta.Run, meta.Timeout, sink)
}
```

`internal/exercise/service.go` — mirror Task 4's shape:

1. Runner interface method becomes:

```go
	RunExercise(ctx context.Context, meta *course.CodeMeta, editable map[string]string,
		sink func(string)) (output string, exitCode int, err error)
```

2. `Service` gains `broker *runstream.Broker`, initialized in `NewService` with `runstream.NewBroker()` (imports: `"strconv"`, runstream).

3. `View` (in `internal/exercise/models.go`) gains `Live bool`.

4. Scheduling — `Run` replaces `s.runAsync(func() { s.evaluate(id) })` with `s.startRun(id)`:

```go
func runKey(id int64) string { return "exercise/" + strconv.FormatInt(id, 10) }

// startRun registers the live run BEFORE scheduling the pipeline goroutine,
// so a Watch/Cancel arriving right after the run response always finds it.
func (s *Service) startRun(id int64) {
	runCtx, cancel := context.WithCancel(context.Background())
	live := s.broker.Register(runKey(id), cancel)
	s.runAsync(func() {
		defer cancel()
		s.evaluate(runCtx, id, live)
	})
}
```

5. `evaluate` becomes `func (s *Service) evaluate(runCtx context.Context, id int64, live *runstream.Run)` with `defer live.Finish()` first; the runner call and its aftermath become:

```go
	out, code, err := s.runner.RunExercise(runCtx, step.Code, files, live.Append)
	live.Finish()
	if live.Canceled() {
		if uerr := s.subs.UpdateSubmission(ctx, id, eval.StatusFailed, out+"\n\ncanceled by user"); uerr != nil {
			log.Printf("exercise evaluate: update submission %d to failed (canceled): %v", id, uerr)
		}
		return
	}
```

(everything else in the function unchanged, still using `ctx := context.Background()` for persistence).

6. `Watch` and `Cancel` — identical to Task 4's implementations except `labRunKey` → `runKey` and doc comments referencing exercise runs.

7. `State` sets `Live` right after loading the submission into the view:

```go
	if view.Submission != nil &&
		(view.Submission.Status == eval.StatusPending || view.Submission.Status == eval.StatusRunning) {
		_, view.Live = s.broker.Get(runKey(view.Submission.ID))
	}
```

(No `Kind` check needed: `LatestForStep` on a code-step ref only ever returns exercise submissions.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/exercise/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/exercise/
git add internal/exercise/
git commit -m "feat: exercise service streams runs and supports cancel"
```

---

### Task 6: eval transport + template — SSE/cancel endpoints, live pane

**Files:**
- Modify: `internal/eval/endpoint.go` (extend `EvalService` interface)
- Modify: `internal/eval/transport.go` (routes + VM mapping)
- Modify: `templates/viewmodels.go` (`EvalSectionVM.Live bool`)
- Create: `templates/runstream.templ` (shared `RunLive` component)
- Modify: `templates/eval.templ` (`LabSection` running branch)
- Test: `templates/viewmodels_test.go` only if it asserts VM field sets; otherwise covered by e2e in Task 8.

**Interfaces:**
- Consumes: `Service.Watch/Cancel` (Task 4), `runstream.ServeSSE` (Task 2).
- Produces:
  - `GET /submissions/{id}/stream` — SSE.
  - `POST /submissions/{id}/cancel` — 204 on success.
  - `templ RunLive(base string, sectionURL string, target string)` in `templates/runstream.templ` (reused by Task 7) — renders `<div class="run-live" data-stream-url={base+"/stream"} data-cancel-url={base+"/cancel"} data-section-url={sectionURL} data-target={target}>` with a `.run-live-output` pre, `.run-live-cancel` button, and a `/static/runstream.js` script tag.

- [ ] **Step 1: Extend the service contract**

In `internal/eval/endpoint.go`, add to `EvalService` (import runstream):

```go
	Watch(ctx context.Context, id int64) (<-chan runstream.Event, error)
	Cancel(ctx context.Context, id int64) error
```

- [ ] **Step 2: Add routes**

In `internal/eval/transport.go` `RegisterRoutes`, after the existing `/submissions/{id}/section` route:

```go
	mux.HandleFunc("GET /submissions/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		events, err := svc.Watch(r.Context(), id)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		runstream.ServeSSE(w, r, events)
	})

	mux.HandleFunc("POST /submissions/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		if err := svc.Cancel(r.Context(), id); err != nil {
			api.RenderError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
```

(Streaming and cancel bypass the `api.Endpoint` indirection deliberately: SSE writes a long-lived response, and cancel returns no body — neither fits the request→response-model→render shape.)

- [ ] **Step 3: View model + templates**

`templates/viewmodels.go`: add `Live bool` to `EvalSectionVM`.

`internal/eval/transport.go` `sectionVM`: add `vm.Live = v.Live` next to the other top-level assignments.

Create `templates/runstream.templ`:

```templ
package templates

// RunLive renders the live-output pane for an in-flight test run. base is
// the submission URL prefix (e.g. "/submissions/42"); sectionURL is fetched
// into target once the run finishes. runstream.js binds the behavior and is
// idempotent across htmx swaps.
templ RunLive(base string, sectionURL string, target string) {
	<div
		class="run-live"
		data-stream-url={ base + "/stream" }
		data-cancel-url={ base + "/cancel" }
		data-section-url={ sectionURL }
		data-target={ target }
	>
		<p class="run-live-status">⏳ Tests running — output streams below.</p>
		<pre class="test-output run-live-output"></pre>
		<button class="btn danger run-live-cancel" type="button">Cancel run</button>
	</div>
	<script src="/static/runstream.js"></script>
}
```

`templates/eval.templ` — in `LabSection`, replace the pending/running branch:

```templ
	if v.Status == "pending" || v.Status == "running" {
		if v.Live {
			@RunLive(fmt.Sprintf("/submissions/%d", v.SubmissionID),
				fmt.Sprintf("/submissions/%d/section", v.SubmissionID), "#eval-section")
		} else {
			<div hx-get={ fmt.Sprintf("/submissions/%d/section", v.SubmissionID) }
				hx-trigger="every 3s" hx-target="#eval-section" hx-swap="innerHTML">
				<p>⏳ Evaluation running — lab tests can take several minutes.</p>
			</div>
		}
	} else {
```

(The non-live branch is the existing poll div, now also covering the LLM phase after the stream closes.)

- [ ] **Step 4: Generate templates and run tests**

Run: `make test`
Expected: PASS (`templ generate` regenerates `*_templ.go`; e2e still passes because non-live states render exactly as before).

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/eval/ templates/
git add internal/eval/ templates/
git commit -m "feat: SSE stream and cancel endpoints for lab submissions"
```

---

### Task 7: exercise transport + template — SSE/cancel endpoints, live pane

**Files:**
- Modify: `internal/exercise/endpoint.go` (extend `ExerciseService` interface — it lives in this file next to the endpoints; match how `EvalService` was extended)
- Modify: `internal/exercise/transport.go` (routes + VM mapping)
- Modify: `templates/viewmodels.go` (`ExerciseVM.Live bool`)
- Modify: `templates/exercise.templ` (`ExerciseStatus` running branch)

**Interfaces:**
- Consumes: exercise `Service.Watch/Cancel` (Task 5), `runstream.ServeSSE` (Task 2), `RunLive` templ component (Task 6).
- Produces:
  - `GET /exercises/submissions/{id}/stream` — SSE.
  - `POST /exercises/submissions/{id}/cancel` — 204 on success.

- [ ] **Step 1: Extend the service contract**

In `internal/exercise/endpoint.go`, add to the `ExerciseService` interface:

```go
	Watch(ctx context.Context, id int64) (<-chan runstream.Event, error)
	Cancel(ctx context.Context, id int64) error
```

- [ ] **Step 2: Add routes**

In `internal/exercise/transport.go` `RegisterRoutes`, after the `/exercises/submissions/{id}/status` route — same two handlers as Task 6 Step 2 verbatim, with paths `GET /exercises/submissions/{id}/stream` and `POST /exercises/submissions/{id}/cancel` (import runstream).

- [ ] **Step 3: View model + template**

`templates/viewmodels.go`: add `Live bool` to `ExerciseVM`.
`internal/exercise/transport.go` `exerciseVM`: inside the `if v.Submission != nil` block add `vm.Live = v.Live`.

`templates/exercise.templ` — in `ExerciseStatus`, replace the pending/running branch:

```templ
	if v.Status == "pending" || v.Status == "running" {
		if v.Live {
			@RunLive(fmt.Sprintf("/exercises/submissions/%d", v.SubmissionID),
				fmt.Sprintf("/exercises/submissions/%d/status", v.SubmissionID), "#exercise-status")
		} else {
			<div hx-get={ fmt.Sprintf("/exercises/submissions/%d/status", v.SubmissionID) }
				hx-trigger="every 1s" hx-target="#exercise-status" hx-swap="innerHTML">
				<p>⏳ Running tests…</p>
			</div>
		}
	}
```

- [ ] **Step 4: Generate templates and run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/exercise/ templates/
git add internal/exercise/ templates/
git commit -m "feat: SSE stream and cancel endpoints for exercise runs"
```

---

### Task 8: `static/runstream.js` — live pane behavior

**Files:**
- Create: `static/runstream.js`
- Modify: `static/styles.css` (pane styles)
- Test: `static/static_test.go` (assert the file is embedded, matching how existing assets are asserted there)

**Interfaces:**
- Consumes: the `RunLive` markup contract from Task 6 (`data-stream-url`, `data-cancel-url`, `data-section-url`, `data-target`, `.run-live-output`, `.run-live-cancel`), SSE events `chunk` (JSON-string data) / `dropped` / `done`, `window.htmx.ajax`.
- Produces: self-initializing script; safe to include on every htmx swap (idempotent via `data-init`).

- [ ] **Step 1: Extend the embed test**

In `static/static_test.go`, add `"runstream.js"` to however the test enumerates served assets (follow the existing pattern for `exercise.js`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./static/`
Expected: FAIL — `runstream.js` missing.

- [ ] **Step 3: Implement**

Create `static/runstream.js`:

```js
// Live run pane: tails the SSE stream into the output pre, offers cancel,
// and swaps in the final section when the run completes. Initialization is
// idempotent — the script tag rides along with every htmx swap of the pane.
(function () {
  "use strict";

  function init(pane) {
    if (pane.dataset.init) { return; }
    pane.dataset.init = "1";

    var out = pane.querySelector(".run-live-output");
    var cancelBtn = pane.querySelector(".run-live-cancel");
    var refreshed = false;
    var es = new EventSource(pane.dataset.streamUrl);

    // Loads the authoritative section state exactly once; the server decides
    // what renders next (result, LLM-phase polling, or failure).
    function refresh() {
      if (refreshed) { return; }
      refreshed = true;
      es.close();
      window.htmx.ajax("GET", pane.dataset.sectionUrl,
        { target: pane.dataset.target, swap: "innerHTML" });
    }

    es.addEventListener("chunk", function (ev) {
      out.textContent += JSON.parse(ev.data);
      out.scrollTop = out.scrollHeight;
    });
    es.addEventListener("dropped", function () {
      out.textContent += "…(earlier output dropped)…\n";
    });
    es.addEventListener("done", function () {
      // Small delay lets the pipeline persist its final status before the
      // section re-render reads it.
      setTimeout(refresh, 500);
    });
    es.onerror = function () {
      // Broken stream (server restart, proxy hiccup): fall back to a single
      // delayed section reload — the server-rendered result includes the
      // 3s-polling fallback if the run is still going.
      setTimeout(refresh, 3000);
    };

    cancelBtn.addEventListener("click", function () {
      cancelBtn.disabled = true;
      fetch(pane.dataset.cancelUrl, { method: "POST" });
      // No UI update here: cancellation surfaces through the stream's done
      // event and the section re-render.
    });
  }

  document.querySelectorAll(".run-live").forEach(init);
})();
```

Append to `static/styles.css`:

```css
/* Live run streaming pane */
.run-live-output {
  min-height: 6em;
  max-height: 24em;
  overflow-y: auto;
}
.run-live-status {
  font-weight: 600;
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./static/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -l static/
git add static/
git commit -m "feat: runstream.js live output pane with cancel"
```

---

### Task 9: e2e coverage — streaming and cancellation over HTTP

**Files:**
- Modify: `e2e/harness_test.go` (`options.AsyncRuns`)
- Create: `e2e/runstream_test.go`
- Create: `e2e/testdata/content/modules/03-test-code/exercises/02-slow/slow.go`
- Create: `e2e/testdata/content/modules/03-test-code/exercises/02-slow/slow_test.go`
- Create: `e2e/testdata/content/modules/03-test-code/steps/02-slow.md`

**Interfaces:**
- Consumes: everything from Tasks 4–7 over HTTP; existing harness (`newApp`, `options`).
- Produces: nothing downstream.

- [ ] **Step 1: Harness async option**

In `e2e/harness_test.go`, add `AsyncRuns bool` to `options`, and in `newApp` compute the scheduler once and pass it to both services:

```go
	runAsync := func(f func()) { f() } // synchronous: tests see final state
	if o.AsyncRuns {
		runAsync = func(f func()) { go f() } // real async: streaming tests watch mid-run
	}
```

Use `eval.WithRunAsync(runAsync)` and `exercise.WithRunAsync(runAsync)` in the two constructor calls (replacing the inline literals).

- [ ] **Step 2: Slow exercise testdata**

`e2e/testdata/content/modules/03-test-code/steps/02-slow.md`:

```markdown
---
title: Slow exercise
type: code
code:
  mode: fix
  editable: ["slow.go"]
  readonly: ["slow_test.go"]
  run: ["go", "test", "-v", "."]
  timeout: 1m
---

Watch the output stream.
```

(`-v` matters: it makes `go test` relay test log lines as they happen instead of buffering to the end.)

`e2e/testdata/content/modules/03-test-code/exercises/02-slow/slow.go`:

```go
package exercise

// Value is what the slow test inspects.
func Value() int { return 1 }
```

`e2e/testdata/content/modules/03-test-code/exercises/02-slow/slow_test.go`:

```go
package exercise

import (
	"testing"
	"time"
)

func TestSlow(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Logf("tick %d", i)
		time.Sleep(150 * time.Millisecond)
	}
	if Value() != 1 {
		t.Fatal("wrong value")
	}
}
```

(~3s runtime: long enough to watch mid-run, short enough for CI.)

- [ ] **Step 3: Write the e2e tests**

Create `e2e/runstream_test.go`:

```go
package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// latestSubmissionID reads the newest submission row for a step directly
// from the app database.
func latestSubmissionID(t *testing.T, a *app, module, step string) int64 {
	t.Helper()
	var id int64
	err := a.DB.QueryRow(
		`SELECT id FROM submissions WHERE module_slug = ? AND step_slug = ? ORDER BY id DESC LIMIT 1`,
		module, step).Scan(&id)
	if err != nil {
		t.Fatalf("latest submission: %v", err)
	}
	return id
}

// readStream consumes an SSE response until a done event, the predicate
// says stop, or the deadline passes. Returns concatenated chunk payloads
// and whether done was seen.
func readStream(t *testing.T, body io.Reader, stopAfter func(chunks string) bool) (string, bool) {
	t.Helper()
	var chunks strings.Builder
	scanner := bufio.NewScanner(body)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if event == "chunk" {
				var s string
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &s); err != nil {
					t.Fatalf("chunk payload not a JSON string: %v", err)
				}
				chunks.WriteString(s)
			}
		case line == "":
			if event == "done" {
				return chunks.String(), true
			}
			event = ""
			if stopAfter != nil && stopAfter(chunks.String()) {
				return chunks.String(), false
			}
		}
	}
	return chunks.String(), false
}

func TestExerciseRunStreamsLiveOutput(t *testing.T) {
	a := newApp(t, options{AsyncRuns: true})

	resp, err := http.Post(a.TS.URL+"/exercises/03-test-code/02-slow/run", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run: status %d", resp.StatusCode)
	}
	id := latestSubmissionID(t, a, "03-test-code", "02-slow")

	// The running-state markup must offer the live pane (Live=true).
	page, err := http.Get(a.TS.URL + "/exercises/03-test-code/02-slow")
	if err != nil {
		t.Fatal(err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if !strings.Contains(string(pageBody), "data-stream-url") {
		t.Fatalf("running exercise section lacks live pane:\n%s", pageBody)
	}

	stream, err := http.Get(a.TS.URL + "/exercises/submissions/" + strconvItoa(id) + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if ct := stream.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	// Stop as soon as an early tick arrives — before the ~3s run can finish —
	// proving delivery is incremental, then confirm the run is still going.
	chunks, done := readStream(t, stream.Body, func(c string) bool {
		return strings.Contains(c, "tick 2")
	})
	if done {
		t.Fatal("run finished before any mid-run chunk was observed")
	}
	if !strings.Contains(chunks, "tick 0") {
		t.Fatalf("streamed output missing early tick: %q", chunks)
	}
}

func TestExerciseRunCancelMidRun(t *testing.T) {
	a := newApp(t, options{AsyncRuns: true})

	resp, err := http.Post(a.TS.URL+"/exercises/03-test-code/02-slow/run", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	id := latestSubmissionID(t, a, "03-test-code", "02-slow")
	idStr := strconvItoa(id)

	stream, err := http.Get(a.TS.URL + "/exercises/submissions/" + idStr + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()

	cancelResp, err := http.Post(a.TS.URL+"/exercises/submissions/"+idStr+"/cancel", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusNoContent {
		t.Fatalf("cancel: status %d", cancelResp.StatusCode)
	}

	if _, done := readStream(t, stream.Body, nil); !done {
		t.Fatal("stream did not end with done after cancel")
	}

	// The canceled outcome must be persisted (poll briefly: the pipeline
	// finishes the row just after the stream closes).
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status, output string
		err := a.DB.QueryRow(`SELECT status, test_output FROM submissions WHERE id = ?`, id).
			Scan(&status, &output)
		if err != nil {
			t.Fatal(err)
		}
		if status == "failed" && strings.Contains(output, "canceled by user") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status=%q output=%q — canceled outcome never recorded", status, output)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Cancel again: run no longer live → 400.
	again, err := http.Post(a.TS.URL+"/exercises/submissions/"+idStr+"/cancel", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	again.Body.Close()
	if again.StatusCode != http.StatusBadRequest {
		t.Fatalf("second cancel: status %d, want 400", again.StatusCode)
	}
}

func TestStreamFinishedRunYieldsImmediateDone(t *testing.T) {
	a := newApp(t, options{}) // synchronous: the run is complete when POST returns
	resp, err := http.Post(a.TS.URL+"/exercises/03-test-code/01-fix/run", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	id := latestSubmissionID(t, a, "03-test-code", "01-fix")

	stream, err := http.Get(a.TS.URL + "/exercises/submissions/" + strconvItoa(id) + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if _, done := readStream(t, stream.Body, nil); !done {
		t.Fatal("finished run must synthesize an immediate done event")
	}
}

func TestStreamUnknownSubmission404(t *testing.T) {
	a := newApp(t, options{})
	resp, err := http.Get(a.TS.URL + "/exercises/submissions/99999/stream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func strconvItoa(id int64) string { return strconv.FormatInt(id, 10) }
```

The SQL column names (`module_slug`, `step_slug`, `status`, `test_output`) match `internal/sqlite/migrations/`; the harness exposes `a.DB` for exactly this kind of assertion.

Also add a lab-side streaming test using a scripted in-test `eval.LabRepo` fake (deterministic — no sleeps): a fake whose `RunTests` emits a chunk, blocks on a channel, and honors ctx — mirroring Task 4's `fakeLab`. Drive it over HTTP: `POST /modules/02-test-lab/steps/01-submit/submit-lab` with `options{AsyncRuns: true, Lab: fake}`, `GET /submissions/{id}/stream`, release the fake, assert the chunk arrived and `done` follows; then a cancel variant asserting the stored `canceled by user` marker. Reuse `readStream`.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./e2e/ -run 'Stream|Cancel'`
Then: `make test`
Expected: PASS for both.

- [ ] **Step 5: Commit**

```bash
gofmt -l e2e/
git add e2e/
git commit -m "test: e2e coverage for live run streaming and cancel"
```

---

### Task 10: docs + final verification

**Files:**
- Modify: `README.md` (document streaming + cancel in the Evaluation mode / Interactive exercises sections)
- Modify: `docs/superpowers/specs/2026-08-11-live-run-streaming-design.md` (record the one deviation)

- [ ] **Step 1: README**

In the "Evaluation mode" section, after the Labs bullet, add:

```markdown
Lab and exercise test runs stream their output to the page live as tests
execute, and an in-flight run can be canceled; a canceled run is recorded
as failed and can be retried.
```

- [ ] **Step 2: Spec deviation note**

In the spec's `internal/runstream` section, append:

```markdown
Implementation notes (two deviations): (1) each service constructs its own
Broker rather than sharing one instance — the namespaced key scheme is kept
so brokers could be shared later without collisions, but per-service
ownership avoids any wiring changes in `cmd/tour` and the e2e harness.
(2) `Finish()` takes no result and the terminal SSE event carries no
payload — on `done` the client refetches the server-rendered section, which
is the single source of truth for outcome display, so duplicating outcome
data in the event bought nothing.
```

- [ ] **Step 3: Full verification**

Run, in order:

```bash
make test
go test -race ./internal/runstream/ ./internal/eval/ ./internal/exercise/ ./e2e/
gofmt -l . | grep -v static/codemirror
```

Expected: all tests PASS; gofmt prints nothing.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/superpowers/specs/2026-08-11-live-run-streaming-design.md
git commit -m "docs: document live run streaming and cancellation"
```
