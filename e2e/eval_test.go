package e2e

import (
	"context"
	"errors"
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

type fakeLLM struct {
	resp string
	err  error
}

func (f fakeLLM) Complete(_ context.Context, _, _ string) (string, error) { return f.resp, f.err }
func (f fakeLLM) Model() string                                           { return "fake/model" }

const goodVerdict = "```json\n" +
	`{"criteria":[{"name":"Correctness","score":4,"justification":"solid"}],` +
	`"summary":"Good answer.","next_steps":["reread the failure model section"]}` + "\n```"

func TestQuestionEvaluated(t *testing.T) {
	app := newApp(t, options{LLM: fakeLLM{resp: goodVerdict}})
	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/02-question/answer",
		url.Values{"answer": {"my considered answer"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	for _, want := range []string{"Correctness", "4/5", "Good answer.", "fake/model", "rubric v1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("report missing %q: %q", want, body)
		}
	}
}

func TestLLMFailureIsRecordedAndRetryable(t *testing.T) {
	app := newApp(t, options{LLM: fakeLLM{err: errors.New("boom")}})
	resp, err := http.PostForm(app.TS.URL+"/modules/01-test-lecture/steps/02-question/answer",
		url.Values{"answer": {"attempt"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Evaluation failed") || !strings.Contains(string(b), "/retry") {
		t.Fatalf("failure UI: %q", string(b))
	}
	// retry with a still-failing LLM stays failed but responds 200
	resp, err = http.Post(app.TS.URL+"/submissions/1/retry", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "Evaluation failed") {
		t.Fatalf("retry: %d %q", resp.StatusCode, string(b))
	}
}
