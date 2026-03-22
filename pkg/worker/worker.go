package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Task is one unit of work for a Handler.
type Task struct {
	ID      uuid.UUID
	JobID   uuid.UUID
	Kind    string
	Payload json.RawMessage
	// Attempt is the 1-based count for this delivery (at-least-once execution).
	Attempt int
}

// Handler executes a single task. Nil means success; a non-nil error is handled per orchestrator retry/fail policy.
type Handler func(ctx context.Context, task Task) error

// Options configures timing and lease behavior. Zero values select defaults from configuration or environment.
type Options struct {
	// DefaultTimeout caps handler execution when the task does not specify its own deadline.
	DefaultTimeout time.Duration
	// HeartbeatEvery is the interval between Redis lease extensions while a handler runs.
	HeartbeatEvery time.Duration
}

// Runtime registers handlers by kind and runs until ctx is cancelled. Run should block until shutdown completes.
type Runtime interface {
	Register(kind string, h Handler)
	Run(ctx context.Context) error
}
