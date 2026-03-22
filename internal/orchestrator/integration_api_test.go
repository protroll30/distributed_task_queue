package orchestrator_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
	"github.com/distributed_task_queue/distributed_task_queue/internal/orchestrator"
	appredis "github.com/distributed_task_queue/distributed_task_queue/internal/redis"
)

func TestIntegration_SubmitAndReadTaskAndJob(t *testing.T) {
	dsn := os.Getenv("CRDB_DSN")
	if dsn == "" {
		t.Skip("CRDB_DSN not set; start CockroachDB and export CRDB_DSN to run this test")
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("database unavailable: %v", err)
	}

	rdb := appredis.New(redisAddr)
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}

	prefix := "dto:itest:" + uuid.NewString() + ":"
	srv := &orchestrator.Server{Pool: pool, RDB: rdb, Prefix: prefix}
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	body := `{"kind":"echo","payload":{"n":1}}`
	resp, err := http.Post(ts.URL+"/v1/tasks", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit status: got %d", resp.StatusCode)
	}
	var sub struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sub); err != nil {
		t.Fatal(err)
	}
	if sub.TaskID == "" {
		t.Fatal("empty task_id")
	}

	getTask, err := http.Get(ts.URL + "/v1/tasks/" + sub.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	defer getTask.Body.Close()
	if getTask.StatusCode != http.StatusOK {
		t.Fatalf("get task status: got %d", getTask.StatusCode)
	}
	var task struct {
		ID     string `json:"id"`
		JobID  string `json:"job_id"`
		Status string `json:"status"`
		Kind   string `json:"kind"`
	}
	if err := json.NewDecoder(getTask.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.ID != sub.TaskID || task.Kind != "echo" {
		t.Fatalf("task: %+v", task)
	}
	if task.Status != "queued" && task.Status != "running" && task.Status != "completed" {
		t.Fatalf("unexpected status %q", task.Status)
	}

	getJob, err := http.Get(ts.URL + "/v1/jobs/" + task.JobID)
	if err != nil {
		t.Fatal(err)
	}
	defer getJob.Body.Close()
	if getJob.StatusCode != http.StatusOK {
		t.Fatalf("get job status: got %d", getJob.StatusCode)
	}
	var job struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Tasks  []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.NewDecoder(getJob.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	if job.ID != task.JobID || len(job.Tasks) != 1 {
		t.Fatalf("job: %+v", job)
	}
}

func TestIntegration_GetTask_notFound(t *testing.T) {
	dsn := os.Getenv("CRDB_DSN")
	if dsn == "" {
		t.Skip("CRDB_DSN not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("database unavailable: %v", err)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	rdb := appredis.New(redisAddr)
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}

	srv := &orchestrator.Server{Pool: pool, RDB: rdb, Prefix: "dto:"}
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	id := uuid.New()
	resp, err := http.Get(ts.URL + "/v1/tasks/" + id.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", resp.StatusCode)
	}
}
