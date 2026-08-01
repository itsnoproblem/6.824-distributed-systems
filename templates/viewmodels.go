// Package templates renders every page and partial. It depends on nothing in
// internal/ — feature transports map their view types into these VMs.
package templates

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
}

func StepURL(module, step string) string { return "/modules/" + module + "/steps/" + step }
