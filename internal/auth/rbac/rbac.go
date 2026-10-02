package rbac

import (
	"context"
	"net/http"
	"strings"
)

// Role represents a user authorization level.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// ValidRoles lists all supported roles in the system.
var ValidRoles = []Role{RoleAdmin, RoleOperator, RoleViewer}

// roleHierarchy defines the numerical tier of each role.
// Admin (3) > Operator (2) > Viewer (1).
var roleHierarchy = map[Role]int{
	RoleViewer:   1,
	RoleOperator: 2,
	RoleAdmin:    3,
}

// Level returns the numeric tier for the role (0 for unknown/empty).
func (r Role) Level() int {
	return roleHierarchy[r]
}

// String returns the string value of the role.
func (r Role) String() string {
	return string(r)
}

// IsValid reports whether the role is recognized in the hierarchy.
func (r Role) IsValid() bool {
	return r.Level() > 0
}

// HasRole checks whether the role meets or exceeds minRole in hierarchy.
func (r Role) HasRole(minRole Role) bool {
	level := r.Level()
	minLevel := minRole.Level()
	if level == 0 || minLevel == 0 {
		return false
	}
	return level >= minLevel
}

// HasRole is a package-level helper that compares actual role to minRole.
func HasRole(actualRole, minRole Role) bool {
	return actualRole.HasRole(minRole)
}

// Permission defines a specific granular action in the system.
type Permission string

const (
	// PermRead grants read-only access to all GET endpoints.
	PermRead Permission = "read"
	// PermRulesModify grants access to create, update, delete routing rules.
	PermRulesModify Permission = "rules:modify"
	// PermCatchAllModify grants access to update catch-all configurations.
	PermCatchAllModify Permission = "catchall:modify"
	// PermDestModify grants access to create, update, delete destinations.
	PermDestModify Permission = "destinations:modify"
	// PermSyncExecute grants access to trigger zone synchronization runs.
	PermSyncExecute Permission = "sync:execute"
	// PermAccountsModify grants access to create, update, delete Cloudflare accounts.
	PermAccountsModify Permission = "accounts:modify"
	// PermKeysManage grants access to cryptographic keys and security configuration.
	PermKeysManage Permission = "keys:manage"
)

// rolePermissions defines the explicit permissions mapped to each role.
var rolePermissions = map[Role]map[Permission]bool{
	RoleViewer: {
		PermRead: true,
	},
	RoleOperator: {
		PermRead:           true,
		PermRulesModify:    true,
		PermCatchAllModify: true,
		PermDestModify:     true,
		PermSyncExecute:    true,
	},
	RoleAdmin: {
		PermRead:           true,
		PermRulesModify:    true,
		PermCatchAllModify: true,
		PermDestModify:     true,
		PermSyncExecute:    true,
		PermAccountsModify: true,
		PermKeysManage:     true,
	},
}

// HasPermission reports whether a given role holds the specified permission.
func HasPermission(role Role, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[perm]
}

// CanAccessEndpoint evaluates role access against HTTP method and target resource.
// Role hierarchy:
// - Viewer: read-only access (GET/HEAD endpoints)
// - Operator: read + modify rules, catch-all, destinations, and sync
// - Admin: all permissions including accounts and key management
func CanAccessEndpoint(role Role, method string, resource string) bool {
	if !role.IsValid() {
		return false
	}

	method = strings.ToUpper(method)
	resource = strings.ToLower(strings.TrimSpace(resource))

	// Read-only access (GET / HEAD / OPTIONS) allowed for Viewer, Operator, Admin
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		return true
	}

	// Any state-modifying method (POST, PUT, PATCH, DELETE)
	if role == RoleViewer {
		return false // Viewer is strictly read-only
	}

	// Admin has full permissions across all resources
	if role == RoleAdmin {
		return true
	}

	// Operator permissions:
	// Allowed: rules, catch-all, destinations, sync, zones/routing
	// Forbidden: accounts, keys
	if role == RoleOperator {
		switch {
		case strings.Contains(resource, "account"):
			return false // Admin only
		case strings.Contains(resource, "key") || strings.Contains(resource, "secret"):
			return false // Admin only
		case strings.Contains(resource, "rule") ||
			strings.Contains(resource, "catch-all") ||
			strings.Contains(resource, "catchall") ||
			strings.Contains(resource, "destination") ||
			strings.Contains(resource, "sync") ||
			strings.Contains(resource, "zone") ||
			strings.Contains(resource, "routing"):
			return true
		default:
			return false
		}
	}

	return false
}

