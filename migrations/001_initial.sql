-- Initial schema — see INSTRUCTIONS.md §4.
-- Apply with: cockroach sql --url "$CRDB_DSN" -f migrations/001_initial.sql

CREATE TABLE jobs (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    name STRING NOT NULL,
    status STRING NOT NULL,
    metadata JSONB,
    idempotency_key STRING UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_jobs_status_updated ON jobs (status, updated_at DESC);

CREATE TABLE tasks (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    name STRING NOT NULL DEFAULT '',
    kind STRING NOT NULL,
    status STRING NOT NULL,
    payload JSONB,
    max_attempts INT NOT NULL DEFAULT 3,
    attempt INT NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    last_error STRING,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tasks_job_id ON tasks (job_id);
CREATE INDEX idx_tasks_status_scheduled ON tasks (status, scheduled_at);
CREATE INDEX idx_tasks_kind_status ON tasks (kind, status);

CREATE TABLE task_dependencies (
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    depends_on_task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, depends_on_task_id),
    CONSTRAINT no_self_dependency CHECK (task_id <> depends_on_task_id)
);

CREATE INDEX idx_task_dependencies_depends ON task_dependencies (depends_on_task_id);

CREATE TABLE task_runs (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    attempt_number INT NOT NULL,
    worker_id STRING NOT NULL,
    status STRING NOT NULL,
    error STRING,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_task_runs_task_id ON task_runs (task_id, started_at DESC);

CREATE TABLE workers (
    id STRING NOT NULL PRIMARY KEY,
    hostname STRING,
    last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB
);
