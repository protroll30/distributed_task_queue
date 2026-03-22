package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"

	"github.com/distributed_task_queue/distributed_task_queue/internal/config"
	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
	appredis "github.com/distributed_task_queue/distributed_task_queue/internal/redis"
	wruntime "github.com/distributed_task_queue/distributed_task_queue/internal/worker"
	pkgworker "github.com/distributed_task_queue/distributed_task_queue/pkg/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.CRDBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	rdb := appredis.New(cfg.RedisAddr)
	defer rdb.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("db ping: %v", err)
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	rt := wruntime.NewRuntime(pkgworker.Options{}, cfg, pool, rdb)
	rt.Register("echo", func(ctx context.Context, t pkgworker.Task) error {
		log.Printf("echo: kind=%s attempt=%d payload=%s", t.Kind, t.Attempt, string(t.Payload))
		return nil
	})

	log.Printf("worker: id=%s concurrency=%d (echo handler registered)", cfg.WorkerID, cfg.WorkerConcurrency)
	if err := rt.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("worker: %v", err)
	}
}
