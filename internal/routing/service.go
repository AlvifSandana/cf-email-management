package routing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/destination"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
	"github.com/bariskode/email-management-service/internal/storage"
)

// Service provides zone, explicit rules, and catch-all management operations.
type Service struct {
	repos       *storage.Repositories
	provider    provider.EmailProvider
	destService *destination.Service
	audit       *audit.Service
}

// NewService creates a new Routing Service.
func NewService(
	repos *storage.Repositories,
	prov provider.EmailProvider,
	destService *destination.Service,
	audit *audit.Service,
) *Service {
	return &Service{
		repos:       repos,
		provider:    prov,
		destService: destService,
		audit:       audit,
	}
}

// --- Zone Management ---

// ListZones returns all locally tracked zones.
func (s *Service) ListZones(ctx context.Context) ([]domain.Zone, error) {
	return s.repos.Zones.List(ctx)
}

// GetZone returns a zone by its ID.
func (s *Service) GetZone(ctx context.Context, id string) (*domain.Zone, error) {
	return s.repos.Zones.Get(ctx, id)
}

// DiscoverRemoteZones queries Cloudflare for all zones available under the account.
func (s *Service) DiscoverRemoteZones(ctx context.Context) ([]domain.Zone, error) {
	return s.provider.ListZones(ctx)
}

// ImportZone imports a zone from Cloudflare into EMS local state.
func (s *Service) ImportZone(ctx context.Context, accountID, providerZoneID, requestID string) (*domain.Zone, error) {
	remoteZones, err := s.provider.ListZones(ctx)
	if err != nil {
		return nil, err
	}

	var found *domain.Zone
	for _, rz := range remoteZones {
		if rz.ProviderZoneID == providerZoneID {
			found = &rz
			break
		}
	}
	if found == nil {
		return nil, domain.NewNotFoundError("remote zone", providerZoneID)
	}

	// Fetch routing settings
	settings, err := s.provider.GetEmailRoutingSettings(ctx, providerZoneID)
	routingEnabled := false
	if err == nil {
		routingEnabled = settings.Enabled
	}

	existing, err := s.repos.Zones.GetByProviderZoneID(ctx, providerZoneID)
	now := time.Now().UTC()
	var zone *domain.Zone

	if err == nil && existing != nil {
		zone = existing
		zone.Status = found.Status
		zone.EmailRoutingEnabled = routingEnabled
		zone.UpdatedAt = now
	} else {
		zone = &domain.Zone{
			ID:                  generateID("zon"),
			ProviderAccountID:   accountID,
			ProviderZoneID:      providerZoneID,
			Name:                found.Name,
			Status:              found.Status,
			EmailRoutingEnabled: routingEnabled,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
	}

	if err := s.repos.Zones.Save(ctx, zone); err != nil {
		return nil, domain.NewInternalError("failed to save imported zone", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:    "IMPORT_ZONE",
		ResourceType: "zone",
		ResourceID:   zone.ID,
		RequestID:    requestID,
		After:        zone,
		Status:       "SUCCESS",
	})

	return zone, nil
}

// GetRoutingSettings returns the email routing settings from provider.
func (s *Service) GetRoutingSettings(ctx context.Context, zoneID string) (provider.RoutingSettings, error) {
	zone, err := s.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return provider.RoutingSettings{}, err
	}
	return s.provider.GetEmailRoutingSettings(ctx, zone.ProviderZoneID)
}

// UpdateRoutingSettings updates email routing settings for a zone.
func (s *Service) UpdateRoutingSettings(ctx context.Context, zoneID string, enabled bool, requestID string) error {
	zone, err := s.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return err
	}

	before := *zone
	if err := s.provider.UpdateEmailRoutingSettings(ctx, zone.ProviderZoneID, provider.RoutingSettingsUpdate{Enabled: enabled}); err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "UPDATE_ROUTING_SETTINGS",
			ResourceType:     "zone",
			ResourceID:       zoneID,
			RequestID:        requestID,
			Before:           before,
			After:            map[string]bool{"enabled": enabled},
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return err
	}

	zone.EmailRoutingEnabled = enabled
	zone.UpdatedAt = time.Now().UTC()
	if err := s.repos.Zones.Save(ctx, zone); err != nil {
		return domain.NewInternalError("failed to save updated zone settings", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:    "UPDATE_ROUTING_SETTINGS",
		ResourceType: "zone",
		ResourceID:   zoneID,
		RequestID:    requestID,
		Before:       before,
		After:        zone,
		Status:       "SUCCESS",
	})

	return nil
}

// --- Explicit Rules ---

// ListRules returns all explicit rules for a zone.
func (s *Service) ListRules(ctx context.Context, zoneID string) ([]domain.RoutingRule, error) {
	return s.repos.Rules.ListByZone(ctx, zoneID)
}

// GetRule returns a specific rule by ID.
func (s *Service) GetRule(ctx context.Context, ruleID string) (*domain.RoutingRule, error) {
	return s.repos.Rules.Get(ctx, ruleID)
}

