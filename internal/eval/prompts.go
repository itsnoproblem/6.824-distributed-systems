package eval

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"gopkg.in/yaml.v3"
)

// Rubric is versioned evaluation criteria loaded from content/rubric/*.md.
type Rubric struct {
	Version string
	Body    string
}

func LoadRubric(path string) (Rubric, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Rubric{}, err
	}
	s := string(raw)
	if !strings.HasPrefix(s, "---\n") {
		return Rubric{}, fmt.Errorf("%s: missing frontmatter", path)
	}
	rest := s[len("---\n"):]
	i := strings.Index(rest, "\n---")
	if i < 0 {
		return Rubric{}, fmt.Errorf("%s: unterminated frontmatter", path)
	}
	var fm struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(rest[:i+1]), &fm); err != nil {
		return Rubric{}, fmt.Errorf("%s: %w", path, err)
	}
	if fm.Version == "" {
		return Rubric{}, fmt.Errorf("%s: version is required", path)
	}
	return Rubric{
		Version: fm.Version,
		Body:    strings.TrimSpace(strings.TrimPrefix(rest[i+len("\n---"):], "\n")),
	}, nil
}

const verdictShape = `{"criteria":[{"name":"<criterion>","score":<1-5>,"justification":"<why>"}],` +
	`"summary":"<2-4 sentences>","next_steps":["<concrete action>"]}`

const taRole = "You are a strict but constructive teaching assistant for MIT 6.824 " +
	"(Distributed Systems). Respond with ONLY a JSON object in exactly this shape:\n"

func BuildQuestionPrompt(r Rubric, mod course.Module, step course.Step, answer string) (system, user string) {
	system = taRole + verdictShape +
		"\n\nEvaluate the student's answer to a reading question.\n\nRubric (version " +
		r.Version + "):\n" + r.Body
	user = fmt.Sprintf("Module: %s\n\nQuestion:\n%s\n\nStudent answer:\n%s",
		mod.Title, step.Question, answer)
	return system, user
}

func BuildLabPrompt(r Rubric, guidance string, mod course.Module, step course.Step,
	files map[string]string, testOutput string) (system, user string) {
	system = taRole + verdictShape +
		"\n\nReview the student's lab code and its test output.\n\nRubric (version " +
		r.Version + "):\n" + r.Body
	if guidance != "" {
		system += "\n\nLab-specific guidance:\n" + guidance
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Module: %s — %s\n\nTest output:\n%s\n\nCode:\n", mod.Title, step.Title, testOutput)
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
