package coursefs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
)

func TestLoadValid(t *testing.T) {
	c, err := coursefs.Load("testdata/valid")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Modules) != 2 {
		t.Fatalf("modules = %d, want 2", len(c.Modules))
	}
	alpha := c.Modules[0]
	if alpha.Slug != "01-alpha" || alpha.Kind != course.KindLecture || alpha.Links.Paper == "" {
		t.Fatalf("alpha parsed wrong: %+v", alpha)
	}
	if len(alpha.Steps) != 2 || alpha.Steps[0].Slug != "01-read" {
		t.Fatalf("alpha steps: %+v", alpha.Steps)
	}
	if !strings.Contains(alpha.Steps[0].BodyHTML, "<strong>paper</strong>") {
		t.Errorf("markdown not rendered: %q", alpha.Steps[0].BodyHTML)
	}
	if q := alpha.Steps[1].Question; !strings.Contains(q, "sky blue") {
		t.Errorf("question = %q", q)
	}
	sub := c.Modules[1].Steps[0]
	if sub.Eval == nil || sub.Eval.Workdir != "src/beta" || sub.Eval.Timeout != 5*time.Minute ||
		len(sub.Eval.TestCmd) != 3 {
		t.Fatalf("eval meta: %+v", sub.Eval)
	}
}

// writeModule builds a minimal module tree for error-case tests.
func writeModule(t *testing.T, root, slug, moduleYAML string, steps map[string]string) {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(filepath.Join(dir, "steps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.yaml"), []byte(moduleYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range steps {
		if err := os.WriteFile(filepath.Join(dir, "steps", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	goodStep := "---\ntitle: T\ntype: reading\n---\nbody"
	cases := []struct {
		name, moduleYAML string
		steps            map[string]string
		wantErr          string
	}{
		{"bad kind", "title: X\nkind: seminar\norder: 1", map[string]string{"01-a.md": goodStep}, "kind"},
		{"missing title", "kind: lecture\norder: 1", map[string]string{"01-a.md": goodStep}, "title"},
		{"bad step type", "title: X\nkind: lecture\norder: 1",
			map[string]string{"01-a.md": "---\ntitle: T\ntype: quiz\n---\nb"}, "type"},
		{"question without question", "title: X\nkind: lecture\norder: 1",
			map[string]string{"01-a.md": "---\ntitle: T\ntype: question\n---\nb"}, "question"},
		{"submit without eval", "title: X\nkind: lab\norder: 1",
			map[string]string{"01-a.md": "---\ntitle: T\ntype: submit\n---\nb"}, "eval"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeModule(t, root, "01-x", tc.moduleYAML, tc.steps)
			_, err := coursefs.Load(root)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRepo(t *testing.T) {
	c, err := coursefs.Load("testdata/valid")
	if err != nil {
		t.Fatal(err)
	}
	if coursefs.NewRepo(c).Course() != c {
		t.Fatal("repo should hand back the loaded course")
	}
}
