# distributed_task_queue

Distributed task orchestrator in Go (CockroachDB + Redis). Design and schema: [INSTRUCTIONS.md](INSTRUCTIONS.md).

## Prerequisites

- Go 1.22+
- **Either** Docker (recommended for local dependencies) **or** your own CockroachDB + Redis

## Docker Compose (local CockroachDB + Redis)

From the repo root:

```bash
docker compose up -d
```

Stop: `docker compose down`.

- **SQL** (from the host): `postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable`
- **CockroachDB UI** (mapped to avoid clashing with the orchestrator): [http://127.0.0.1:8089](http://127.0.0.1:8089)
- **Redis**: `127.0.0.1:6379`

Apply the schema (pick one):

```bash
# Make (Git Bash / WSL / Unix shell)
make migrate

# Or bash helper
./scripts/migrate.sh

# Or PowerShell
powershell -File scripts/migrate.ps1
```

Copy [`.env.example`](.env.example) to `.env` and load it in your shell if you use a tool that reads `.env` automatically; otherwise set `CRDB_DSN` / `REDIS_ADDR` as in the table below.

## Database migrations (any cluster)

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
| `ORCHESTRATOR_LISTEN` | `:8080` | Orchestrator HTTP bind (`GET /healthz`, task/job read + submit routes below) |
| `RECONCILE_INTERVAL` | `30s` | Background reconciler: re-enqueue due `queued` tasks (CRDB → Redis) |
| `STALE_RUNNING_AFTER` | `2 × LEASE_DURATION` | Min time a task stays `running` before the reconciler may reclaim it if the Redis lease key is missing |
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

Task names must be unique per job. Dependency edges are validated (unknown names, self-deps, and cycles return **400**). A dependent is promoted to `queued` only when **every** dependency is `completed`. If a task **fails permanently**, downstream tasks still in `pending` are **cascaded** to `failed` (transitive, BFS).

The worker logs a line like `echo: kind=echo attempt=1 payload=...`. `GET http://127.0.0.1:8080/healthz` checks DB + Redis connectivity.

**Read APIs** — `GET /v1/tasks/{id}` returns a task row (status, attempts, timestamps, payload). `GET /v1/jobs/{id}` returns the job plus all tasks in that job. Unknown UUIDs return **404**; malformed IDs return **400**.

```bash
curl -sS "http://127.0.0.1:8080/v1/tasks/<task_id>"
curl -sS "http://127.0.0.1:8080/v1/jobs/<job_id>"
```

Integration tests that hit the DB and Redis run when `CRDB_DSN` is set (e.g. after `docker compose up`):

```bash
set CRDB_DSN=postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable
go test ./internal/orchestrator/ -count=1 -v
```

Jobs move **`pending` → `running` → `completed`** (or **`failed`**) as tasks finish; the orchestrator **reconciler** periodically re-enqueues **`queued`** rows that are due (`scheduled_at <= now()`), using Redis **pending** markers to avoid spamming duplicate LPUSHes. If a worker dies after claiming a task, the Redis lease **TTL** expires (heartbeats stop); the reconciler also **reclaims** long-running `running` rows with **no lease**—closing the open `task_run`, re-queuing the task (same retry budget), and LPUSHing again.

## Layout

- [`docker-compose.yml`](docker-compose.yml) — local CockroachDB + Redis
- [`Makefile`](Makefile) — `make compose-up`, `make migrate`, …
- `scripts/` — `migrate.sh` / `migrate.ps1` helpers
- `cmd/orchestrator` — HTTP API, DB ping, task submission
- `cmd/worker` — worker process (`echo` demo handler)
- `internal/config` — environment configuration
- `internal/db` — Cockroach pool (`pgxpool`), jobs/tasks, task_run lifecycle
- `internal/redis` — Redis client, key layout, ready LIST, lease hashes, scheduled ZSET helpers
- `internal/orchestrator` — submit path, HTTP handlers (`GET`/`POST` v1), reconciler (`ReconcileOnce`, `ReclaimStaleRunningOnce`)
- `internal/worker` — `pkg/worker.Runtime` implementation
- `pkg/worker` — task handler API types
