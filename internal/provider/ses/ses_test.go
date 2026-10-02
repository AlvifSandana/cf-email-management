package ses_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
	"github.com/bariskode/email-management-service/internal/provider/ses"
)

// Ensure Provider implements provider.EmailProvider at compile time.
var _ provider.EmailProvider = (*ses.Provider)(nil)

func TestSESProvider_ListZones(t *testing.T) {
	ctx := context.Background()
	inMem := ses.NewInMemoryClient()
	p := ses.New(ses.WithClient(inMem))

	// 1. When no domain identities exist
	zones, err := p.ListZones(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(zones) != 0 {
		t.Fatalf("expected 0 zones, got %d", len(zones))
	}

	// 2. Add domain identities with different statuses
	inMem.AddDomain("bariskode.com", "Success")
	inMem.AddDomain("pending.com", "Pending")

	zones, err = p.ListZones(ctx)
	if err != nil {
		t.Fatalf("ListZones failed: %v", err)
	}
	if len(zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(zones))
	}

	zoneMap := make(map[string]domain.Zone)
	for _, z := range zones {
		zoneMap[z.Name] = z
	}

	bkZone, ok := zoneMap["bariskode.com"]
	if !ok {
		t.Fatalf("missing bariskode.com zone")
	}
	if bkZone.Status != "active" {
		t.Errorf("expected status 'active', got '%s'", bkZone.Status)
	}
	if !bkZone.EmailRoutingEnabled {
		t.Errorf("expected EmailRoutingEnabled to be true (active rule set exists)")
	}

	pZone, ok := zoneMap["pending.com"]
	if !ok {
		t.Fatalf("missing pending.com zone")
	}
	if pZone.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", pZone.Status)
	}
}

func TestSESProvider_EmailRoutingSettings(t *testing.T) {
	ctx := context.Background()
	inMem := ses.NewInMemoryClient()
	p := ses.New(ses.WithClient(inMem))

	// Initially active rule set is configured
	settings, err := p.GetEmailRoutingSettings(ctx, "bariskode.com")
	if err != nil {
		t.Fatalf("GetEmailRoutingSettings failed: %v", err)
	}
	if !settings.Enabled || settings.Status != "active" {
		t.Errorf("expected enabled=true, status='active', got enabled=%v status=%s", settings.Enabled, settings.Status)
	}

	// Deactivate routing
	err = p.UpdateEmailRoutingSettings(ctx, "bariskode.com", provider.RoutingSettingsUpdate{Enabled: false})
	if err != nil {
		t.Fatalf("UpdateEmailRoutingSettings failed: %v", err)
	}

	settings, err = p.GetEmailRoutingSettings(ctx, "bariskode.com")
	if err != nil {
		t.Fatalf("GetEmailRoutingSettings failed: %v", err)
	}
	if settings.Enabled || settings.Status != "inactive" {
		t.Errorf("expected enabled=false, status='inactive', got enabled=%v status=%s", settings.Enabled, settings.Status)
	}

	// Re-activate routing
	err = p.UpdateEmailRoutingSettings(ctx, "bariskode.com", provider.RoutingSettingsUpdate{Enabled: true})
	if err != nil {
		t.Fatalf("UpdateEmailRoutingSettings failed: %v", err)
	}

	settings, err = p.GetEmailRoutingSettings(ctx, "bariskode.com")
	if err != nil {
		t.Fatalf("GetEmailRoutingSettings failed: %v", err)
	}
	if !settings.Enabled || settings.Status != "active" {
		t.Errorf("expected enabled=true, status='active', got enabled=%v status=%s", settings.Enabled, settings.Status)
	}
}

