package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
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

// fakeLab is eval.LabRepo for e2e tests: RunTests emits a single chunk
// through sink, then blocks on release until it's closed or ctx is
// canceled, mirroring internal/eval/service_test.go's stubLabRepo. Snapshot
// returns a non-empty file map so the submit pipeline has something to
// persist.
type fakeLab struct {
	release chan struct{}
	started chan struct{}
}

func (f *fakeLab) Snapshot(_ string, _ []string) (map[string]string, error) {
	return map[string]string{"src/x/x.go": "package x"}, nil
}

func (f *fakeLab) RunTests(ctx context.Context, _ string, _ []string,
	_ time.Duration, sink func(string)) (string, error) {
	if sink != nil {
		sink("chunk-1\n")
	}
	if f.started != nil {
		close(f.started)
	}
	select {
	case <-f.release:
		return "chunk-1\n", nil
	case <-ctx.Done():
		return "chunk-1\n", fmt.Errorf("run canceled: %w", ctx.Err())
	}
}

func TestLabRunStreamsLiveOutput(t *testing.T) {
	lab := &fakeLab{release: make(chan struct{}), started: make(chan struct{})}
	a := newApp(t, options{AsyncRuns: true, Lab: lab})

	resp, err := http.Post(a.TS.URL+"/modules/02-test-lab/steps/01-submit/submit-lab", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit-lab: status %d", resp.StatusCode)
	}
	id := latestSubmissionID(t, a, "02-test-lab", "01-submit")

	stream, err := http.Get(a.TS.URL + "/submissions/" + strconvItoa(id) + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()

	<-lab.started
	close(lab.release)

	chunks, done := readStream(t, stream.Body, nil)
	if !done {
		t.Fatal("stream did not end with done")
	}
	if !strings.Contains(chunks, "chunk-1") {
		t.Fatalf("streamed output missing chunk: %q", chunks)
	}
}

func TestLabRunCancelMidRun(t *testing.T) {
	lab := &fakeLab{release: make(chan struct{}), started: make(chan struct{})}
	a := newApp(t, options{AsyncRuns: true, Lab: lab})

	resp, err := http.Post(a.TS.URL+"/modules/02-test-lab/steps/01-submit/submit-lab", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	id := latestSubmissionID(t, a, "02-test-lab", "01-submit")
	idStr := strconvItoa(id)

	stream, err := http.Get(a.TS.URL + "/submissions/" + idStr + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()

	<-lab.started

	cancelResp, err := http.Post(a.TS.URL+"/submissions/"+idStr+"/cancel", "", nil)
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
}

var _ eval.LabRepo = (*fakeLab)(nil)