// CreateRuleRequest defines params for creating a rule.
type CreateRuleRequest struct {
	Name         string `json:"name"`
	MatcherType  string `json:"matcher_type"`
	MatcherField string `json:"matcher_field"`
	MatcherValue string `json:"matcher_value"`
	ActionType   string `json:"action_type"`
	Destination  string `json:"destination"`
	Enabled      bool   `json:"enabled"`
}

// CreateRule validates destination verification, creates rule at provider, and persists locally.
func (s *Service) CreateRule(ctx context.Context, zoneID string, req CreateRuleRequest, requestID string) (*domain.RoutingRule, error) {
	zone, err := s.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return nil, err
	}

	if req.MatcherType == "" {
		req.MatcherType = domain.MatcherTypeLiteral
	}
	if req.MatcherField == "" {
		req.MatcherField = domain.MatcherFieldTo
	}
	if req.ActionType == "" {
		req.ActionType = domain.ActionTypeForward
	}

	req.MatcherValue = strings.TrimSpace(strings.ToLower(req.MatcherValue))
	req.Destination = strings.TrimSpace(strings.ToLower(req.Destination))

	// Validate matcher
	if req.MatcherValue == "" {
		return nil, domain.NewValidationError("matcher_value is required")
	}

	// Validate action and verified destination
	if req.ActionType == domain.ActionTypeForward {
		if req.Destination == "" {
			return nil, domain.NewValidationError("destination is required for forward action")
		}
		verified, err := s.destService.VerifyDestination(ctx, zone.ProviderAccountID, req.Destination)
		if err != nil || !verified {
			return nil, domain.NewDestinationNotVerifiedError(req.Destination)
		}
	}

	// Call provider
	provRule, err := s.provider.CreateRule(ctx, zone.ProviderZoneID, provider.CreateRoutingRule{
		Name:         req.Name,
		MatcherType:  req.MatcherType,
		MatcherField: req.MatcherField,
		MatcherValue: req.MatcherValue,
		ActionType:   req.ActionType,
		Destination:  req.Destination,
		Enabled:      req.Enabled,
	})
	if err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "CREATE_RULE",
			ResourceType:     "rule",
			ResourceID:       req.Name,
			RequestID:        requestID,
			After:            req,
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return nil, err
	}

	now := time.Now().UTC()
	rule := &domain.RoutingRule{
		ID:             generateID("rul"),
		ZoneID:         zoneID,
		ProviderRuleID: provRule.ProviderRuleID,
		Name:           req.Name,
		MatcherType:    req.MatcherType,
		MatcherField:   req.MatcherField,
		MatcherValue:   req.MatcherValue,
		ActionType:     req.ActionType,
		Destination:    req.Destination,
		Enabled:        req.Enabled,
		Source:         "local",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repos.Rules.Save(ctx, rule); err != nil {
		return nil, domain.NewInternalError("failed to save rule locally", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:        "CREATE_RULE",
		ResourceType:     "rule",
		ResourceID:       rule.ID,
		RequestID:        requestID,
		After:            rule,
		ProviderResponse: provRule,
		Status:           "SUCCESS",
	})

	return rule, nil
}

// UpdateRule updates an existing routing rule.
func (s *Service) UpdateRule(ctx context.Context, zoneID, ruleID string, req CreateRuleRequest, requestID string) (*domain.RoutingRule, error) {
	zone, err := s.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return nil, err
	}

	rule, err := s.repos.Rules.Get(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	if req.MatcherType == "" {
		req.MatcherType = domain.MatcherTypeLiteral
	}
	if req.MatcherField == "" {
		req.MatcherField = domain.MatcherFieldTo
	}
	if req.ActionType == "" {
		req.ActionType = domain.ActionTypeForward
	}

	req.MatcherValue = strings.TrimSpace(strings.ToLower(req.MatcherValue))
	req.Destination = strings.TrimSpace(strings.ToLower(req.Destination))

	if req.ActionType == domain.ActionTypeForward {
		if req.Destination == "" {
			return nil, domain.NewValidationError("destination is required for forward action")
		}
		verified, err := s.destService.VerifyDestination(ctx, zone.ProviderAccountID, req.Destination)
		if err != nil || !verified {
			return nil, domain.NewDestinationNotVerifiedError(req.Destination)
		}
	}

	before := *rule
	provRule, err := s.provider.UpdateRule(ctx, zone.ProviderZoneID, rule.ProviderRuleID, provider.UpdateRoutingRule{
		Name:         req.Name,
		MatcherType:  req.MatcherType,
		MatcherField: req.MatcherField,
		MatcherValue: req.MatcherValue,
		ActionType:   req.ActionType,
		Destination:  req.Destination,
		Enabled:      req.Enabled,
	})
	if err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "UPDATE_RULE",
			ResourceType:     "rule",
			ResourceID:       ruleID,
			RequestID:        requestID,
			Before:           before,
			After:            req,
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return nil, err
	}

	rule.Name = req.Name
	rule.MatcherType = req.MatcherType
	rule.MatcherField = req.MatcherField
	rule.MatcherValue = req.MatcherValue
	rule.ActionType = req.ActionType
	rule.Destination = req.Destination
	rule.Enabled = req.Enabled
	rule.UpdatedAt = time.Now().UTC()

	if err := s.repos.Rules.Save(ctx, rule); err != nil {
		return nil, domain.NewInternalError("failed to update rule locally", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:        "UPDATE_RULE",
		ResourceType:     "rule",
		ResourceID:       rule.ID,
		RequestID:        requestID,
		Before:           before,
		After:            rule,
		ProviderResponse: provRule,
		Status:           "SUCCESS",
	})

	return rule, nil
}

