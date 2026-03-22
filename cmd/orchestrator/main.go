package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/distributed_task_queue/distributed_task_queue/internal/config"
	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
	"github.com/distributed_task_queue/distributed_task_queue/internal/orchestrator"
	appredis "github.com/distributed_task_queue/distributed_task_queue/internal/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
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

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go reconcileLoop(sigCtx, pool, rdb, cfg)

	mux := http.NewServeMux()
	srv := &orchestrator.Server{Pool: pool, RDB: rdb, Prefix: cfg.RedisKeyPrefix}
	srv.Register(mux)
	handler := orchestrator.MetricsMiddleware(mux)

	httpServer := &http.Server{
		Addr:              cfg.OrchestratorListen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("orchestrator: listening on %s", cfg.OrchestratorListen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-sigCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

func reconcileLoop(ctx context.Context, pool *pgxpool.Pool, rdb *goredis.Client, cfg config.Config) {
	t := time.NewTicker(cfg.ReconcileInterval)
	defer t.Stop()
	for {
		n, err := orchestrator.ReconcileOnce(ctx, pool, rdb, cfg)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("reconciler: %v", err)
		} else {
			orchestrator.ReconcileEnqueuedTasksTotal.Add(float64(n))
			if n > 0 {
				log.Printf("reconciler: enqueued %d due queued task(s)", n)
			}
		}

		r, err := orchestrator.ReclaimStaleRunningOnce(ctx, pool, rdb, cfg)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("reconciler: stale running: %v", err)
		} else {
			orchestrator.ReconcileStaleRunningReclaimedTotal.Add(float64(r))
			if r > 0 {
				log.Printf("reconciler: reclaimed %d stale running task(s)", r)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
