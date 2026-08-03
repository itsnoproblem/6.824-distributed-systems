// Package sqlite implements the app's repositories on a local SQLite file.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite",
		path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer; a single conn avoids SQLITE_BUSY entirely.
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

// Migrate applies every embedded migration not yet recorded.
func Migrate(db *sql.DB) error { return MigrateUpTo(db, "") }

// MigrateUpTo applies migrations in filename order, stopping after `last`
// when it is non-empty. Exists so tests can construct historical schemas.
func MigrateUpTo(db *sql.DB, last string) error {
	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)"); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := db.QueryRow(
			"SELECT count(*) FROM schema_migrations WHERE version = ?", name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if err := applyMigration(db, name, string(raw)); err != nil {
			return err
		}
		if name == last {
			return nil
		}
	}
	return nil
}

// applyMigration runs a migration's SQL and records its version atomically,
// so a crash between the two never leaves the schema and schema_migrations
// out of sync (which would otherwise wedge Migrate on the next startup).
//
// Some migrations rebuild an FK-referenced table via the documented
// create-copy-DROP-rename dance (e.g. 002_exercises.sql rebuilds
// submissions, which evaluations.submission_id references). With foreign
// keys enforced (Open() sets foreign_keys=1), DROP TABLE on a table with
// live incoming references fails outright. SQLite's documented procedure
// for this is: turn foreign_keys off for the duration (a no-op if toggled
// inside a transaction, so it must happen on a dedicated connection before
// BEGIN), apply the schema changes, verify no dangling references remain
// with PRAGMA foreign_key_check before committing, then turn foreign_keys
// back on. Applied to every migration, not just 002, so migration files
// stay plain declarative SQL.
func applyMigration(db *sql.DB, name, sqlText string) error {
	ctx := context.Background()

	// PRAGMAs are per-connection and database/sql pools connections, so the
	// foreign_keys toggle and the transaction below must share one conn.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	defer conn.Close() //nolint:errcheck // best-effort return to pool

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("migration %s: disable foreign_keys: %w", name, err)
	}
	// Always try to restore enforcement, even on an error path, so a
	// failed migration doesn't leave a connection with FKs silently off.
	defer conn.ExecContext(context.Background(), "PRAGMA foreign_keys=ON") //nolint:errcheck

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once Commit succeeds

	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version) VALUES (?)", name); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}

	if violation, err := checkForeignKeys(ctx, tx); err != nil {
		return fmt.Errorf("migration %s: foreign_key_check: %w", name, err)
	} else if violation != "" {
		return fmt.Errorf("migration %s: left dangling foreign key: %s", name, violation)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	return nil
}

// checkForeignKeys runs PRAGMA foreign_key_check and returns a description
// of the first violation found, or "" if the schema is consistent.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	if !rows.Next() {
		return "", rows.Err()
	}
	var table string
	var rowid sql.NullInt64
	var parent string
	var fkid int
	if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
		return "", err
	}
	return fmt.Sprintf("table=%s rowid=%v parent=%s fkid=%d", table, rowid, parent, fkid), nil
}
