package db

import (
	"context"

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
