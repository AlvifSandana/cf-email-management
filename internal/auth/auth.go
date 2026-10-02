package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/bariskode/email-management-service/internal/auth/rbac"
	"github.com/bariskode/email-management-service/internal/domain"
)

type contextKey string

const (
	ActorTypeKey contextKey = "actor_type"
	ActorIDKey   contextKey = "actor_id"
)

// Middleware validates API keys from X-API-Key or Authorization: Bearer <key>
func Middleware(expectedAPIKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract API key
			key := r.Header.Get("X-API-Key")
			if key == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					key = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if key == "" || subtle.ConstantTimeCompare([]byte(key), []byte(expectedAPIKey)) != 1 {
				http.Error(w, `{"error":{"code":"`+domain.ErrCodeUnauthorized+`","message":"Invalid or missing API key"}}`, http.StatusUnauthorized)
				return
			}

			// Add actor details to context
			ctx := context.WithValue(r.Context(), ActorTypeKey, "api_key")
			ctx = context.WithValue(ctx, ActorIDKey, "admin")
			ctx = context.WithValue(ctx, ActorRoleKey, string(rbac.RoleAdmin))
			ctx = rbac.WithRole(ctx, rbac.RoleAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetActor extracts the actor type and actor ID from request context.
func GetActor(ctx context.Context) (actorType, actorID string) {
	t, _ := ctx.Value(ActorTypeKey).(string)
	id, _ := ctx.Value(ActorIDKey).(string)
	if t == "" {
		t = "system"
	}
	if id == "" {
		id = "system"
	}
	return t, id
}
