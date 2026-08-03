package exercise

import (
	"fmt"
	"sort"
	"strings"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

const verdictShape = `{"criteria":[{"name":"<criterion>","score":<1-5>,"justification":"<why>"}],` +
	`"summary":"<2-3 sentences>","next_steps":["<concrete action>"]}`

// BuildExercisePrompt assembles the feedback prompt for a completed
// exercise run. passed reports whether the tests were green.
func BuildExercisePrompt(r eval.Rubric, mod course.Module, step course.Step,
	files map[string]string, testOutput string, passed bool) (system, user string) {
	system = "You are a strict but constructive teaching assistant for MIT 6.824 " +
		"(Distributed Systems). Review a student's short coding exercise. Respond with " +
		"ONLY a JSON object in exactly this shape:\n" + verdictShape +
		"\n\nRubric (version " + r.Version + "):\n" + r.Body
	state := "tests are failing"
	if passed {
		state = "tests are passing"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Module: %s — %s (%s)\n\nTest output:\n%s\n\nStudent code:\n",
		mod.Title, step.Title, state, testOutput)
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&b, "\n--- %s ---\n%s\n", p, files[p])
	}
	return system, b.String()
}
