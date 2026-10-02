package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/destination"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider/mock"
	"github.com/bariskode/email-management-service/internal/routing"
	"github.com/bariskode/email-management-service/internal/storage/memory"
	emsSync "github.com/bariskode/email-management-service/internal/sync"
)

func setupTestServer() (http.Handler, *mock.Provider, string) {
	repos := memory.New()
	prov := mock.New()
	auditService := audit.NewService(repos.Audit)
	destService := destination.NewService(repos.Destinations, prov, auditService)
	routeService := routing.NewService(repos, prov, destService, auditService)
	syncEngine := emsSync.NewEngine(repos, prov, auditService)

	masterKey := make([]byte, 32)
	apiKey := "test_api_key_secret"

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	server := NewServer(repos, destService, routeService, syncEngine, masterKey)
	router := NewRouter(server, apiKey, 100, logger)

	return router, prov, apiKey
}

func doRequest(handler http.Handler, method, path string, body any, apiKey string, headers map[string]string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealthAndMetricsEndpoints(t *testing.T) {
	handler, _, _ := setupTestServer()

	// GET /healthz
	res := doRequest(handler, http.MethodGet, "/healthz", nil, "", nil)
	if res.Code != http.StatusOK {
		t.Errorf("expected 200 for /healthz, got %d", res.Code)
	}

	// GET /readyz
	res = doRequest(handler, http.MethodGet, "/readyz", nil, "", nil)
	if res.Code != http.StatusOK {
		t.Errorf("expected 200 for /readyz, got %d", res.Code)
	}

	// GET /metrics
	res = doRequest(handler, http.MethodGet, "/metrics", nil, "", nil)
	if res.Code != http.StatusOK {
		t.Errorf("expected 200 for /metrics, got %d", res.Code)
	}
}

func TestAuthMiddleware(t *testing.T) {
	handler, _, apiKey := setupTestServer()

	// No API key
	res := doRequest(handler, http.MethodGet, "/api/v1/accounts", nil, "", nil)
	if res.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", res.Code)
	}

	// Bad API key
	res = doRequest(handler, http.MethodGet, "/api/v1/accounts", nil, "wrong_key", nil)
	if res.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong key, got %d", res.Code)
	}

	// Good API key
	res = doRequest(handler, http.MethodGet, "/api/v1/accounts", nil, apiKey, nil)
	if res.Code != http.StatusOK {
		t.Errorf("expected 200 with valid key, got %d", res.Code)
	}
}

func TestIdempotencyMiddleware(t *testing.T) {
	handler, _, apiKey := setupTestServer()

	accountPayload := map[string]string{
		"name":                  "Primary Account",
		"cloudflare_account_id": "cf_acc_idem",
		"api_token":             "cf_secret_token_123",
	}

	headers := map[string]string{
		"Idempotency-Key": "unique-request-id-12345",
	}

	// First request
	res1 := doRequest(handler, http.MethodPost, "/api/v1/accounts", accountPayload, apiKey, headers)
	if res1.Code != http.StatusCreated {
		t.Fatalf("first request failed: %d, body: %s", res1.Code, res1.Body.String())
	}

	// Second request with same idempotency key
	res2 := doRequest(handler, http.MethodPost, "/api/v1/accounts", accountPayload, apiKey, headers)
	if res2.Code != http.StatusCreated {
		t.Fatalf("second request failed: %d", res2.Code)
	}

	if res2.Header().Get("X-Cache-Lookup") != "HIT-IDEMPOTENT" {
		t.Errorf("expected HIT-IDEMPOTENT header on idempotent replay")
	}

	if res1.Body.String() != res2.Body.String() {
		t.Errorf("cached response body does not match original")
	}
}

