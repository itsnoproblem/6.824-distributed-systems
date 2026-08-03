package coursefs_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
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

	mod, _ := c.Module("02-rpc-and-threads")
	var codeSteps int
	for _, s := range mod.Steps {
		if s.Type == course.StepCode {
			codeSteps++
		}
	}
	if codeSteps != 2 {
		t.Errorf("lecture 2 code steps = %d, want 2", codeSteps)
	}
	if lab, _ := c.Module("lab-01-mapreduce"); len(lab.Steps) != 4 {
		t.Errorf("lab 1 steps = %d, want 4 (incl. warm-up)", len(lab.Steps))
	}
	if _, step, ok := c.Step(course.StepRef{Module: "01-introduction", Step: "01-read-the-paper"}); !ok || step.Video == "" {
		t.Error("lecture 1 read step should carry a video URL")
	}
	if _, step, ok := c.Step(course.StepRef{Module: "02-rpc-and-threads", Step: "01-read-the-paper"}); !ok || step.Video == "" {
		t.Error("lecture 2 read step should carry a video URL")
	}
}
