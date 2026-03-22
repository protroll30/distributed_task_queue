# distributed_task_queue

[![CI](https://github.com/protroll30/distributed_task_queue/actions/workflows/ci.yml/badge.svg)](https://github.com/protroll30/distributed_task_queue/actions/workflows/ci.yml) *(forks: edit the badge URL in README to your repo)*

Distributed task orchestrator in Go (CockroachDB + Redis). Design and schema: [INSTRUCTIONS.md](INSTRUCTIONS.md).

## Prerequisites

- **Go 1.23+** (see `go` / `toolchain` in [`go.mod`](go.mod); CI uses [`go-version-file`](.github/workflows/ci.yml))
- **Either** Docker (recommended for local dependencies) **or** your own CockroachDB + Redis

### Go build: `cannot find package` / `-mod=vendor`

If `go build` or `go run` fails with **cannot find package**, or *import lookup disabled by -mod=vendor*:

- **Incomplete `vendor/`**: When a `vendor/` directory exists, Go may use **vendor mode** (`-mod=vendor`). Every dependency must appear under `vendor/` (for example `vendor/github.com/jackc/...`, `vendor/github.com/redis/...`). From the repo root run **`go mod vendor`** after `go.mod` is complete, or delete `vendor/` if you prefer module cache builds only.
- **`GOFLAGS`**: If you set **`GOFLAGS=-mod=vendor`** globally (`go env -w`, shell profile, or Cursor/VS Code `go.buildFlags` / `go.toolsEnvVars`), either keep `vendor/` in sync with **`go mod vendor`** or remove `-mod=vendor` when `vendor/` is incomplete. Cloud sync (e.g. OneDrive) can sometimes leave `vendor/` partially synced—re-run **`go mod vendor`** locally if needed.

## Reproducibility

| Item | How it’s pinned |
|------|-------------------|
| Go toolchain | `go` / `toolchain` in [`go.mod`](go.mod) |
| Modules | Committed [`go.sum`](go.sum); run `go mod download` and `go mod verify` after clone |
| Containers | Pinned image tags in [`docker-compose.yml`](docker-compose.yml) (`cockroachdb/cockroach:v24.3.4`, `redis:7.4.2-alpine`) |
| Schema | [`migrations/001_initial.sql`](migrations/001_initial.sql) via [`Makefile`](Makefile) `migrate`, [`scripts/migrate.sh`](scripts/migrate.sh), [`scripts/migrate.ps1`](scripts/migrate.ps1), or [`scripts/migrate.cmd`](scripts/migrate.cmd) (Windows **cmd**) |
| CI | [`.github/workflows/ci.yml`](.github/workflows/ci.yml): `go mod verify`, `go vet`, `go test ./...`, `go build ./cmd/...` |

After a fresh clone (Docker running), from the repo root:

```bat
docker compose up -d
scripts\migrate.cmd
set CRDB_DSN=postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable
go run .\cmd\orchestrator
```

Use a **second** `cmd` window for `go run .\cmd\worker` with the same `CRDB_DSN`.

Same checks as CI locally:

```bash
make verify
```

```bat
scripts\verify.cmd
```

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

# Or PowerShell (with Docker Compose Cockroach running)
powershell -ExecutionPolicy Bypass -File .\scripts\migrate.ps1

# PowerShell without Docker: set CRDB_DSN and use the Cockroach CLI on PATH
# $env:CRDB_DSN = "postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable"
# powershell -ExecutionPolicy Bypass -File .\scripts\migrate.ps1
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
| `ORCHESTRATOR_LISTEN` | `:8080` | Orchestrator HTTP bind (`GET /healthz`, `GET /metrics`, task/job routes below) |
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

**Prometheus** — `GET http://127.0.0.1:8080/metrics` exposes Go process/runtime metrics, `orchestrator_http_*`, `orchestrator_tasks_submitted_total`, `orchestrator_jobs_submitted_total`, and reconciler counters (`orchestrator_reconcile_*`).

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
- `scripts/` — `migrate.sh` / `migrate.ps1` / `migrate.cmd`, `verify.cmd` (same checks as CI / `make verify`)
- `cmd/orchestrator` — HTTP API, DB ping, task submission
- `cmd/worker` — worker process (`echo` demo handler)
- `internal/config` — environment configuration
- `internal/db` — Cockroach pool (`pgxpool`), jobs/tasks, task_run lifecycle
- `internal/redis` — Redis client, key layout, ready LIST, lease hashes, scheduled ZSET helpers
- `internal/orchestrator` — submit path, HTTP handlers (`GET`/`POST` v1), reconciler (`ReconcileOnce`, `ReclaimStaleRunningOnce`)
- `internal/worker` — `pkg/worker.Runtime` implementation
- `pkg/worker` — task handler API types
