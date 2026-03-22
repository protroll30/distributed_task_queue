# distributed_task_queue

Distributed task orchestrator in Go (CockroachDB + Redis). Design and schema: [INSTRUCTIONS.md](INSTRUCTIONS.md).

## Prerequisites

- Go 1.22+
- CockroachDB (PostgreSQL wire protocol)
- Redis

## Database migrations

Apply the initial schema:

```bash
cockroach sql --url "$CRDB_DSN" -f migrations/001_initial.sql
```

(Use your cluster URL or `postgresql://` DSN with pgx-compatible options.)

## Configuration

Loaded by both binaries via [`internal/config`](internal/config/config.go). `CRDB_DSN` is required.

| Variable | Default | Purpose |
|----------|---------|---------|
| `CRDB_DSN` | — | CockroachDB / Postgres DSN for `pgx` |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis address |
| `REDIS_KEY_PREFIX` | `dto:` | Prefix for Redis keys (see INSTRUCTIONS) |
| `ORCHESTRATOR_LISTEN` | `:8080` | Orchestrator HTTP bind (`GET /healthz`, `POST /v1/tasks`) |
| `RECONCILE_INTERVAL` | `30s` | Background reconciler: re-enqueue due `queued` tasks (CRDB → Redis) |
| `WORKER_ID` | random UUID | Stable worker identity if set |
| `WORKER_CONCURRENCY` | `1` | Parallel BRPOP worker loops |
| `LEASE_DURATION` | `30s` | Logical lease window; Redis key TTL adds a buffer |
| `RETRY_BACKOFF` | `5s` | Delay before a failed task is re-queued (when attempts remain) |

## Run (end-to-end)

1. Apply [migrations](migrations/001_initial.sql) to your CockroachDB cluster.
2. Start **Redis** (default `127.0.0.1:6379`).
3. In one terminal, run the **orchestrator** (HTTP API + enqueue).
4. In another, run the **worker** (registers a demo `echo` handler).

```bash
export CRDB_DSN='postgresql://root@localhost:26257/defaultdb?sslmode=disable'
go run ./cmd/orchestrator
go run ./cmd/worker
```

Submit a task (orchestrator listens on `:8080` by default):

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{"kind":"echo","payload":{"msg":"hi"}}'
```

Multi-task job with a DAG (`b` runs after `a` completes successfully):

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{"tasks":[{"name":"a","kind":"echo","payload":{"n":1}},{"name":"b","kind":"echo","payload":{"n":2},"depends_on":["a"]}]}'
```

Task names must be unique per job. Dependency edges are validated (unknown names, self-deps, and cycles return **400**). A dependent is promoted to `queued` only when **every** dependency is `completed` (a permanently failed upstream leaves dependents stuck in `pending` until a future release adds cascade).

The worker logs a line like `echo: kind=echo attempt=1 payload=...`. `GET http://127.0.0.1:8080/healthz` checks DB + Redis connectivity.

Jobs move **`pending` → `running` → `completed`** (or **`failed`**) as tasks finish; the orchestrator **reconciler** periodically re-enqueues **`queued`** rows that are due (`scheduled_at <= now()`), using Redis **pending** markers to avoid spamming duplicate LPUSHes.

## Layout

- `cmd/orchestrator` — HTTP API, DB ping, task submission
- `cmd/worker` — worker process (`echo` demo handler)
- `internal/config` — environment configuration
- `internal/db` — Cockroach pool (`pgxpool`), jobs/tasks, task_run lifecycle
- `internal/redis` — Redis client, key layout, ready LIST, lease hashes, scheduled ZSET helpers
- `internal/orchestrator` — submit path, HTTP handlers, reconciler (`ReconcileOnce`)
- `internal/worker` — `pkg/worker.Runtime` implementation
- `pkg/worker` — task handler API types
