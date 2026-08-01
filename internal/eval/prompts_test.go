package eval_test

import (
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

func TestBuildQuestionPrompt(t *testing.T) {
	r := eval.Rubric{Version: "1", Body: "RUBRIC-BODY"}
	mod := course.Module{Title: "Lecture X"}
	step := course.Step{Question: "Why is the sky blue?"}
	system, user := eval.BuildQuestionPrompt(r, mod, step, "scattering")
	if !strings.Contains(system, "RUBRIC-BODY") || !strings.Contains(system, `"criteria"`) {
		t.Fatalf("system: %q", system)
	}
	for _, want := range []string{"Lecture X", "Why is the sky blue?", "scattering"} {
		if !strings.Contains(user, want) {
			t.Fatalf("user missing %q: %q", want, user)
		}
	}
}

func TestBuildLabPrompt(t *testing.T) {
	r := eval.Rubric{Version: "1", Body: "LAB-RUBRIC"}
	system, user := eval.BuildLabPrompt(r, "GUIDANCE-TEXT",
		course.Module{Title: "Lab X"}, course.Step{Title: "Submit"},
		map[string]string{"b.go": "package b", "a.go": "package a"}, "TEST-OUT")
	if !strings.Contains(system, "LAB-RUBRIC") || !strings.Contains(system, "GUIDANCE-TEXT") {
		t.Fatalf("system: %q", system)
	}
	// files appear sorted with path headers
	ai := strings.Index(user, "--- a.go ---")
	bi := strings.Index(user, "--- b.go ---")
	if ai < 0 || bi < 0 || ai > bi || !strings.Contains(user, "TEST-OUT") {
		t.Fatalf("user: %q", user)
	}
}
