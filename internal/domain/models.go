package domain

import (
	"time"
)

// SyncStatus enum
const (
	DiffStatusMatched    = "MATCHED"
	DiffStatusLocalOnly  = "LOCAL_ONLY"
	DiffStatusRemoteOnly = "REMOTE_ONLY"
	DiffStatusChanged    = "CHANGED"
	DiffStatusUnknown    = "UNKNOWN"
)

// Action types
const (
	ActionTypeForward = "forward"
	ActionTypeDrop    = "drop"
	ActionTypeWorker  = "worker"
)

// Matcher types and fields
const (
	MatcherTypeLiteral = "literal"
	MatcherTypeAll     = "all"
	MatcherFieldTo     = "to"
)

// Destination status
const (
	DestinationStatusVerified   = "verified"
	DestinationStatusPending    = "pending"
	DestinationStatusUnverified = "unverified"
)

// CloudflareAccount represents a Cloudflare account credential configuration.
type CloudflareAccount struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	CloudflareAccountID string    `json:"cloudflare_account_id"`
	CredentialRef       string    `json:"-"` // Encrypted token stored at rest, omitted from JSON
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Zone represents a domain zone managed in Cloudflare.
type Zone struct {
	ID                  string     `json:"id"`
	ProviderAccountID   string     `json:"provider_account_id"`
	ProviderZoneID      string     `json:"provider_zone_id"`
	Name                string     `json:"name"`
	Status              string     `json:"status"`
	EmailRoutingEnabled bool       `json:"email_routing_enabled"`
	LastSyncedAt        *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// DestinationAddress represents a verified forwarding destination.
type DestinationAddress struct {
	ID                string     `json:"id"`
	ProviderAccountID string     `json:"provider_account_id"`
	ProviderAddressID string     `json:"provider_address_id"`
	Email             string     `json:"email"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// IsVerified checks if destination is verified.
func (d *DestinationAddress) IsVerified() bool {
	return d.Status == DestinationStatusVerified || d.VerifiedAt != nil
}

// RoutingRule represents an explicit routing rule (e.g. support@bariskode.com -> dest).
type RoutingRule struct {
	ID             string    `json:"id"`
	ZoneID         string    `json:"zone_id"`
	ProviderRuleID string    `json:"provider_rule_id"`
	Name           string    `json:"name"`
	MatcherType    string    `json:"matcher_type"`
	MatcherField   string    `json:"matcher_field"`
	MatcherValue   string    `json:"matcher_value"`
	ActionType     string    `json:"action_type"`
	Destination    string    `json:"destination"`
	Enabled        bool      `json:"enabled"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CatchAllRule represents the catch-all routing rule for a zone.
type CatchAllRule struct {
	ID             string     `json:"id"`
	ZoneID         string     `json:"zone_id"`
	ProviderRuleID string     `json:"provider_rule_id"`
	ActionType     string     `json:"action_type"`
	Destination    string     `json:"destination"`
	Enabled        bool       `json:"enabled"`
	Source         string     `json:"source"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// SyncRun records an execution of sync or drift detection.
type SyncRun struct {
	ID           string     `json:"id"`
	ZoneID       string     `json:"zone_id"`
	Direction    string     `json:"direction"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	ChangesCount int        `json:"changes_count"`
	ErrorCount   int        `json:"error_count"`
	Summary      string     `json:"summary"`
}

// AuditEvent records a single mutating operation.
type AuditEvent struct {
	ID                   string    `json:"id"`
	ActorType            string    `json:"actor_type"`
	ActorID              string    `json:"actor_id"`
	Operation            string    `json:"operation"`
	ResourceType         string    `json:"resource_type"`
	ResourceID           string    `json:"resource_id"`
	RequestID            string    `json:"request_id"`
	BeforeJSON           string    `json:"before_json,omitempty"`
	AfterJSON            string    `json:"after_json,omitempty"`
	ProviderResponseJSON string    `json:"provider_response_json,omitempty"`
	Status               string    `json:"status"`
	ErrorCode            string    `json:"error_code,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

// DiffItem reports difference between local and provider states.
type DiffItem struct {
	Status       string `json:"status"` // MATCHED, LOCAL_ONLY, REMOTE_ONLY, CHANGED, UNKNOWN
	ResourceType string `json:"resource_type"`
	Identifier   string `json:"identifier"`
	LocalState   any    `json:"local_state,omitempty"`
	RemoteState  any    `json:"remote_state,omitempty"`
	Details      string `json:"details,omitempty"`
}

// IdempotencyRecord stores cached responses for idempotent operations.
type IdempotencyRecord struct {
	Key        string    `json:"key"`
	StatusCode int       `json:"status_code"`
	Body       []byte    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}
