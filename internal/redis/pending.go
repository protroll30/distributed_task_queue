package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// PendingEnqueueKey marks that a task id was recently placed on the ready list (or is in flight) to reduce duplicate LPUSH from the reconciler.
// Format: {prefix}queue:pending:{task_id}
func PendingEnqueueKey(prefix, taskID string) string {
	return fmt.Sprintf("%squeue:pending:%s", prefix, taskID)
}

// TryReservePending sets a short-lived key with SET NX so only one producer enqueues at a time.
// Returns true if this caller reserved the key (should enqueue to Redis).
func TryReservePending(ctx context.Context, rdb *goredis.Client, prefix, taskID string, ttl time.Duration) (bool, error) {
	return rdb.SetNX(ctx, PendingEnqueueKey(prefix, taskID), "1", ttl).Result()
}

// ReleasePending deletes the pending marker (e.g. after a successful claim).
func ReleasePending(ctx context.Context, rdb *goredis.Client, prefix, taskID string) error {
	return rdb.Del(ctx, PendingEnqueueKey(prefix, taskID)).Err()
}
