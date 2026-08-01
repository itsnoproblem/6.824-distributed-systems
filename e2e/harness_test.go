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
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/internal/tour"
)

type options struct {
	ContentDir string       // defaults to e2e/testdata/content
	LLM        eval.LLM     // nil = locked mode
	Lab        eval.LabRepo // nil until a test needs lab submission
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
	mux := http.NewServeMux()
	tour.RegisterRoutes(mux, tour.NewService(courseRepo, sqlite.NewProgressRepo(db)))
	notes.RegisterRoutes(mux, notes.NewService(courseRepo, sqlite.NewNotesRepo(db)))

	evalSvc, err := eval.NewService(courseRepo, sqlite.NewSubmissionRepo(db),
		sqlite.NewProgressRepo(db), o.LLM, o.Lab, o.ContentDir,
		eval.WithRunAsync(func(f func()) { f() })) // synchronous: tests see final state
	if err != nil {
		t.Fatalf("eval service: %v", err)
	}
	if _, err := evalSvc.RecoverInterrupted(context.Background()); err != nil {
		t.Fatalf("recover interrupted submissions: %v", err)
	}
	eval.RegisterRoutes(mux, evalSvc)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &app{TS: ts, DB: db}
}
