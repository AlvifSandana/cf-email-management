package sync

import (
	"context"
	"sync"
	"testing"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
	"github.com/bariskode/email-management-service/internal/provider/mock"
	"github.com/bariskode/email-management-service/internal/storage/memory"
)

func TestSyncEngine_DriftDetectionAndReconciliation(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	prov := mock.New()
	auditService := audit.NewService(repos.Audit)
	engine := NewEngine(repos, prov, auditService)

	zoneID := "zon_test"
	providerZoneID := "cf_zone_test"

	// 1. Setup local zone
	zone := &domain.Zone{
		ID:                  zoneID,
		ProviderAccountID:   "acc_test",
		ProviderZoneID:      providerZoneID,
		Name:                "bariskode.com",
		Status:              "active",
		EmailRoutingEnabled: true,
	}
	_ = repos.Zones.Save(ctx, zone)

	// Local rule
	localRule := &domain.RoutingRule{
		ID:             "rul_local_1",
		ZoneID:         zoneID,
		ProviderRuleID: "cf_rul_1",
		Name:           "Support",
		MatcherType:    "literal",
		MatcherField:   "to",
		MatcherValue:   "support@bariskode.com",
		ActionType:     "forward",
		Destination:    "support@gmail.com",
		Enabled:        true,
	}
	_ = repos.Rules.Save(ctx, localRule)

	// Setup remote in mock provider
	// Provider settings enabled
	prov.Settings[providerZoneID] = provider.RoutingSettings{Enabled: true, Status: "ready"}
	// Remote has support@bariskode.com -> ops@gmail.com (CHANGED destination)
	prov.Rules[providerZoneID] = map[string]domain.RoutingRule{
		"cf_rul_1": {
			ProviderRuleID: "cf_rul_1",
			Name:           "Support",
			MatcherType:    "literal",
			MatcherField:   "to",
			MatcherValue:   "support@bariskode.com",
			ActionType:     "forward",
			Destination:    "ops@gmail.com", // Differs from local!
			Enabled:        true,
		},
		"cf_rul_2": {
			ProviderRuleID: "cf_rul_2",
			Name:           "Info",
			MatcherType:    "literal",
			MatcherField:   "to",
			MatcherValue:   "info@bariskode.com",
			ActionType:     "forward",
			Destination:    "info@gmail.com", // REMOTE_ONLY
			Enabled:        true,
		},
	}

	// 2. Run drift_check
	res, err := engine.SyncZone(ctx, zoneID, "drift_check", "req_sync_1")
	if err != nil {
		t.Fatalf("SyncZone failed: %v", err)
	}

	foundChanged := false
	foundRemoteOnly := false
	for _, diff := range res.Diffs {
		if diff.Identifier == "support@bariskode.com" && diff.Status == domain.DiffStatusChanged {
			foundChanged = true
		}
		if diff.Identifier == "info@bariskode.com" && diff.Status == domain.DiffStatusRemoteOnly {
			foundRemoteOnly = true
		}
	}

	if !foundChanged {
		t.Errorf("expected support@bariskode.com to have status CHANGED")
	}
	if !foundRemoteOnly {
		t.Errorf("expected info@bariskode.com to have status REMOTE_ONLY")
	}

	// 3. Run pull (Remote -> Local)
	pullRes, err := engine.SyncZone(ctx, zoneID, "pull", "req_sync_2")
	if err != nil {
		t.Fatalf("SyncZone pull failed: %v", err)
	}
	if pullRes.Status != "success" {
		t.Fatalf("expected pull status success, got %s", pullRes.Status)
	}

	// Verify local state updated to remote state
	rulesAfterPull, err := repos.Rules.ListByZone(ctx, zoneID)
	if err != nil || len(rulesAfterPull) != 2 {
		t.Fatalf("expected 2 rules locally after pull, got %d", len(rulesAfterPull))
	}

	for _, r := range rulesAfterPull {
		if r.MatcherValue == "support@bariskode.com" && r.Destination != "ops@gmail.com" {
			t.Errorf("expected support rule destination to be updated to ops@gmail.com")
		}
	}
}

func TestSyncEngine_PerZoneLocking(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	prov := mock.New()
	auditService := audit.NewService(repos.Audit)
	engine := NewEngine(repos, prov, auditService)

	zoneID := "zon_lock_test"
	providerZoneID := "cf_lock_test"

	zone := &domain.Zone{
		ID:             zoneID,
		ProviderZoneID: providerZoneID,
	}
	_ = repos.Zones.Save(ctx, zone)

	// Acquire zone lock manually
	lock := engine.getZoneLock(zoneID)
	lock.Lock()

	// Running sync in another goroutine should fail with ErrZoneSyncLocked
	var wg sync.WaitGroup
	var syncErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, syncErr = engine.SyncZone(ctx, zoneID, "drift_check", "req_concurrent")
	}()

	wg.Wait()
	lock.Unlock()

	if syncErr != domain.ErrZoneSyncLocked {
		t.Fatalf("expected ErrZoneSyncLocked, got %v", syncErr)
	}
}
