package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

// Server exposes HTTP handlers for the control plane.
type Server struct {
	Pool   *pgxpool.Pool
	RDB    *goredis.Client
	Prefix string
}

type submitRequest struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

type submitResponse struct {
	TaskID string `json:"task_id"`
}

type jobTaskRequest struct {
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	DependsOn []string        `json:"depends_on"`
}

type jobRequest struct {
	Tasks []jobTaskRequest `json:"tasks"`
}

type jobResponse struct {
	JobID string            `json:"job_id"`
	Tasks map[string]string `json:"tasks"`
}

// Register attaches routes to mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.Handle("GET /metrics", MetricsHTTPHandler())
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /v1/tasks", s.handleSubmit)
	mux.HandleFunc("GET /v1/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("POST /v1/jobs", s.handleSubmitJob)
	mux.HandleFunc("GET /v1/jobs/{id}", s.handleGetJob)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Pool.Ping(ctx); err != nil {
		http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := s.RDB.Ping(ctx).Err(); err != nil {
		http.Error(w, "redis: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Kind == "" {
		http.Error(w, "kind is required", http.StatusBadRequest)
		return
	}
	payload := []byte(req.Payload)
	if len(req.Payload) == 0 {
		payload = nil
	}
	id, err := SubmitTask(r.Context(), s.Pool, s.RDB, s.Prefix, req.Kind, payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	TasksSubmittedTotal.Inc()
	_ = json.NewEncoder(w).Encode(submitResponse{TaskID: id.String()})
}

func (s *Server) handleSubmitJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req jobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(req.Tasks) == 0 {
		http.Error(w, "tasks is required", http.StatusBadRequest)
		return
	}
	specs := make([]db.TaskSpec, 0, len(req.Tasks))
	for _, t := range req.Tasks {
		payload := []byte(t.Payload)
		if len(t.Payload) == 0 {
			payload = nil
		}
		specs = append(specs, db.TaskSpec{
			Name:      t.Name,
			Kind:      t.Kind,
			Payload:   payload,
			DependsOn: t.DependsOn,
		})
	}
	if err := validateJobSpecs(specs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	jobID, nameToID, err := SubmitJob(r.Context(), s.Pool, s.RDB, s.Prefix, specs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make(map[string]string, len(nameToID))
	for n, id := range nameToID {
		out[n] = id.String()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	JobsSubmittedTotal.Inc()
	_ = json.NewEncoder(w).Encode(jobResponse{JobID: jobID.String(), Tasks: out})
}
