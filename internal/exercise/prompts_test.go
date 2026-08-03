package exercise_test

import (
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
)

func TestBuildExercisePrompt(t *testing.T) {
	r := eval.Rubric{Version: "1", Body: "EX-RUBRIC"}
	step := course.Step{Title: "Fix adder", Attribution: "adapted"}
	system, user := exercise.BuildExercisePrompt(r, course.Module{Title: "Lecture X"}, step,
		map[string]string{"b.go": "package b", "a.go": "package a"}, "TEST-OUT", true)
	if !strings.Contains(system, "EX-RUBRIC") || !strings.Contains(system, `"criteria"`) {
		t.Fatalf("system: %q", system)
	}
	ai, bi := strings.Index(user, "--- a.go ---"), strings.Index(user, "--- b.go ---")
	if ai < 0 || bi < 0 || ai > bi || !strings.Contains(user, "TEST-OUT") ||
		!strings.Contains(user, "tests are passing") {
		t.Fatalf("user: %q", user)
	}
}