func TestSESProvider_DestinationAddresses(t *testing.T) {
	ctx := context.Background()
	inMem := ses.NewInMemoryClient()
	p := ses.New(ses.WithClient(inMem))
	accountID := "acc_ses_123"

	// 1. Initial list should be empty
	dests, err := p.ListDestinationAddresses(ctx, accountID)
	if err != nil {
		t.Fatalf("ListDestinationAddresses failed: %v", err)
	}
	if len(dests) != 0 {
		t.Fatalf("expected 0 destinations, got %d", len(dests))
	}

	// 2. Create destination address
	created, err := p.CreateDestinationAddress(ctx, accountID, "forward@example.com")
	if err != nil {
		t.Fatalf("CreateDestinationAddress failed: %v", err)
	}
	if created.Email != "forward@example.com" {
		t.Errorf("expected email 'forward@example.com', got '%s'", created.Email)
	}
	if created.Status != domain.DestinationStatusPending {
		t.Errorf("expected initial status 'pending', got '%s'", created.Status)
	}

	// 3. Mark as verified in backend
	inMem.SetVerified("forward@example.com", true)

	dests, err = p.ListDestinationAddresses(ctx, accountID)
	if err != nil {
		t.Fatalf("ListDestinationAddresses failed: %v", err)
	}
	if len(dests) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(dests))
	}
	if dests[0].Status != domain.DestinationStatusVerified || dests[0].VerifiedAt == nil {
		t.Errorf("expected status 'verified' with verifiedAt timestamp, got status=%s verifiedAt=%v", dests[0].Status, dests[0].VerifiedAt)
	}
	if !dests[0].IsVerified() {
		t.Errorf("expected IsVerified() to be true")
	}

	// 4. Update destination address with new email
	updated, err := p.UpdateDestinationAddress(ctx, accountID, "forward@example.com", "new_forward@example.com")
	if err != nil {
		t.Fatalf("UpdateDestinationAddress failed: %v", err)
	}
	if updated.Email != "new_forward@example.com" {
		t.Errorf("expected updated email 'new_forward@example.com', got '%s'", updated.Email)
	}

	// 5. Update non-existent address
	_, err = p.UpdateDestinationAddress(ctx, accountID, "nonexistent@example.com", "other@example.com")
	if err == nil {
		t.Fatalf("expected error updating non-existent address, got nil")
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) && appErr.Code != domain.ErrCodeNotFound {
		t.Errorf("expected ErrCodeNotFound, got %s", appErr.Code)
	}

	// 6. Delete destination address
	err = p.DeleteDestinationAddress(ctx, accountID, "new_forward@example.com")
	if err != nil {
		t.Fatalf("DeleteDestinationAddress failed: %v", err)
	}

	dests, err = p.ListDestinationAddresses(ctx, accountID)
	if err != nil {
		t.Fatalf("ListDestinationAddresses failed: %v", err)
	}
	if len(dests) != 0 {
		t.Errorf("expected 0 destinations after delete, got %d", len(dests))
	}

	// 7. Delete non-existent address
	err = p.DeleteDestinationAddress(ctx, accountID, "new_forward@example.com")
	if err == nil {
		t.Fatalf("expected error deleting non-existent address, got nil")
	}
}

