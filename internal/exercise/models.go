// Package exercise is the interactive coding-exercise feature: in-browser
// drafts, throwaway workspaces, go-test validation.
package exercise

import (
	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

// Diagnostic is a single gofmt/go vet finding surfaced to the editor.
type Diagnostic struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Message string `json:"message"`
}

// FileView is one file in the editor: either the student's editable source
// (scaffold, overlaid with any draft) or a readonly scaffold file.
type FileView struct {
	Name     string
	Content  string
	Readonly bool
}

// View is everything the editor needs to render a code step.
type View struct {
	Meta       *course.CodeMeta
	Step       course.Step
	Files      []FileView // editable first, then readonly, each group in meta order
	HasDraft   bool
	Submission *eval.Submission // latest exercise run, nil if none
}
