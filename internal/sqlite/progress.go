package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
)

type ProgressRepo struct{ db *sql.DB }

func NewProgressRepo(db *sql.DB) *ProgressRepo { return &ProgressRepo{db} }

func (p *ProgressRepo) SetComplete(ctx context.Context, ref course.StepRef, done bool) error {
	if !done {
		_, err := p.db.ExecContext(ctx,
			"DELETE FROM progress WHERE module_slug = ? AND step_slug = ?", ref.Module, ref.Step)
		return err
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO progress (module_slug, step_slug, completed_at) VALUES (?, ?, ?)`,
		ref.Module, ref.Step, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (p *ProgressRepo) Completed(ctx context.Context) (map[course.StepRef]time.Time, error) {
	rows, err := p.db.QueryContext(ctx,
		"SELECT module_slug, step_slug, completed_at FROM progress")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[course.StepRef]time.Time{}
	for rows.Next() {
		var ref course.StepRef
		var at string
		if err := rows.Scan(&ref.Module, &ref.Step, &at); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return nil, err
		}
		out[ref] = ts
	}
	return out, rows.Err()
}
