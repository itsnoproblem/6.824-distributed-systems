// Package api holds the shared endpoint contract and HTTP rendering helpers.
package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/a-h/templ"
)

// Endpoint is the transport-agnostic unit of work: decoded request in, response model out.
type Endpoint func(ctx context.Context, request any) (any, error)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid request")
)

func RenderHTML(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = c.Render(r.Context(), w)
}

func RenderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
	}
}
