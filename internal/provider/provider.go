package provider

import (
	"context"

	"github.com/bariskode/email-management-service/internal/domain"
)

// RoutingSettings describes email routing status for a zone.
type RoutingSettings struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}

// RoutingSettingsUpdate defines payload to enable/disable routing.
type RoutingSettingsUpdate struct {
	Enabled bool `json:"enabled"`
}

// CreateRoutingRule contains parameters to create an explicit routing rule.
type CreateRoutingRule struct {
	Name         string `json:"name"`
	MatcherType  string `json:"matcher_type"`
	MatcherField string `json:"matcher_field"`
	MatcherValue string `json:"matcher_value"`
	ActionType   string `json:"action_type"`
	Destination  string `json:"destination"`
	Enabled      bool   `json:"enabled"`
}

// UpdateRoutingRule contains parameters to update an explicit routing rule.
type UpdateRoutingRule struct {
	Name         string `json:"name"`
	MatcherType  string `json:"matcher_type"`
	MatcherField string `json:"matcher_field"`
	MatcherValue string `json:"matcher_value"`
	ActionType   string `json:"action_type"`
	Destination  string `json:"destination"`
	Enabled      bool   `json:"enabled"`
}

// UpdateCatchAll contains parameters to update catch-all rule.
type UpdateCatchAll struct {
	ActionType  string `json:"action_type"`
	Destination string `json:"destination"`
	Enabled     bool   `json:"enabled"`
}

// EmailProvider defines the standard interface for interacting with email routing providers (Cloudflare, etc.).
type EmailProvider interface {
	ListZones(ctx context.Context) ([]domain.Zone, error)

	GetEmailRoutingSettings(
		ctx context.Context,
		zoneID string,
	) (RoutingSettings, error)

	UpdateEmailRoutingSettings(
		ctx context.Context,
		zoneID string,
		settings RoutingSettingsUpdate,
	) error

	ListDestinationAddresses(
		ctx context.Context,
		accountID string,
	) ([]domain.DestinationAddress, error)

	CreateDestinationAddress(
		ctx context.Context,
		accountID string,
		email string,
	) (domain.DestinationAddress, error)

	UpdateDestinationAddress(
		ctx context.Context,
		accountID, addressID string,
		email string,
	) (domain.DestinationAddress, error)

	DeleteDestinationAddress(
		ctx context.Context,
		accountID, addressID string,
	) error

	ListRules(
		ctx context.Context,
		zoneID string,
	) ([]domain.RoutingRule, error)

	CreateRule(
		ctx context.Context,
		zoneID string,
		rule CreateRoutingRule,
	) (domain.RoutingRule, error)

	UpdateRule(
		ctx context.Context,
		zoneID, ruleID string,
		rule UpdateRoutingRule,
	) (domain.RoutingRule, error)

	DeleteRule(
		ctx context.Context,
		zoneID, ruleID string,
	) error

	GetCatchAll(
		ctx context.Context,
		zoneID string,
	) (domain.CatchAllRule, error)

	UpdateCatchAll(
		ctx context.Context,
		zoneID string,
		rule UpdateCatchAll,
	) (domain.CatchAllRule, error)
}
