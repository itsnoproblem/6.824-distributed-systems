package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestBrowseAndComplete(t *testing.T) {
	app := newApp(t, options{})

	code, body := get(t, app.TS.URL+"/")
	if code != 200 || !strings.Contains(body, "Test Lecture") || !strings.Contains(body, "Test Lab") {
		t.Fatalf("course map: %d %q", code, body)
	}

	code, body = get(t, app.TS.URL+"/modules/01-test-lecture/steps/01-read")
	if code != 200 || !strings.Contains(body, "Test reading body") {
		t.Fatalf("step page: %d", code)
	}
	if !strings.Contains(body, "/notes/drawer?module=01-test-lecture") {
		t.Error("notes drawer container missing")
	}
	if !strings.Contains(body, "Next →") {
		t.Error("next nav missing")
	}

	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/01-read/complete",
		url.Values{"done": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Completed") {
		t.Fatalf("toggle response: %q", string(b))
	}

	_, body = get(t, app.TS.URL+"/")
	if !strings.Contains(body, "1/2") {
		t.Fatalf("expected 1/2 module progress on map, got: %q", body)
	}

	code, _ = get(t, app.TS.URL+"/modules/nope/steps/nah")
	if code != 404 {
		t.Fatalf("unknown step = %d, want 404", code)
	}
}

func TestAttributionPage(t *testing.T) {
	app := newApp(t, options{})
	code, body := get(t, app.TS.URL+"/attribution")
	if code != 200 || !strings.Contains(body, "Attribution") {
		t.Fatalf("attribution page: %d", code)
	}
}
