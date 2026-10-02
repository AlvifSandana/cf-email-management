package memory

import (
	"context"
	"testing"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
)

func TestMemoryStorage_CRUD(t *testing.T) {
	ctx := context.Background()
	repos := New()

	// 1. Account
	acc := &domain.CloudflareAccount{
		ID:                  "acc_1",
		Name:                "Test Account",
		CloudflareAccountID: "cf_acc_123",
		Status:              "active",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
	if err := repos.Accounts.Save(ctx, acc); err != nil {
		t.Fatalf("failed to save account: %v", err)
	}

	gotAcc, err := repos.Accounts.Get(ctx, "acc_1")
	if err != nil || gotAcc.Name != "Test Account" {
		t.Fatalf("failed to get account: %v", err)
	}

	// 2. Zone
	zone := &domain.Zone{
		ID:                  "zon_1",
		ProviderAccountID:   "cf_acc_123",
		ProviderZoneID:      "cf_zone_123",
		Name:                "bariskode.com",
		Status:              "active",
		EmailRoutingEnabled: true,
	}
	if err := repos.Zones.Save(ctx, zone); err != nil {
		t.Fatalf("failed to save zone: %v", err)
	}

	gotZone, err := repos.Zones.GetByProviderZoneID(ctx, "cf_zone_123")
	if err != nil || gotZone.Name != "bariskode.com" {
		t.Fatalf("failed to get zone by provider ID: %v", err)
	}

	// 3. Destination
	dest := &domain.DestinationAddress{
		ID:                "dst_1",
		ProviderAccountID: "cf_acc_123",
		ProviderAddressID: "cf_addr_1",
		Email:             "ops@example.com",
		Status:            domain.DestinationStatusVerified,
	}
	if err := repos.Destinations.Save(ctx, dest); err != nil {
		t.Fatalf("failed to save destination: %v", err)
	}

	gotDest, err := repos.Destinations.GetByEmail(ctx, "cf_acc_123", "ops@example.com")
	if err != nil || gotDest.ID != "dst_1" {
		t.Fatalf("failed to get destination by email: %v", err)
	}

	// 4. Rule
	rule := &domain.RoutingRule{
		ID:             "rul_1",
		ZoneID:         "zon_1",
		ProviderRuleID: "cf_rul_1",
		Name:           "Support",
		MatcherType:    "literal",
		MatcherField:   "to",
		MatcherValue:   "support@bariskode.com",
		ActionType:     "forward",
		Destination:    "ops@example.com",
		Enabled:        true,
	}
	if err := repos.Rules.Save(ctx, rule); err != nil {
		t.Fatalf("failed to save rule: %v", err)
	}

	rules, err := repos.Rules.ListByZone(ctx, "zon_1")
	if err != nil || len(rules) != 1 {
		t.Fatalf("failed to list rules: %v", err)
	}

	// 5. Catch-All
	ca := &domain.CatchAllRule{
		ID:             "cal_1",
		ZoneID:         "zon_1",
		ProviderRuleID: "cf_cal_1",
		ActionType:     "forward",
		Destination:    "ops@example.com",
		Enabled:        true,
	}
	if err := repos.CatchAll.Save(ctx, ca); err != nil {
		t.Fatalf("failed to save catch-all: %v", err)
	}

	gotCA, err := repos.CatchAll.GetByZone(ctx, "zon_1")
	if err != nil || gotCA.Destination != "ops@example.com" {
		t.Fatalf("failed to get catch-all: %v", err)
	}

	// 6. Idempotency
	rec := &domain.IdempotencyRecord{
		Key:        "actor:GET:/test:key1",
		StatusCode: 200,
		Body:       []byte("cached-data"),
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	if err := repos.Idempotency.Save(ctx, rec); err != nil {
		t.Fatalf("failed to save idempotency record: %v", err)
	}

	gotRec, err := repos.Idempotency.Get(ctx, "actor:GET:/test:key1")
	if err != nil || string(gotRec.Body) != "cached-data" {
		t.Fatalf("failed to get idempotency record: %v", err)
	}
}
