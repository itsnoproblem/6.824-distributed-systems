// Command tour serves the guided MIT 6.824 course UI.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/itsnoproblem/mit-distributed-systems/internal/config"
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
	mux := newMux()
	log.Printf("tour listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
