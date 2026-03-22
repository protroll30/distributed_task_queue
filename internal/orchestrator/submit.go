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
	if err := appredis.EnqueueTaskIDs(ctx, rdb, prefix, appredis.DefaultPriority, []string{taskID.String()}); err != nil {
		return uuid.Nil, err
	}
	return taskID, nil
}

// SubmitJob creates a job with multiple tasks and edges. Tasks without dependencies are enqueued immediately.
// Call validateJobSpecs first.
func SubmitJob(ctx context.Context, pool *pgxpool.Pool, rdb *goredis.Client, prefix string, specs []db.TaskSpec) (jobID uuid.UUID, nameToID map[string]uuid.UUID, err error) {
	jobName := fmt.Sprintf("job-%s", uuid.New().String())
	jobID, nameToID, initial, err := db.CreateJobWithTasks(ctx, pool, jobName, 3, specs)
	if err != nil {
		return uuid.Nil, nil, err
	}
	strs := make([]string, 0, len(initial))
	for _, id := range initial {
		strs = append(strs, id.String())
	}
	if err := appredis.EnqueueTaskIDs(ctx, rdb, prefix, appredis.DefaultPriority, strs); err != nil {
		return uuid.Nil, nil, err
	}
	return jobID, nameToID, nil
}
