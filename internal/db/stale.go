package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrTaskNotReclaimable is returned when ReclaimStaleRunningTask finds the task is not running (race or already finished).
var ErrTaskNotReclaimable = errors.New("db: task not reclaimable")

// ListStaleRunningCandidates returns running tasks that have been in-flight longer than minAge.
// The orchestrator should still verify Redis lease absence before reclaiming.
func ListStaleRunningCandidates(ctx context.Context, pool *pgxpool.Pool, minAge time.Duration, limit int) ([]uuid.UUID, error) {
	if limit < 1 {
		limit = 200
	}
	const q = `
SELECT id FROM tasks
WHERE status = 'running'
  AND started_at IS NOT NULL
  AND started_at < now() - ($1::BIGINT * '1 microsecond'::interval)
ORDER BY started_at ASC
LIMIT $2`
	rows, err := pool.Query(ctx, q, minAge.Microseconds(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return out, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ReclaimStaleRunningTask marks open task_runs as failed and returns the task to queued with one attempt
// rolled back so the next claim retries the same logical attempt budget.
func ReclaimStaleRunningTask(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, reason string) (jobID uuid.UUID, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
UPDATE task_runs SET status = 'failed', finished_at = now(), error = $2
WHERE task_id = $1 AND status = 'running' AND finished_at IS NULL`, taskID, reason); err != nil {
		return uuid.Nil, err
	}

	err = tx.QueryRow(ctx, `
UPDATE tasks SET
  status = 'queued',
  attempt = GREATEST(attempt - 1, 0),
  started_at = NULL,
  last_error = $2,
  scheduled_at = now(),
  updated_at = now()
WHERE id = $1 AND status = 'running'
RETURNING job_id`, taskID, reason).Scan(&jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrTaskNotReclaimable
		}
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return jobID, nil
}
