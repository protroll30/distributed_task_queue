package orchestrator

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
	appredis "github.com/distributed_task_queue/distributed_task_queue/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

// SubmitTask creates a job with one queued task, commits, then enqueues the task ID on the default ready list.
func SubmitTask(ctx context.Context, pool *pgxpool.Pool, rdb *goredis.Client, prefix, kind string, payload []byte) (taskID uuid.UUID, err error) {
	jobName := fmt.Sprintf("job-%s", uuid.New().String())
	taskID, err = db.CreateJobWithQueuedTask(ctx, pool, jobName, "", kind, payload, 3)
	if err != nil {
		return uuid.Nil, err
	}
	key := appredis.ReadyList(prefix, appredis.DefaultPriority)
	if err := appredis.EnqueueReady(ctx, rdb, key, taskID.String()); err != nil {
		return uuid.Nil, err
	}
	return taskID, nil
}
