package openrouter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/openrouter"
)

func TestComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth = %q", got)
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "test/model" {
			t.Errorf("model = %v", req["model"])
		}
		msgs := req["messages"].([]any)
		if len(msgs) != 2 {
			t.Errorf("messages = %d", len(msgs))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "the verdict"}}},
		})
	}))
	defer srv.Close()

	c := openrouter.New("test-key", "test/model")
	c.BaseURL = srv.URL
	out, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil || out != "the verdict" {
		t.Fatalf("got %q %v", out, err)
	}
	if c.Model() != "test/model" {
		t.Fatalf("Model() = %q", c.Model())
	}
}

func TestCompleteErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := openrouter.New("k", "m")
	c.BaseURL = srv.URL
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil ||
		!strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v", err)
	}
}
