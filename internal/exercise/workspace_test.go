package exercise_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
)

func adderMeta() *course.CodeMeta {
	return &course.CodeMeta{
		Mode:     "fix",
		Editable: []string{"adder.go"},
		Readonly: []string{"adder_test.go"},
		Run:      []string{"go", "test", "."},
		Timeout:  time.Minute,
		Files: map[string]string{
			"adder.go":      "package adder\n\nfunc Add(a, b int) int { return a - b }\n",
			"adder_test.go": "package adder\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(Add(2, 3))\n\t}\n}\n",
		},
	}
}

func TestRunExerciseScaffoldFails(t *testing.T) {
	out, code, err := exercise.Workspace{}.RunExercise(context.Background(), adderMeta(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 || !strings.Contains(out, "FAIL") {
		t.Fatalf("buggy scaffold should fail: code=%d out=%q", code, out)
	}
}

func TestRunExerciseDraftOverlayPasses(t *testing.T) {
	draft := map[string]string{"adder.go": "package adder\n\nfunc Add(a, b int) int { return a + b }\n"}
	out, code, err := exercise.Workspace{}.RunExercise(context.Background(), adderMeta(), draft, nil)
	if err != nil || code != 0 {
		t.Fatalf("fixed draft should pass: code=%d err=%v out=%q", code, err, out)
	}
}

func TestRunExerciseIgnoresNonEditableOverlay(t *testing.T) {
	// overlaying the read-only test file must not take effect
	draft := map[string]string{
		"adder_test.go": "package adder\n", // would delete the test
	}
	_, code, err := exercise.Workspace{}.RunExercise(context.Background(), adderMeta(), draft, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Fatal("read-only overlay must be ignored; scaffold bug should still fail")
	}
}

func TestCheckExerciseReportsSyntaxAndVet(t *testing.T) {
	broken := map[string]string{"adder.go": "package adder\n\nfunc Add(a, b int) int { return a +\n"}
	diags, err := exercise.Workspace{}.CheckExercise(context.Background(), adderMeta(), broken)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 || diags[0].File != "adder.go" || diags[0].Line == 0 {
		t.Fatalf("diags = %+v", diags)
	}
	clean := map[string]string{"adder.go": "package adder\n\nfunc Add(a, b int) int { return a + b }\n"}
	diags, err = exercise.Workspace{}.CheckExercise(context.Background(), adderMeta(), clean)
	if err != nil || len(diags) != 0 {
		t.Fatalf("clean code: diags=%+v err=%v", diags, err)
	}
}

// TestCheckExerciseGofmtBareFilenameProducesDiagnostic covers controller
// resolution 3: `gofmt -l` reports a mis-formatted-but-parseable file as a
// bare filename (no "file:line:col:" prefix). That must still surface as a
// diagnostic, not be silently dropped by the file:line:col regex.
func TestCheckExerciseGofmtBareFilenameProducesDiagnostic(t *testing.T) {
	unformatted := map[string]string{
		"adder.go": "package adder\n\nfunc   Add(a, b int) int { return a+b }\n",
	}
	diags, err := exercise.Workspace{}.CheckExercise(context.Background(), adderMeta(), unformatted)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 {
		t.Fatalf("diags = %+v, want exactly one gofmt-formatting diagnostic", diags)
	}
	d := diags[0]
	if d.File != "adder.go" || d.Line != 1 || d.Col != 1 || !strings.Contains(d.Message, "gofmt") {
		t.Fatalf("diag = %+v", d)
	}
}

// TestCheckExerciseRunnerFailureNeverErrors covers controller resolution 1:
// when execx.Run itself fails while running gofmt/go vet (e.g. a spawn
// failure or a check timeout), CheckExercise must still return (diags, nil)
// — never propagate a Go error, which would surface as a 500 at the
// transport layer for something that is entirely about the student's code
// (or the check environment) rather than the app itself.
func TestCheckExerciseRunnerFailureNeverErrors(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	diags, err := exercise.Workspace{}.CheckExercise(context.Background(), adderMeta(), nil)
	if err != nil {
		t.Fatalf("CheckExercise must never return an error for a tool-run failure, got: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("diags = %+v, want a single file-level diagnostic describing the failure", diags)
	}
	d := diags[0]
	if d.File != "adder.go" || d.Line != 1 || d.Col != 1 || !strings.Contains(d.Message, "check failed") {
		t.Fatalf("diag = %+v", d)
	}
}
