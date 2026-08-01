package eval

import (
	"net/http"
	"strconv"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func RegisterRoutes(mux *http.ServeMux, svc EvalService) {
	section := makeSectionEndpoint(svc)
	answer := makeAnswerEndpoint(svc)

	renderSection := func(w http.ResponseWriter, r *http.Request, resp any, err error) {
		if err != nil {
			api.RenderError(w, r, err)
			return
		}
		res := resp.(SectionResponse)
		api.RenderHTML(w, r, http.StatusOK, templates.EvalSection(sectionVM(res.Ref, res.View)))
	}

	mux.HandleFunc("GET /eval/section", func(w http.ResponseWriter, r *http.Request) {
		req := SectionRequest{Module: r.URL.Query().Get("module"), Step: r.URL.Query().Get("step")}
		resp, err := section(r.Context(), req)
		renderSection(w, r, resp, err)
	})

	mux.HandleFunc("POST /modules/{module}/steps/{step}/answer", func(w http.ResponseWriter, r *http.Request) {
		req := AnswerRequest{
			Module: r.PathValue("module"), Step: r.PathValue("step"),
			Answer: r.FormValue("answer"),
		}
		resp, err := answer(r.Context(), req)
		renderSection(w, r, resp, err)
	})

	retry := makeRetryEndpoint(svc)
	mux.HandleFunc("POST /submissions/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.RenderError(w, r, api.ErrInvalid)
			return
		}
		resp, err := retry(r.Context(), id)
		renderSection(w, r, resp, err)
	})
}

func sectionVM(ref course.StepRef, v StepEvalView) templates.EvalSectionVM {
	vm := templates.EvalSectionVM{
		ModuleSlug: ref.Module, StepSlug: ref.Step,
		Type: string(v.Step.Type), Enabled: v.Enabled, Question: v.Step.Question,
	}
	if v.Submission != nil {
		vm.SubmissionID = v.Submission.ID
		vm.Status = string(v.Submission.Status)
		vm.TestOutput = v.Submission.TestOutput
		if v.Submission.Kind == KindQuestion {
			vm.Answer = v.Submission.Content
		}
	}
	if v.Evaluation != nil {
		r := templates.ReportVM{
			Model: v.Evaluation.Model, RubricVersion: v.Evaluation.RubricVersion,
			Summary: v.Evaluation.Verdict.Summary, NextSteps: v.Evaluation.Verdict.NextSteps,
		}
		for _, c := range v.Evaluation.Verdict.Criteria {
			r.Criteria = append(r.Criteria, templates.CriterionVM{
				Name: c.Name, Score: c.Score, Justification: c.Justification,
			})
		}
		vm.Report = &r
	}
	return vm
}
