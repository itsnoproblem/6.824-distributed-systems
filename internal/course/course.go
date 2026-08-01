// Package course defines the immutable domain model of the guided course.
// It has no I/O; internal/coursefs builds these values from content files.
package course

import "time"

type Kind string

const (
	KindLecture Kind = "lecture"
	KindLab     Kind = "lab"
	KindProject Kind = "project"
)

type StepType string

const (
	StepReading  StepType = "reading"
	StepQuestion StepType = "question"
	StepExercise StepType = "exercise"
	StepSubmit   StepType = "submit"
)

// StepRef is the canonical identity of a step across the whole app.
type StepRef struct{ Module, Step string }

func (r StepRef) String() string { return r.Module + "/" + r.Step }

type Links struct{ Paper, Lab, Video string }

// EvalMeta configures lab evaluation for a submit step, relative to the lab repo root.
type EvalMeta struct {
	Workdir string
	Globs   []string
	TestCmd []string
	Timeout time.Duration
}

type Step struct {
	Slug, Title string
	Type        StepType
	BodyHTML    string
	Question    string
	Eval        *EvalMeta
}

type Module struct {
	Slug, Title string
	Kind        Kind
	Order       int
	Links       Links
	Steps       []Step
}

// Course holds modules sorted by Order ascending.
type Course struct{ Modules []Module }

func (c *Course) Module(slug string) (*Module, bool) {
	for i := range c.Modules {
		if c.Modules[i].Slug == slug {
			return &c.Modules[i], true
		}
	}
	return nil, false
}

func (c *Course) Step(ref StepRef) (*Module, *Step, bool) {
	mod, ok := c.Module(ref.Module)
	if !ok {
		return nil, nil, false
	}
	for i := range mod.Steps {
		if mod.Steps[i].Slug == ref.Step {
			return mod, &mod.Steps[i], true
		}
	}
	return nil, nil, false
}

// flat returns every step in course order.
func (c *Course) flat() []StepRef {
	var refs []StepRef
	for _, m := range c.Modules {
		for _, s := range m.Steps {
			refs = append(refs, StepRef{Module: m.Slug, Step: s.Slug})
		}
	}
	return refs
}

func (c *Course) Next(ref StepRef) (StepRef, bool) {
	refs := c.flat()
	for i, r := range refs {
		if r == ref && i+1 < len(refs) {
			return refs[i+1], true
		}
	}
	return StepRef{}, false
}

func (c *Course) Prev(ref StepRef) (StepRef, bool) {
	refs := c.flat()
	for i, r := range refs {
		if r == ref && i > 0 {
			return refs[i-1], true
		}
	}
	return StepRef{}, false
}

func (c *Course) TotalSteps() int { return len(c.flat()) }
