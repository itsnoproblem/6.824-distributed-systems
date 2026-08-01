package tour

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

// TourService is the contract this feature's endpoints require; *Service satisfies it.
type TourService interface {
	CourseMap(ctx context.Context) (CourseMapView, error)
	StepPage(ctx context.Context, ref course.StepRef) (StepView, error)
	SetComplete(ctx context.Context, ref course.StepRef, done bool) error
}

type StepPageRequest struct{ Module, Step string }

func (r StepPageRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type SetCompleteRequest struct {
	Module, Step string
	Done         bool
}

func (r SetCompleteRequest) Validate() error {
	return StepPageRequest{Module: r.Module, Step: r.Step}.Validate()
}

type SetCompleteResponse struct {
	Module, Step string
	Done         bool
}

func makeCourseMapEndpoint(svc TourService) api.Endpoint {
	return func(ctx context.Context, _ any) (any, error) {
		return svc.CourseMap(ctx)
	}
}

func makeStepPageEndpoint(svc TourService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(StepPageRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		return svc.StepPage(ctx, course.StepRef{Module: req.Module, Step: req.Step})
	}
}

func makeSetCompleteEndpoint(svc TourService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(SetCompleteRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if err := svc.SetComplete(ctx, ref, req.Done); err != nil {
			return nil, err
		}
		return SetCompleteResponse{Module: req.Module, Step: req.Step, Done: req.Done}, nil
	}
}
