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
