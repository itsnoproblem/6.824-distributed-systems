// Package tour is the course-browsing feature: course map, step pages,
// and per-step completion.
package tour

import (
	"context"
	"fmt"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type CourseRepo interface{ Course() *course.Course }

type ProgressRepo interface {
	SetComplete(ctx context.Context, ref course.StepRef, done bool) error
	Completed(ctx context.Context) (map[course.StepRef]time.Time, error)
}

type ModuleProgress struct {
	Module      course.Module
	Done, Total int
}

type KindGroup struct {
	Kind    course.Kind
	Modules []ModuleProgress
}

type CourseMapView struct {
	Groups      []KindGroup
	Done, Total int
}

type StepView struct {
	Module       course.Module
	Step         course.Step
	Ref          course.StepRef
	Completed    bool
	Prev, Next   *course.StepRef
	Index, Total int
}

type Service struct {
	course   CourseRepo
	progress ProgressRepo
}

func NewService(c CourseRepo, p ProgressRepo) *Service { return &Service{c, p} }

func (s *Service) CourseMap(ctx context.Context) (CourseMapView, error) {
	done, err := s.progress.Completed(ctx)
	if err != nil {
		return CourseMapView{}, err
	}
	crs := s.course.Course()
	view := CourseMapView{Total: crs.TotalSteps()}
	for _, kind := range []course.Kind{course.KindLecture, course.KindLab, course.KindProject} {
		group := KindGroup{Kind: kind}
		for _, m := range crs.Modules {
			if m.Kind != kind {
				continue
			}
			mp := ModuleProgress{Module: m, Total: len(m.Steps)}
			for _, st := range m.Steps {
				if _, ok := done[course.StepRef{Module: m.Slug, Step: st.Slug}]; ok {
					mp.Done++
				}
			}
			view.Done += mp.Done
			group.Modules = append(group.Modules, mp)
		}
		if len(group.Modules) > 0 {
			view.Groups = append(view.Groups, group)
		}
	}
	return view, nil
}

func (s *Service) StepPage(ctx context.Context, ref course.StepRef) (StepView, error) {
	crs := s.course.Course()
	mod, step, ok := crs.Step(ref)
	if !ok {
		return StepView{}, fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	done, err := s.progress.Completed(ctx)
	if err != nil {
		return StepView{}, err
	}
	view := StepView{Module: *mod, Step: *step, Ref: ref, Total: len(mod.Steps)}
	_, view.Completed = done[ref]
	for i, st := range mod.Steps {
		if st.Slug == ref.Step {
			view.Index = i + 1
		}
	}
	if prev, ok := crs.Prev(ref); ok {
		view.Prev = &prev
	}
	if next, ok := crs.Next(ref); ok {
		view.Next = &next
	}
	return view, nil
}

func (s *Service) SetComplete(ctx context.Context, ref course.StepRef, done bool) error {
	if _, _, ok := s.course.Course().Step(ref); !ok {
		return fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	return s.progress.SetComplete(ctx, ref, done)
}
