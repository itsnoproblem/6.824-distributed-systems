package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func fetch(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestQuestionLockedMode(t *testing.T) {
	app := newApp(t, options{})

	body := fetch(t, app.TS.URL+"/eval/section?module=01-test-lecture&step=02-question")
	if !strings.Contains(body, "locked") || !strings.Contains(body, "What is a distributed system?") {
		t.Fatalf("section: %q", body)
	}

	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/02-question/answer",
		url.Values{"answer": {"machines that fail independently"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Answer saved") {
		t.Fatalf("after answer: %q", string(b))
	}

	// answer survives reload, prefilled in the form
	body = fetch(t, app.TS.URL+"/eval/section?module=01-test-lecture&step=02-question")
	if !strings.Contains(body, "machines that fail independently") {
		t.Fatal("answer not prefilled")
	}

	// answering auto-completed the step
	body = fetch(t, app.TS.URL+"/modules/01-test-lecture/steps/02-question")
	if !strings.Contains(body, "Completed") {
		t.Fatal("step not auto-completed")
	}
}
