package redis

import "fmt"

// DefaultPriority is the ready-queue tier used when none is specified.
const DefaultPriority = "default"

// ReadyList returns the Redis LIST key for runnable tasks (LPUSH / BRPOP).
// Format: {prefix}queue:ready:{priority} — see INSTRUCTIONS.md §5.
func ReadyList(prefix, priority string) string {
	return fmt.Sprintf("%squeue:ready:%s", prefix, priority)
}

// LeaseTask returns the hash key for in-flight lease metadata for a task ID string.
// Format: {prefix}lease:task:{task_id}
func LeaseTask(prefix, taskID string) string {
	return fmt.Sprintf("%slease:task:%s", prefix, taskID)
}

// ScheduledZSet returns the sorted set key for delayed work (score = run-after Unix ms).
// Format: {prefix}queue:scheduled
func ScheduledZSet(prefix string) string {
	return fmt.Sprintf("%squeue:scheduled", prefix)
}

// ClaimToken returns an optional short-lived dedup key for claim races (SET NX EX).
// Format: {prefix}claim:{task_id}
func ClaimToken(prefix, taskID string) string {
	return fmt.Sprintf("%sclaim:%s", prefix, taskID)
}

// WakeScheduler returns the pub/sub channel used to nudge the reconciler.
// Format: {prefix}wake:scheduler
func WakeScheduler(prefix string) string {
	return fmt.Sprintf("%swake:scheduler", prefix)
}
