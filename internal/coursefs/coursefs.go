// Package coursefs loads the course from a content directory tree:
// <dir>/<module-slug>/{module.yaml,steps/*.md}. It is the file-backed
// implementation of every feature's CourseRepo interface.
package coursefs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type moduleYAML struct {
	Title string `yaml:"title"`
	Kind  string `yaml:"kind"`
	Order int    `yaml:"order"`
	Links struct {
		Paper string `yaml:"paper"`
		Lab   string `yaml:"lab"`
		Video string `yaml:"video"`
	} `yaml:"links"`
}

type stepYAML struct {
	Title    string `yaml:"title"`
	Type     string `yaml:"type"`
	Question string `yaml:"question"`
	Eval     *struct {
		Workdir string   `yaml:"workdir"`
		Globs   []string `yaml:"globs"`
		TestCmd []string `yaml:"test_cmd"`
		Timeout string   `yaml:"timeout"`
	} `yaml:"eval"`
}

var validKinds = map[string]course.Kind{
	"lecture": course.KindLecture, "lab": course.KindLab, "project": course.KindProject,
}

var validTypes = map[string]course.StepType{
	"reading": course.StepReading, "question": course.StepQuestion,
	"exercise": course.StepExercise, "submit": course.StepSubmit,
}

// Load parses every module under dir and returns the assembled course.
func Load(dir string) (*course.Course, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read content dir %s: %w", dir, err)
	}
	var modules []course.Module
	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if seen[e.Name()] {
			return nil, fmt.Errorf("duplicate module slug %q", e.Name())
		}
		seen[e.Name()] = true
		m, err := loadModule(filepath.Join(dir, e.Name()), e.Name())
		if err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no modules found under %s", dir)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Order < modules[j].Order })
	return &course.Course{Modules: modules}, nil
}

func loadModule(dir, slug string) (course.Module, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "module.yaml"))
	if err != nil {
		return course.Module{}, fmt.Errorf("%s: %w", dir, err)
	}
	var my moduleYAML
	if err := yaml.Unmarshal(raw, &my); err != nil {
		return course.Module{}, fmt.Errorf("%s/module.yaml: %w", dir, err)
	}
	kind, ok := validKinds[my.Kind]
	if !ok {
		return course.Module{}, fmt.Errorf("%s/module.yaml: invalid kind %q", dir, my.Kind)
	}
	if my.Title == "" {
		return course.Module{}, fmt.Errorf("%s/module.yaml: title is required", dir)
	}
	if my.Order <= 0 {
		return course.Module{}, fmt.Errorf("%s/module.yaml: order must be > 0", dir)
	}
	stepFiles, err := filepath.Glob(filepath.Join(dir, "steps", "*.md"))
	if err != nil || len(stepFiles) == 0 {
		return course.Module{}, fmt.Errorf("%s: no step files (%v)", dir, err)
	}
	sort.Strings(stepFiles)
	var steps []course.Step
	seenSteps := map[string]bool{}
	for _, f := range stepFiles {
		s, err := loadStep(f)
		if err != nil {
			return course.Module{}, err
		}
		if seenSteps[s.Slug] {
			return course.Module{}, fmt.Errorf("%s: duplicate step slug %q", dir, s.Slug)
		}
		seenSteps[s.Slug] = true
		steps = append(steps, s)
	}
	return course.Module{
		Slug: slug, Title: my.Title, Kind: kind, Order: my.Order,
		Links: course.Links{Paper: my.Links.Paper, Lab: my.Links.Lab, Video: my.Links.Video},
		Steps: steps,
	}, nil
}

func loadStep(path string) (course.Step, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return course.Step{}, fmt.Errorf("%s: %w", path, err)
	}
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return course.Step{}, fmt.Errorf("%s: %w", path, err)
	}
	var sy stepYAML
	if err := yaml.Unmarshal(fm, &sy); err != nil {
		return course.Step{}, fmt.Errorf("%s: %w", path, err)
	}
	typ, ok := validTypes[sy.Type]
	if !ok {
		return course.Step{}, fmt.Errorf("%s: invalid step type %q", path, sy.Type)
	}
	if sy.Title == "" {
		return course.Step{}, fmt.Errorf("%s: title is required", path)
	}
	if typ == course.StepQuestion && strings.TrimSpace(sy.Question) == "" {
		return course.Step{}, fmt.Errorf("%s: question steps require a question", path)
	}
	step := course.Step{
		Slug:     strings.TrimSuffix(filepath.Base(path), ".md"),
		Title:    sy.Title,
		Type:     typ,
		Question: strings.TrimSpace(sy.Question),
	}
	if typ == course.StepSubmit {
		if sy.Eval == nil {
			return course.Step{}, fmt.Errorf("%s: submit steps require an eval block", path)
		}
		if sy.Eval.Workdir == "" || len(sy.Eval.Globs) == 0 || len(sy.Eval.TestCmd) == 0 {
			return course.Step{}, fmt.Errorf("%s: eval requires workdir, globs, test_cmd", path)
		}
		timeout, err := time.ParseDuration(sy.Eval.Timeout)
		if err != nil {
			return course.Step{}, fmt.Errorf("%s: eval.timeout: %w", path, err)
		}
		step.Eval = &course.EvalMeta{
			Workdir: sy.Eval.Workdir, Globs: sy.Eval.Globs,
			TestCmd: sy.Eval.TestCmd, Timeout: timeout,
		}
	}
	var buf bytes.Buffer
	if err := goldmark.New().Convert(body, &buf); err != nil {
		return course.Step{}, fmt.Errorf("%s: render markdown: %w", path, err)
	}
	step.BodyHTML = buf.String()
	return step, nil
}

// Repo is the injectable wrapper satisfying each feature's CourseRepo interface.
type Repo struct{ c *course.Course }

func NewRepo(c *course.Course) *Repo     { return &Repo{c} }
func (r *Repo) Course() *course.Course   { return r.c }
