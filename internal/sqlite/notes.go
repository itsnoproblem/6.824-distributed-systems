package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/notes"
	"github.com/itsnoproblem/mit-distributed-systems/pkg/api"
)

type NotesRepo struct{ db *sql.DB }

func NewNotesRepo(db *sql.DB) *NotesRepo { return &NotesRepo{db} }

const noteCols = "id, module_slug, step_slug, body_md, created_at, updated_at"

func scanNote(row interface{ Scan(...any) error }) (notes.Note, error) {
	var n notes.Note
	var created, updated string
	if err := row.Scan(&n.ID, &n.Ref.Module, &n.Ref.Step, &n.Body, &created, &updated); err != nil {
		return notes.Note{}, err
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339, created)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return n, nil
}

func (r *NotesRepo) Insert(ctx context.Context, n notes.Note) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO notes (module_slug, step_slug, body_md, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		n.Ref.Module, n.Ref.Step, n.Body,
		n.CreatedAt.Format(time.RFC3339), n.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *NotesRepo) Update(ctx context.Context, id int64, body string, updatedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE notes SET body_md = ?, updated_at = ? WHERE id = ?",
		body, updatedAt.Format(time.RFC3339), id)
	return err
}

// Delete is idempotent: removing an already-deleted note is a no-op, so a
// double-click on the drawer's delete button never surfaces an error.
func (r *NotesRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM notes WHERE id = ?", id)
	return err
}

func (r *NotesRepo) Get(ctx context.Context, id int64) (notes.Note, error) {
	n, err := scanNote(r.db.QueryRowContext(ctx,
		"SELECT "+noteCols+" FROM notes WHERE id = ?", id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notes.Note{}, fmt.Errorf("%w: note %d", api.ErrNotFound, id)
		}
		return notes.Note{}, err
	}
	return n, nil
}

func (r *NotesRepo) ForStep(ctx context.Context, ref course.StepRef) ([]notes.Note, error) {
	return r.query(ctx,
		"SELECT "+noteCols+" FROM notes WHERE module_slug = ? AND step_slug = ? ORDER BY created_at DESC",
		ref.Module, ref.Step)
}

func (r *NotesRepo) All(ctx context.Context) ([]notes.Note, error) {
	return r.query(ctx,
		"SELECT "+noteCols+" FROM notes ORDER BY module_slug, created_at DESC")
}

func (r *NotesRepo) query(ctx context.Context, q string, args ...any) ([]notes.Note, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []notes.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
