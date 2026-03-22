package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// ErrLeaseKeyMissing is returned when RefreshLeaseTTL finds no lease hash (expired or never set).
var ErrLeaseKeyMissing = errors.New("redis: lease key missing")

// SetLease writes lease metadata to a hash and sets keyTTL so the key expires if the worker disappears.
// keyTTL should exceed the logical lease duration (e.g. lease + buffer) so orphaned hashes vanish; CRDB reconciliation remains authoritative.
func SetLease(ctx context.Context, rdb *goredis.Client, key, workerID string, deadline time.Time, runID *uuid.UUID, keyTTL time.Duration) error {
	ms := strconv.FormatInt(deadline.UnixMilli(), 10)
	fields := map[string]interface{}{
		"worker_id":   workerID,
		"deadline_ms": ms,
	}
	if runID != nil {
		fields["run_id"] = runID.String()
	}
	pipe := rdb.TxPipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, keyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// DeleteLease removes the lease hash after successful ack or explicit release.
func DeleteLease(ctx context.Context, rdb *goredis.Client, key string) error {
	return rdb.Del(ctx, key).Err()
}

// RefreshLeaseTTL extends EXPIRE for heartbeat. If the key no longer exists, returns ErrLeaseKeyMissing.
func RefreshLeaseTTL(ctx context.Context, rdb *goredis.Client, key string, keyTTL time.Duration) error {
	ok, err := rdb.Expire(ctx, key, keyTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		return ErrLeaseKeyMissing
	}
	return nil
}
