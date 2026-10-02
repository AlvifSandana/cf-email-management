package routing

import (
	"context"
	"testing"
	"time"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/destination"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider/mock"
	"github.com/bariskode/email-management-service/internal/storage/memory"
)

func TestRoutingService_ExplicitRulesAndCatchAll(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	prov := mock.New()
	auditService := audit.NewService(repos.Audit)
	destService := destination.NewService(repos.Destinations, prov, auditService)
	routeService := NewService(repos, prov, destService, auditService)

	accountID := "acc_123"
	providerZoneID := "cf_zone_test"

	// Setup remote zone in mock provider
	prov.Zones[providerZoneID] = domain.Zone{
		ProviderZoneID: providerZoneID,
		Name:           "bariskode.com",
		Status:         "active",
	}

	// 1. Import Zone
	zone, err := routeService.ImportZone(ctx, accountID, providerZoneID, "req_import")
	if err != nil {
		t.Fatalf("failed to import zone: %v", err)
	}
	if zone.Name != "bariskode.com" {
		t.Fatalf("expected zone name bariskode.com, got %s", zone.Name)
	}

	// 2. Update Email Routing Settings
	err = routeService.UpdateRoutingSettings(ctx, zone.ID, true, "req_routing")
	if err != nil {
		t.Fatalf("failed to update routing settings: %v", err)
	}

	// 3. Rule creation with UNVERIFIED destination should fail
	_, err = routeService.CreateRule(ctx, zone.ID, CreateRuleRequest{
		Name:         "Support",
		MatcherValue: "support@bariskode.com",
		ActionType:   "forward",
		Destination:  "unverified@gmail.com",
		Enabled:      true,
	}, "req_rule_fail")
	if err == nil {
		t.Fatalf("expected rule creation to fail with unverified destination")
	}

	// Add verified destination
	now := time.Now()
	verifiedDest := &domain.DestinationAddress{
		ID:                "dst_verified",
		ProviderAccountID: accountID,
		ProviderAddressID: "cf_dst_1",
		Email:             "verified@gmail.com",
		Status:            domain.DestinationStatusVerified,
		VerifiedAt:        &now,
	}
	_ = repos.Destinations.Save(ctx, verifiedDest)

	// 4. Rule creation with VERIFIED destination should succeed
	rule, err := routeService.CreateRule(ctx, zone.ID, CreateRuleRequest{
		Name:         "Support",
		MatcherValue: "support@bariskode.com",
		ActionType:   "forward",
		Destination:  "verified@gmail.com",
		Enabled:      true,
	}, "req_rule_success")
	if err != nil {
		t.Fatalf("expected rule creation to succeed with verified destination: %v", err)
	}
	if rule.MatcherValue != "support@bariskode.com" {
		t.Fatalf("expected matcher support@bariskode.com, got %s", rule.MatcherValue)
	}

	// 5. Update Rule
	updatedRule, err := routeService.UpdateRule(ctx, zone.ID, rule.ID, CreateRuleRequest{
		Name:         "Support Updated",
		MatcherValue: "help@bariskode.com",
		ActionType:   "forward",
		Destination:  "verified@gmail.com",
		Enabled:      true,
	}, "req_rule_update")
	if err != nil {
		t.Fatalf("failed to update rule: %v", err)
	}
	if updatedRule.Name != "Support Updated" {
		t.Fatalf("expected Support Updated, got %s", updatedRule.Name)
	}

	// 6. Catch-All dedicated resource test
	// Catch-all with unverified forward destination should fail when enabled
	_, err = routeService.UpdateCatchAll(ctx, zone.ID, CatchAllRequest{
		Enabled:     true,
		ActionType:  "forward",
		Destination: "unverified@gmail.com",
	}, "req_ca_fail")
	if err == nil {
		t.Fatalf("expected catch-all to fail with unverified destination")
	}

	// Catch-all with drop action should succeed without destination
	caDrop, err := routeService.UpdateCatchAll(ctx, zone.ID, CatchAllRequest{
		Enabled:    true,
		ActionType: "drop",
	}, "req_ca_drop")
	if err != nil {
		t.Fatalf("failed to set catch-all to drop: %v", err)
	}
	if caDrop.ActionType != "drop" {
		t.Fatalf("expected drop action, got %s", caDrop.ActionType)
	}

	// Catch-all with verified forward destination should succeed
	caForward, err := routeService.UpdateCatchAll(ctx, zone.ID, CatchAllRequest{
		Enabled:     true,
		ActionType:  "forward",
		Destination: "verified@gmail.com",
	}, "req_ca_forward")
	if err != nil {
		t.Fatalf("failed to set catch-all to forward: %v", err)
	}
	if caForward.Destination != "verified@gmail.com" {
		t.Fatalf("expected verified@gmail.com, got %s", caForward.Destination)
	}

	// Get Catch-All
	gotCA, err := routeService.GetCatchAll(ctx, zone.ID)
	if err != nil || gotCA.Destination != "verified@gmail.com" {
		t.Fatalf("failed to get catch-all: %v", err)
	}

	// 7. Delete Rule
	err = routeService.DeleteRule(ctx, zone.ID, rule.ID, "req_rule_del")
	if err != nil {
		t.Fatalf("failed to delete rule: %v", err)
	}
}
