package tour_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/coursefs"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
	"github.com/itsnoproblem/mit-distributed-systems/internal/tour"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

func fixtureCourse() *course.Course {
	return &course.Course{Modules: []course.Module{
		{Slug: "m1", Title: "Lecture M1", Kind: course.KindLecture, Order: 1, Steps: []course.Step{
			{Slug: "s1", Title: "One", Type: course.StepReading},
			{Slug: "s2", Title: "Two", Type: course.StepReading},
		}},
		{Slug: "m2", Title: "Lab M2", Kind: course.KindLab, Order: 2, Steps: []course.Step{
			{Slug: "s1", Title: "Three", Type: course.StepReading},
		}},
	}}
}

func newSvc(t *testing.T) *tour.Service {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return tour.NewService(coursefs.NewRepo(fixtureCourse()), sqlite.NewProgressRepo(db))
}

func TestCourseMapAndProgress(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.SetComplete(ctx, course.StepRef{Module: "m1", Step: "s1"}, true); err != nil {
		t.Fatal(err)
	}
	v, err := svc.CourseMap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Total != 3 || v.Done != 1 {
		t.Fatalf("overall = %d/%d, want 1/3", v.Done, v.Total)
	}
	if len(v.Groups) != 2 || v.Groups[0].Kind != course.KindLecture {
		t.Fatalf("groups: %+v", v.Groups)
	}
	if mp := v.Groups[0].Modules[0]; mp.Done != 1 || mp.Total != 2 {
		t.Fatalf("m1 progress = %d/%d", mp.Done, mp.Total)
	}
}

func TestStepPage(t *testing.T) {
	svc := newSvc(t)
	v, err := svc.StepPage(context.Background(), course.StepRef{Module: "m1", Step: "s2"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Index != 2 || v.Total != 2 || v.Completed {
		t.Fatalf("view: %+v", v)
	}
	if v.Prev == nil || v.Prev.Step != "s1" || v.Next == nil || v.Next.Module != "m2" {
		t.Fatalf("nav: prev=%v next=%v", v.Prev, v.Next)
	}
}

func TestUnknownStepIsNotFound(t *testing.T) {
	svc := newSvc(t)
	if _, err := svc.StepPage(context.Background(), course.StepRef{Module: "x", Step: "y"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := svc.SetComplete(context.Background(), course.StepRef{Module: "x", Step: "y"}, true); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
