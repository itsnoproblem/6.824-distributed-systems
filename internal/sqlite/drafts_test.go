package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func draftsRepo(t *testing.T) *sqlite.DraftsRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewDraftsRepo(db)
}

func TestDraftLifecycle(t *testing.T) {
	repo := draftsRepo(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "c1"}

	if _, ok, err := repo.Get(ctx, ref); err != nil || ok {
		t.Fatalf("expected no draft: ok=%v err=%v", ok, err)
	}
	if err := repo.Upsert(ctx, ref, map[string]string{"a.go": "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, ref, map[string]string{"a.go": "v2"}); err != nil {
		t.Fatal(err)
	}
	files, ok, err := repo.Get(ctx, ref)
	if err != nil || !ok || files["a.go"] != "v2" {
		t.Fatalf("get: %v %v %v", files, ok, err)
	}
	if err := repo.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ = repo.Get(ctx, ref); ok {
		t.Fatal("expected draft gone after delete")
	}
}
