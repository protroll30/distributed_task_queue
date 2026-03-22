package db

import (
	"context"

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
