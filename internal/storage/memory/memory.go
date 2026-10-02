package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/storage"
)

// Storage is an in-memory thread-safe implementation of all repositories.
type Storage struct {
	mu            sync.RWMutex
	accounts      map[string]domain.CloudflareAccount
	zones         map[string]domain.Zone
	destinations  map[string]domain.DestinationAddress
	rules         map[string]domain.RoutingRule
	catchAll      map[string]domain.CatchAllRule // zoneID -> CatchAllRule
	syncRuns      map[string]domain.SyncRun
	auditEvents   []domain.AuditEvent
	idempotencies map[string]domain.IdempotencyRecord
}

// New creates an initialized in-memory Storage and returns *storage.Repositories.
func New() *storage.Repositories {
	s := &Storage{
		accounts:      make(map[string]domain.CloudflareAccount),
		zones:         make(map[string]domain.Zone),
		destinations:  make(map[string]domain.DestinationAddress),
		rules:         make(map[string]domain.RoutingRule),
		catchAll:      make(map[string]domain.CatchAllRule),
		syncRuns:      make(map[string]domain.SyncRun),
		auditEvents:   make([]domain.AuditEvent, 0),
		idempotencies: make(map[string]domain.IdempotencyRecord),
	}

	return &storage.Repositories{
		Accounts:     &accountRepo{s},
		Zones:        &zoneRepo{s},
		Destinations: &destinationRepo{s},
		Rules:        &ruleRepo{s},
		CatchAll:     &catchAllRepo{s},
		SyncRuns:     &syncRunRepo{s},
		Audit:        &auditRepo{s},
		Idempotency:  &idempotencyRepo{s},
	}
}

// --- Account Repo ---
type accountRepo struct{ *Storage }

func (r *accountRepo) Save(ctx context.Context, acc *domain.CloudflareAccount) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[acc.ID] = *acc
	return nil
}

func (r *accountRepo) Get(ctx context.Context, id string) (*domain.CloudflareAccount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	acc, ok := r.accounts[id]
	if !ok {
		return nil, domain.NewNotFoundError("account", id)
	}
	return &acc, nil
}

func (r *accountRepo) List(ctx context.Context) ([]domain.CloudflareAccount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]domain.CloudflareAccount, 0, len(r.accounts))
	for _, acc := range r.accounts {
		list = append(list, acc)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	return list, nil
}

func (r *accountRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[id]; !ok {
		return domain.NewNotFoundError("account", id)
	}
	delete(r.accounts, id)
	return nil
}

// --- Zone Repo ---
type zoneRepo struct{ *Storage }

func (r *zoneRepo) Save(ctx context.Context, z *domain.Zone) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.zones[z.ID] = *z
	return nil
}

func (r *zoneRepo) Get(ctx context.Context, id string) (*domain.Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	z, ok := r.zones[id]
	if !ok {
		return nil, domain.NewNotFoundError("zone", id)
	}
	return &z, nil
}

func (r *zoneRepo) GetByProviderZoneID(ctx context.Context, providerZoneID string) (*domain.Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, z := range r.zones {
		if z.ProviderZoneID == providerZoneID {
			return &z, nil
		}
	}
	return nil, domain.NewNotFoundError("zone", providerZoneID)
}

func (r *zoneRepo) List(ctx context.Context) ([]domain.Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]domain.Zone, 0, len(r.zones))
	for _, z := range r.zones {
		list = append(list, z)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func (r *zoneRepo) ListByAccount(ctx context.Context, accountID string) ([]domain.Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []domain.Zone
	for _, z := range r.zones {
		if z.ProviderAccountID == accountID {
			list = append(list, z)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func (r *zoneRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.zones[id]; !ok {
		return domain.NewNotFoundError("zone", id)
	}
	delete(r.zones, id)
	return nil
}

// --- Destination Repo ---
type destinationRepo struct{ *Storage }

func (r *destinationRepo) Save(ctx context.Context, d *domain.DestinationAddress) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.destinations[d.ID] = *d
	return nil
}

func (r *destinationRepo) Get(ctx context.Context, id string) (*domain.DestinationAddress, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.destinations[id]
	if !ok {
		return nil, domain.NewNotFoundError("destination", id)
	}
	return &d, nil
}

func (r *destinationRepo) GetByEmail(ctx context.Context, accountID, email string) (*domain.DestinationAddress, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range r.destinations {
		if d.ProviderAccountID == accountID && d.Email == email {
			return &d, nil
		}
	}
	return nil, domain.NewNotFoundError("destination", email)
}

func (r *destinationRepo) List(ctx context.Context) ([]domain.DestinationAddress, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]domain.DestinationAddress, 0, len(r.destinations))
	for _, d := range r.destinations {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Email < list[j].Email })
	return list, nil
}

