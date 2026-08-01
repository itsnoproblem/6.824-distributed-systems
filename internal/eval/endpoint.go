package eval

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// EvalService is the contract this feature's endpoints require; *Service
// satisfies it, covering question and lab submission, retry, and step state.
type EvalService interface {
	StepState(ctx context.Context, ref course.StepRef) (StepEvalView, error)
	SubmitAnswer(ctx context.Context, ref course.StepRef, answer string) error
	SubmitLab(ctx context.Context, ref course.StepRef) error
	Retry(ctx context.Context, id int64) error
	RefForSubmission(ctx context.Context, id int64) (course.StepRef, error)
}

type SectionRequest struct{ Module, Step string }

func (r SectionRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type AnswerRequest struct{ Module, Step, Answer string }

type SectionResponse struct {
	Ref  course.StepRef
	View StepEvalView
}

func makeSectionEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SectionRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}

func makeAnswerEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(AnswerRequest)
		if err := (SectionRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.SubmitAnswer(ctx, ref, req.Answer); err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}

type SubmitLabRequest struct{ Module, Step string }

func makeSubmitLabEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SubmitLabRequest)
		if err := (SectionRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.SubmitLab(ctx, ref); err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}

func makeSectionBySubmissionEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		ref, err := svc.RefForSubmission(ctx, request.(int64))
		if err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}

func makeRetryEndpoint(svc EvalService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		id := request.(int64)
		if err := svc.Retry(ctx, id); err != nil {
			return nil, err
		}
		ref, err := svc.RefForSubmission(ctx, id)
		if err != nil {
			return nil, err
		}
		view, err := svc.StepState(ctx, ref)
		if err != nil {
			return nil, err
		}
		return SectionResponse{Ref: ref, View: view}, nil
	}
}
