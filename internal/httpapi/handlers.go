package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/bariskode/email-management-service/internal/auth"
	"github.com/bariskode/email-management-service/internal/destination"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/observability"
	"github.com/bariskode/email-management-service/internal/routing"
	"github.com/bariskode/email-management-service/internal/storage"
	emsSync "github.com/bariskode/email-management-service/internal/sync"
)

// Server coordinates HTTP endpoints.
type Server struct {
	repos       *storage.Repositories
	destService *destination.Service
	routeService *routing.Service
	syncEngine  *emsSync.Engine
	masterKey   []byte
}

// NewServer creates a new HTTP server handler instance.
func NewServer(
	repos *storage.Repositories,
	destService *destination.Service,
	routeService *routing.Service,
	syncEngine *emsSync.Engine,
	masterKey []byte,
) *Server {
	return &Server{
		repos:        repos,
		destService:  destService,
		routeService: routeService,
		syncEngine:   syncEngine,
		masterKey:    masterKey,
	}
}

// --- Accounts Handlers ---

type createAccountRequest struct {
	Name                string `json:"name"`
	CloudflareAccountID string `json:"cloudflare_account_id"`
	APIToken            string `json:"api_token"`
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.repos.Accounts.List(r.Context())
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, accounts)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	if req.Name == "" || req.CloudflareAccountID == "" || req.APIToken == "" {
		respondError(w, r, domain.NewValidationError("name, cloudflare_account_id, and api_token are required"))
		return
	}

	// Encrypt API token at rest
	encryptedToken, err := auth.Encrypt([]byte(req.APIToken), s.masterKey)
	if err != nil {
		respondError(w, r, domain.NewInternalError("failed to encrypt account credentials", err))
		return
	}

	now := time.Now().UTC()
	account := &domain.CloudflareAccount{
		ID:                  generateID("acc"),
		Name:                req.Name,
		CloudflareAccountID: req.CloudflareAccountID,
		CredentialRef:       encryptedToken,
		Status:              "active",
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.repos.Accounts.Save(r.Context(), account); err != nil {
		respondError(w, r, domain.NewInternalError("failed to save account", err))
		return
	}

	respondJSON(w, r, http.StatusCreated, account)
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	account, err := s.repos.Accounts.Get(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, account)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	account, err := s.repos.Accounts.Get(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}

	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	if req.Name != "" {
		account.Name = req.Name
	}
	if req.CloudflareAccountID != "" {
		account.CloudflareAccountID = req.CloudflareAccountID
	}
	if req.APIToken != "" {
		enc, err := auth.Encrypt([]byte(req.APIToken), s.masterKey)
		if err != nil {
			respondError(w, r, domain.NewInternalError("failed to encrypt token", err))
			return
		}
		account.CredentialRef = enc
	}
	account.UpdatedAt = time.Now().UTC()

	if err := s.repos.Accounts.Save(r.Context(), account); err != nil {
		respondError(w, r, domain.NewInternalError("failed to update account", err))
		return
	}

	respondJSON(w, r, http.StatusOK, account)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.repos.Accounts.Delete(r.Context(), id); err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

// --- Destination Handlers ---

type createDestinationRequest struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
}

func (s *Server) handleListDestinations(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	list, err := s.destService.List(r.Context(), accountID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, list)
}

func (s *Server) handleCreateDestination(w http.ResponseWriter, r *http.Request) {
	var req createDestinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	reqID := observability.GetRequestID(r.Context())
	dest, err := s.destService.Create(r.Context(), req.AccountID, req.Email, reqID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusCreated, dest)
}

func (s *Server) handleGetDestination(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dest, err := s.destService.Get(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, dest)
}

func (s *Server) handleUpdateDestination(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	reqID := observability.GetRequestID(r.Context())
	dest, err := s.destService.Update(r.Context(), id, req.Email, reqID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, dest)
}

