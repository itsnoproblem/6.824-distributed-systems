package eval_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

func TestSnapshot(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	files, err := repo.Snapshot("src/hello", []string{"*.go"})
	if err != nil {
		t.Fatal(err)
	}
	src, ok := files["src/hello/hello_test.go"]
	if !ok || !strings.Contains(src, "TestAlwaysPasses") {
		t.Fatalf("files: %v", files)
	}
}

func TestRunTestsPasses(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	out, err := repo.RunTests(context.Background(), "src/hello",
		[]string{"go", "test"}, time.Minute)
	if err != nil {
		t.Fatalf("err = %v, out = %q", err, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("out = %q", out)
	}
}

func TestRunTestsTimeout(t *testing.T) {
	repo := eval.FSLabRepo{Dir: "testdata/fakerepo"}
	_, err := repo.RunTests(context.Background(), "src/hello",
		[]string{"sleep", "5"}, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}
