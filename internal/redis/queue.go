package redis

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// EnqueueReady pushes taskID onto the head of the ready LIST (LPUSH).
func EnqueueReady(ctx context.Context, rdb *goredis.Client, key, taskID string) error {
	return rdb.LPush(ctx, key, taskID).Err()
}

// BlockingPopReady blocks up to block for a task ID from the tail of the ready LIST (BRPOP).
// On timeout it returns ("", redis.Nil). Other errors are returned as-is.
func BlockingPopReady(ctx context.Context, rdb *goredis.Client, key string, block time.Duration) (taskID string, err error) {
	res, err := rdb.BRPop(ctx, block, key).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return "", err
		}
		return "", err
	}
	if len(res) < 2 {
		return "", errors.New("redis: unexpected BRPOP payload")
	}
	return res[1], nil
}

// ReadyLen returns the length of the ready LIST (LLEN), for metrics or backpressure.
func ReadyLen(ctx context.Context, rdb *goredis.Client, key string) (int64, error) {
	return rdb.LLen(ctx, key).Result()
}

// EnqueueTaskIDs LPUSHes each task ID on the default ready list after TryReservePending.
func EnqueueTaskIDs(ctx context.Context, rdb *goredis.Client, prefix, priority string, ids []string) error {
	key := ReadyList(prefix, priority)
	for _, id := range ids {
		if _, err := TryReservePending(ctx, rdb, prefix, id, 5*time.Minute); err != nil {
			return err
		}
		if err := EnqueueReady(ctx, rdb, key, id); err != nil {
			_ = ReleasePending(ctx, rdb, prefix, id)
			return err
		}
	}
	return nil
}
