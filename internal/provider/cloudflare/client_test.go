package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bariskode/email-management-service/internal/provider"
)

func TestCloudflareClient(t *testing.T) {
	mux := http.NewServeMux()

	// 1. Zones
	mux.HandleFunc("GET /zones", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[[]cfZone]{
			Success: true,
			Result: []cfZone{
				{ID: "zone_123", Name: "bariskode.com", Status: "active"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 2. Email routing settings
	mux.HandleFunc("GET /zones/zone_123/email/routing", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[cfRoutingSettings]{
			Success: true,
			Result: cfRoutingSettings{
				Enabled: true,
				Status:  "ready",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("PUT /zones/zone_123/email/routing", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[cfRoutingSettings]{
			Success: true,
			Result: cfRoutingSettings{
				Enabled: true,
				Status:  "ready",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 3. Destination Addresses
	mux.HandleFunc("GET /accounts/acc_123/email/routing/addresses", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[[]cfDestinationAddress]{
			Success: true,
			Result: []cfDestinationAddress{
				{ID: "addr_1", Email: "inbox@gmail.com"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /accounts/acc_123/email/routing/addresses", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[cfDestinationAddress]{
			Success: true,
			Result: cfDestinationAddress{
				ID:    "addr_2",
				Email: "ops@gmail.com",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("DELETE /accounts/acc_123/email/routing/addresses/addr_2", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[map[string]any]{
			Success: true,
			Result:  map[string]any{"id": "addr_2"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 4. Explicit Rules
	mux.HandleFunc("GET /zones/zone_123/email/routing/rules", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[[]cfRule]{
			Success: true,
			Result: []cfRule{
				{
					ID:      "rule_1",
					Name:    "Support Rule",
					Enabled: true,
					Matchers: []cfMatcher{
						{Type: "literal", Field: "to", Value: "support@bariskode.com"},
					},
					Actions: []cfAction{
						{Type: "forward", Value: []string{"ops@gmail.com"}},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /zones/zone_123/email/routing/rules", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[cfRule]{
			Success: true,
			Result: cfRule{
				ID:      "rule_new",
				Name:    "Sales",
				Enabled: true,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("DELETE /zones/zone_123/email/routing/rules/rule_new", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[map[string]any]{
			Success: true,
			Result:  map[string]any{"id": "rule_new"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 5. Catch-All Dedicated Endpoint
	mux.HandleFunc("GET /zones/zone_123/email/routing/rules/catch_all", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[cfCatchAllRule]{
			Success: true,
			Result: cfCatchAllRule{
				ID:      "catch_1",
				Name:    "Catch-all",
				Enabled: true,
				Matchers: []cfMatcher{
					{Type: "all"},
				},
				Actions: []cfAction{
					{Type: "forward", Value: []string{"inbox@gmail.com"}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("PUT /zones/zone_123/email/routing/rules/catch_all", func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse[cfCatchAllRule]{
			Success: true,
			Result: cfCatchAllRule{
				ID:      "catch_1",
				Enabled: true,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("mock_token", WithBaseURL(server.URL))
	ctx := context.Background()

	// Test Zones
	zones, err := client.ListZones(ctx)
	if err != nil || len(zones) != 1 || zones[0].Name != "bariskode.com" {
		t.Fatalf("ListZones failed: %v", err)
	}

	// Test Routing Settings
	settings, err := client.GetEmailRoutingSettings(ctx, "zone_123")
	if err != nil || !settings.Enabled {
		t.Fatalf("GetEmailRoutingSettings failed: %v", err)
	}

	err = client.UpdateEmailRoutingSettings(ctx, "zone_123", provider.RoutingSettingsUpdate{Enabled: true})
	if err != nil {
		t.Fatalf("UpdateEmailRoutingSettings failed: %v", err)
	}

	// Test Destinations
	dests, err := client.ListDestinationAddresses(ctx, "acc_123")
	if err != nil || len(dests) != 1 {
		t.Fatalf("ListDestinationAddresses failed: %v", err)
	}

	newDest, err := client.CreateDestinationAddress(ctx, "acc_123", "ops@gmail.com")
	if err != nil || newDest.Email != "ops@gmail.com" {
		t.Fatalf("CreateDestinationAddress failed: %v", err)
	}

	err = client.DeleteDestinationAddress(ctx, "acc_123", "addr_2")
	if err != nil {
		t.Fatalf("DeleteDestinationAddress failed: %v", err)
	}

	// Test Rules
	rules, err := client.ListRules(ctx, "zone_123")
	if err != nil || len(rules) != 1 || rules[0].MatcherValue != "support@bariskode.com" {
		t.Fatalf("ListRules failed: %v", err)
	}

	newRule, err := client.CreateRule(ctx, "zone_123", provider.CreateRoutingRule{
		Name:         "Sales",
		MatcherType:  "literal",
		MatcherField: "to",
		MatcherValue: "sales@bariskode.com",
		ActionType:   "forward",
		Destination:  "sales@gmail.com",
		Enabled:      true,
	})
	if err != nil || newRule.ProviderRuleID != "rule_new" {
		t.Fatalf("CreateRule failed: %v", err)
	}

	err = client.DeleteRule(ctx, "zone_123", "rule_new")
	if err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	// Test Dedicated Catch-All Endpoint
	ca, err := client.GetCatchAll(ctx, "zone_123")
	if err != nil || ca.Destination != "inbox@gmail.com" || !ca.Enabled {
		t.Fatalf("GetCatchAll failed: %v", err)
	}

	updatedCA, err := client.UpdateCatchAll(ctx, "zone_123", provider.UpdateCatchAll{
		ActionType:  "forward",
		Destination: "new_inbox@gmail.com",
		Enabled:     true,
	})
	if err != nil || !updatedCA.Enabled {
		t.Fatalf("UpdateCatchAll failed: %v", err)
	}
}

func TestCloudflareClient_ErrorHandling(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /zones", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := cfResponse[any]{
			Success: false,
			Errors: []cfError{
				{Code: 1001, Message: "Invalid authentication"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("bad_token", WithBaseURL(server.URL))
	_, err := client.ListZones(context.Background())
	if err == nil {
		t.Fatalf("expected error from failed request, got nil")
	}
}
