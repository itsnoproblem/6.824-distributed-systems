package exercise

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

const maxDraftBytes = 1 << 20

func RegisterRoutes(mux *http.ServeMux, svc ExerciseService) {
	section := makeSectionEndpoint(svc)
	saveDraft := makeSaveDraftEndpoint(svc)
	check := makeCheckEndpoint(svc)
	run := makeRunEndpoint(svc)
	reset := makeResetEndpoint(svc)
	status := makeStatusEndpoint(svc)

	pathReq := func(r *http.Request) SectionRequest {
		return SectionRequest{Module: r.PathValue("module"), Step: r.PathValue("step")}
	}
	renderSection := func(w http.ResponseWriter, r *http.Request, resp any, err error) {
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(StateResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.ExerciseSection(exerciseVM(res.Ref, res.View)))
	}
	renderStatus := func(w http.ResponseWriter, r *http.Request, resp any, err error) {
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(StateResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.ExerciseStatus(exerciseVM(res.Ref, res.View)))
	}

	mux.HandleFunc("GET /exercises/{module}/{step}", func(w http.ResponseWriter, r *http.Request) {
		resp, err := section(r.Context(), pathReq(r))
		renderSection(w, r, resp, err)
	})

	mux.HandleFunc("PUT /exercises/{module}/{step}/draft", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Files map[string]string `json:"files"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxDraftBytes)).Decode(&body); err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		req := SaveDraftRequest{Module: r.PathValue("module"), Step: r.PathValue("step"), Files: body.Files}
		if _, err := saveDraft(r.Context(), req); err != nil {
			api.RenderError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// check never renders an error for problems the student's code has —
	// Service.Check only returns an error for genuine infrastructure
	// failures (step lookup, draft-repo I/O); tool failures on student code
	// come back as a synthetic diagnostic with a nil error. So the err path
	// below is exclusively the infrastructure case.
	mux.HandleFunc("POST /exercises/{module}/{step}/check", func(w http.ResponseWriter, r *http.Request) {
		resp, err := check(r.Context(), pathReq(r))
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		diags := resp.([]Diagnostic)
		if diags == nil {
			diags = []Diagnostic{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(diags)
	})

	mux.HandleFunc("POST /exercises/{module}/{step}/run", func(w http.ResponseWriter, r *http.Request) {
		resp, err := run(r.Context(), pathReq(r))
		renderStatus(w, r, resp, err)
	})

	mux.HandleFunc("POST /exercises/{module}/{step}/reset", func(w http.ResponseWriter, r *http.Request) {
		resp, err := reset(r.Context(), pathReq(r))
		renderSection(w, r, resp, err)
	})

	mux.HandleFunc("GET /exercises/submissions/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		resp, err := status(r.Context(), id)
		renderStatus(w, r, resp, err)
	})
}

func exerciseVM(ref course.StepRef, v View) templates.ExerciseVM {
	files := make([]templates.ExerciseFileVM, 0, len(v.Files))
	for _, f := range v.Files {
		files = append(files, templates.ExerciseFileVM{Name: f.Name, Content: f.Content, Readonly: f.Readonly})
	}
	base := "/exercises/" + ref.Module + "/" + ref.Step
	cfg, _ := json.Marshal(struct {
		Files    []templates.ExerciseFileVM `json:"files"`
		SaveURL  string                     `json:"saveUrl"`
		CheckURL string                     `json:"checkUrl"`
		RunURL   string                     `json:"runUrl"`
	}{files, base + "/draft", base + "/check", base + "/run"})
	vm := templates.ExerciseVM{
		ModuleSlug: ref.Module, StepSlug: ref.Step,
		Mode: v.Meta.Mode, ModeLabel: modeLabel(v.Meta.Mode),
		Attribution: v.Step.Attribution, ConfigJSON: string(cfg),
		Files: files,
	}
	if v.Submission != nil {
		vm.Status = string(v.Submission.Status)
		vm.Output = v.Submission.TestOutput
		vm.SubmissionID = v.Submission.ID
		if v.Submission.Passed != nil {
			vm.Passed = *v.Submission.Passed
		}
	}
	return vm
}

func modeLabel(mode string) string {
	if mode == "create" {
		return "Build it"
	}
	return "Fix the bug"
}
