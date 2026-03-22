package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Config holds process configuration from the environment. Values match INSTRUCTIONS.md (operational reference).
type Config struct {
	CRDBDSN            string
	RedisAddr          string
	RedisKeyPrefix     string
	OrchestratorListen string
	ReconcileInterval  time.Duration
	WorkerID             string
	WorkerConcurrency    int
	LeaseDuration        time.Duration
	RetryBackoff         time.Duration
	// StaleRunningAfter is the minimum time a task may stay status=running before the
	// reconciler considers reclaiming it when the Redis lease key is absent.
	StaleRunningAfter time.Duration
}

// Load reads configuration from the environment. CRDB_DSN is required; other fields use documented defaults.
func Load() (Config, error) {
	c := Config{
		RedisAddr:          getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisKeyPrefix:     getEnv("REDIS_KEY_PREFIX", "dto:"),
		OrchestratorListen: getEnv("ORCHESTRATOR_LISTEN", ":8080"),
		ReconcileInterval:  30 * time.Second,
		LeaseDuration:      30 * time.Second,
		RetryBackoff:       5 * time.Second,
		WorkerConcurrency:  1,
	}

	if v := os.Getenv("CRDB_DSN"); v == "" {
		return Config{}, fmt.Errorf("CRDB_DSN is required")
	} else {
		c.CRDBDSN = v
	}

	if v := os.Getenv("LEASE_DURATION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("LEASE_DURATION: %w", err)
		}
		c.LeaseDuration = d
	}

	if v := os.Getenv("RECONCILE_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("RECONCILE_INTERVAL: %w", err)
		}
		c.ReconcileInterval = d
	}

	if v := os.Getenv("RETRY_BACKOFF"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("RETRY_BACKOFF: %w", err)
		}
		c.RetryBackoff = d
	}

	c.StaleRunningAfter = 2 * c.LeaseDuration
	if v := os.Getenv("STALE_RUNNING_AFTER"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("STALE_RUNNING_AFTER: %w", err)
		}
		c.StaleRunningAfter = d
	}

	if v := os.Getenv("WORKER_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("WORKER_CONCURRENCY must be a positive integer")
		}
		c.WorkerConcurrency = n
	}

	c.WorkerID = os.Getenv("WORKER_ID")
	if c.WorkerID == "" {
		c.WorkerID = uuid.New().String()
	}

	return c, nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
