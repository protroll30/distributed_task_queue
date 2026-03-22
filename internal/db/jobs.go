package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateJobWithQueuedTask inserts a pending job and one queued task in a single transaction.
func CreateJobWithQueuedTask(ctx context.Context, pool *pgxpool.Pool, jobName, taskName, kind string, payload []byte, maxAttempts int) (taskID uuid.UUID, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var jobID uuid.UUID
	if err := tx.QueryRow(ctx, `
INSERT INTO jobs (name, status) VALUES ($1, 'pending') RETURNING id`, jobName).Scan(&jobID); err != nil {
		return uuid.Nil, err
	}
	if err := tx.QueryRow(ctx, `
INSERT INTO tasks (job_id, name, kind, status, payload, max_attempts, attempt, scheduled_at)
VALUES ($1, $2, $3, 'queued', $4, $5, 0, now())
RETURNING id`, jobID, taskName, kind, payload, maxAttempts).Scan(&taskID); err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return taskID, nil
}

// RefreshJobStatus sets jobs.status from tasks: any failed → failed; all completed → completed;
// all cancelled → cancelled; otherwise running.
func RefreshJobStatus(ctx context.Context, pool *pgxpool.Pool, jobID uuid.UUID) error {
	const q = `
SELECT
	count(*) FILTER (WHERE status = 'failed') AS n_failed,
	count(*) FILTER (WHERE status = 'completed') AS n_done,
	count(*) FILTER (WHERE status = 'cancelled') AS n_cancelled,
	count(*) AS n_total
FROM tasks WHERE job_id = $1`
	var nFailed, nDone, nCancel, nTotal int
	if err := pool.QueryRow(ctx, q, jobID).Scan(&nFailed, &nDone, &nCancel, &nTotal); err != nil {
		return err
	}
	if nTotal == 0 {
		return fmt.Errorf("db: refresh job: no tasks for job %s", jobID)
	}
	var status string
	switch {
	case nFailed > 0:
		status = "failed"
	case nDone+nCancel == nTotal:
		if nCancel == nTotal {
			status = "cancelled"
		} else {
			status = "completed"
		}
	default:
		status = "running"
	}
	tag, err := pool.Exec(ctx, `UPDATE jobs SET status = $2, updated_at = now() WHERE id = $1`, jobID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("db: refresh job: no job row for %s", jobID)
	}
	return nil
}
