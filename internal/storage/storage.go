package storage

import (
	"context"

	"github.com/bariskode/email-management-service/internal/domain"
)

// AccountRepository manages CloudflareAccount entities.
type AccountRepository interface {
	Save(ctx context.Context, acc *domain.CloudflareAccount) error
	Get(ctx context.Context, id string) (*domain.CloudflareAccount, error)
	List(ctx context.Context) ([]domain.CloudflareAccount, error)
	Delete(ctx context.Context, id string) error
}

// ZoneRepository manages Zone entities.
type ZoneRepository interface {
	Save(ctx context.Context, zone *domain.Zone) error
	Get(ctx context.Context, id string) (*domain.Zone, error)
	GetByProviderZoneID(ctx context.Context, providerZoneID string) (*domain.Zone, error)
	List(ctx context.Context) ([]domain.Zone, error)
	ListByAccount(ctx context.Context, accountID string) ([]domain.Zone, error)
	Delete(ctx context.Context, id string) error
}

// DestinationRepository manages DestinationAddress entities.
type DestinationRepository interface {
	Save(ctx context.Context, dest *domain.DestinationAddress) error
	Get(ctx context.Context, id string) (*domain.DestinationAddress, error)
	GetByEmail(ctx context.Context, accountID, email string) (*domain.DestinationAddress, error)
	List(ctx context.Context) ([]domain.DestinationAddress, error)
	ListByAccount(ctx context.Context, accountID string) ([]domain.DestinationAddress, error)
	Delete(ctx context.Context, id string) error
}

// RoutingRuleRepository manages RoutingRule entities.
type RoutingRuleRepository interface {
	Save(ctx context.Context, rule *domain.RoutingRule) error
	Get(ctx context.Context, id string) (*domain.RoutingRule, error)
	ListByZone(ctx context.Context, zoneID string) ([]domain.RoutingRule, error)
	Delete(ctx context.Context, id string) error
	DeleteByZone(ctx context.Context, zoneID string) error
}

// CatchAllRepository manages CatchAllRule entities.
type CatchAllRepository interface {
	Save(ctx context.Context, rule *domain.CatchAllRule) error
	GetByZone(ctx context.Context, zoneID string) (*domain.CatchAllRule, error)
	Delete(ctx context.Context, id string) error
}

// SyncRunRepository manages SyncRun history.
type SyncRunRepository interface {
	Save(ctx context.Context, run *domain.SyncRun) error
	Get(ctx context.Context, id string) (*domain.SyncRun, error)
	ListByZone(ctx context.Context, zoneID string) ([]domain.SyncRun, error)
	List(ctx context.Context, limit int) ([]domain.SyncRun, error)
}

// AuditRepository records mutating operations.
type AuditRepository interface {
	Record(ctx context.Context, event *domain.AuditEvent) error
	List(ctx context.Context, limit int) ([]domain.AuditEvent, error)
}

// IdempotencyRepository stores idempotency keys and cached HTTP responses.
type IdempotencyRepository interface {
	Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error)
	Save(ctx context.Context, record *domain.IdempotencyRecord) error
}

// Repositories aggregates all repository interfaces.
type Repositories struct {
	Accounts     AccountRepository
	Zones        ZoneRepository
	Destinations DestinationRepository
	Rules        RoutingRuleRepository
	CatchAll     CatchAllRepository
	SyncRuns     SyncRunRepository
	Audit        AuditRepository
	Idempotency  IdempotencyRepository
}
