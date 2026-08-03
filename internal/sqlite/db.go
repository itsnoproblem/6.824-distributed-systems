// Package sqlite implements the app's repositories on a local SQLite file.
package sqlite

import (
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
func applyMigration(db *sql.DB, name, sqlText string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once Commit succeeds

	if _, err := tx.Exec(sqlText); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version) VALUES (?)", name); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %s: %w", name, err)
	}
	return nil
}
