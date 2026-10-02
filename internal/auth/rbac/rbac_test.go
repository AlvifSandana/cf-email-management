package rbac

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoleHierarchy(t *testing.T) {
	// Level validation
	if RoleAdmin.Level() != 3 {
		t.Errorf("expected Admin level 3, got %d", RoleAdmin.Level())
	}
	if RoleOperator.Level() != 2 {
		t.Errorf("expected Operator level 2, got %d", RoleOperator.Level())
	}
	if RoleViewer.Level() != 1 {
		t.Errorf("expected Viewer level 1, got %d", RoleViewer.Level())
	}
	if Role("guest").Level() != 0 {
		t.Errorf("expected guest level 0, got %d", Role("guest").Level())
	}

	// Validity checks
	if !RoleAdmin.IsValid() || !RoleOperator.IsValid() || !RoleViewer.IsValid() {
		t.Errorf("all standard roles should be valid")
	}
	if Role("invalid").IsValid() || Role("").IsValid() {
		t.Errorf("unknown roles should be invalid")
	}

	if RoleAdmin.String() != "admin" {
		t.Errorf("expected 'admin', got '%s'", RoleAdmin.String())
	}

	// Admin permissions in hierarchy
	if !RoleAdmin.HasRole(RoleViewer) {
		t.Errorf("Admin should satisfy Viewer requirement")
	}
	if !RoleAdmin.HasRole(RoleOperator) {
		t.Errorf("Admin should satisfy Operator requirement")
	}
	if !RoleAdmin.HasRole(RoleAdmin) {
		t.Errorf("Admin should satisfy Admin requirement")
	}

	// Operator permissions in hierarchy
	if !RoleOperator.HasRole(RoleViewer) {
		t.Errorf("Operator should satisfy Viewer requirement")
	}
	if !RoleOperator.HasRole(RoleOperator) {
		t.Errorf("Operator should satisfy Operator requirement")
	}
	if RoleOperator.HasRole(RoleAdmin) {
		t.Errorf("Operator should NOT satisfy Admin requirement")
	}

	// Viewer permissions in hierarchy
	if !RoleViewer.HasRole(RoleViewer) {
		t.Errorf("Viewer should satisfy Viewer requirement")
	}
	if RoleViewer.HasRole(RoleOperator) {
		t.Errorf("Viewer should NOT satisfy Operator requirement")
	}
	if RoleViewer.HasRole(RoleAdmin) {
		t.Errorf("Viewer should NOT satisfy Admin requirement")
	}

	// Invalid / empty role
	if Role("guest").HasRole(RoleViewer) {
		t.Errorf("guest should not satisfy any valid role")
	}
	if RoleAdmin.HasRole(Role("invalid")) {
		t.Errorf("admin should not satisfy invalid role")
	}
	if !HasRole(RoleAdmin, RoleOperator) {
		t.Errorf("HasRole helper should return true for Admin >= Operator")
	}
}

func TestPermissions(t *testing.T) {
	// Viewer
	if !HasPermission(RoleViewer, PermRead) {
		t.Errorf("Viewer must have PermRead")
	}
	if HasPermission(RoleViewer, PermRulesModify) {
		t.Errorf("Viewer must NOT have PermRulesModify")
	}
	if HasPermission(RoleViewer, PermAccountsModify) {
		t.Errorf("Viewer must NOT have PermAccountsModify")
	}

	// Operator
	if !HasPermission(RoleOperator, PermRead) {
		t.Errorf("Operator must have PermRead")
	}
	if !HasPermission(RoleOperator, PermRulesModify) {
		t.Errorf("Operator must have PermRulesModify")
	}
	if !HasPermission(RoleOperator, PermCatchAllModify) {
		t.Errorf("Operator must have PermCatchAllModify")
	}
	if !HasPermission(RoleOperator, PermDestModify) {
		t.Errorf("Operator must have PermDestModify")
	}
	if !HasPermission(RoleOperator, PermSyncExecute) {
		t.Errorf("Operator must have PermSyncExecute")
	}
	if HasPermission(RoleOperator, PermAccountsModify) {
		t.Errorf("Operator must NOT have PermAccountsModify")
	}
	if HasPermission(RoleOperator, PermKeysManage) {
		t.Errorf("Operator must NOT have PermKeysManage")
	}

	// Admin
	if !HasPermission(RoleAdmin, PermRead) ||
		!HasPermission(RoleAdmin, PermRulesModify) ||
		!HasPermission(RoleAdmin, PermCatchAllModify) ||
		!HasPermission(RoleAdmin, PermDestModify) ||
		!HasPermission(RoleAdmin, PermSyncExecute) ||
		!HasPermission(RoleAdmin, PermAccountsModify) ||
		!HasPermission(RoleAdmin, PermKeysManage) {
		t.Errorf("Admin must have all permissions")
	}

	// Invalid role
	if HasPermission(Role("unknown"), PermRead) {
		t.Errorf("Unknown role should have no permissions")
	}
}

