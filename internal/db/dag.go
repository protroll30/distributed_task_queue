package db

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PromoteUnblockedTasks moves tasks in the same job as triggerTaskID from pending → queued
// when every dependency is completed. Returns task IDs that were promoted (caller should enqueue).
func PromoteUnblockedTasks(ctx context.Context, pool *pgxpool.Pool, triggerTaskID uuid.UUID) ([]uuid.UUID, error) {
	const q = `
UPDATE tasks SET
  status = 'queued',
  scheduled_at = now(),
  updated_at = now()
WHERE job_id = (SELECT job_id FROM tasks WHERE id = $1)
  AND status = 'pending'
  AND NOT EXISTS (
    SELECT 1 FROM task_dependencies td
    JOIN tasks t2 ON t2.id = td.depends_on_task_id
    WHERE td.task_id = tasks.id AND t2.status <> 'completed'
  )
RETURNING id`
	rows, err := pool.Query(ctx, q, triggerTaskID)
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

// CascadeFailPendingDependents fails pending tasks that depend (transitively) on failedTaskID.
// The root task must already be marked failed in the database.
func CascadeFailPendingDependents(ctx context.Context, pool *pgxpool.Pool, failedTaskID uuid.UUID, upstreamErr string) error {
	msg := "upstream task failed: " + strings.TrimSpace(upstreamErr)
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	frontier := []uuid.UUID{failedTaskID}
	for len(frontier) > 0 {
		u := frontier[0]
		frontier = frontier[1:]
		rows, err := pool.Query(ctx, `
SELECT t.id FROM tasks t
INNER JOIN task_dependencies td ON td.task_id = t.id
WHERE td.depends_on_task_id = $1 AND t.status = 'pending'`, u)
		if err != nil {
			return err
		}
		var children []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			children = append(children, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range children {
			tag, err := pool.Exec(ctx, `
UPDATE tasks SET status = 'failed', finished_at = now(), last_error = $2, updated_at = now()
WHERE id = $1 AND status = 'pending'`, id, msg)
			if err != nil {
				return err
			}
			if tag.RowsAffected() > 0 {
				frontier = append(frontier, id)
			}
		}
	}
	return nil
}
