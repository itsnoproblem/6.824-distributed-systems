// Command tour serves the guided MIT 6.824 course UI.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/itsnoproblem/mit-distributed-systems/internal/config"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
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

	evalSvc, err := eval.NewService(courseRepo, sqlite.NewSubmissionRepo(db),
		progressRepo, nil, nil, cfg.ContentDir)
	if err != nil {
		log.Fatalf("eval service: %v", err)
	}
	eval.RegisterRoutes(mux, evalSvc)
	log.Printf("evaluation mode enabled: %v", evalSvc.Enabled())

	log.Printf("tour listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
