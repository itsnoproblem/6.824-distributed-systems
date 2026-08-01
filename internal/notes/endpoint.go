package notes

import (
	"context"
	"fmt"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type NotesService interface {
	Add(ctx context.Context, ref course.StepRef, body string) (Note, error)
	Edit(ctx context.Context, id int64, body string) (Note, error)
	Remove(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (Note, error)
	ForStep(ctx context.Context, ref course.StepRef) ([]Note, error)
	GroupedByModule(ctx context.Context) ([]ModuleNotes, error)
}

type DrawerRequest struct{ Module, Step string }

func (r DrawerRequest) Validate() error {
	if r.Module == "" || r.Step == "" {
		return fmt.Errorf("%w: module and step are required", api.ErrInvalid)
	}
	return nil
}

type AddNoteRequest struct{ Module, Step, Body string }

type DrawerResponse struct {
	Ref   course.StepRef
	Notes []Note
}

func makeDrawerEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(DrawerRequest)
		if err := req.Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		ns, err := svc.ForStep(ctx, ref)
		if err != nil {
			return nil, err
		}
		return DrawerResponse{Ref: ref, Notes: ns}, nil
	}
}

func makeAddEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(AddNoteRequest)
		if err := (DrawerRequest{Module: req.Module, Step: req.Step}).Validate(); err != nil {
			return nil, err
		}
		ref := course.StepRef{Module: req.Module, Step: req.Step}
		if _, err := svc.Add(ctx, ref, req.Body); err != nil {
			return nil, err
		}
		ns, err := svc.ForStep(ctx, ref)
		if err != nil {
			return nil, err
		}
		return DrawerResponse{Ref: ref, Notes: ns}, nil
	}
}

type EditNoteRequest struct {
	ID   int64
	Body string
}

func makeEditEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(EditNoteRequest)
		return svc.Edit(ctx, req.ID, req.Body)
	}
}

func makeRemoveEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		return nil, svc.Remove(ctx, request.(int64))
	}
}

func makeIndexEndpoint(svc NotesService) api.Endpoint {
	return func(ctx context.Context, _ any) (any, error) {
		return svc.GroupedByModule(ctx)
	}
}
