package openrouter_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/openrouter"
)

// TestLiveComplete hits the real API. Run manually:
//
//	OPENROUTER_LIVE=1 OPENROUTER_API_KEY=... go test ./internal/openrouter/ -run Live -v
func TestLiveComplete(t *testing.T) {
	if os.Getenv("OPENROUTER_LIVE") != "1" {
		t.Skip("set OPENROUTER_LIVE=1 and OPENROUTER_API_KEY to run")
	}
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Fatal("OPENROUTER_API_KEY is required with OPENROUTER_LIVE=1")
	}
	c := openrouter.New(key, "anthropic/claude-sonnet-4")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := c.Complete(ctx, "Reply with exactly the word: pong", "ping")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("live response: %q", out)
	if out == "" {
		t.Fatal("empty response")
	}
}
