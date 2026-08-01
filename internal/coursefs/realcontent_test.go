package coursefs_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
)

// TestRealContentParses guards the actual content tree: any authoring error
// that would crash boot fails this test first.
func TestRealContentParses(t *testing.T) {
	c, err := coursefs.Load("../../content/modules")
	if err != nil {
		t.Fatal(err)
	}
	// 22 lectures + 5 labs + 1 project
	if len(c.Modules) != 28 {
		t.Fatalf("expected 28 modules, got %d", len(c.Modules))
	}
	if _, ok := c.Module("01-introduction"); !ok {
		t.Error("missing module 01-introduction")
	}
	if _, ok := c.Module("lab-01-mapreduce"); !ok {
		t.Error("missing module lab-01-mapreduce")
	}
}