func TestCanAccessEndpoint(t *testing.T) {
	checker := NewPermissionChecker()

	// 1. Viewer: read-only access (GET endpoints)
	if !checker.CanAccess(RoleViewer, "GET", "/api/v1/accounts") {
		t.Errorf("Viewer should be allowed GET accounts")
	}
	if !checker.CanAccess(RoleViewer, "HEAD", "/api/v1/rules") {
		t.Errorf("Viewer should be allowed HEAD rules")
	}
	if checker.CanAccess(RoleViewer, "POST", "/api/v1/rules") {
		t.Errorf("Viewer should NOT be allowed POST rules")
	}
	if checker.CanAccess(RoleViewer, "PUT", "/api/v1/catch-all") {
		t.Errorf("Viewer should NOT be allowed PUT catch-all")
	}
	if checker.CanAccess(RoleViewer, "DELETE", "/api/v1/destinations/123") {
		t.Errorf("Viewer should NOT be allowed DELETE destinations")
	}

	// 2. Operator: read + modify rules, catch-all, destinations, and sync
	if !checker.CanAccess(RoleOperator, "GET", "/api/v1/accounts") {
		t.Errorf("Operator should be allowed GET accounts")
	}
	if !checker.CanAccess(RoleOperator, "POST", "/api/v1/zones/z1/rules") {
		t.Errorf("Operator should be allowed POST rules")
	}
	if !checker.CanAccess(RoleOperator, "PUT", "/api/v1/zones/z1/catch-all") {
		t.Errorf("Operator should be allowed PUT catch-all")
	}
	if !checker.CanAccess(RoleOperator, "POST", "/api/v1/destinations") {
		t.Errorf("Operator should be allowed POST destinations")
	}
	if !checker.CanAccess(RoleOperator, "POST", "/api/v1/zones/z1/sync") {
		t.Errorf("Operator should be allowed POST sync")
	}
	if !checker.CanAccess(RoleOperator, "PATCH", "/api/v1/zones/z1/routing") {
		t.Errorf("Operator should be allowed PATCH routing")
	}
	// Operator cannot modify accounts or manage keys
	if checker.CanAccess(RoleOperator, "POST", "/api/v1/accounts") {
		t.Errorf("Operator should NOT be allowed POST accounts")
	}
	if checker.CanAccess(RoleOperator, "PUT", "/api/v1/accounts/acc1") {
		t.Errorf("Operator should NOT be allowed PUT accounts")
	}
	if checker.CanAccess(RoleOperator, "DELETE", "/api/v1/accounts/acc1") {
		t.Errorf("Operator should NOT be allowed DELETE accounts")
	}
	if checker.CanAccess(RoleOperator, "POST", "/api/v1/keys/rotate") {
		t.Errorf("Operator should NOT be allowed keys management")
	}

	// 3. Admin: all permissions including accounts and key management
	if !checker.CanAccess(RoleAdmin, "GET", "/api/v1/accounts") {
		t.Errorf("Admin should be allowed GET accounts")
	}
	if !checker.CanAccess(RoleAdmin, "POST", "/api/v1/accounts") {
		t.Errorf("Admin should be allowed POST accounts")
	}
	if !checker.CanAccess(RoleAdmin, "DELETE", "/api/v1/accounts/acc1") {
		t.Errorf("Admin should be allowed DELETE accounts")
	}
	if !checker.CanAccess(RoleAdmin, "POST", "/api/v1/zones/z1/rules") {
		t.Errorf("Admin should be allowed POST rules")
	}
	if !checker.CanAccess(RoleAdmin, "POST", "/api/v1/keys/rotate") {
		t.Errorf("Admin should be allowed POST keys")
	}

	// 4. Invalid role
	if checker.CanAccess(Role("guest"), "GET", "/api/v1/accounts") {
		t.Errorf("Guest should NOT be allowed any endpoint")
	}
}

func fromNilContext() (Role, bool) {
	//lint:ignore SA1012 testing nil context resilience
	//nolint:staticcheck // testing nil context resilience
	return FromContext(nil)
}

func withRawStringKey(ctx context.Context, key string, val any) context.Context {
	//lint:ignore SA1029 testing raw string key fallback
	//nolint:staticcheck // testing raw string key fallback
	return context.WithValue(ctx, key, val)
}



func TestContextRoleStorage(t *testing.T) {
	// Empty context
	r, ok := FromContext(context.Background())
	if ok || r != "" {
		t.Errorf("expected empty role from empty context, got '%s'", r)
	}
	if GetRole(context.Background()) != "" {
		t.Errorf("expected empty GetRole from empty context")
	}

	// Nil context
	r, ok = fromNilContext()
	if ok || r != "" {
		t.Errorf("expected empty role from nil context")
	}

	// WithRole typed
	ctx := WithRole(context.Background(), RoleOperator)
	r, ok = FromContext(ctx)
	if !ok || r != RoleOperator {
		t.Errorf("expected RoleOperator, got '%s'", r)
	}
	if GetRole(ctx) != RoleOperator {
		t.Errorf("expected GetRole to return RoleOperator")
	}

	// String value in typed key
	ctxString := context.WithValue(context.Background(), ActorRoleKey, "admin")
	r, ok = FromContext(ctxString)
	if !ok || r != RoleAdmin {
		t.Errorf("expected RoleAdmin from string value, got '%s'", r)
	}

	// String key "actor_role"
	ctxStringKey := withRawStringKey(context.Background(), "actor_role", "viewer")
	r, ok = FromContext(ctxStringKey)
	if !ok || r != RoleViewer {
		t.Errorf("expected RoleViewer from 'actor_role' string key, got '%s'", r)
	}

	// String key "role"
	ctxRoleKey := withRawStringKey(context.Background(), "role", "operator")
	r, ok = FromContext(ctxRoleKey)
	if !ok || r != RoleOperator {
		t.Errorf("expected RoleOperator from 'role' string key, got '%s'", r)
	}

	// API key actor_type fallback
	ctxAPIKey := withRawStringKey(context.Background(), "actor_type", "api_key")
	r, ok = FromContext(ctxAPIKey)
	if !ok || r != RoleAdmin {
		t.Errorf("expected RoleAdmin from api_key actor_type, got '%s'", r)
	}
}


func TestRequireRoleMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		minRole      Role
		actorRole    Role
		contextMod   func(context.Context) context.Context
		expectStatus int
	}{
		// Viewer requirement
		{
			name:         "Viewer requirement with Viewer role -> Allowed",
			minRole:      RoleViewer,
			actorRole:    RoleViewer,
			expectStatus: http.StatusOK,
		},
		{
			name:         "Viewer requirement with Operator role -> Allowed",
			minRole:      RoleViewer,
			actorRole:    RoleOperator,
			expectStatus: http.StatusOK,
		},
		{
			name:         "Viewer requirement with Admin role -> Allowed",
			minRole:      RoleViewer,
			actorRole:    RoleAdmin,
			expectStatus: http.StatusOK,
		},
		{
			name:         "Viewer requirement with No role -> 403 Forbidden",
			minRole:      RoleViewer,
			expectStatus: http.StatusForbidden,
		},
		{
			name:      "Viewer requirement with API key in context -> Allowed (Admin)",
			minRole:   RoleViewer,
			actorRole: "",
			contextMod: func(ctx context.Context) context.Context {
				return withRawStringKey(ctx, "actor_type", "api_key")
			},

			expectStatus: http.StatusOK,
		},

		// Operator requirement
		{
			name:         "Operator requirement with Viewer role -> 403 Forbidden",
			minRole:      RoleOperator,
			actorRole:    RoleViewer,
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "Operator requirement with Operator role -> Allowed",
			minRole:      RoleOperator,
			actorRole:    RoleOperator,
			expectStatus: http.StatusOK,
		},
		{
			name:         "Operator requirement with Admin role -> Allowed",
			minRole:      RoleOperator,
			actorRole:    RoleAdmin,
			expectStatus: http.StatusOK,
		},
		{
			name:         "Operator requirement with No role -> 403 Forbidden",
			minRole:      RoleOperator,
			expectStatus: http.StatusForbidden,
		},

		// Admin requirement
		{
			name:         "Admin requirement with Viewer role -> 403 Forbidden",
			minRole:      RoleAdmin,
			actorRole:    RoleViewer,
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "Admin requirement with Operator role -> 403 Forbidden",
			minRole:      RoleAdmin,
			actorRole:    RoleOperator,
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "Admin requirement with Admin role -> Allowed",
			minRole:      RoleAdmin,
			actorRole:    RoleAdmin,
			expectStatus: http.StatusOK,
		},
		{
			name:         "Admin requirement with No role -> 403 Forbidden",
			minRole:      RoleAdmin,
			expectStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			})

			handler := RequireRole(tc.minRole)(dummyHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			ctx := req.Context()
			if tc.actorRole != "" {
				ctx = WithRole(ctx, tc.actorRole)
			}
			if tc.contextMod != nil {
				ctx = tc.contextMod(ctx)
			}
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rec.Code)
			}

			if tc.expectStatus == http.StatusForbidden {
				contentType := rec.Header().Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("expected Content-Type application/json, got %s", contentType)
				}

				// Check exact JSON body
				var errResp struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode 403 JSON: %v, body was: %s", err, rec.Body.String())
				}
				if errResp.Error.Code != "FORBIDDEN" {
					t.Errorf("expected code FORBIDDEN, got %s", errResp.Error.Code)
				}
				if errResp.Error.Message != "Insufficient permissions" {
					t.Errorf("expected message 'Insufficient permissions', got '%s'", errResp.Error.Message)
				}
			}
		})
	}
}

func TestRequirePermissionMiddleware(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequirePermission(PermAccountsModify)(dummyHandler)

	// Operator cannot modify accounts
	req1 := httptest.NewRequest("POST", "/test", nil)
	req1 = req1.WithContext(WithRole(req1.Context(), RoleOperator))
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusForbidden {
		t.Errorf("expected 403 for operator on accounts modify, got %d", rec1.Code)
	}

	// Admin can modify accounts
	req2 := httptest.NewRequest("POST", "/test", nil)
	req2 = req2.WithContext(WithRole(req2.Context(), RoleAdmin))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 for admin on accounts modify, got %d", rec2.Code)
	}
}
