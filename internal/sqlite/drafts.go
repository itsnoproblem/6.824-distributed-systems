package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type DraftsRepo struct{ db *sql.DB }

func NewDraftsRepo(db *sql.DB) *DraftsRepo { return &DraftsRepo{db} }

func (r *DraftsRepo) Upsert(ctx context.Context, ref course.StepRef, files map[string]string) error {
	raw, err := json.Marshal(files)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO drafts (module_slug, step_slug, files_json, updated_at)
		 VALUES (?, ?, ?, ?)`,
		ref.Module, ref.Step, string(raw), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (r *DraftsRepo) Get(ctx context.Context, ref course.StepRef) (map[string]string, bool, error) {
	var raw string
	err := r.db.QueryRowContext(ctx,
		"SELECT files_json FROM drafts WHERE module_slug = ? AND step_slug = ?",
		ref.Module, ref.Step).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var files map[string]string
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return nil, false, err
	}
	return files, true, nil
}

func (r *DraftsRepo) Delete(ctx context.Context, ref course.StepRef) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM drafts WHERE module_slug = ? AND step_slug = ?", ref.Module, ref.Step)
	return err
}
