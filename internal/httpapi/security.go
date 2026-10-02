package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
)

// SecurityHeadersMiddleware adds production security headers to all HTTP responses.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none';")
		h.Set("Referrer-Policy", "no-referrer")

		next.ServeHTTP(w, r)
	})
}

// clientLimiter tracks request timestamps for rate limiting.
type clientLimiter struct {
	timestamps []time.Time
}

// RateLimiter implements an in-memory sliding-window rate limiter per client IP.
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientLimiter
	limit    int           // max requests per window
	window   time.Duration // window size, e.g. 1 minute
	stopChan chan struct{}
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 100
	}
	if window <= 0 {
		window = time.Minute
	}

	rl := &RateLimiter{
		clients:  make(map[string]*clientLimiter),
		limit:    limit,
		window:   window,
		stopChan: make(chan struct{}),
	}

	// Periodically purge stale IP entries every 5 minutes
	go rl.cleanLoop(5 * time.Minute)

	return rl
}

func (rl *RateLimiter) cleanLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.window)
			for ip, client := range rl.clients {
				// filter timestamps
				valid := make([]time.Time, 0, len(client.timestamps))
				for _, t := range client.timestamps {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.clients, ip)
				} else {
					client.timestamps = valid
				}
			}
			rl.mu.Unlock()
		case <-rl.stopChan:
			return
		}
	}
}

// Middleware creates an HTTP middleware that enforces the rate limit.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract client IP
			ip := getClientIP(r)

			rl.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-rl.window)

			client, exists := rl.clients[ip]
			if !exists {
				client = &clientLimiter{}
				rl.clients[ip] = client
			}

			// Prune timestamps older than window
			validIdx := 0
			for i, t := range client.timestamps {
				if t.After(cutoff) {
					validIdx = i
					break
				}
				if i == len(client.timestamps)-1 {
					validIdx = len(client.timestamps)
				}
			}
			client.timestamps = client.timestamps[validIdx:]

			if len(client.timestamps) >= rl.limit {
				rl.mu.Unlock()
				w.Header().Set("Retry-After", "60")
				respondError(w, r, &domain.AppError{
					Code:       "RATE_LIMIT_EXCEEDED",
					Message:    "Rate limit exceeded. Please try again later.",
					HTTPStatus: http.StatusTooManyRequests,
				})
				return
			}

			client.timestamps = append(client.timestamps, now)
			rl.mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For if behind a proxy
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		clientIP := strings.TrimSpace(parts[0])
		if clientIP != "" {
			return clientIP
		}
	}

	// Check X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}

	// RemoteAddr fallback
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
