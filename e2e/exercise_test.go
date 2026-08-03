package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func put(t *testing.T, url, body string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func post(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestExerciseFlow(t *testing.T) {
	app := newApp(t, options{})
	base := app.TS.URL + "/exercises/03-test-code/01-fix"

	// editor section renders scaffold, config, and both tabs' content
	body := fetch(t, base)
	for _, want := range []string{"exercise-root", "data-config", "saveUrl", "adder_test.go", "Fix the bug"} {
		if !strings.Contains(body, want) {
			t.Fatalf("section missing %q", want)
		}
	}

	// run the buggy scaffold: completes with failing tests
	if _, out := post(t, base+"/run"); !strings.Contains(out, "Tests failing") {
		t.Fatalf("buggy run: %q", out)
	}

	// fix it via a draft, then run again: passes and completes the step
	if code := put(t, base+"/draft",
		`{"files":{"adder.go":"package adder\n\nfunc Add(a, b int) int { return a + b }\n"}}`); code != 204 {
		t.Fatalf("draft save = %d", code)
	}
	if _, out := post(t, base+"/run"); !strings.Contains(out, "Tests pass") {
		t.Fatalf("fixed run: %q", out)
	}
	if !strings.Contains(fetch(t, app.TS.URL+"/modules/03-test-code/steps/01-fix"), "Completed") {
		t.Fatal("passing run should auto-complete the step")
	}

	// polling endpoint serves the latest state
	if code, out := func() (int, string) {
		resp, err := http.Get(app.TS.URL + "/exercises/submissions/2/status")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}(); code != 200 || !strings.Contains(out, "Tests pass") {
		t.Fatalf("status: %d %q", code, out)
	}
}

func TestExerciseCheckDiagnostics(t *testing.T) {
	app := newApp(t, options{})
	base := app.TS.URL + "/exercises/03-test-code/01-fix"

	if code := put(t, base+"/draft",
		`{"files":{"adder.go":"package adder\n\nfunc Add(a, b int) int { return a +\n"}}`); code != 204 {
		t.Fatalf("draft save = %d", code)
	}
	code, out := post(t, base+"/check")
	if code != 200 || !strings.Contains(out, `"adder.go"`) || !strings.Contains(out, `"line"`) {
		t.Fatalf("check: %d %q", code, out)
	}

	// draft touching a read-only file is rejected
	if code := put(t, base+"/draft", `{"files":{"adder_test.go":"package adder\n"}}`); code != 400 {
		t.Fatalf("read-only draft = %d, want 400", code)
	}
}

func TestVideoEmbedOnStepPage(t *testing.T) {
	app := newApp(t, options{})
	body := fetch(t, app.TS.URL+"/modules/03-test-code/steps/01-fix")
	if !strings.Contains(body, "youtube-nocookie.com/embed/testvideo1") ||
		!strings.Contains(body, "Watch on YouTube") {
		t.Fatal("video embed missing")
	}
}
