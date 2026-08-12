package runstream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// failingWriter is an http.ResponseWriter and http.Flusher whose first
// Write succeeds (so ServeSSE's initial status-line flush goes through)
// and every Write after that fails, simulating a client that vanished
// mid-stream without the request context ever being canceled.
type failingWriter struct {
	header http.Header
	writes int
}

func (w *failingWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingWriter) WriteHeader(int) {}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > 1 {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

func (w *failingWriter) Flush() {}

func TestServeSSEStopsOnWriteError(t *testing.T) {
	w := &failingWriter{}
	req := httptest.NewRequest("GET", "/stream", nil)
	// Left open and never closed: this proves ServeSSE stops because of the
	// write error itself, not because the channel closed or the heartbeat
	// ticker fired.
	events := make(chan Event, 2)
	events <- Event{Kind: KindChunk, Data: "one"}
	events <- Event{Kind: KindChunk, Data: "two"}

	done := make(chan struct{})
	go func() {
		ServeSSE(w, req, events)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeSSE kept looping after a write error instead of returning")
	}
}
