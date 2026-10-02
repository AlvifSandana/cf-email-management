package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
)

// Metrics records basic operational metrics.
type Metrics struct {
	TotalRequests    atomic.Uint64
	TotalErrors      atomic.Uint64
	ProviderRequests atomic.Uint64
	SyncRuns         atomic.Uint64
}

var DefaultMetrics = &Metrics{}

// RequestIDMiddleware injects a unique request ID into context and response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok && id != "" {
		return id
	}
	return generateRequestID()
}

// LoggingMiddleware logs HTTP requests and updates metrics.
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			DefaultMetrics.TotalRequests.Add(1)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			duration := time.Since(start)
			reqID := GetRequestID(r.Context())

			if rec.status >= 400 {
				DefaultMetrics.TotalErrors.Add(1)
			}

			logger.Info("http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", duration),
				slog.String("request_id", reqID),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("req_%s", hex.EncodeToString(b))
}

// HealthzHandler returns 200 OK for liveness probes.
func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// ReadyzHandler returns 200 OK for readiness probes.
func ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

// MetricsHandler exports Prometheus-style plain text metrics.
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "# HELP ems_api_requests_total Total number of HTTP requests\n")
	fmt.Fprintf(w, "# TYPE ems_api_requests_total counter\n")
	fmt.Fprintf(w, "ems_api_requests_total %d\n", DefaultMetrics.TotalRequests.Load())

	fmt.Fprintf(w, "# HELP ems_api_errors_total Total number of HTTP error responses (>=400)\n")
	fmt.Fprintf(w, "# TYPE ems_api_errors_total counter\n")
	fmt.Fprintf(w, "ems_api_errors_total %d\n", DefaultMetrics.TotalErrors.Load())

	fmt.Fprintf(w, "# HELP ems_provider_requests_total Total provider API requests made\n")
	fmt.Fprintf(w, "# TYPE ems_provider_requests_total counter\n")
	fmt.Fprintf(w, "ems_provider_requests_total %d\n", DefaultMetrics.ProviderRequests.Load())

	fmt.Fprintf(w, "# HELP ems_sync_runs_total Total synchronization runs\n")
	fmt.Fprintf(w, "# TYPE ems_sync_runs_total counter\n")
	fmt.Fprintf(w, "ems_sync_runs_total %d\n", DefaultMetrics.SyncRuns.Load())
}
