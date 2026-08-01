package notes_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

func fixtureCourse() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Module One", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "s1", Title: "One", Type: course.StepReading},
		}},
		{Slug: "m2", Title: "Module Two", Kind: course.KindLab, Order: 2, Steps: []course.Step{
			{Slug: "s1", Title: "Two", Type: course.StepReading},
		}},
	}}
}

func newSvcAndRepo(t *testing.T) (*notes.Service, *sqlite.NotesRepo) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewNotesRepo(db)
	return notes.NewService(coursefs.NewRepo(fixtureCourse()), repo), repo
}

func newSvc(t *testing.T) *notes.Service {
	t.Helper()
	svc, _ := newSvcAndRepo(t)
	return svc
}

func TestAddAndGroup(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if _, err := svc.Add(ctx, course.StepRef{Module: "m2", Step: "s1"}, "lab note"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, course.StepRef{Module: "m1", Step: "s1"}, "lecture note"); err != nil {
		t.Fatal(err)
	}
	groups, err := svc.GroupedByModule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// grouped in course order, not insertion order
	if len(groups) != 2 || groups[0].ModuleTitle != "Module One" || groups[1].ModuleTitle != "Module Two" {
		t.Fatalf("groups: %+v", groups)
	}
}

func TestGroupedByModuleAppendsOrphans(t *testing.T) {
	svc, repo := newSvcAndRepo(t)
	ctx := context.Background()
	if _, err := svc.Add(ctx, course.StepRef{Module: "m2", Step: "s1"}, "lab note"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, course.StepRef{Module: "m1", Step: "s1"}, "lecture note"); err != nil {
		t.Fatal(err)
	}
	// insert directly via the repo for a module slug the fixture course
	// doesn't know about, bypassing Add's course-step validation.
	now := time.Now().UTC()
	orphanRef := course.StepRef{Module: "orphan-module", Step: "s1"}
	if _, err := repo.Insert(ctx, notes.Note{Ref: orphanRef, Body: "orphan note", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	groups, err := svc.GroupedByModule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 {
		t.Fatalf("groups: %+v", groups)
	}
	if groups[0].ModuleTitle != "Module One" || groups[1].ModuleTitle != "Module Two" {
		t.Fatalf("course-order groups out of place: %+v", groups)
	}
	last := groups[2]
	if last.ModuleSlug != "orphan-module" || last.ModuleTitle != "orphan-module" {
		t.Fatalf("orphan group: %+v", last)
	}
	if len(last.Notes) != 1 || last.Notes[0].Body != "orphan note" {
		t.Fatalf("orphan group notes: %+v", last.Notes)
	}
}

func TestAddValidates(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if _, err := svc.Add(ctx, course.StepRef{Module: "nope", Step: "s1"}, "x"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("unknown step err = %v", err)
	}
	if _, err := svc.Add(ctx, course.StepRef{Module: "m1", Step: "s1"}, "   "); !errors.Is(err, api.ErrInvalid) {
		t.Fatalf("empty body err = %v", err)
	}
}

func TestEditAndRemove(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	n, err := svc.Add(ctx, course.StepRef{Module: "m1", Step: "s1"}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	edited, err := svc.Edit(ctx, n.ID, "v2")
	if err != nil || edited.Body != "v2" {
		t.Fatalf("edit: %v %+v", err, edited)
	}
	if err := svc.Remove(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ForStep(ctx, course.StepRef{Module: "m1", Step: "s1"})
	if err != nil || len(got) != 0 {
		t.Fatalf("after remove: %v %d", err, len(got))
	}
}
