package exercise

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/runstream"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// ExerciseService is the contract the endpoints require; *Service satisfies it.
type ExerciseService interface {
	State(ctx context.Context, ref course.StepRef) (View, error)
	SaveDraft(ctx context.Context, ref course.StepRef, files map[string]string) error
	ResetDraft(ctx context.Context, ref course.StepRef) error
	Check(ctx context.Context, ref course.StepRef) ([]Diagnostic, error)
	Run(ctx context.Context, ref course.StepRef) error
	Feedback(ctx context.Context, ref course.StepRef) error
	FeedbackEnabled() bool
	RefForSubmission(ctx context.Context, id int64) (course.StepRef, error)
	Watch(ctx context.Context, id int64) (<-chan runstream.Event, error)
	Cancel(ctx context.Context, id int64) error
}

type SectionRequest struct{ Module, Step string }

func (r SectionRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type SaveDraftRequest struct {
	Module, Step string
	Files        map[string]string
}

type StateResponse struct {
	Ref  course.StepRef
	View View
}

func makeSectionEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}

func makeSaveDraftEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SaveDraftRequest)
		if err := (SectionRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		return nil, svc.SaveDraft(ctx, ref, req.Files)
	}
}

func makeCheckEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		return svc.Check(ctx, course.StepRef{Module: req.Module, Step: req.Step})
	}
}

func makeRunEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.Run(ctx, ref); err != nil {
			return nil, err
		}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}

func makeFeedbackEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.Feedback(ctx, ref); err != nil {
			return nil, err
		}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}

func makeResetEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.ResetDraft(ctx, ref); err != nil {
			return nil, err
		}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}

func makeStatusEndpoint(svc ExerciseService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		ref, err := svc.RefForSubmission(ctx, request.(int64))
		if err != nil {
			return nil, err
		}
		view, err := svc.State(ctx, ref)
		if err != nil {
			return nil, err
		}
		return StateResponse{Ref: ref, View: view}, nil
	}
}
