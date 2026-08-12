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