func TestSESProvider_Rules(t *testing.T) {
	ctx := context.Background()
	inMem := ses.NewInMemoryClient()
	p := ses.New(ses.WithClient(inMem))
	zoneID := "bariskode.com"

	// 1. Initial list empty
	rules, err := p.ListRules(ctx, zoneID)
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(rules))
	}

	// 2. Create first rule
	rule1, err := p.CreateRule(ctx, zoneID, provider.CreateRoutingRule{
		Name:         "support-rule",
		MatcherType:  domain.MatcherTypeLiteral,
		MatcherField: domain.MatcherFieldTo,
		MatcherValue: "support@bariskode.com",
		ActionType:   domain.ActionTypeForward,
		Destination:  "ops@gmail.com",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}
	if rule1.ProviderRuleID != "support-rule" || rule1.MatcherValue != "support@bariskode.com" {
		t.Errorf("unexpected created rule: %+v", rule1)
	}

	// 3. Create second rule for bariskode.com
	_, err = p.CreateRule(ctx, zoneID, provider.CreateRoutingRule{
		Name:         "sales-rule",
		MatcherValue: "sales@bariskode.com",
		ActionType:   domain.ActionTypeForward,
		Destination:  "sales@gmail.com",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("CreateRule 2 failed: %v", err)
	}

	// 4. Create rule for a different zone (example.org)
	_, err = p.CreateRule(ctx, "example.org", provider.CreateRoutingRule{
		Name:         "other-rule",
		MatcherValue: "info@example.org",
		ActionType:   domain.ActionTypeForward,
		Destination:  "admin@example.org",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("CreateRule other zone failed: %v", err)
	}

	// 5. ListRules for bariskode.com - should return 2 rules, not 3
	rules, err = p.ListRules(ctx, zoneID)
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules for zone bariskode.com, got %d", len(rules))
	}

	// 6. Update rule
	updated, err := p.UpdateRule(ctx, zoneID, "support-rule", provider.UpdateRoutingRule{
		Name:         "support-rule-updated",
		MatcherValue: "support@bariskode.com",
		ActionType:   domain.ActionTypeForward,
		Destination:  "new_ops@gmail.com",
		Enabled:      false,
	})
	if err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	if updated.Destination != "new_ops@gmail.com" || updated.Enabled != false {
		t.Errorf("unexpected updated rule: %+v", updated)
	}

	// 7. Update non-existent rule
	_, err = p.UpdateRule(ctx, zoneID, "ghost-rule", provider.UpdateRoutingRule{
		MatcherValue: "ghost@bariskode.com",
	})
	if err == nil {
		t.Fatalf("expected error updating non-existent rule, got nil")
	}

	// 8. Delete rule
	err = p.DeleteRule(ctx, zoneID, "support-rule")
	if err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	rules, err = p.ListRules(ctx, zoneID)
	if err != nil {
		t.Fatalf("ListRules after delete failed: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule remaining, got %d", len(rules))
	}

	// 9. Delete non-existent rule
	err = p.DeleteRule(ctx, zoneID, "support-rule")
	if err == nil {
		t.Fatalf("expected error deleting non-existent rule, got nil")
	}
}

func TestSESProvider_CatchAll(t *testing.T) {
	ctx := context.Background()
	inMem := ses.NewInMemoryClient()
	p := ses.New(ses.WithClient(inMem))
	zoneID := "bariskode.com"

	// 1. Initial catch-all is unconfigured/disabled drop rule
	ca, err := p.GetCatchAll(ctx, zoneID)
	if err != nil {
		t.Fatalf("GetCatchAll failed: %v", err)
	}
	if ca.Enabled || ca.ActionType != domain.ActionTypeDrop {
		t.Errorf("expected disabled drop rule, got enabled=%v action=%s", ca.Enabled, ca.ActionType)
	}

	// 2. Configure / Update catch-all
	updatedCA, err := p.UpdateCatchAll(ctx, zoneID, provider.UpdateCatchAll{
		ActionType:  domain.ActionTypeForward,
		Destination: "catchall@gmail.com",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("UpdateCatchAll failed: %v", err)
	}
	if !updatedCA.Enabled || updatedCA.Destination != "catchall@gmail.com" {
		t.Errorf("unexpected catch-all after update: %+v", updatedCA)
	}

	// 3. GetCatchAll returns configured rule
	ca, err = p.GetCatchAll(ctx, zoneID)
	if err != nil {
		t.Fatalf("GetCatchAll after update failed: %v", err)
	}
	if !ca.Enabled || ca.Destination != "catchall@gmail.com" || ca.ActionType != domain.ActionTypeForward {
		t.Errorf("unexpected catch-all: %+v", ca)
	}

	// 4. Catch-all rule MUST NOT appear in ListRules
	rules, err := p.ListRules(ctx, zoneID)
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected catch-all rule to be excluded from ListRules, but found %d rules", len(rules))
	}

	// 5. Update catch-all again (e.g. modify destination or disable)
	updatedCA2, err := p.UpdateCatchAll(ctx, zoneID, provider.UpdateCatchAll{
		ActionType:  domain.ActionTypeDrop,
		Destination: "",
		Enabled:     false,
	})
	if err != nil {
		t.Fatalf("UpdateCatchAll 2 failed: %v", err)
	}
	if updatedCA2.Enabled || updatedCA2.ActionType != domain.ActionTypeDrop {
		t.Errorf("expected disabled drop catch-all, got: %+v", updatedCA2)
	}

	// 6. Test multiple zones catch-all isolation
	otherZone := "example.org"
	_, err = p.UpdateCatchAll(ctx, otherZone, provider.UpdateCatchAll{
		ActionType:  domain.ActionTypeForward,
		Destination: "all@example.org",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("UpdateCatchAll other zone failed: %v", err)
	}

	otherCA, err := p.GetCatchAll(ctx, otherZone)
	if err != nil {
		t.Fatalf("GetCatchAll other zone failed: %v", err)
	}
	if !otherCA.Enabled || otherCA.Destination != "all@example.org" {
		t.Errorf("unexpected other zone catch-all: %+v", otherCA)
	}

	// Bariskode catch-all should remain disabled
	ca, err = p.GetCatchAll(ctx, zoneID)
	if err != nil {
		t.Fatalf("GetCatchAll bariskode failed: %v", err)
	}
	if ca.Enabled {
		t.Errorf("bariskode catch-all was affected by example.org")
	}
}

func TestSESProvider_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	inMem := ses.NewInMemoryClient()
	p := ses.New(ses.WithClient(inMem))

	simulatedErr := errors.New("AWS SES rate limit exceeded")
	inMem.SetSimulateError(simulatedErr)

	// ListZones error
	_, err := p.ListZones(ctx)
	if err == nil {
		t.Fatalf("expected error from ListZones, got nil")
	}

	// GetEmailRoutingSettings error
	_, err = p.GetEmailRoutingSettings(ctx, "bariskode.com")
	if err == nil {
		t.Fatalf("expected error from GetEmailRoutingSettings, got nil")
	}

	// UpdateEmailRoutingSettings error
	err = p.UpdateEmailRoutingSettings(ctx, "bariskode.com", provider.RoutingSettingsUpdate{Enabled: true})
	if err == nil {
		t.Fatalf("expected error from UpdateEmailRoutingSettings, got nil")
	}

	// ListDestinationAddresses error
	_, err = p.ListDestinationAddresses(ctx, "acc_1")
	if err == nil {
		t.Fatalf("expected error from ListDestinationAddresses, got nil")
	}

	// CreateDestinationAddress error
	_, err = p.CreateDestinationAddress(ctx, "acc_1", "test@example.com")
	if err == nil {
		t.Fatalf("expected error from CreateDestinationAddress, got nil")
	}

	// ListRules error
	_, err = p.ListRules(ctx, "bariskode.com")
	if err == nil {
		t.Fatalf("expected error from ListRules, got nil")
	}

	// CreateRule error
	_, err = p.CreateRule(ctx, "bariskode.com", provider.CreateRoutingRule{Name: "r1"})
	if err == nil {
		t.Fatalf("expected error from CreateRule, got nil")
	}

	// GetCatchAll error
	_, err = p.GetCatchAll(ctx, "bariskode.com")
	if err == nil {
		t.Fatalf("expected error from GetCatchAll, got nil")
	}

	// UpdateCatchAll error
	_, err = p.UpdateCatchAll(ctx, "bariskode.com", provider.UpdateCatchAll{})
	if err == nil {
		t.Fatalf("expected error from UpdateCatchAll, got nil")
	}
}

func TestSESProvider_CustomOptions(t *testing.T) {
	customSet := "custom-rule-set"
	p := ses.New(
		ses.WithRuleSetName(customSet),
	)

	if p.RuleSetName() != customSet {
		t.Errorf("expected rule set name '%s', got '%s'", customSet, p.RuleSetName())
	}
	if p.Client() == nil {
		t.Errorf("expected non-nil default client")
	}
}

func TestSESProvider_Concurrency(t *testing.T) {
	ctx := context.Background()
	p := ses.New()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)

		// Concurrent destination creations
		go func(idx int) {
			defer wg.Done()
			_, _ = p.CreateDestinationAddress(ctx, "acc_conc", "user@bariskode.com")
		}(i)

		// Concurrent rule creations
		go func(idx int) {
			defer wg.Done()
			_, _ = p.CreateRule(ctx, "bariskode.com", provider.CreateRoutingRule{
				Name:         "conc-rule",
				MatcherValue: "conc@bariskode.com",
				ActionType:   domain.ActionTypeForward,
				Destination:  "ops@gmail.com",
				Enabled:      true,
			})
		}(i)

		// Concurrent settings reads
		go func(idx int) {
			defer wg.Done()
			_, _ = p.GetEmailRoutingSettings(ctx, "bariskode.com")
		}(i)
	}

	wg.Wait()
}
