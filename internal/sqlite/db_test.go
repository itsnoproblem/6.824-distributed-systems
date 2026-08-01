package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/sqlite"
)

func TestOpenAndMigrate(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	// idempotent
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	for _, table := range []string{"progress", "notes", "submissions", "evaluations"} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}
