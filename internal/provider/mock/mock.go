package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
)

// Provider is an in-memory mock implementation of provider.EmailProvider.
type Provider struct {
	mu           sync.RWMutex
	Zones        map[string]domain.Zone
	Settings     map[string]provider.RoutingSettings
	Destinations map[string]map[string]domain.DestinationAddress // accountID -> addressID -> address
	Rules        map[string]map[string]domain.RoutingRule        // zoneID -> ruleID -> rule
	CatchAll     map[string]domain.CatchAllRule                  // zoneID -> catchAll

	// Customizable hooks for simulating failures
	SimulateError error
}

// New creates an initialized mock Provider.
func New() *Provider {
	return &Provider{
		Zones:        make(map[string]domain.Zone),
		Settings:     make(map[string]provider.RoutingSettings),
		Destinations: make(map[string]map[string]domain.DestinationAddress),
		Rules:        make(map[string]map[string]domain.RoutingRule),
		CatchAll:     make(map[string]domain.CatchAllRule),
	}
}

func (m *Provider) ListZones(ctx context.Context) ([]domain.Zone, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SimulateError != nil {
		return nil, m.SimulateError
	}
	res := make([]domain.Zone, 0, len(m.Zones))
	for _, z := range m.Zones {
		res = append(res, z)
	}
	return res, nil
}

func (m *Provider) GetEmailRoutingSettings(ctx context.Context, zoneID string) (provider.RoutingSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SimulateError != nil {
		return provider.RoutingSettings{}, m.SimulateError
	}
	s, ok := m.Settings[zoneID]
	if !ok {
		return provider.RoutingSettings{Enabled: false, Status: "unconfigured"}, nil
	}
	return s, nil
}

func (m *Provider) UpdateEmailRoutingSettings(ctx context.Context, zoneID string, settings provider.RoutingSettingsUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return m.SimulateError
	}
	status := "unconfigured"
	if settings.Enabled {
		status = "ready"
	}
	m.Settings[zoneID] = provider.RoutingSettings{
		Enabled: settings.Enabled,
		Status:  status,
	}
	return nil
}

func (m *Provider) ListDestinationAddresses(ctx context.Context, accountID string) ([]domain.DestinationAddress, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SimulateError != nil {
		return nil, m.SimulateError
	}
	accDests, ok := m.Destinations[accountID]
	if !ok {
		return []domain.DestinationAddress{}, nil
	}
	res := make([]domain.DestinationAddress, 0, len(accDests))
	for _, d := range accDests {
		res = append(res, d)
	}
	return res, nil
}

func (m *Provider) CreateDestinationAddress(ctx context.Context, accountID string, email string) (domain.DestinationAddress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return domain.DestinationAddress{}, m.SimulateError
	}
	if _, ok := m.Destinations[accountID]; !ok {
		m.Destinations[accountID] = make(map[string]domain.DestinationAddress)
	}
	addrID := fmt.Sprintf("cf_addr_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	// Mock: automatically verify for ease of testing or unverified
	addr := domain.DestinationAddress{
		ProviderAccountID: accountID,
		ProviderAddressID: addrID,
		Email:             email,
		Status:            domain.DestinationStatusPending,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	m.Destinations[accountID][addrID] = addr
	return addr, nil
}

func (m *Provider) UpdateDestinationAddress(ctx context.Context, accountID, addressID string, email string) (domain.DestinationAddress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return domain.DestinationAddress{}, m.SimulateError
	}
	accDests, ok := m.Destinations[accountID]
	if !ok {
		return domain.DestinationAddress{}, domain.NewNotFoundError("destination", addressID)
	}
	addr, ok := accDests[addressID]
	if !ok {
		return domain.DestinationAddress{}, domain.NewNotFoundError("destination", addressID)
	}
	addr.Email = email
	addr.UpdatedAt = time.Now().UTC()
	m.Destinations[accountID][addressID] = addr
	return addr, nil
}

func (m *Provider) DeleteDestinationAddress(ctx context.Context, accountID, addressID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return m.SimulateError
	}
	if accDests, ok := m.Destinations[accountID]; ok {
		delete(accDests, addressID)
	}
	return nil
}

func (m *Provider) ListRules(ctx context.Context, zoneID string) ([]domain.RoutingRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SimulateError != nil {
		return nil, m.SimulateError
	}
	zoneRules, ok := m.Rules[zoneID]
	if !ok {
		return []domain.RoutingRule{}, nil
	}
	res := make([]domain.RoutingRule, 0, len(zoneRules))
	for _, r := range zoneRules {
		res = append(res, r)
	}
	return res, nil
}

func (m *Provider) CreateRule(ctx context.Context, zoneID string, rule provider.CreateRoutingRule) (domain.RoutingRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return domain.RoutingRule{}, m.SimulateError
	}
	if _, ok := m.Rules[zoneID]; !ok {
		m.Rules[zoneID] = make(map[string]domain.RoutingRule)
	}
	ruleID := fmt.Sprintf("cf_rule_%d", time.Now().UnixNano())
	r := domain.RoutingRule{
		ProviderRuleID: ruleID,
		Name:           rule.Name,
		MatcherType:    rule.MatcherType,
		MatcherField:   rule.MatcherField,
		MatcherValue:   rule.MatcherValue,
		ActionType:     rule.ActionType,
		Destination:    rule.Destination,
		Enabled:        rule.Enabled,
		Source:         "provider",
	}
	m.Rules[zoneID][ruleID] = r
	return r, nil
}

func (m *Provider) UpdateRule(ctx context.Context, zoneID, ruleID string, rule provider.UpdateRoutingRule) (domain.RoutingRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return domain.RoutingRule{}, m.SimulateError
	}
	zoneRules, ok := m.Rules[zoneID]
	if !ok {
		return domain.RoutingRule{}, domain.NewNotFoundError("rule", ruleID)
	}
	r, ok := zoneRules[ruleID]
	if !ok {
		return domain.RoutingRule{}, domain.NewNotFoundError("rule", ruleID)
	}
	r.Name = rule.Name
	r.MatcherType = rule.MatcherType
	r.MatcherField = rule.MatcherField
	r.MatcherValue = rule.MatcherValue
	r.ActionType = rule.ActionType
	r.Destination = rule.Destination
	r.Enabled = rule.Enabled
	m.Rules[zoneID][ruleID] = r
	return r, nil
}

func (m *Provider) DeleteRule(ctx context.Context, zoneID, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return m.SimulateError
	}
	if zoneRules, ok := m.Rules[zoneID]; ok {
		delete(zoneRules, ruleID)
	}
	return nil
}

func (m *Provider) GetCatchAll(ctx context.Context, zoneID string) (domain.CatchAllRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SimulateError != nil {
		return domain.CatchAllRule{}, m.SimulateError
	}
	c, ok := m.CatchAll[zoneID]
	if !ok {
		return domain.CatchAllRule{
			ProviderRuleID: "",
			ActionType:     domain.ActionTypeDrop,
			Enabled:        false,
		}, nil
	}
	return c, nil
}

func (m *Provider) UpdateCatchAll(ctx context.Context, zoneID string, rule provider.UpdateCatchAll) (domain.CatchAllRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SimulateError != nil {
		return domain.CatchAllRule{}, m.SimulateError
	}
	c := domain.CatchAllRule{
		ProviderRuleID: fmt.Sprintf("cf_catchall_%s", zoneID),
		ActionType:     rule.ActionType,
		Destination:    rule.Destination,
		Enabled:        rule.Enabled,
		Source:         "provider",
	}
	m.CatchAll[zoneID] = c
	return c, nil
}
