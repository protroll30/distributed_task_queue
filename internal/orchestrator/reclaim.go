package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/distributed_task_queue/distributed_task_queue/internal/config"
	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
	appredis "github.com/distributed_task_queue/distributed_task_queue/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

const reclaimBatch = 200

// ReclaimStaleRunningOnce finds long-running tasks with no Redis lease, returns them to queued, and LPUSHes after TryReservePending.
func ReclaimStaleRunningOnce(ctx context.Context, pool *pgxpool.Pool, rdb *goredis.Client, cfg config.Config) (reclaimed int, err error) {
	ids, err := db.ListStaleRunningCandidates(ctx, pool, cfg.StaleRunningAfter, reclaimBatch)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	readyKey := appredis.ReadyList(cfg.RedisKeyPrefix, appredis.DefaultPriority)
	reason := "stale running: no lease (reclaimed by orchestrator)"

	for _, id := range ids {
		leaseKey := appredis.LeaseTask(cfg.RedisKeyPrefix, id.String())
		ok, err := appredis.LeaseExists(ctx, rdb, leaseKey)
		if err != nil {
			return reclaimed, err
		}
		if ok {
			continue
		}

		jobID, err := db.ReclaimStaleRunningTask(ctx, pool, id, reason)
		if err != nil {
			if errors.Is(err, db.ErrTaskNotReclaimable) {
				continue
			}
			return reclaimed, err
		}
		_ = db.RefreshJobStatus(ctx, pool, jobID)

		reserved, err := appredis.TryReservePending(ctx, rdb, cfg.RedisKeyPrefix, id.String(), 5*time.Minute)
		if err != nil {
			return reclaimed, err
		}
		if !reserved {
			continue
		}
		if err := appredis.EnqueueReady(ctx, rdb, readyKey, id.String()); err != nil {
			_ = appredis.ReleasePending(ctx, rdb, cfg.RedisKeyPrefix, id.String())
			return reclaimed, err
		}
		reclaimed++
	}
	return reclaimed, nil
}
