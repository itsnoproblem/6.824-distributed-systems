// Package eval is the evaluation feature: submissions of lab code and
// reading-question answers, optionally reviewed by an LLM against a rubric.
package eval

import (
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type Kind string

const (
	KindLab      Kind = "lab"
	KindQuestion Kind = "question"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
)

type Submission struct {
	ID         int64
	Ref        course.StepRef
	Kind       Kind
	Content    string // answer text, or JSON map[path]source for labs
	TestOutput string
	Status     Status
	CreatedAt  time.Time
}

type Criterion struct {
	Name          string `json:"name"`
	Score         int    `json:"score"`
	Justification string `json:"justification"`
}

type Verdict struct {
	Criteria  []Criterion `json:"criteria"`
	Summary   string      `json:"summary"`
	NextSteps []string    `json:"next_steps"`
}

type Evaluation struct {
	ID            int64
	SubmissionID  int64
	Model         string
	RubricVersion string
	Verdict       Verdict
	CreatedAt     time.Time
}
