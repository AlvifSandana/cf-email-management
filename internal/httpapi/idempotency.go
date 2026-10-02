package httpapi

import (
	"bytes"
	"net/http"
	"time"

	"github.com/bariskode/email-management-service/internal/auth"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/storage"
)

type bufferedResponseWriter struct {
	http.ResponseWriter
	buf        bytes.Buffer
	statusCode int
}

func (w *bufferedResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *bufferedResponseWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

// IdempotencyMiddleware caches responses for mutating requests with an Idempotency-Key header.
func IdempotencyMiddleware(repo storage.IdempotencyRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			actorType, actorID := auth.GetActor(r.Context())
			scopedKey := actorType + ":" + actorID + ":" + r.Method + ":" + r.URL.Path + ":" + key

			// Check existing cached response
			record, err := repo.Get(r.Context(), scopedKey)
			if err == nil && record != nil {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache-Lookup", "HIT-IDEMPOTENT")
				w.WriteHeader(record.StatusCode)
				_, _ = w.Write(record.Body)
				return
			}

			bw := &bufferedResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(bw, r)

			// Save to idempotency store if non-5xx
			if bw.statusCode < 500 {
				_ = repo.Save(r.Context(), &domain.IdempotencyRecord{
					Key:        scopedKey,
					StatusCode: bw.statusCode,
					Body:       bw.buf.Bytes(),
					CreatedAt:  time.Now().UTC(),
					ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
				})
			}
		})
	}
}
