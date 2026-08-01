package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func notesRepo(t *testing.T) *sqlite.NotesRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewNotesRepo(db)
}

func TestNotesCRUD(t *testing.T) {
	repo := notesRepo(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "s1"}
	now := time.Now().UTC().Truncate(time.Second)

	id, err := repo.Insert(ctx, notes.Note{Ref: ref, Body: "first", CreatedAt: now, UpdatedAt: now})
	if err != nil || id == 0 {
		t.Fatalf("insert: %v id=%d", err, id)
	}
	got, err := repo.Get(ctx, id)
	if err != nil || got.Body != "first" || got.Ref != ref {
		t.Fatalf("get: %v %+v", err, got)
	}
	if err := repo.Update(ctx, id, "edited", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	forStep, err := repo.ForStep(ctx, ref)
	if err != nil || len(forStep) != 1 || forStep[0].Body != "edited" {
		t.Fatalf("forStep: %v %+v", err, forStep)
	}
	if _, err := repo.Insert(ctx, notes.Note{Ref: course.StepRef{Module: "m2", Step: "s1"},
		Body: "other", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	all, err := repo.All(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("all: %v %d", err, len(all))
	}
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if all, _ = repo.All(ctx); len(all) != 1 {
		t.Fatalf("after delete: %d", len(all))
	}
}
