package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bariskode/email-management-service/internal/auth"
	"github.com/bariskode/email-management-service/internal/observability"
)

// NewRouter constructs the HTTP handler with all middlewares and routing table.
func NewRouter(server *Server, apiKey string, rateLimit int, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Public observability endpoints
	mux.HandleFunc("GET /healthz", observability.HealthzHandler)
	mux.HandleFunc("GET /readyz", observability.ReadyzHandler)
	mux.HandleFunc("GET /metrics", observability.MetricsHandler)

	// API v1 sub-mux protected by Auth & Idempotency
	apiMux := http.NewServeMux()

	// Cloudflare Accounts
	apiMux.HandleFunc("GET /api/v1/accounts", server.handleListAccounts)
	apiMux.HandleFunc("POST /api/v1/accounts", server.handleCreateAccount)
	apiMux.HandleFunc("GET /api/v1/accounts/{id}", server.handleGetAccount)
	apiMux.HandleFunc("PUT /api/v1/accounts/{id}", server.handleUpdateAccount)
	apiMux.HandleFunc("DELETE /api/v1/accounts/{id}", server.handleDeleteAccount)

	// Destinations
	apiMux.HandleFunc("GET /api/v1/destinations", server.handleListDestinations)
	apiMux.HandleFunc("POST /api/v1/destinations", server.handleCreateDestination)
	apiMux.HandleFunc("GET /api/v1/destinations/{id}", server.handleGetDestination)
	apiMux.HandleFunc("PATCH /api/v1/destinations/{id}", server.handleUpdateDestination)
	apiMux.HandleFunc("DELETE /api/v1/destinations/{id}", server.handleDeleteDestination)

	// Zones
	apiMux.HandleFunc("GET /api/v1/zones", server.handleListZones)
	apiMux.HandleFunc("GET /api/v1/zones/remote", server.handleDiscoverRemoteZones)
	apiMux.HandleFunc("POST /api/v1/zones/import", server.handleImportZone)
	apiMux.HandleFunc("GET /api/v1/zones/{id}", server.handleGetZone)
	apiMux.HandleFunc("GET /api/v1/zones/{id}/routing", server.handleGetRoutingSettings)
	apiMux.HandleFunc("PATCH /api/v1/zones/{id}/routing", server.handleUpdateRoutingSettings)
	apiMux.HandleFunc("POST /api/v1/zones/{id}/sync", server.handleSyncZone)

	// Explicit Routing Rules
	apiMux.HandleFunc("GET /api/v1/zones/{id}/rules", server.handleListRules)
	apiMux.HandleFunc("POST /api/v1/zones/{id}/rules", server.handleCreateRule)
	apiMux.HandleFunc("GET /api/v1/zones/{id}/rules/{ruleID}", server.handleGetRule)
	apiMux.HandleFunc("PUT /api/v1/zones/{id}/rules/{ruleID}", server.handleUpdateRule)
	apiMux.HandleFunc("DELETE /api/v1/zones/{id}/rules/{ruleID}", server.handleDeleteRule)

	// Catch-All (Dedicated Resource)
	apiMux.HandleFunc("GET /api/v1/zones/{id}/catch-all", server.handleGetCatchAll)
	apiMux.HandleFunc("PUT /api/v1/zones/{id}/catch-all", server.handleUpdateCatchAll)

	// Audit & Sync History
	apiMux.HandleFunc("GET /api/v1/audit-events", server.handleListAuditEvents)
	apiMux.HandleFunc("GET /api/v1/sync-runs", server.handleListSyncRuns)

	// Wrap apiMux with Auth and Idempotency
	authMiddleware := auth.Middleware(apiKey)
	idempotencyMiddleware := IdempotencyMiddleware(server.repos.Idempotency)
	protectedAPI := authMiddleware(idempotencyMiddleware(apiMux))

	// Mount protected API on main mux
	mux.Handle("/api/v1/", protectedAPI)

	// Rate limiter
	rateLimiter := NewRateLimiter(rateLimit, 0)

	// Apply global middleware chain:
	// SecurityHeaders -> RequestID -> RateLimiter -> Logging -> Recovery
	var handler http.Handler = mux
	handler = recoveryMiddleware(handler, logger)
	handler = observability.LoggingMiddleware(logger)(handler)
	handler = rateLimiter.Middleware()(handler)
	handler = observability.RequestIDMiddleware(handler)
	handler = SecurityHeadersMiddleware(handler)

	return handler
}

func recoveryMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic_recovered", slog.Any("panic", rec))
				respondError(w, r, fmt.Errorf("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
