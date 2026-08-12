package runstream

import (
	"context"
	"net/http"
	"strconv"

	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// RunService is what a service must expose for its runs to be streamed and
// canceled over HTTP; the eval and exercise services both satisfy it.
type RunService interface {
	Watch(ctx context.Context, id int64) (<-chan Event, error)
	Cancel(ctx context.Context, id int64) error
}

// RegisterRunRoutes wires the live-run endpoints for one surface under
// prefix (e.g. "/submissions" or "/exercises/submissions"):
// GET {prefix}/{id}/stream (SSE) and POST {prefix}/{id}/cancel (204).
// These bypass the api.Endpoint indirection deliberately: SSE writes a
// long-lived response and cancel returns no body — neither fits the
// request→response-model→render shape.
func RegisterRunRoutes(mux *http.ServeMux, prefix string, svc RunService) {
	mux.HandleFunc("GET "+prefix+"/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		events, err := svc.Watch(r.Context(), id)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		ServeSSE(w, r, events)
	})

	mux.HandleFunc("POST "+prefix+"/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		if err := svc.Cancel(r.Context(), id); err != nil {
			api.RenderError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
