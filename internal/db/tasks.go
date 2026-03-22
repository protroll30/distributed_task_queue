package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Task is a tasks row used by the worker and orchestrator.
type Task struct {
	ID          uuid.UUID
	JobID       uuid.UUID
	Name        string
	Kind        string
	Status      string
	Payload     []byte
	MaxAttempts int
	Attempt     int
	ScheduledAt time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	LastError   *string
}

var (
	// ErrTaskNotFound is returned when no row matches the task id.
	ErrTaskNotFound = errors.New("db: task not found")
	// ErrTaskNotClaimable is returned when ClaimQueuedTask cannot move queued → running (wrong status or lost race).
	ErrTaskNotClaimable = errors.New("db: task not claimable")
)

// GetTaskByID loads a task by primary key.
func GetTaskByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (Task, error) {
	const q = `
SELECT id, job_id, name, kind, status, payload, max_attempts, attempt, scheduled_at, started_at, finished_at, last_error
FROM tasks WHERE id = $1`
	var t Task
	err := pool.QueryRow(ctx, q, id).Scan(
		&t.ID, &t.JobID, &t.Name, &t.Kind, &t.Status, &t.Payload, &t.MaxAttempts, &t.Attempt,
		&t.ScheduledAt, &t.StartedAt, &t.FinishedAt, &t.LastError,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, err
	}
	return t, nil
}

// ListTasksByJobID returns all tasks for a job, ordered by name then id.
func ListTasksByJobID(ctx context.Context, pool *pgxpool.Pool, jobID uuid.UUID) ([]Task, error) {
	const q = `
SELECT id, job_id, name, kind, status, payload, max_attempts, attempt, scheduled_at, started_at, finished_at, last_error
FROM tasks WHERE job_id = $1
ORDER BY name ASC, id ASC`
	rows, err := pool.Query(ctx, q, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.JobID, &t.Name, &t.Kind, &t.Status, &t.Payload, &t.MaxAttempts, &t.Attempt,
			&t.ScheduledAt, &t.StartedAt, &t.FinishedAt, &t.LastError,
		); err != nil {
			return out, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ClaimQueuedTask atomically increments attempt, sets status=running, and timestamps for a queued task.
// If the task is not queued, returns ErrTaskNotClaimable.
func ClaimQueuedTask(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (Task, error) {
	const q = `
UPDATE tasks SET
  attempt = attempt + 1,
  status = 'running',
  started_at = now(),
  updated_at = now()
WHERE id = $1 AND status = 'queued'
RETURNING id, job_id, name, kind, status, payload, max_attempts, attempt, scheduled_at, started_at, finished_at, last_error`
	var t Task
	err := pool.QueryRow(ctx, q, id).Scan(
		&t.ID, &t.JobID, &t.Name, &t.Kind, &t.Status, &t.Payload, &t.MaxAttempts, &t.Attempt,
		&t.ScheduledAt, &t.StartedAt, &t.FinishedAt, &t.LastError,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrTaskNotClaimable
		}
		return Task{}, err
	}
	return t, nil
}

// InsertTaskRun records the start of an attempt; status is running.
func InsertTaskRun(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, attemptNumber int, workerID string) (runID uuid.UUID, err error) {
	const q = `
INSERT INTO task_runs (task_id, attempt_number, worker_id, status)
VALUES ($1, $2, $3, 'running')
RETURNING id`
	err = pool.QueryRow(ctx, q, taskID, attemptNumber, workerID).Scan(&runID)
	if err != nil {
		return uuid.Nil, err
	}
	return runID, nil
}

// CompleteTaskSuccess marks the run succeeded and the task completed in one transaction.
func CompleteTaskSuccess(ctx context.Context, pool *pgxpool.Pool, runID, taskID uuid.UUID) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
UPDATE task_runs SET status = 'succeeded', finished_at = now() WHERE id = $1`, runID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
UPDATE tasks SET status = 'completed', finished_at = now(), updated_at = now(), last_error = NULL WHERE id = $1`, taskID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("db: complete task: no row for id %s", taskID)
	}
	return tx.Commit(ctx)
}

// FailTaskRetry marks the run failed, requeues the task with last_error and scheduled_at, for another attempt.
func FailTaskRetry(ctx context.Context, pool *pgxpool.Pool, runID, taskID uuid.UUID, errMsg string, runAfter time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
UPDATE task_runs SET status = 'failed', finished_at = now(), error = $2 WHERE id = $1`, runID, errMsg); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE tasks SET status = 'queued', last_error = $2, scheduled_at = $3, updated_at = now() WHERE id = $1`,
		taskID, errMsg, runAfter); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FailTaskPermanent marks the run failed and the task failed (no further retries).
func FailTaskPermanent(ctx context.Context, pool *pgxpool.Pool, runID, taskID uuid.UUID, errMsg string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
UPDATE task_runs SET status = 'failed', finished_at = now(), error = $2 WHERE id = $1`, runID, errMsg); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE tasks SET status = 'failed', finished_at = now(), last_error = $2, updated_at = now() WHERE id = $1`,
		taskID, errMsg); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