// DeleteRule removes a routing rule from provider and local storage.
func (s *Service) DeleteRule(ctx context.Context, zoneID, ruleID, requestID string) error {
	zone, err := s.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return err
	}

	rule, err := s.repos.Rules.Get(ctx, ruleID)
	if err != nil {
		return err
	}

	if err := s.provider.DeleteRule(ctx, zone.ProviderZoneID, rule.ProviderRuleID); err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "DELETE_RULE",
			ResourceType:     "rule",
			ResourceID:       ruleID,
			RequestID:        requestID,
			Before:           rule,
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return err
	}

	if err := s.repos.Rules.Delete(ctx, ruleID); err != nil {
		return domain.NewInternalError("failed to delete rule locally", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:    "DELETE_RULE",
		ResourceType: "rule",
		ResourceID:   ruleID,
		RequestID:    requestID,
		Before:       rule,
		Status:       "SUCCESS",
	})

	return nil
}

// --- Catch-All (First-Class Dedicated Resource) ---

// CatchAllRequest defines params to configure catch-all.
type CatchAllRequest struct {
	Enabled     bool   `json:"enabled"`
	ActionType  string `json:"action_type"`
	Destination string `json:"destination"`
}

// GetCatchAll retrieves the catch-all configuration for a zone.
func (s *Service) GetCatchAll(ctx context.Context, zoneID string) (*domain.CatchAllRule, error) {
	rule, err := s.repos.CatchAll.GetByZone(ctx, zoneID)
	if err == nil && rule != nil {
		return rule, nil
	}

	// Fallback to provider if not stored yet
	zone, err := s.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return nil, err
	}

	provCatchAll, err := s.provider.GetCatchAll(ctx, zone.ProviderZoneID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ca := &domain.CatchAllRule{
		ID:             generateID("cal"),
		ZoneID:         zoneID,
		ProviderRuleID: provCatchAll.ProviderRuleID,
		ActionType:     provCatchAll.ActionType,
		Destination:    provCatchAll.Destination,
		Enabled:        provCatchAll.Enabled,
		Source:         "provider",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_ = s.repos.CatchAll.Save(ctx, ca)
	return ca, nil
}

// UpdateCatchAll configures the catch-all using Cloudflare's dedicated catch-all endpoint.
func (s *Service) UpdateCatchAll(ctx context.Context, zoneID string, req CatchAllRequest, requestID string) (*domain.CatchAllRule, error) {
	zone, err := s.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return nil, err
	}

	if req.ActionType == "" {
		req.ActionType = domain.ActionTypeForward
	}

	req.Destination = strings.TrimSpace(strings.ToLower(req.Destination))

	if req.Enabled && req.ActionType == domain.ActionTypeForward {
		if req.Destination == "" {
			return nil, domain.NewValidationError("destination is required for enabled forward catch-all")
		}
		verified, err := s.destService.VerifyDestination(ctx, zone.ProviderAccountID, req.Destination)
		if err != nil || !verified {
			return nil, domain.NewDestinationNotVerifiedError(req.Destination)
		}
	}

	before, _ := s.repos.CatchAll.GetByZone(ctx, zoneID)

	provRule, err := s.provider.UpdateCatchAll(ctx, zone.ProviderZoneID, provider.UpdateCatchAll{
		ActionType:  req.ActionType,
		Destination: req.Destination,
		Enabled:     req.Enabled,
	})
	if err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "UPDATE_CATCH_ALL",
			ResourceType:     "catch_all",
			ResourceID:       zoneID,
			RequestID:        requestID,
			Before:           before,
			After:            req,
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return nil, err
	}

	now := time.Now().UTC()
	caID := generateID("cal")
	if before != nil {
		caID = before.ID
	}

	ca := &domain.CatchAllRule{
		ID:             caID,
		ZoneID:         zoneID,
		ProviderRuleID: provRule.ProviderRuleID,
		ActionType:     req.ActionType,
		Destination:    req.Destination,
		Enabled:        req.Enabled,
		Source:         "local",
		LastSyncedAt:   &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repos.CatchAll.Save(ctx, ca); err != nil {
		return nil, domain.NewInternalError("failed to save catch-all locally", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:        "UPDATE_CATCH_ALL",
		ResourceType:     "catch_all",
		ResourceID:       zoneID,
		RequestID:        requestID,
		Before:           before,
		After:            ca,
		ProviderResponse: provRule,
		Status:           "SUCCESS",
	})

	return ca, nil
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