func (r *destinationRepo) ListByAccount(ctx context.Context, accountID string) ([]domain.DestinationAddress, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []domain.DestinationAddress
	for _, d := range r.destinations {
		if d.ProviderAccountID == accountID {
			list = append(list, d)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Email < list[j].Email })
	return list, nil
}

func (r *destinationRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.destinations[id]; !ok {
		return domain.NewNotFoundError("destination", id)
	}
	delete(r.destinations, id)
	return nil
}

// --- Routing Rule Repo ---
type ruleRepo struct{ *Storage }

func (r *ruleRepo) Save(ctx context.Context, rule *domain.RoutingRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[rule.ID] = *rule
	return nil
}

func (r *ruleRepo) Get(ctx context.Context, id string) (*domain.RoutingRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.rules[id]
	if !ok {
		return nil, domain.NewNotFoundError("rule", id)
	}
	return &rule, nil
}

func (r *ruleRepo) ListByZone(ctx context.Context, zoneID string) ([]domain.RoutingRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []domain.RoutingRule
	for _, rule := range r.rules {
		if rule.ZoneID == zoneID {
			list = append(list, rule)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func (r *ruleRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[id]; !ok {
		return domain.NewNotFoundError("rule", id)
	}
	delete(r.rules, id)
	return nil
}

func (r *ruleRepo) DeleteByZone(ctx context.Context, zoneID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, rule := range r.rules {
		if rule.ZoneID == zoneID {
			delete(r.rules, id)
		}
	}
	return nil
}

// --- Catch All Repo ---
type catchAllRepo struct{ *Storage }

func (r *catchAllRepo) Save(ctx context.Context, rule *domain.CatchAllRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.catchAll[rule.ZoneID] = *rule
	return nil
}

func (r *catchAllRepo) GetByZone(ctx context.Context, zoneID string) (*domain.CatchAllRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.catchAll[zoneID]
	if !ok {
		return nil, domain.NewNotFoundError("catch-all", zoneID)
	}
	return &rule, nil
}

func (r *catchAllRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for zoneID, rule := range r.catchAll {
		if rule.ID == id || zoneID == id {
			delete(r.catchAll, zoneID)
			return nil
		}
	}
	return domain.NewNotFoundError("catch-all", id)
}

// --- Sync Run Repo ---
type syncRunRepo struct{ *Storage }

func (r *syncRunRepo) Save(ctx context.Context, run *domain.SyncRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.syncRuns[run.ID] = *run
	return nil
}

func (r *syncRunRepo) Get(ctx context.Context, id string) (*domain.SyncRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.syncRuns[id]
	if !ok {
		return nil, domain.NewNotFoundError("sync_run", id)
	}
	return &run, nil
}

func (r *syncRunRepo) ListByZone(ctx context.Context, zoneID string) ([]domain.SyncRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []domain.SyncRun
	for _, run := range r.syncRuns {
		if run.ZoneID == zoneID {
			list = append(list, run)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].StartedAt.After(list[j].StartedAt) })
	return list, nil
}

func (r *syncRunRepo) List(ctx context.Context, limit int) ([]domain.SyncRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]domain.SyncRun, 0, len(r.syncRuns))
	for _, run := range r.syncRuns {
		list = append(list, run)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].StartedAt.After(list[j].StartedAt) })
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// --- Audit Repo ---
type auditRepo struct{ *Storage }

func (r *auditRepo) Record(ctx context.Context, event *domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditEvents = append(r.auditEvents, *event)
	return nil
}

func (r *auditRepo) List(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := len(r.auditEvents)
	list := make([]domain.AuditEvent, n)
	copy(list, r.auditEvents)
	// reverse sort by CreatedAt descending
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// --- Idempotency Repo ---
type idempotencyRepo struct{ *Storage }

func (r *idempotencyRepo) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.idempotencies[key]
	if !ok || time.Now().After(rec.ExpiresAt) {
		return nil, domain.NewNotFoundError("idempotency_record", key)
	}
	return &rec, nil
}

func (r *idempotencyRepo) Save(ctx context.Context, record *domain.IdempotencyRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idempotencies[record.Key] = *record
	return nil
}
