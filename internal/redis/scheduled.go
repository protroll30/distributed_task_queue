package redis

import (
	"context"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

// ScheduleAt adds taskID to the delayed ZSET with score = runAfterMs (Unix milliseconds).
func ScheduleAt(ctx context.Context, rdb *goredis.Client, key, taskID string, runAfterMs int64) error {
	z := goredis.Z{Score: float64(runAfterMs), Member: taskID}
	return rdb.ZAdd(ctx, key, z).Err()
}

// DueTaskIDs returns up to limit task IDs with score <= nowMs, lowest scores first.
func DueTaskIDs(ctx context.Context, rdb *goredis.Client, key string, nowMs int64, limit int64) ([]string, error) {
	if limit < 1 {
		limit = 100
	}
	// ZRANGEBYSCORE key min max WITHSCORES optional — use ZRangeByScore
	opt := &goredis.ZRangeBy{
		Min:   "-inf",
		Max:   strconv.FormatInt(nowMs, 10),
		Count: limit,
	}
	return rdb.ZRangeByScore(ctx, key, opt).Result()
}

// ZRem removes a task ID from the scheduled ZSET after promotion to the ready list.
func ZRem(ctx context.Context, rdb *goredis.Client, key string, taskIDs ...interface{}) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return rdb.ZRem(ctx, key, taskIDs...).Err()
}
