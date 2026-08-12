// Command tour serves the guided MIT 6.824 course UI.
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/config"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/exercise"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/internal/openrouter"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/internal/tour"
	"github.com/itsnoproblem/mit-distributed-systems/static"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static.FS)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// newServer wraps mux in an http.Server with a header-read timeout so a
// non-loopback deployment isn't left open to Slowloris-style connection
// exhaustion (CodeAnt CWE-400 finding).
func newServer(cfg config.Config, mux *http.ServeMux) *http.Server {
	return &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func main() {
	cfg := config.FromEnv(os.Getenv)
	crs, err := coursefs.Load(filepath.Join(cfg.ContentDir, "modules"))
	if err != nil {
		log.Fatalf("load content: %v", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := sqlite.Open(filepath.Join(cfg.DataDir, "tour.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	courseRepo := coursefs.NewRepo(crs)
	progressRepo := sqlite.NewProgressRepo(db)

	mux := newMux()
	tour.RegisterRoutes(mux, tour.NewService(courseRepo, progressRepo))
	notes.RegisterRoutes(mux, notes.NewService(courseRepo, sqlite.NewNotesRepo(db)))

	attributionHTML, err := coursefs.RenderMarkdownFile(filepath.Join(cfg.ContentDir, "ATTRIBUTION.md"))
	if err != nil {
		log.Fatalf("attribution page: %v", err)
	}
	tour.RegisterAttribution(mux, attributionHTML)

	subsRepo := sqlite.NewSubmissionRepo(db)

	var llm eval.LLM
	if cfg.OpenRouterKey != "" {
		llm = openrouter.New(cfg.OpenRouterKey, cfg.OpenRouterModel)
	}
	evalSvc, err := eval.NewService(courseRepo, subsRepo,
		progressRepo, llm, eval.FSLabRepo{Dir: cfg.LabRepoDir}, cfg.ContentDir)
	if err != nil {
		log.Fatalf("eval service: %v", err)
	}
	if n, err := evalSvc.RecoverInterrupted(context.Background()); err != nil {
		log.Fatalf("recover interrupted submissions: %v", err)
	} else if n > 0 {
		log.Printf("marked %d interrupted submission(s) failed", n)
	}
	eval.RegisterRoutes(mux, evalSvc)
	log.Printf("evaluation mode enabled: %v", evalSvc.Enabled())

	exerciseSvc, err := exercise.NewService(courseRepo, sqlite.NewDraftsRepo(db),
		subsRepo, progressRepo, exercise.Workspace{}, llm, cfg.ContentDir)
	if err != nil {
		log.Fatalf("exercise service: %v", err)
	}
	exercise.RegisterRoutes(mux, exerciseSvc)

	srv := newServer(cfg, mux)
	log.Printf("tour listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
