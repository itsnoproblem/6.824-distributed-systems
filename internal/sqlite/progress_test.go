package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func testDB(t *testing.T) *sqlite.ProgressRepo {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewProgressRepo(db)
}

func TestProgressRoundTrip(t *testing.T) {
	repo := testDB(t)
	ctx := context.Background()
	ref := course.StepRef{Module: "m1", Step: "s1"}

	if err := repo.SetComplete(ctx, ref, true); err != nil {
		t.Fatal(err)
	}
	// marking twice is fine
	if err := repo.SetComplete(ctx, ref, true); err != nil {
		t.Fatal(err)
	}
	done, err := repo.Completed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := done[ref]; !ok || len(done) != 1 {
		t.Fatalf("completed = %v", done)
	}
	if err := repo.SetComplete(ctx, ref, false); err != nil {
		t.Fatal(err)
	}
	done, _ = repo.Completed(ctx)
	if len(done) != 0 {
		t.Fatalf("expected empty after unmark, got %v", done)
	}
}
