package orchestrator

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
)

type taskReadResponse struct {
	ID          string          `json:"id"`
	JobID       string          `json:"job_id"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Status      string          `json:"status"`
	MaxAttempts int             `json:"max_attempts"`
	Attempt     int             `json:"attempt"`
	ScheduledAt time.Time       `json:"scheduled_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	LastError   *string         `json:"last_error,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}

type jobReadResponse struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	Metadata  json.RawMessage    `json:"metadata"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Tasks     []taskReadResponse `json:"tasks"`
}

func taskToReadResponse(t db.Task) taskReadResponse {
	var payload json.RawMessage
	switch {
	case len(t.Payload) == 0:
		payload = json.RawMessage("null")
	default:
		payload = json.RawMessage(t.Payload)
	}
	return taskReadResponse{
		ID:          t.ID.String(),
		JobID:       t.JobID.String(),
		Name:        t.Name,
		Kind:        t.Kind,
		Status:      t.Status,
		MaxAttempts: t.MaxAttempts,
		Attempt:     t.Attempt,
		ScheduledAt: t.ScheduledAt,
		StartedAt:   t.StartedAt,
		FinishedAt:  t.FinishedAt,
		LastError:   t.LastError,
		Payload:     payload,
	}
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	t, err := db.GetTaskByID(r.Context(), s.Pool, id)
	if err != nil {
		if errors.Is(err, db.ErrTaskNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(taskToReadResponse(t))
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}
	j, err := db.GetJobByID(r.Context(), s.Pool, id)
	if err != nil {
		if errors.Is(err, db.ErrJobNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tasks, err := db.ListTasksByJobID(r.Context(), s.Pool, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	meta := json.RawMessage("null")
	if len(j.Metadata) > 0 {
		meta = json.RawMessage(j.Metadata)
	}
	out := jobReadResponse{
		ID:        j.ID.String(),
		Name:      j.Name,
		Status:    j.Status,
		Metadata:  meta,
		CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
		Tasks:     make([]taskReadResponse, 0, len(tasks)),
	}
	for _, t := range tasks {
		out.Tasks = append(out.Tasks, taskToReadResponse(t))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
