// Package templates renders every page and partial. It depends on nothing in
// internal/ — feature transports map their view types into these VMs.
package templates

import (
	"net/url"
	"strings"
)

type CourseMapVM struct {
	Groups      []KindGroupVM
	Done, Total int
}

type KindGroupVM struct {
	Label   string
	Modules []ModuleCardVM
}

type ModuleCardVM struct {
	Slug, Title  string
	Done, Total  int
	FirstStepURL string
}

type StepVM struct {
	ModuleSlug, StepSlug       string
	ModuleTitle, Title         string
	Type                       string
	BodyHTML                   string
	Completed                  bool
	PrevURL, NextURL           string
	Index, Total               int
	PaperURL, LabURL, VideoURL string
	VideoWatchURL              string
	VideoEmbedURL              string
}

func StepURL(module, step string) string { return "/modules/" + module + "/steps/" + step }

type NoteVM struct {
	ID                   int64
	ModuleSlug, StepSlug string
	Body                 string
	CreatedAt            string
}

type NotesDrawerVM struct {
	ModuleSlug, StepSlug string
	Notes                []NoteVM
}

type ModuleNotesVM struct {
	ModuleTitle string
	Notes       []NoteVM
}

type NotesIndexVM struct{ Groups []ModuleNotesVM }

type CriterionVM struct {
	Name          string
	Score         int
	Justification string
}

type ReportVM struct {
	Model, RubricVersion, Summary string
	Criteria                      []CriterionVM
	NextSteps                     []string
}

type EvalSectionVM struct {
	ModuleSlug, StepSlug string
	Type                 string
	Enabled              bool
	Question             string
	Answer               string
	Status               string
	SubmissionID         int64
	TestOutput           string
	Report               *ReportVM
}

type ExerciseFileVM struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	Readonly bool   `json:"readonly"`
}

type ExerciseVM struct {
	ModuleSlug, StepSlug string
	Mode, ModeLabel      string // "fix"/"create", "Fix the bug"/"Build it"
	Attribution          string
	ConfigJSON           string
	Files                []ExerciseFileVM
	Status               string
	Passed               bool
	Output               string
	SubmissionID         int64
	FeedbackEnabled      bool
	Report               *ReportVM
}

// YouTubeEmbedURL converts a YouTube watch URL into a privacy-enhanced
// embed URL, or returns "" for anything it does not recognize (the caller
// then renders only the plain link).
func YouTubeEmbedURL(watch string) string {
	u, err := url.Parse(watch)
	if err != nil {
		return ""
	}
	var id string
	switch {
	case strings.HasSuffix(u.Host, "youtube.com"):
		id = u.Query().Get("v")
	case u.Host == "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	}
	if id == "" {
		return ""
	}
	return "https://www.youtube-nocookie.com/embed/" + id
}