func (s *Server) handleDeleteDestination(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reqID := observability.GetRequestID(r.Context())
	if err := s.destService.Delete(r.Context(), id, reqID); err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

// --- Zone Handlers ---

func (s *Server) handleListZones(w http.ResponseWriter, r *http.Request) {
	zones, err := s.routeService.ListZones(r.Context())
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, zones)
}

func (s *Server) handleDiscoverRemoteZones(w http.ResponseWriter, r *http.Request) {
	zones, err := s.routeService.DiscoverRemoteZones(r.Context())
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, zones)
}

type importZoneRequest struct {
	AccountID      string `json:"account_id"`
	ProviderZoneID string `json:"provider_zone_id"`
}

func (s *Server) handleImportZone(w http.ResponseWriter, r *http.Request) {
	var req importZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	reqID := observability.GetRequestID(r.Context())
	zone, err := s.routeService.ImportZone(r.Context(), req.AccountID, req.ProviderZoneID, reqID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusCreated, zone)
}

func (s *Server) handleGetZone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	zone, err := s.routeService.GetZone(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, zone)
}

func (s *Server) handleGetRoutingSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	settings, err := s.routeService.GetRoutingSettings(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, settings)
}

func (s *Server) handleUpdateRoutingSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	reqID := observability.GetRequestID(r.Context())
	if err := s.routeService.UpdateRoutingSettings(r.Context(), id, req.Enabled, reqID); err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, map[string]any{"id": id, "enabled": req.Enabled})
}

// --- Explicit Rules Handlers ---

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rules, err := s.routeService.ListRules(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, rules)
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req routing.CreateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	reqID := observability.GetRequestID(r.Context())
	rule, err := s.routeService.CreateRule(r.Context(), id, req, reqID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusCreated, rule)
}

func (s *Server) handleGetRule(w http.ResponseWriter, r *http.Request) {
	ruleID := r.PathValue("ruleID")
	rule, err := s.routeService.GetRule(r.Context(), ruleID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, rule)
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	zoneID := r.PathValue("id")
	ruleID := r.PathValue("ruleID")

	var req routing.CreateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	reqID := observability.GetRequestID(r.Context())
	rule, err := s.routeService.UpdateRule(r.Context(), zoneID, ruleID, req, reqID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, rule)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	zoneID := r.PathValue("id")
	ruleID := r.PathValue("ruleID")
	reqID := observability.GetRequestID(r.Context())

	if err := s.routeService.DeleteRule(r.Context(), zoneID, ruleID, reqID); err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, map[string]string{"id": ruleID, "status": "deleted"})
}

// --- Catch-All Handlers ---

func (s *Server) handleGetCatchAll(w http.ResponseWriter, r *http.Request) {
	zoneID := r.PathValue("id")
	ca, err := s.routeService.GetCatchAll(r.Context(), zoneID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, ca)
}

func (s *Server) handleUpdateCatchAll(w http.ResponseWriter, r *http.Request) {
	zoneID := r.PathValue("id")
	var req routing.CatchAllRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("invalid request body"))
		return
	}

	reqID := observability.GetRequestID(r.Context())
	ca, err := s.routeService.UpdateCatchAll(r.Context(), zoneID, req, reqID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, ca)
}

// --- Sync Handlers ---

func (s *Server) handleSyncZone(w http.ResponseWriter, r *http.Request) {
	zoneID := r.PathValue("id")
	var req struct {
		Direction string `json:"direction"` // pull, push, drift_check
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Direction == "" {
		req.Direction = "drift_check"
	}

	reqID := observability.GetRequestID(r.Context())
	res, err := s.syncEngine.SyncZone(r.Context(), zoneID, req.Direction, reqID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, res)
}

// --- Audit & History Handlers ---

func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	events, err := s.repos.Audit.List(r.Context(), limit)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, events)
}

func (s *Server) handleListSyncRuns(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	runs, err := s.repos.SyncRuns.List(r.Context(), limit)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respondJSON(w, r, http.StatusOK, runs)
}
