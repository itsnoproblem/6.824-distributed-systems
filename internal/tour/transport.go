package tour

import (
	"net/http"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func RegisterRoutes(mux *http.ServeMux, svc TourService) {
	courseMap := makeCourseMapEndpoint(svc)
	stepPage := makeStepPageEndpoint(svc)
	setComplete := makeSetCompleteEndpoint(svc)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		resp, err := courseMap(r.Context(), nil)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		vm := courseMapVM(resp.(CourseMapView))
		api.RenderHTML(w, r, http.StatusOK,
			templates.Document("MIT 6.824 — Guided Tour", templates.CourseMap(vm)))
	})

	mux.HandleFunc("GET /modules/{module}/steps/{step}", func(w http.ResponseWriter, r *http.Request) {
		req := StepPageRequest{Module: r.PathValue("module"), Step: r.PathValue("step")}
		resp, err := stepPage(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		vm := stepVM(resp.(StepView))
		api.RenderHTML(w, r, http.StatusOK, templates.Document(vm.Title, templates.StepPage(vm)))
	})

	mux.HandleFunc("POST /modules/{module}/steps/{step}/complete", func(w http.ResponseWriter, r *http.Request) {
		req := SetCompleteRequest{
			Module: r.PathValue("module"), Step: r.PathValue("step"),
			Done: r.FormValue("done") == "true",
		}
		resp, err := setComplete(r.Context(), req)
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(SetCompleteResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.CompleteToggle(res.Module, res.Step, res.Done))
	})
}

func courseMapVM(v CourseMapView) templates.CourseMapVM {
	out := templates.CourseMapVM{Done: v.Done, Total: v.Total}
	for _, g := range v.Groups {
		gv := templates.KindGroupVM{Label: kindLabel(g.Kind)}
		for _, mp := range g.Modules {
			first := ""
			if len(mp.Module.Steps) > 0 {
				first = templates.StepURL(mp.Module.Slug, mp.Module.Steps[0].Slug)
			}
			gv.Modules = append(gv.Modules, templates.ModuleCardVM{
				Slug: mp.Module.Slug, Title: mp.Module.Title,
				Done: mp.Done, Total: mp.Total, FirstStepURL: first,
			})
		}
		out.Groups = append(out.Groups, gv)
	}
	return out
}

func kindLabel(k course.Kind) string {
	switch k {
	case course.KindLecture:
		return "Lectures"
	case course.KindLab:
		return "Labs"
	default:
		return "Final project"
	}
}

func stepVM(v StepView) templates.StepVM {
	vm := templates.StepVM{
		ModuleSlug: v.Ref.Module, StepSlug: v.Ref.Step,
		ModuleTitle: v.Module.Title, Title: v.Step.Title,
		Type: string(v.Step.Type), BodyHTML: v.Step.BodyHTML,
		Completed: v.Completed, Index: v.Index, Total: v.Total,
		PaperURL: v.Module.Links.Paper, LabURL: v.Module.Links.Lab, VideoURL: v.Module.Links.Video,
	}
	vm.VideoWatchURL = v.Step.Video
	vm.VideoEmbedURL = templates.YouTubeEmbedURL(v.Step.Video)
	if v.Prev != nil {
		vm.PrevURL = templates.StepURL(v.Prev.Module, v.Prev.Step)
	}
	if v.Next != nil {
		vm.NextURL = templates.StepURL(v.Next.Module, v.Next.Step)
	}
	return vm
}