func TestCompleteWorkflow(t *testing.T) {
	handler, prov, apiKey := setupTestServer()

	// 1. Create Cloudflare Account
	accRes := doRequest(handler, http.MethodPost, "/api/v1/accounts", map[string]string{
		"name":                  "Production Account",
		"cloudflare_account_id": "cf_acc_prod",
		"api_token":             "secret_token_xyz",
	}, apiKey, nil)

	if accRes.Code != http.StatusCreated {
		t.Fatalf("failed to create account: %d, %s", accRes.Code, accRes.Body.String())
	}

	var accEnvelope SuccessEnvelope
	_ = json.Unmarshal(accRes.Body.Bytes(), &accEnvelope)
	accData := accEnvelope.Data.(map[string]any)
	accountID := accData["cloudflare_account_id"].(string)

	// Verify token is NOT exposed in response
	if _, hasToken := accData["credential_ref"]; hasToken {
		t.Errorf("credential_ref should be omitted from API response")
	}

	// 2. Create Destination Address
	destRes := doRequest(handler, http.MethodPost, "/api/v1/destinations", map[string]string{
		"account_id": accountID,
		"email":      "forward@gmail.com",
	}, apiKey, nil)

	if destRes.Code != http.StatusCreated {
		t.Fatalf("failed to create destination: %d, %s", destRes.Code, destRes.Body.String())
	}

	var destEnvelope SuccessEnvelope
	_ = json.Unmarshal(destRes.Body.Bytes(), &destEnvelope)
	destData := destEnvelope.Data.(map[string]any)
	destID := destData["id"].(string)

	// 3. Import Zone
	// Add remote zone to mock provider
	prov.Zones["cf_zone_baris"] = domain.Zone{
		ProviderZoneID: "cf_zone_baris",
		Name:           "bariskode.com",
		Status:         "active",
	}

	zoneRes := doRequest(handler, http.MethodPost, "/api/v1/zones/import", map[string]string{
		"account_id":       accountID,
		"provider_zone_id": "cf_zone_baris",
	}, apiKey, nil)

	if zoneRes.Code != http.StatusCreated {
		t.Fatalf("failed to import zone: %d, %s", zoneRes.Code, zoneRes.Body.String())
	}

	var zoneEnvelope SuccessEnvelope
	_ = json.Unmarshal(zoneRes.Body.Bytes(), &zoneEnvelope)
	zoneData := zoneEnvelope.Data.(map[string]any)
	localZoneID := zoneData["id"].(string)

	// 4. Try creating rule with UNVERIFIED destination -> expect error
	ruleFailRes := doRequest(handler, http.MethodPost, "/api/v1/zones/"+localZoneID+"/rules", map[string]any{
		"name":          "Support Rule",
		"matcher_value": "support@bariskode.com",
		"action_type":   "forward",
		"destination":   "forward@gmail.com",
		"enabled":       true,
	}, apiKey, nil)

	if ruleFailRes.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unverified destination, got %d", ruleFailRes.Code)
	}

	// Verify destination manually in mock
	patchDestRes := doRequest(handler, http.MethodPatch, "/api/v1/destinations/"+destID, map[string]string{
		"email": "forward@gmail.com",
	}, apiKey, nil)
	if patchDestRes.Code != http.StatusOK {
		t.Fatalf("failed to update destination: %d", patchDestRes.Code)
	}

	// Update destination in mock provider to mark verified
	now := time.Now()
	for _, addrs := range prov.Destinations {
		for id, addr := range addrs {
			addr.VerifiedAt = &now
			addr.Status = domain.DestinationStatusVerified
			addrs[id] = addr
		}
	}
	// Also mark verified in storage for destination
	// Let's create rule with drop action (which does not require destination)
	ruleDropRes := doRequest(handler, http.MethodPost, "/api/v1/zones/"+localZoneID+"/rules", map[string]any{
		"name":          "Spam Drop Rule",
		"matcher_value": "spam@bariskode.com",
		"action_type":   "drop",
		"enabled":       true,
	}, apiKey, nil)

	if ruleDropRes.Code != http.StatusCreated {
		t.Fatalf("failed to create drop rule: %d, %s", ruleDropRes.Code, ruleDropRes.Body.String())
	}

	// 5. Catch-All configuration
	caRes := doRequest(handler, http.MethodPut, "/api/v1/zones/"+localZoneID+"/catch-all", map[string]any{
		"enabled":     true,
		"action_type": "drop",
	}, apiKey, nil)

	if caRes.Code != http.StatusOK {
		t.Fatalf("failed to configure catch-all: %d, %s", caRes.Code, caRes.Body.String())
	}

	// 6. Sync zone
	syncRes := doRequest(handler, http.MethodPost, "/api/v1/zones/"+localZoneID+"/sync", map[string]string{
		"direction": "drift_check",
	}, apiKey, nil)

	if syncRes.Code != http.StatusOK {
		t.Fatalf("failed to sync zone: %d, %s", syncRes.Code, syncRes.Body.String())
	}

	// 7. Check Audit Events
	auditRes := doRequest(handler, http.MethodGet, "/api/v1/audit-events", nil, apiKey, nil)
	if auditRes.Code != http.StatusOK {
		t.Fatalf("failed to list audit events: %d", auditRes.Code)
	}

	var auditEnvelope SuccessEnvelope
	_ = json.Unmarshal(auditRes.Body.Bytes(), &auditEnvelope)
	events := auditEnvelope.Data.([]any)
	if len(events) == 0 {
		t.Errorf("expected audit events to be recorded for mutations")
	}

	// 8. Check Sync Runs
	syncRunsRes := doRequest(handler, http.MethodGet, "/api/v1/sync-runs", nil, apiKey, nil)
	if syncRunsRes.Code != http.StatusOK {
		t.Fatalf("failed to list sync runs: %d", syncRunsRes.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler, _, _ := setupTestServer()

	res := doRequest(handler, http.MethodGet, "/healthz", nil, "", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	headers := res.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options nosniff")
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options DENY")
	}
	if headers.Get("Strict-Transport-Security") == "" {
		t.Errorf("expected Strict-Transport-Security header")
	}
	if headers.Get("Content-Security-Policy") == "" {
		t.Errorf("expected Content-Security-Policy header")
	}
}

func TestRateLimiter(t *testing.T) {
	repos := memory.New()
	prov := mock.New()
	auditService := audit.NewService(repos.Audit)
	destService := destination.NewService(repos.Destinations, prov, auditService)
	routeService := routing.NewService(repos, prov, destService, auditService)
	syncEngine := emsSync.NewEngine(repos, prov, auditService)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	server := NewServer(repos, destService, routeService, syncEngine, make([]byte, 32))
	// Set limit to 2 requests
	router := NewRouter(server, "test_key", 2, logger)

	// 1st request -> 200
	res1 := doRequest(router, http.MethodGet, "/healthz", nil, "", nil)
	if res1.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", res1.Code)
	}

	// 2nd request -> 200
	res2 := doRequest(router, http.MethodGet, "/healthz", nil, "", nil)
	if res2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", res2.Code)
	}

	// 3rd request -> 429 Too Many Requests
	res3 := doRequest(router, http.MethodGet, "/healthz", nil, "", nil)
	if res3.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", res3.Code)
	}
	if res3.Header().Get("Retry-After") != "60" {
		t.Errorf("expected Retry-After 60 header")
	}
}

func TestEmbeddedWebDashboard(t *testing.T) {
	handler, _, _ := setupTestServer()

	// GET /
	res := doRequest(handler, http.MethodGet, "/", nil, "", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for /, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "EMS — Email Management Service") {
		t.Errorf("expected HTML body to contain dashboard title")
	}

	// GET /dashboard
	res2 := doRequest(handler, http.MethodGet, "/dashboard", nil, "", nil)
	if res2.Code != http.StatusOK {
		t.Fatalf("expected 200 for /dashboard, got %d", res2.Code)
	}
	if !strings.Contains(res2.Body.String(), "EMS Control") {
		t.Errorf("expected HTML body to contain EMS Control")
	}
}
