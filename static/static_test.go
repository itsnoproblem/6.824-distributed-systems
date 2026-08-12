package static_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/static"
)

func TestEmbedsCodeMirrorBundle(t *testing.T) {
	data, err := static.FS.ReadFile("codemirror/codemirror.js")
	if err != nil {
		t.Fatalf("codemirror/codemirror.js not embedded: %v", err)
	}
	if len(data) < 1024 {
		t.Fatalf("codemirror/codemirror.js embedded but suspiciously small: %d bytes", len(data))
	}
}

func TestEmbedsExerciseJS(t *testing.T) {
	data, err := static.FS.ReadFile("exercise.js")
	if err != nil {
		t.Fatalf("exercise.js not embedded: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("exercise.js embedded but empty")
	}
}

func TestEmbedsRunstreamJS(t *testing.T) {
	data, err := static.FS.ReadFile("runstream.js")
	if err != nil {
		t.Fatalf("runstream.js not embedded: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("runstream.js embedded but empty")
	}
}
