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

## Configuration (reference)

See INSTRUCTIONS.md §7 — e.g. `CRDB_DSN`, `REDIS_ADDR`, `REDIS_KEY_PREFIX`.

## Layout

- `cmd/orchestrator` — API + scheduler (stub)
- `cmd/worker` — worker process (stub)
- `internal/db` — Cockroach pool helper (`pgxpool`)
- `internal/redis` — Redis client
- `pkg/worker` — task handler API types
