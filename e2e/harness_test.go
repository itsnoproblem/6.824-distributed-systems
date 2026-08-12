// Package e2e wires a full application over a temp database and testdata
// content, and drives it over HTTP the way a browser would. Dedicated
// integration-test package; contains only _test.go files.
package e2e

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/internal/tour"
)

type options struct {
	ContentDir string       // defaults to e2e/testdata/content
	LLM        eval.LLM     // nil = locked mode
	Lab        eval.LabRepo // nil until a test needs lab submission
	AsyncRuns  bool         // false = synchronous (tests see final state); true = real async (streaming tests watch mid-run)
}

type app struct {
	TS *httptest.Server
	DB *sql.DB
}

func newApp(t *testing.T, o options) *app {
	t.Helper()
	if o.ContentDir == "" {
		o.ContentDir = "testdata/content" // go test runs with the package dir as cwd
	}
	crs, err := coursefs.Load(filepath.Join(o.ContentDir, "modules"))
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "tour.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	courseRepo := coursefs.NewRepo(crs)
	progressRepo := sqlite.NewProgressRepo(db)
	subsRepo := sqlite.NewSubmissionRepo(db)
	mux := http.NewServeMux()
	tour.RegisterRoutes(mux, tour.NewService(courseRepo, progressRepo))
	notes.RegisterRoutes(mux, notes.NewService(courseRepo, sqlite.NewNotesRepo(db)))

	attributionHTML, err := coursefs.RenderMarkdownFile(filepath.Join(o.ContentDir, "ATTRIBUTION.md"))
	if err != nil {
		t.Fatalf("attribution page: %v", err)
	}
	tour.RegisterAttribution(mux, attributionHTML)

	runAsync := func(f func()) { f() } // synchronous: tests see final state
	if o.AsyncRuns {
		runAsync = func(f func()) { go f() } // real async: streaming tests watch mid-run
	}

	evalSvc, err := eval.NewService(courseRepo, subsRepo,
		progressRepo, o.LLM, o.Lab, o.ContentDir,
		eval.WithRunAsync(runAsync))
	if err != nil {
		t.Fatalf("eval service: %v", err)
	}
	if _, err := evalSvc.RecoverInterrupted(context.Background()); err != nil {
		t.Fatalf("recover interrupted submissions: %v", err)
	}
	eval.RegisterRoutes(mux, evalSvc)

	exerciseSvc, err := exercise.NewService(courseRepo, sqlite.NewDraftsRepo(db),
		subsRepo, progressRepo, exercise.Workspace{}, o.LLM, o.ContentDir,
		exercise.WithRunAsync(runAsync))
	if err != nil {
		t.Fatalf("exercise service: %v", err)
	}
	exercise.RegisterRoutes(mux, exerciseSvc)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &app{TS: ts, DB: db}
}
