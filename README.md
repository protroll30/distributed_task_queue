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
| `ORCHESTRATOR_LISTEN` | `:8080` | Orchestrator HTTP/gRPC bind (future) |
| `RECONCILE_INTERVAL` | `30s` | Scheduler/reconciler tick (future) |
| `WORKER_ID` | random UUID | Stable worker identity if set |
| `WORKER_CONCURRENCY` | `1` | Parallel task executions (future) |
| `LEASE_DURATION` | `30s` | Redis lease TTL for claimed tasks (future) |

## Run (smoke)

```bash
export CRDB_DSN='postgresql://root@localhost:26257/defaultdb?sslmode=disable'
go run ./cmd/orchestrator
go run ./cmd/worker
```

PowerShell (`$env:` is only valid here, not in **cmd.exe**):

```powershell
$env:CRDB_DSN = 'postgresql://root@localhost:26257/defaultdb?sslmode=disable'
go run ./cmd/orchestrator
```

Command Prompt (**cmd.exe**): run `set` on its **own line** (or use `&&`). Pasting `go run` on the same line as `set` without a separator can merge into a bad path like `cmd\orchestratorset`.

```bat
set CRDB_DSN=postgresql://root@localhost:26257/defaultdb?sslmode=disable
go run ./cmd/orchestrator
```

One line:

```bat
set CRDB_DSN=postgresql://root@localhost:26257/defaultdb?sslmode=disable && go run ./cmd/orchestrator
```

Each process loads config, logs non-secret settings, then waits for Ctrl+C.

## Layout

- `cmd/orchestrator` — API + scheduler (stub; loads config)
- `cmd/worker` — worker process (stub; loads config)
- `internal/config` — environment configuration
- `internal/db` — Cockroach pool (`pgxpool`), task/task_run lifecycle queries
- `internal/redis` — Redis client, key layout, ready LIST, lease hashes, scheduled ZSET helpers
- `pkg/worker` — task handler API types
