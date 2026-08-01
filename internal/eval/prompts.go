package eval

import (
	"fmt"
	"os"
	"strings"

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
