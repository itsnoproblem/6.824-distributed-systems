package course_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

func fixture() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Module One", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "s1", Title: "Step 1", Type: course.StepReading},
			{Slug: "s2", Title: "Step 2", Type: course.StepQuestion, Question: "Why?"},
		}},
		{Slug: "m2", Title: "Module Two", Kind: course.KindLab, Order: 2, Steps: []course.Step{
			{Slug: "s1", Title: "Lab step", Type: course.StepSubmit},
		}},
	}}
}

func TestStepLookup(t *testing.T) {
	c := fixture()
	mod, step, ok := c.Step(course.StepRef{Module: "m1", Step: "s2"})
	if !ok || mod.Slug != "m1" || step.Question != "Why?" {
		t.Fatalf("lookup failed: %v %v %v", mod, step, ok)
	}
	if _, _, ok := c.Step(course.StepRef{Module: "m1", Step: "nope"}); ok {
		t.Fatal("expected miss")
	}
}

func TestNextCrossesModules(t *testing.T) {
	c := fixture()
	next, ok := c.Next(course.StepRef{Module: "m1", Step: "s2"})
	if !ok || next != (course.StepRef{Module: "m2", Step: "s1"}) {
		t.Fatalf("next = %v %v", next, ok)
	}
	if _, ok := c.Next(course.StepRef{Module: "m2", Step: "s1"}); ok {
		t.Fatal("expected no next at course end")
	}
}

func TestPrevCrossesModules(t *testing.T) {
	c := fixture()
	prev, ok := c.Prev(course.StepRef{Module: "m2", Step: "s1"})
	if !ok || prev != (course.StepRef{Module: "m1", Step: "s2"}) {
		t.Fatalf("prev = %v %v", prev, ok)
	}
	if _, ok := c.Prev(course.StepRef{Module: "m1", Step: "s1"}); ok {
		t.Fatal("expected no prev at course start")
	}
}

func TestTotalSteps(t *testing.T) {
	if got := fixture().TotalSteps(); got != 3 {
		t.Fatalf("TotalSteps = %d, want 3", got)
	}
}
