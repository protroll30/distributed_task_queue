package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/distributed_task_queue/distributed_task_queue/internal/config"
	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
	appredis "github.com/distributed_task_queue/distributed_task_queue/internal/redis"
	pkgworker "github.com/distributed_task_queue/distributed_task_queue/pkg/worker"
)

// Runtime implements pkg/worker.Runtime using Redis ready lists and Cockroach task state.
type Runtime struct {
	mu       sync.RWMutex
	handlers map[string]pkgworker.Handler
	opts     pkgworker.Options
	cfg      config.Config
	pool     *pgxpool.Pool
	rdb      *goredis.Client
	readyKey string
}

// NewRuntime builds a worker runtime. opts zero fields are replaced with safe defaults.
func NewRuntime(opts pkgworker.Options, cfg config.Config, pool *pgxpool.Pool, rdb *goredis.Client) pkgworker.Runtime {
	if opts.DefaultTimeout == 0 {
		opts.DefaultTimeout = 5 * time.Minute
	}
	if opts.HeartbeatEvery == 0 {
		opts.HeartbeatEvery = 5 * time.Second
	}
	return &Runtime{
		opts:     opts,
		cfg:      cfg,
		pool:     pool,
		rdb:      rdb,
		readyKey: appredis.ReadyList(cfg.RedisKeyPrefix, appredis.DefaultPriority),
	}
}

// Register binds a handler for kind. Panics if kind is registered twice.
func (r *Runtime) Register(kind string, h pkgworker.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = make(map[string]pkgworker.Handler)
	}
	if _, dup := r.handlers[kind]; dup {
		panic("worker: duplicate handler kind: " + kind)
	}
	r.handlers[kind] = h
}

func (r *Runtime) handlerFor(kind string) pkgworker.Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.handlers == nil {
		return nil
	}
	return r.handlers[kind]
}

// Run starts WorkerConcurrency polling loops until ctx is cancelled, then waits for them to exit.
func (r *Runtime) Run(ctx context.Context) error {
	n := r.cfg.WorkerConcurrency
	if n < 1 {
		n = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.runLoop(ctx)
		}()
	}
	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (r *Runtime) runLoop(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		taskIDStr, err := appredis.BlockingPopReady(ctx, r.rdb, r.readyKey, 2*time.Second)
		if err != nil {
			if errors.Is(err, goredis.Nil) {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("worker: brpop: %v", err)
			continue
		}
		id, err := uuid.Parse(taskIDStr)
		if err != nil {
			log.Printf("worker: bad task id %q: %v", taskIDStr, err)
			continue
		}
		r.processTask(ctx, id)
	}
}

func (r *Runtime) processTask(ctx context.Context, id uuid.UUID) {
	leaseKey := appredis.LeaseTask(r.cfg.RedisKeyPrefix, id.String())
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = appredis.DeleteLease(cctx, r.rdb, leaseKey)
	}()

	task, err := db.ClaimQueuedTask(ctx, r.pool, id)
	if err != nil {
		if errors.Is(err, db.ErrTaskNotClaimable) {
			return
		}
		log.Printf("worker: claim %s: %v", id, err)
		return
	}

	runID, err := db.InsertTaskRun(ctx, r.pool, task.ID, task.Attempt, r.cfg.WorkerID)
	if err != nil {
		log.Printf("worker: insert run: %v", err)
		return
	}

	deadline := time.Now().Add(r.cfg.LeaseDuration)
	keyTTL := r.cfg.LeaseDuration + 30*time.Second
	if err := appredis.SetLease(ctx, r.rdb, leaseKey, r.cfg.WorkerID, deadline, &runID, keyTTL); err != nil {
		log.Printf("worker: set lease: %v", err)
		return
	}

	h := r.handlerFor(task.Kind)
	if h == nil {
		_ = db.FailTaskPermanent(ctx, r.pool, runID, task.ID, "unknown task kind: "+task.Kind)
		return
	}

	hbCtx, hbStop := context.WithCancel(ctx)
	defer hbStop()
	go r.heartbeatLease(hbCtx, leaseKey, keyTTL)

	execCtx, cancel := context.WithTimeout(ctx, r.opts.DefaultTimeout)
	defer cancel()

	payload := json.RawMessage(task.Payload)
	if task.Payload == nil {
		payload = json.RawMessage(`null`)
	}
	pt := pkgworker.Task{
		ID:      task.ID,
		JobID:   task.JobID,
		Kind:    task.Kind,
		Payload: payload,
		Attempt: task.Attempt,
	}

	err = h(execCtx, pt)
	hbStop()
	if err == nil {
		if err := db.CompleteTaskSuccess(ctx, r.pool, runID, task.ID); err != nil {
			log.Printf("worker: complete: %v", err)
		}
		return
	}

	errMsg := err.Error()
	if task.Attempt >= task.MaxAttempts {
		if err := db.FailTaskPermanent(ctx, r.pool, runID, task.ID, errMsg); err != nil {
			log.Printf("worker: fail permanent: %v", err)
		}
		return
	}
	after := time.Now().Add(r.cfg.RetryBackoff)
	if err := db.FailTaskRetry(ctx, r.pool, runID, task.ID, errMsg, after); err != nil {
		log.Printf("worker: fail retry: %v", err)
		return
	}
	if err := appredis.EnqueueReady(ctx, r.rdb, r.readyKey, task.ID.String()); err != nil {
		log.Printf("worker: requeue: %v", err)
	}
}

func (r *Runtime) heartbeatLease(ctx context.Context, leaseKey string, keyTTL time.Duration) {
	t := time.NewTicker(r.opts.HeartbeatEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := appredis.RefreshLeaseTTL(ctx, r.rdb, leaseKey, keyTTL); err != nil {
				if errors.Is(err, appredis.ErrLeaseKeyMissing) {
					return
				}
				return
			}
		}
	}
}