// PermissionChecker provides methods for checking access and role permissions.
type PermissionChecker struct{}

// NewPermissionChecker creates a new PermissionChecker instance.
func NewPermissionChecker() *PermissionChecker {
	return &PermissionChecker{}
}

// HasRole checks whether actual role satisfies required minRole.
func (p *PermissionChecker) HasRole(actual, required Role) bool {
	return HasRole(actual, required)
}

// HasPermission checks whether role has the specified permission.
func (p *PermissionChecker) HasPermission(role Role, perm Permission) bool {
	return HasPermission(role, perm)
}

// CanAccess checks whether role can perform method on resource.
func (p *PermissionChecker) CanAccess(role Role, method, resource string) bool {
	return CanAccessEndpoint(role, method, resource)
}

// Context keys
type contextKey string

const (
	// ActorRoleKey is the context key for the actor's RBAC role.
	ActorRoleKey contextKey = "actor_role"
)

// WithRole returns a copy of parent context with the actor role set.
func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, ActorRoleKey, role)
}

// FromContext extracts the actor role from context.
// Supports typed ActorRoleKey, string keys "actor_role" and "role", and
// falls back to checking "actor_type" == "api_key" (which defaults to RoleAdmin).
func FromContext(ctx context.Context) (Role, bool) {
	if ctx == nil {
		return "", false
	}

	// 1. Check typed key ActorRoleKey
	if val := ctx.Value(ActorRoleKey); val != nil {
		switch v := val.(type) {
		case Role:
			if v.IsValid() {
				return v, true
			}
		case string:
			r := Role(strings.ToLower(v))
			if r.IsValid() {
				return r, true
			}
		}
	}

	// 2. Check string key "actor_role"
	if val := ctx.Value("actor_role"); val != nil {
		switch v := val.(type) {
		case Role:
			if v.IsValid() {
				return v, true
			}
		case string:
			r := Role(strings.ToLower(v))
			if r.IsValid() {
				return r, true
			}
		}
	}

	// 3. Check string key "role"
	if val := ctx.Value("role"); val != nil {
		switch v := val.(type) {
		case Role:
			if v.IsValid() {
				return v, true
			}
		case string:
			r := Role(strings.ToLower(v))
			if r.IsValid() {
				return r, true
			}
		}
	}

	// 4. Fallback for API key actor type in context
	if val := ctx.Value("actor_type"); val != nil {
		if s, ok := val.(string); ok && s == "api_key" {
			return RoleAdmin, true
		}
	}

	return "", false
}

// GetRole returns the role from context, or empty Role if not found.
func GetRole(ctx context.Context) Role {
	r, _ := FromContext(ctx)
	return r
}

// ForbiddenResponseBody is the standard JSON payload returned on 403 Forbidden.
const ForbiddenResponseBody = `{"error":{"code":"FORBIDDEN","message":"Insufficient permissions"}}`

// RequireRole returns an HTTP middleware that verifies the actor in context has at least minRole.
// If insufficient permissions, it responds with 403 Forbidden:
// {"error":{"code":"FORBIDDEN","message":"Insufficient permissions"}}
func RequireRole(minRole Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actualRole, ok := FromContext(r.Context())
			if !ok || !actualRole.HasRole(minRole) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(ForbiddenResponseBody))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission returns an HTTP middleware checking that the actor role has the specified permission.
func RequirePermission(perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actualRole, ok := FromContext(r.Context())
			if !ok || !HasPermission(actualRole, perm) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(ForbiddenResponseBody))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
