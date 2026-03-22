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

// Job is a jobs row for read APIs.
type Job struct {
	ID        uuid.UUID
	Name      string
	Status    string
	Metadata  []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ErrJobNotFound is returned when no row matches the job id.
var ErrJobNotFound = errors.New("db: job not found")

// GetJobByID loads a job by primary key.
func GetJobByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (Job, error) {
	const q = `
SELECT id, name, status, COALESCE(metadata, 'null'::jsonb), created_at, updated_at
FROM jobs WHERE id = $1`
	var j Job
	err := pool.QueryRow(ctx, q, id).Scan(
		&j.ID, &j.Name, &j.Status, &j.Metadata, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrJobNotFound
		}
		return Job{}, err
	}
	return j, nil
}

// TaskSpec defines one task inside a job. DependsOn lists other task names in the same job.
type TaskSpec struct {
	Name      string
	Kind      string
	Payload   []byte
	DependsOn []string
}

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

// CreateJobWithTasks inserts a job, tasks, and dependency edges in one transaction.
// Tasks with no dependencies are queued; others start pending until dependencies complete.
func CreateJobWithTasks(ctx context.Context, pool *pgxpool.Pool, jobName string, maxAttempts int, specs []TaskSpec) (jobID uuid.UUID, nameToID map[string]uuid.UUID, initialEnqueue []uuid.UUID, err error) {
	if len(specs) == 0 {
		return uuid.Nil, nil, nil, fmt.Errorf("db: at least one task is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, nil, nil, err
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx, `
INSERT INTO jobs (name, status) VALUES ($1, 'pending') RETURNING id`, jobName).Scan(&jobID); err != nil {
		return uuid.Nil, nil, nil, err
	}

	nameToID = make(map[string]uuid.UUID, len(specs))
	for _, sp := range specs {
		st := "pending"
		if len(sp.DependsOn) == 0 {
			st = "queued"
		}
		var tid uuid.UUID
		if err := tx.QueryRow(ctx, `
INSERT INTO tasks (job_id, name, kind, status, payload, max_attempts, attempt, scheduled_at)
VALUES ($1, $2, $3, $4, $5, $6, 0, now())
RETURNING id`, jobID, sp.Name, sp.Kind, st, sp.Payload, maxAttempts).Scan(&tid); err != nil {
			return uuid.Nil, nil, nil, err
		}
		nameToID[sp.Name] = tid
		if st == "queued" {
			initialEnqueue = append(initialEnqueue, tid)
		}
	}

	for _, sp := range specs {
		tid := nameToID[sp.Name]
		for _, depName := range sp.DependsOn {
			did, ok := nameToID[depName]
			if !ok {
				return uuid.Nil, nil, nil, fmt.Errorf("db: unknown dependency name %q for task %q", depName, sp.Name)
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO task_dependencies (task_id, depends_on_task_id) VALUES ($1, $2)`, tid, did); err != nil {
				return uuid.Nil, nil, nil, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, nil, nil, err
	}
	return jobID, nameToID, initialEnqueue, nil
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
