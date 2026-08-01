// Package notes is the note-taking feature: notes attach to the step where
// they were taken and are browsed grouped by module.
package notes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type Note struct {
	ID        int64
	Ref       course.StepRef
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ModuleNotes struct {
	ModuleSlug, ModuleTitle string
	Notes                   []Note
}

type CourseRepo interface{ Course() *course.Course }

type Repo interface {
	Insert(ctx context.Context, n Note) (int64, error)
	Update(ctx context.Context, id int64, body string, updatedAt time.Time) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (Note, error)
	ForStep(ctx context.Context, ref course.StepRef) ([]Note, error)
	All(ctx context.Context) ([]Note, error)
}

type Service struct {
	course CourseRepo
	repo   Repo
	now    func() time.Time
}

func NewService(c CourseRepo, r Repo) *Service { return &Service{c, r, time.Now} }

func (s *Service) Add(ctx context.Context, ref course.StepRef, body string) (Note, error) {
	if _, _, ok := s.course.Course().Step(ref); !ok {
		return Note{}, fmt.Errorf("%w: step %s", api.ErrNotFound, ref)
	}
	if strings.TrimSpace(body) == "" {
		return Note{}, fmt.Errorf("%w: note body is empty", api.ErrInvalid)
	}
	now := s.now().UTC()
	n := Note{Ref: ref, Body: body, CreatedAt: now, UpdatedAt: now}
	id, err := s.repo.Insert(ctx, n)
	if err != nil {
		return Note{}, err
	}
	n.ID = id
	return n, nil
}

func (s *Service) Edit(ctx context.Context, id int64, body string) (Note, error) {
	if strings.TrimSpace(body) == "" {
		return Note{}, fmt.Errorf("%w: note body is empty", api.ErrInvalid)
	}
	if err := s.repo.Update(ctx, id, body, s.now().UTC()); err != nil {
		return Note{}, err
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Remove(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }

func (s *Service) ForStep(ctx context.Context, ref course.StepRef) ([]Note, error) {
	return s.repo.ForStep(ctx, ref)
}

func (s *Service) Get(ctx context.Context, id int64) (Note, error) { return s.repo.Get(ctx, id) }

// GroupedByModule returns notes bucketed by module in course order; notes
// whose module no longer exists in the content are appended at the end.
func (s *Service) GroupedByModule(ctx context.Context) ([]ModuleNotes, error) {
	all, err := s.repo.All(ctx)
	if err != nil {
		return nil, err
	}
	byModule := map[string][]Note{}
	for _, n := range all {
		byModule[n.Ref.Module] = append(byModule[n.Ref.Module], n)
	}
	var out []ModuleNotes
	for _, m := range s.course.Course().Modules {
		if ns, ok := byModule[m.Slug]; ok {
			out = append(out, ModuleNotes{ModuleSlug: m.Slug, ModuleTitle: m.Title, Notes: ns})
			delete(byModule, m.Slug)
		}
	}
	orphanSlugs := make([]string, 0, len(byModule))
	for slug := range byModule {
		orphanSlugs = append(orphanSlugs, slug)
	}
	sort.Strings(orphanSlugs)
	for _, slug := range orphanSlugs {
		out = append(out, ModuleNotes{ModuleSlug: slug, ModuleTitle: slug, Notes: byModule[slug]})
	}
	return out, nil
}
