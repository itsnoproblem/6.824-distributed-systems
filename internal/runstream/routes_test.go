package runstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// fakeRunService scripts Watch/Cancel outcomes and records calls.
type fakeRunService struct {
	watchEvents []Event
	watchErr    error
	cancelErr   error
	canceledID  int64
	watchedID   int64
}

func (f *fakeRunService) Watch(ctx context.Context, id int64) (<-chan Event, error) {
	f.watchedID = id
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	ch := make(chan Event, len(f.watchEvents))
	for _, ev := range f.watchEvents {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (f *fakeRunService) Cancel(ctx context.Context, id int64) error {
	f.canceledID = id
	return f.cancelErr
}

func newRoutesServer(t *testing.T, svc RunService) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	RegisterRunRoutes(mux, "/things/submissions", svc)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestRunRoutesStreamServesSSE(t *testing.T) {
	svc := &fakeRunService{watchEvents: []Event{{Kind: KindChunk, Data: "hi"}, {Kind: KindDone}}}
	ts := newRoutesServer(t, svc)
	resp, err := http.Get(ts.URL + "/things/submissions/42/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "event: chunk\ndata: \"hi\"") {
		t.Fatalf("stream body:\n%s", body)
	}
	if svc.watchedID != 42 {
		t.Fatalf("watched id = %d", svc.watchedID)
	}
}

func TestRunRoutesStreamErrors(t *testing.T) {
	svc := &fakeRunService{watchErr: api.ErrNotFound}
	ts := newRoutesServer(t, svc)
	resp, err := http.Get(ts.URL + "/things/submissions/42/stream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRunRoutesStreamRejectsBadID(t *testing.T) {
	ts := newRoutesServer(t, &fakeRunService{})
	resp, err := http.Get(ts.URL + "/things/submissions/nope/stream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRunRoutesCancel(t *testing.T) {
	svc := &fakeRunService{}
	ts := newRoutesServer(t, svc)
	resp, err := http.Post(ts.URL+"/things/submissions/7/cancel", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if svc.canceledID != 7 {
		t.Fatalf("canceled id = %d", svc.canceledID)
	}
}

func TestRunRoutesCancelErrors(t *testing.T) {
	svc := &fakeRunService{cancelErr: errors.Join(api.ErrInvalid, errors.New("no live run"))}
	ts := newRoutesServer(t, svc)
	resp, err := http.Post(ts.URL+"/things/submissions/7/cancel", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
