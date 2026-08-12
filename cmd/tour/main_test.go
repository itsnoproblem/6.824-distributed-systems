package main

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/config"
)

func TestNewServer(t *testing.T) {
	mux := newMux()
	srv := newServer(config.Config{Host: "127.0.0.1", Port: "8080"}, mux)
	if srv.Addr != "127.0.0.1:8080" {
		t.Errorf("Addr = %q, want 127.0.0.1:8080", srv.Addr)
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != time.Minute {
		t.Errorf("ReadTimeout = %v, want 1m", srv.ReadTimeout)
	}
	if srv.IdleTimeout != 2*time.Minute {
		t.Errorf("IdleTimeout = %v, want 2m", srv.IdleTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 — eval runs block the response for minutes", srv.WriteTimeout)
	}
	if srv.Handler == nil {
		t.Error("Handler not set")
	}
}

func TestHealthz(t *testing.T) {
	mux := newMux()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}
