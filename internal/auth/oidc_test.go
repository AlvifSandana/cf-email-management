package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bariskode/email-management-service/internal/auth/rbac"
)

func TestExtractBearerToken(t *testing.T) {
	// Valid standard header
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid.jwt.token")
	token, err := ExtractBearerToken(req)
	if err != nil || token != "valid.jwt.token" {
		t.Fatalf("expected 'valid.jwt.token', got '%s', err: %v", token, err)
	}

	// Case-insensitive "bearer"
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "bearer valid.jwt.token")
	token, err = ExtractBearerToken(req)
	if err != nil || token != "valid.jwt.token" {
		t.Fatalf("expected 'valid.jwt.token' for lowercase bearer, got '%s'", token)
	}

	// Missing header
	req = httptest.NewRequest("GET", "/", nil)
	_, err = ExtractBearerToken(req)
	if err == nil {
		t.Fatalf("expected error for missing Authorization header")
	}

	// Non-Bearer header
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	_, err = ExtractBearerToken(req)
	if err == nil {
		t.Fatalf("expected error for Basic auth header")
	}

	// Empty token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	_, err = ExtractBearerToken(req)
	if err == nil {
		t.Fatalf("expected error for empty bearer token")
	}
}

func TestParseJWTClaims(t *testing.T) {
	secret := []byte("top_secret_jwt_signing_key_12345")

	// 1. Valid token with roles array
	claims1 := &Claims{
		Subject:   "usr_1001",
		Email:     "admin@example.com",
		Roles:     []string{"admin", "viewer"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token1, err := GenerateTestToken(claims1, secret)
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}

	parsed1, err := ParseJWT(token1)
	if err != nil {
		t.Fatalf("failed to parse valid JWT: %v", err)
	}
	if parsed1.Subject != "usr_1001" {
		t.Errorf("expected sub usr_1001, got %s", parsed1.Subject)
	}
	if parsed1.Email != "admin@example.com" {
		t.Errorf("expected email admin@example.com, got %s", parsed1.Email)
	}
	if parsed1.PrimaryRole() != rbac.RoleAdmin {
		t.Errorf("expected primary role Admin, got %s", parsed1.PrimaryRole())
	}

	// 2. Token with single role string
	claims2 := &Claims{
		Subject:   "usr_1002",
		Email:     "operator@example.com",
		Role:      "operator",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token2, err := GenerateTestToken(claims2, secret)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	parsed2, err := ParseJWT(token2)
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}
	if parsed2.PrimaryRole() != rbac.RoleOperator {
		t.Errorf("expected primary role Operator, got %s", parsed2.PrimaryRole())
	}

	// 3. Token with Viewer role
	claims3 := &Claims{
		Subject:   "usr_1003",
		Email:     "viewer@example.com",
		Roles:     []string{"viewer"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token3, err := GenerateTestToken(claims3, secret)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	parsed3, err := ParseJWT(token3)
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}
	if parsed3.PrimaryRole() != rbac.RoleViewer {
		t.Errorf("expected primary role Viewer, got %s", parsed3.PrimaryRole())
	}

	// 4. Token with Keycloak realm_access.roles
	keycloakPayload := `{"sub":"usr_kc","email":"kc@example.com","realm_access":{"roles":["admin","offline_access"]},"exp":` +
		string(mustJSON(time.Now().Add(time.Hour).Unix())) + `}`
	tokenKC, _ := GenerateTestToken(&Claims{Subject: "usr_kc", Email: "kc@example.com", Raw: map[string]any{
		"realm_access": map[string]any{
			"roles": []any{"admin", "offline_access"},
		},
	}})
	// Test custom JSON unmarshal directly
	var parsedKC Claims
	if err := json.Unmarshal([]byte(keycloakPayload), &parsedKC); err != nil {
		t.Fatalf("failed to unmarshal keycloak payload: %v", err)
	}
	if parsedKC.PrimaryRole() != rbac.RoleAdmin {
		t.Errorf("expected Admin from Keycloak realm_access, got %s", parsedKC.PrimaryRole())
	}
	_ = tokenKC

	// 5. Expired token
	expiredClaims := &Claims{
		Subject:   "usr_expired",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	}
	tokenExpired, err := GenerateTestToken(expiredClaims, secret)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}
	_, err = ParseJWT(tokenExpired)
	if err == nil || err.Error() != "token is expired" {
		t.Fatalf("expected 'token is expired' error, got: %v", err)
	}

	// 6. Token with not-before in future
	futureClaims := &Claims{
		Subject:   "usr_future",
		NotBefore: time.Now().Add(time.Hour).Unix(),
		ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
	}
	tokenFuture, _ := GenerateTestToken(futureClaims, secret)
	_, err = ParseJWT(tokenFuture)
	if err == nil || err.Error() != "token not valid yet" {
		t.Fatalf("expected 'token not valid yet' error, got: %v", err)
	}

	// 7. Malformed tokens
	if _, err := ParseJWT("only.twoparts"); err == nil {
		t.Fatalf("expected error for token with only 2 parts")
	}
	if _, err := ParseJWT("not_base64.not_base64.signature"); err == nil {
		t.Fatalf("expected error for invalid base64")
	}
	if _, err := ParseJWT("eyJhbGciOiJub25lIn0.invalid_json.sig"); err == nil {
		t.Fatalf("expected error for invalid JSON payload")
	}
}

func TestTokenSignatureVerification(t *testing.T) {
	secret := []byte("correct_signing_secret_xyz123")
	wrongSecret := []byte("wrong_signing_secret_abc456")

	claims := &Claims{
		Subject:   "usr_sig",
		Email:     "sig@example.com",
		Roles:     []string{"operator"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}

	token, err := GenerateTestToken(claims, secret)
	if err != nil {
		t.Fatalf("failed to generate signed token: %v", err)
	}

	// Validate with correct secret
	validClaims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("expected signature to validate with correct secret, got: %v", err)
	}
	if validClaims.Subject != "usr_sig" {
		t.Errorf("expected sub usr_sig, got %s", validClaims.Subject)
	}

	// Validate with wrong secret
	_, err = ValidateToken(token, wrongSecret)
	if err == nil || err.Error() != "invalid token signature" {
		t.Fatalf("expected 'invalid token signature' error, got: %v", err)
	}

	// Validate with no secret (should parse claims without verifying signature)
	parsedNoSecret, err := ValidateToken(token, nil)
	if err != nil {
		t.Fatalf("validation without secret should parse token, got: %v", err)
	}
	if parsedNoSecret.Subject != "usr_sig" {
		t.Errorf("expected sub usr_sig, got %s", parsedNoSecret.Subject)
	}
}

func TestWithActorContext(t *testing.T) {
	claims := &Claims{
		Subject: "usr_alice",
		Email:   "alice@company.org",
		Roles:   []string{"operator"},
	}

	ctx := WithActorContext(context.Background(), claims)

	// Check GetActor
	actorType, actorID := GetActor(ctx)
	if actorType != "oidc" {
		t.Errorf("expected actorType 'oidc', got '%s'", actorType)
	}
	if actorID != "usr_alice" {
		t.Errorf("expected actorID 'usr_alice', got '%s'", actorID)
	}

	// Check GetActorEmail
	email := GetActorEmail(ctx)
	if email != "alice@company.org" {
		t.Errorf("expected email 'alice@company.org', got '%s'", email)
	}

	// Check GetActorRole
	role := GetActorRole(ctx)
	if role != rbac.RoleOperator {
		t.Errorf("expected role RoleOperator, got '%s'", role)
	}

	// Check RBAC context compatibility
	rbacRole, ok := rbac.FromContext(ctx)
	if !ok || rbacRole != rbac.RoleOperator {
		t.Errorf("rbac.FromContext should return RoleOperator, got '%s'", rbacRole)
	}
}

func TestOIDCMiddleware(t *testing.T) {
	secret := []byte("middleware_test_secret_789")
	validator := NewOIDCValidator(secret)

	var capturedActorType, capturedActorID string
	var capturedRole rbac.Role
	var capturedEmail string

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedActorType, capturedActorID = GetActor(r.Context())
		capturedEmail = GetActorEmail(r.Context())
		capturedRole = GetActorRole(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	mw := validator.Middleware()(testHandler)

	// 1. Success case with valid JWT
	claims := &Claims{
		Subject:   "usr_test",
		Email:     "test@domain.com",
		Roles:     []string{"operator"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token, _ := GenerateTestToken(claims, secret)

	req := httptest.NewRequest("GET", "/api/v1/zones", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedActorType != "oidc" || capturedActorID != "usr_test" {
		t.Errorf("unexpected actor: type=%s, id=%s", capturedActorType, capturedActorID)
	}
	if capturedEmail != "test@domain.com" {
		t.Errorf("unexpected email: %s", capturedEmail)
	}
	if capturedRole != rbac.RoleOperator {
		t.Errorf("unexpected role: %s", capturedRole)
	}

	// 2. Missing Authorization header -> 401
	reqMissing := httptest.NewRequest("GET", "/api/v1/zones", nil)
	recMissing := httptest.NewRecorder()
	mw.ServeHTTP(recMissing, reqMissing)
	if recMissing.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing auth, got %d", recMissing.Code)
	}

	// 3. Invalid signature -> 401
	tokenBadSig, _ := GenerateTestToken(claims, []byte("bad_key"))
	reqBadSig := httptest.NewRequest("GET", "/api/v1/zones", nil)
	reqBadSig.Header.Set("Authorization", "Bearer "+tokenBadSig)
	recBadSig := httptest.NewRecorder()
	mw.ServeHTTP(recBadSig, reqBadSig)
	if recBadSig.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad signature, got %d", recBadSig.Code)
	}
}

func TestCombinedAuthMiddleware(t *testing.T) {
	apiKey := "secret_api_key_value_123"
	secret := []byte("oidc_secret_key_456")
	validator := NewOIDCValidator(secret)

	combinedMW := CombinedAuthMiddleware(apiKey, validator)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actorType, actorID := GetActor(r.Context())
		role := GetActorRole(r.Context())
		w.Header().Set("X-Test-Actor-Type", actorType)
		w.Header().Set("X-Test-Actor-ID", actorID)
		w.Header().Set("X-Test-Role", string(role))
		w.WriteHeader(http.StatusOK)
	})

	handler := combinedMW(testHandler)

	// 1. API Key via X-API-Key header
	req1 := httptest.NewRequest("GET", "/api/v1/zones", nil)
	req1.Header.Set("X-API-Key", apiKey)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 via X-API-Key, got %d", rec1.Code)
	}
	if rec1.Header().Get("X-Test-Actor-Type") != "api_key" || rec1.Header().Get("X-Test-Role") != "admin" {
		t.Errorf("expected api_key/admin, got %s/%s", rec1.Header().Get("X-Test-Actor-Type"), rec1.Header().Get("X-Test-Role"))
	}

	// 2. API Key via Authorization: Bearer <key>
	req2 := httptest.NewRequest("GET", "/api/v1/zones", nil)
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 via Bearer <apiKey>, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Test-Role") != "admin" {
		t.Errorf("expected admin role, got %s", rec2.Header().Get("X-Test-Role"))
	}

	// 3. OIDC JWT via Authorization: Bearer <jwt>
	claims := &Claims{
		Subject:   "oidc_usr_1",
		Email:     "user@oidc.local",
		Roles:     []string{"operator"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token, _ := GenerateTestToken(claims, secret)
	req3 := httptest.NewRequest("GET", "/api/v1/zones", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 via Bearer <jwt>, got %d", rec3.Code)
	}
	if rec3.Header().Get("X-Test-Actor-Type") != "oidc" || rec3.Header().Get("X-Test-Role") != "operator" {
		t.Errorf("expected oidc/operator, got %s/%s", rec3.Header().Get("X-Test-Actor-Type"), rec3.Header().Get("X-Test-Role"))
	}

	// 4. Bad credentials -> 401
	req4 := httptest.NewRequest("GET", "/api/v1/zones", nil)
	req4.Header.Set("Authorization", "Bearer bad_token_string")
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad token, got %d", rec4.Code)
	}
}

