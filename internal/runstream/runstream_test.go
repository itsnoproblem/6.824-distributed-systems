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

func TestFinishOfStaleRunDoesNotDeregisterReplacement(t *testing.T) {
	b := NewBroker()
	runA := b.Register("lab/9", func() {})
	runB := b.Register("lab/9", func() {}) // id reused, e.g. on retry; overwrites runA in the broker

	runA.Finish() // finishes late; must not deregister runB

	got, ok := b.Get("lab/9")
	if !ok {
		t.Fatal("replacement run was deregistered by stale Finish")
	}
	if got != runB {
		t.Fatal("broker entry for lab/9 is not runB after stale runA.Finish()")
	}

	runB.Finish() // the real finish must still deregister
	if _, ok := b.Get("lab/9"); ok {
		t.Fatal("runB.Finish() did not deregister")
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
	if !r.Cancel() {
		t.Fatal("first Cancel() returned false, want true")
	}
	r.Cancel()
	if calls != 1 {
		t.Fatalf("cancel hook ran %d times, want 1", calls)
	}
	if !r.Canceled() {
		t.Fatal("Canceled() false after Cancel()")
	}
}

func TestCancelAfterFinishRejected(t *testing.T) {
	calls := 0
	b := NewBroker()
	r := b.Register("lab/10", func() { calls++ })
	r.Finish()
	if r.Cancel() {
		t.Fatal("Cancel() after Finish() returned true, want false")
	}
	if calls != 0 {
		t.Fatalf("cancel hook ran %d times after finish, want 0", calls)
	}
	if r.Canceled() {
		t.Fatal("Canceled() true after a rejected Cancel()")
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
