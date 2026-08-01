package notes

import (
	"net/http"
	"strconv"

	"github.com/a-h/templ"

	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func RegisterRoutes(mux *http.ServeMux, svc NotesService) {
	drawer := makeDrawerEndpoint(svc)
	add := makeAddEndpoint(svc)
	edit := makeEditEndpoint(svc)
	remove := makeRemoveEndpoint(svc)
	index := makeIndexEndpoint(svc)

	mux.HandleFunc("GET /notes", func(w http.ResponseWriter, r *http.Request) {
		resp, err := index(r.Context(), nil)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK,
			templates.Document("All notes", templates.NotesIndex(indexVM(resp.([]ModuleNotes)))))
	})

	mux.HandleFunc("GET /notes/drawer", func(w http.ResponseWriter, r *http.Request) {
		req := DrawerRequest{Module: r.URL.Query().Get("module"), Step: r.URL.Query().Get("step")}
		resp, err := drawer(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, drawerComponent(resp.(DrawerResponse)))
	})

	mux.HandleFunc("POST /notes", func(w http.ResponseWriter, r *http.Request) {
		req := AddNoteRequest{
			Module: r.FormValue("module"), Step: r.FormValue("step"), Body: r.FormValue("body"),
		}
		resp, err := add(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, drawerComponent(resp.(DrawerResponse)))
	})

	mux.HandleFunc("GET /notes/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		n, err := svc.Get(r.Context(), id)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, templates.NoteEditForm(noteVM(n)))
	})

	mux.HandleFunc("PUT /notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		resp, err := edit(r.Context(), EditNoteRequest{ID: id, Body: r.FormValue("body")})
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		api.RenderHTML(w, r, http.StatusOK, templates.NoteItem(noteVM(resp.(Note))))
	})

	mux.HandleFunc("DELETE /notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		if _, err := remove(r.Context(), id); err != nil {
			api.RenderError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK) // empty body: htmx outerHTML swap removes the node
	})
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		api.RenderError(w, r, api.ErrInvalid)
		return 0, false
	}
	return id, true
}

func drawerComponent(resp DrawerResponse) templ.Component {
	vm := templates.NotesDrawerVM{ModuleSlug: resp.Ref.Module, StepSlug: resp.Ref.Step}
	for _, n := range resp.Notes {
		vm.Notes = append(vm.Notes, noteVM(n))
	}
	return templates.NotesDrawer(vm)
}

func noteVM(n Note) templates.NoteVM {
	return templates.NoteVM{
		ID: n.ID, ModuleSlug: n.Ref.Module, StepSlug: n.Ref.Step,
		Body: n.Body, CreatedAt: n.CreatedAt.Local().Format("Jan 2 15:04"),
	}
}

func indexVM(groups []ModuleNotes) templates.NotesIndexVM {
	var vm templates.NotesIndexVM
	for _, g := range groups {
		gv := templates.ModuleNotesVM{ModuleTitle: g.ModuleTitle}
		for _, n := range g.Notes {
			gv.Notes = append(gv.Notes, noteVM(n))
		}
		vm.Groups = append(vm.Groups, gv)
	}
	return vm
}
