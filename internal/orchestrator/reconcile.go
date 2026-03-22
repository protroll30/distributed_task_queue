package orchestrator

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/distributed_task_queue/distributed_task_queue/internal/config"
	appredis "github.com/distributed_task_queue/distributed_task_queue/internal/redis"
	goredis "github.com/redis/go-redis/v9"
)

// ReconcileOnce finds queued tasks whose scheduled time has passed and LPUSHes them after TryReservePending.
// This heals Redis loss or missed enqueue without flooding duplicates (pending marker + worker claim release).
func ReconcileOnce(ctx context.Context, pool *pgxpool.Pool, rdb *goredis.Client, cfg config.Config) (enqueued int, err error) {
	const q = `
SELECT id FROM tasks
WHERE status = 'queued' AND scheduled_at <= now()
ORDER BY scheduled_at ASC
LIMIT 200`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	readyKey := appredis.ReadyList(cfg.RedisKeyPrefix, appredis.DefaultPriority)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return enqueued, err
		}
		ok, err := appredis.TryReservePending(ctx, rdb, cfg.RedisKeyPrefix, id.String(), 5*time.Minute)
		if err != nil {
			return enqueued, err
		}
		if !ok {
			continue
		}
		if err := appredis.EnqueueReady(ctx, rdb, readyKey, id.String()); err != nil {
			_ = appredis.ReleasePending(ctx, rdb, cfg.RedisKeyPrefix, id.String())
			return enqueued, err
		}
		enqueued++
	}
	return enqueued, rows.Err()
}
