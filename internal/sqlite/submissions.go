package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/itsnoproblem/mit-distributed-systems/internal/course"
	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

type SubmissionRepo struct{ db *sql.DB }

func NewSubmissionRepo(db *sql.DB) *SubmissionRepo { return &SubmissionRepo{db} }

func (r *SubmissionRepo) InsertSubmission(ctx context.Context, s eval.Submission) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO submissions (module_slug, step_slug, kind, content, test_output, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.Ref.Module, s.Ref.Step, string(s.Kind), s.Content, s.TestOutput, string(s.Status),
		s.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *SubmissionRepo) UpdateSubmission(ctx context.Context, id int64, status eval.Status, testOutput string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE submissions SET status = ?, test_output = ? WHERE id = ?",
		string(status), testOutput, id)
	return err
}

const subCols = "id, module_slug, step_slug, kind, content, test_output, status, created_at"

func scanSubmission(row interface{ Scan(...any) error }) (eval.Submission, error) {
	var s eval.Submission
	var kind, status, created string
	if err := row.Scan(&s.ID, &s.Ref.Module, &s.Ref.Step, &kind, &s.Content,
		&s.TestOutput, &status, &created); err != nil {
		return eval.Submission{}, err
	}
	s.Kind, s.Status = eval.Kind(kind), eval.Status(status)
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return s, nil
}

func (r *SubmissionRepo) GetSubmission(ctx context.Context, id int64) (eval.Submission, error) {
	return scanSubmission(r.db.QueryRowContext(ctx,
		"SELECT "+subCols+" FROM submissions WHERE id = ?", id))
}

func (r *SubmissionRepo) LatestForStep(ctx context.Context, ref course.StepRef) (*eval.Submission, error) {
	s, err := scanSubmission(r.db.QueryRowContext(ctx,
		"SELECT "+subCols+` FROM submissions
		 WHERE module_slug = ? AND step_slug = ? ORDER BY id DESC LIMIT 1`,
		ref.Module, ref.Step))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SubmissionRepo) InsertEvaluation(ctx context.Context, e eval.Evaluation) (int64, error) {
	verdict, err := json.Marshal(e.Verdict)
	if err != nil {
		return 0, err
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO evaluations (submission_id, model, rubric_version, verdict_json, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		e.SubmissionID, e.Model, e.RubricVersion, string(verdict),
		e.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *SubmissionRepo) EvaluationForSubmission(ctx context.Context, submissionID int64) (*eval.Evaluation, error) {
	var e eval.Evaluation
	var verdict, created string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, submission_id, model, rubric_version, verdict_json, created_at
		 FROM evaluations WHERE submission_id = ? ORDER BY id DESC LIMIT 1`, submissionID).
		Scan(&e.ID, &e.SubmissionID, &e.Model, &e.RubricVersion, &verdict, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(verdict), &e.Verdict); err != nil {
		return nil, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &e, nil
}
