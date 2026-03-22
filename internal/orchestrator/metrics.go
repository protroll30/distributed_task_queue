package orchestrator

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Business counters (orchestrator control plane).
var (
	TasksSubmittedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orchestrator",
		Name:      "tasks_submitted_total",
		Help:      "Successful POST /v1/tasks submissions",
	})
	JobsSubmittedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orchestrator",
		Name:      "jobs_submitted_total",
		Help:      "Successful POST /v1/jobs submissions",
	})
	ReconcileEnqueuedTasksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orchestrator",
		Name:      "reconcile_enqueued_tasks_total",
		Help:      "Tasks LPUSHed by ReconcileOnce (due queued rows)",
	})
	ReconcileStaleRunningReclaimedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orchestrator",
		Name:      "reconcile_stale_running_reclaimed_total",
		Help:      "Tasks reclaimed by ReclaimStaleRunningOnce",
	})
)

var httpRequestDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "orchestrator",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration",
		Buckets:   prometheus.DefBuckets,
	},
	[]string{"method", "pattern", "code"},
)

var httpRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "orchestrator",
		Name:      "http_requests_total",
		Help:      "HTTP requests",
	},
	[]string{"method", "pattern", "code"},
)

// MetricsHTTPHandler serves Prometheus text exposition on the default registry.
func MetricsHTTPHandler() http.Handler {
	return promhttp.Handler()
}

// MetricsMiddleware records request duration and counts for the wrapped handler (e.g. mux).
// Uses r.Pattern when set (Go 1.23+ on *http.Request) to avoid high-cardinality paths.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		d := time.Since(start).Seconds()
		pat := r.Pattern
		if pat == "" {
			pat = r.URL.Path
		}
		code := strconv.Itoa(sw.status)
		httpRequestDuration.WithLabelValues(r.Method, pat, code).Observe(d)
		httpRequestsTotal.WithLabelValues(r.Method, pat, code).Inc()
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (rw *statusRecorder) WriteHeader(code int) {
	if !rw.wrote {
		rw.status = code
		rw.wrote = true
	}
	rw.ResponseWriter.WriteHeader(code)
}