func TestOIDCAndRBACIntegration(t *testing.T) {
	secret := []byte("integration_test_secret")
	validator := NewOIDCValidator(secret)

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	// Pipeline: OIDC -> RequireRole(Operator)
	operatorPipeline := validator.Middleware()(rbac.RequireRole(rbac.RoleOperator)(dummyHandler))

	// Pipeline: OIDC -> RequireRole(Admin)
	adminPipeline := validator.Middleware()(rbac.RequireRole(rbac.RoleAdmin)(dummyHandler))

	// Viewer token
	viewerClaims := &Claims{
		Subject:   "usr_viewer",
		Roles:     []string{"viewer"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	tokenViewer, _ := GenerateTestToken(viewerClaims, secret)

	// Operator token
	operatorClaims := &Claims{
		Subject:   "usr_operator",
		Roles:     []string{"operator"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	tokenOperator, _ := GenerateTestToken(operatorClaims, secret)

	// Admin token
	adminClaims := &Claims{
		Subject:   "usr_admin",
		Roles:     []string{"admin"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	tokenAdmin, _ := GenerateTestToken(adminClaims, secret)

	// Test 1: Viewer against Operator pipeline -> 403 Forbidden
	req1 := httptest.NewRequest("POST", "/api/v1/zones/z1/rules", nil)
	req1.Header.Set("Authorization", "Bearer "+tokenViewer)
	rec1 := httptest.NewRecorder()
	operatorPipeline.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Viewer on Operator route, got %d", rec1.Code)
	}

	// Test 2: Operator against Operator pipeline -> 200 OK
	req2 := httptest.NewRequest("POST", "/api/v1/zones/z1/rules", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenOperator)
	rec2 := httptest.NewRecorder()
	operatorPipeline.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Operator on Operator route, got %d", rec2.Code)
	}

	// Test 3: Admin against Operator pipeline -> 200 OK
	req3 := httptest.NewRequest("POST", "/api/v1/zones/z1/rules", nil)
	req3.Header.Set("Authorization", "Bearer "+tokenAdmin)
	rec3 := httptest.NewRecorder()
	operatorPipeline.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Admin on Operator route, got %d", rec3.Code)
	}

	// Test 4: Operator against Admin pipeline -> 403 Forbidden
	req4 := httptest.NewRequest("POST", "/api/v1/accounts", nil)
	req4.Header.Set("Authorization", "Bearer "+tokenOperator)
	rec4 := httptest.NewRecorder()
	adminPipeline.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Operator on Admin route, got %d", rec4.Code)
	}

	// Test 5: Admin against Admin pipeline -> 200 OK
	req5 := httptest.NewRequest("POST", "/api/v1/accounts", nil)
	req5.Header.Set("Authorization", "Bearer "+tokenAdmin)
	rec5 := httptest.NewRecorder()
	adminPipeline.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Admin on Admin route, got %d", rec5.Code)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
