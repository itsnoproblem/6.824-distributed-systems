package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNotesFlow(t *testing.T) {
	app := newApp(t, options{})

	// add a note from a step
	resp, err := http.PostForm(app.TS.URL+"/notes", url.Values{
		"module": {"01-test-lecture"}, "step": {"01-read"}, "body": {"remember the shuffle"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "remember the shuffle") {
		t.Fatalf("drawer after add: %q", string(b))
	}

	// drawer partial serves it back
	resp, _ = http.Get(app.TS.URL + "/notes/drawer?module=01-test-lecture&step=01-read")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "remember the shuffle") {
		t.Fatal("drawer GET missing note")
	}

	// index groups under the module title
	resp, _ = http.Get(app.TS.URL + "/notes")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	if !strings.Contains(body, "Test Lecture") || !strings.Contains(body, "remember the shuffle") {
		t.Fatalf("index: %q", body)
	}

	// validation surfaces as 400
	resp, _ = http.PostForm(app.TS.URL+"/notes", url.Values{
		"module": {"01-test-lecture"}, "step": {"01-read"}, "body": {"  "},
	})
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("empty body = %d, want 400", resp.StatusCode)
	}

	// editing a nonexistent note surfaces as 404, not 500
	resp, err = http.Get(app.TS.URL + "/notes/99999/edit")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("missing note edit = %d, want 404", resp.StatusCode)
	}
}
