package ses

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
)

const (
	// DefaultRuleSetName is the default SES receipt rule set name used for email routing.
	DefaultRuleSetName = "default-rule-set"
)

// IdentityVerificationAttributes contains verification details for an SES identity.
type IdentityVerificationAttributes struct {
	VerificationStatus string     `json:"verification_status"` // "Success", "Pending", "Failed", "TemporaryFailure", "NotStarted"
	VerificationToken  string     `json:"verification_token,omitempty"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ReceiptAction represents an action performed when an email matches a receipt rule.
type ReceiptAction struct {
	Type        string `json:"type"`        // "forward", "drop", "worker", "lambda", "s3", "sns", etc.
	Destination string `json:"destination"` // Target recipient, ARN, bucket, etc.
}

// ReceiptRule represents an AWS SES receipt rule.
type ReceiptRule struct {
	Name         string          `json:"name"`
	Enabled      bool            `json:"enabled"`
	Recipients   []string        `json:"recipients"`
	Actions      []ReceiptAction `json:"actions"`
	ScanEnabled  bool            `json:"scan_enabled"`
	TlsPolicy    string          `json:"tls_policy,omitempty"`
	MatcherType  string          `json:"matcher_type,omitempty"`
	MatcherField string          `json:"matcher_field,omitempty"`
}

// ReceiptRuleSet represents an AWS SES receipt rule set.
type ReceiptRuleSet struct {
	Name             string        `json:"name"`
	CreatedTimestamp time.Time     `json:"created_timestamp"`
	Rules            []ReceiptRule `json:"rules"`
}

// SESClient defines the interface for underlying AWS SES client operations.
type SESClient interface {
	ListIdentities(ctx context.Context, identityType string) ([]string, error)
	GetIdentityVerificationAttributes(ctx context.Context, identities []string) (map[string]IdentityVerificationAttributes, error)
	VerifyEmailIdentity(ctx context.Context, email string) error
	VerifyDomainIdentity(ctx context.Context, domainName string) error
	DeleteIdentity(ctx context.Context, identity string) error

	DescribeActiveReceiptRuleSet(ctx context.Context) (*ReceiptRuleSet, error)
	SetActiveReceiptRuleSet(ctx context.Context, ruleSetName string) error
	DescribeReceiptRuleSet(ctx context.Context, ruleSetName string) (*ReceiptRuleSet, error)
	CreateReceiptRuleSet(ctx context.Context, ruleSetName string) error

	CreateReceiptRule(ctx context.Context, ruleSetName string, rule ReceiptRule, after string) error
	UpdateReceiptRule(ctx context.Context, ruleSetName string, rule ReceiptRule) error
	DeleteReceiptRule(ctx context.Context, ruleSetName string, ruleName string) error
}

// InMemoryClient is a thread-safe in-memory testdouble/simulator of the AWS SES API.
type InMemoryClient struct {
	mu                sync.RWMutex
	DomainIdentities  map[string]IdentityVerificationAttributes
	EmailIdentities   map[string]IdentityVerificationAttributes
	RuleSets          map[string]*ReceiptRuleSet
	ActiveRuleSetName string
	AutoVerify        bool

	// SimulateError injects an error into all client operations when non-nil.
	SimulateError error
}

// NewInMemoryClient creates an initialized in-memory SES simulation client.
func NewInMemoryClient() *InMemoryClient {
	c := &InMemoryClient{
		DomainIdentities: make(map[string]IdentityVerificationAttributes),
		EmailIdentities:  make(map[string]IdentityVerificationAttributes),
		RuleSets:         make(map[string]*ReceiptRuleSet),
	}
	c.RuleSets[DefaultRuleSetName] = &ReceiptRuleSet{
		Name:             DefaultRuleSetName,
		CreatedTimestamp: time.Now().UTC(),
		Rules:            []ReceiptRule{},
	}
	c.ActiveRuleSetName = DefaultRuleSetName
	return c
}

// AddDomain registers a domain identity with a specified verification status in the in-memory client.
func (c *InMemoryClient) AddDomain(domainName, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().UTC()
	var verifiedAt *time.Time
	if status == "Success" {
		verifiedAt = &now
	}
	c.DomainIdentities[domainName] = IdentityVerificationAttributes{
		VerificationStatus: status,
		VerifiedAt:         verifiedAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// AddEmail registers an email identity with a specified verification status in the in-memory client.
func (c *InMemoryClient) AddEmail(email, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().UTC()
	var verifiedAt *time.Time
	if status == "Success" {
		verifiedAt = &now
	}
	c.EmailIdentities[email] = IdentityVerificationAttributes{
		VerificationStatus: status,
		VerifiedAt:         verifiedAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// SetVerified sets the verification status of an identity to Success or Pending.
func (c *InMemoryClient) SetVerified(identity string, verified bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := "Pending"
	var verifiedAt *time.Time
	now := time.Now().UTC()
	if verified {
		status = "Success"
		verifiedAt = &now
	}

	if attr, ok := c.DomainIdentities[identity]; ok {
		attr.VerificationStatus = status
		attr.VerifiedAt = verifiedAt
		attr.UpdatedAt = now
		c.DomainIdentities[identity] = attr
	}
	if attr, ok := c.EmailIdentities[identity]; ok {
		attr.VerificationStatus = status
		attr.VerifiedAt = verifiedAt
		attr.UpdatedAt = now
		c.EmailIdentities[identity] = attr
	}
}

// SetSimulateError sets an error to simulate AWS SES failure.
func (c *InMemoryClient) SetSimulateError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SimulateError = err
}

func (c *InMemoryClient) ListIdentities(ctx context.Context, identityType string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.SimulateError != nil {
		return nil, c.SimulateError
	}

	var results []string
	if identityType == "Domain" || identityType == "" {
		for d := range c.DomainIdentities {
			results = append(results, d)
		}
	}
	if identityType == "EmailAddress" || identityType == "" {
		for e := range c.EmailIdentities {
			results = append(results, e)
		}
	}
	return results, nil
}

func (c *InMemoryClient) GetIdentityVerificationAttributes(ctx context.Context, identities []string) (map[string]IdentityVerificationAttributes, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.SimulateError != nil {
		return nil, c.SimulateError
	}

	res := make(map[string]IdentityVerificationAttributes)
	for _, id := range identities {
		if attr, ok := c.DomainIdentities[id]; ok {
			res[id] = attr
		} else if attr, ok := c.EmailIdentities[id]; ok {
			res[id] = attr
		}
	}
	return res, nil
}

func (c *InMemoryClient) VerifyEmailIdentity(ctx context.Context, email string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	now := time.Now().UTC()
	status := "Pending"
	var verifiedAt *time.Time
	if c.AutoVerify {
		status = "Success"
		verifiedAt = &now
	}

	c.EmailIdentities[email] = IdentityVerificationAttributes{
		VerificationStatus: status,
		VerifiedAt:         verifiedAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return nil
}

func (c *InMemoryClient) VerifyDomainIdentity(ctx context.Context, domainName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	now := time.Now().UTC()
	status := "Pending"
	var verifiedAt *time.Time
	if c.AutoVerify {
		status = "Success"
		verifiedAt = &now
	}

	c.DomainIdentities[domainName] = IdentityVerificationAttributes{
		VerificationStatus: status,
		VerifiedAt:         verifiedAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return nil
}

func (c *InMemoryClient) DeleteIdentity(ctx context.Context, identity string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	found := false
	if _, ok := c.DomainIdentities[identity]; ok {
		delete(c.DomainIdentities, identity)
		found = true
	}
	if _, ok := c.EmailIdentities[identity]; ok {
		delete(c.EmailIdentities, identity)
		found = true
	}
	if !found {
		return domain.ErrNotFound
	}
	return nil
}

func (c *InMemoryClient) DescribeActiveReceiptRuleSet(ctx context.Context) (*ReceiptRuleSet, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.SimulateError != nil {
		return nil, c.SimulateError
	}

	if c.ActiveRuleSetName == "" {
		return nil, nil
	}
	rs, ok := c.RuleSets[c.ActiveRuleSetName]
	if !ok {
		return nil, nil
	}
	return cloneReceiptRuleSet(rs), nil
}

func (c *InMemoryClient) SetActiveReceiptRuleSet(ctx context.Context, ruleSetName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	if ruleSetName == "" {
		c.ActiveRuleSetName = ""
		return nil
	}

	if _, ok := c.RuleSets[ruleSetName]; !ok {
		return domain.ErrNotFound
	}
	c.ActiveRuleSetName = ruleSetName
	return nil
}

func (c *InMemoryClient) DescribeReceiptRuleSet(ctx context.Context, ruleSetName string) (*ReceiptRuleSet, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.SimulateError != nil {
		return nil, c.SimulateError
	}

	rs, ok := c.RuleSets[ruleSetName]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneReceiptRuleSet(rs), nil
}

func (c *InMemoryClient) CreateReceiptRuleSet(ctx context.Context, ruleSetName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	if _, ok := c.RuleSets[ruleSetName]; ok {
		return domain.ErrAlreadyExists
	}
	c.RuleSets[ruleSetName] = &ReceiptRuleSet{
		Name:             ruleSetName,
		CreatedTimestamp: time.Now().UTC(),
		Rules:            []ReceiptRule{},
	}
	return nil
}

func (c *InMemoryClient) CreateReceiptRule(ctx context.Context, ruleSetName string, rule ReceiptRule, after string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	rs, ok := c.RuleSets[ruleSetName]
	if !ok {
		rs = &ReceiptRuleSet{
			Name:             ruleSetName,
			CreatedTimestamp: time.Now().UTC(),
			Rules:            []ReceiptRule{},
		}
		c.RuleSets[ruleSetName] = rs
	}

	for _, existing := range rs.Rules {
		if existing.Name == rule.Name {
			return domain.ErrAlreadyExists
		}
	}

	rs.Rules = append(rs.Rules, rule)
	return nil
}

func (c *InMemoryClient) UpdateReceiptRule(ctx context.Context, ruleSetName string, rule ReceiptRule) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	rs, ok := c.RuleSets[ruleSetName]
	if !ok {
		return domain.ErrNotFound
	}

	for i, existing := range rs.Rules {
		if existing.Name == rule.Name {
			rs.Rules[i] = rule
			return nil
		}
	}
	return domain.ErrNotFound
}

func (c *InMemoryClient) DeleteReceiptRule(ctx context.Context, ruleSetName string, ruleName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SimulateError != nil {
		return c.SimulateError
	}

	rs, ok := c.RuleSets[ruleSetName]
	if !ok {
		return domain.ErrNotFound
	}

	for i, existing := range rs.Rules {
		if existing.Name == ruleName {
			rs.Rules = append(rs.Rules[:i], rs.Rules[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func cloneReceiptRuleSet(rs *ReceiptRuleSet) *ReceiptRuleSet {
	if rs == nil {
		return nil
	}
	rulesCopy := make([]ReceiptRule, len(rs.Rules))
	for i, r := range rs.Rules {
		recCopy := make([]string, len(r.Recipients))
		copy(recCopy, r.Recipients)
		actionsCopy := make([]ReceiptAction, len(r.Actions))
		copy(actionsCopy, r.Actions)
		rulesCopy[i] = ReceiptRule{
			Name:         r.Name,
			Enabled:      r.Enabled,
			Recipients:   recCopy,
			Actions:      actionsCopy,
			ScanEnabled:  r.ScanEnabled,
			TlsPolicy:    r.TlsPolicy,
			MatcherType:  r.MatcherType,
			MatcherField: r.MatcherField,
		}
	}
	return &ReceiptRuleSet{
		Name:             rs.Name,
		CreatedTimestamp: rs.CreatedTimestamp,
		Rules:            rulesCopy,
	}
}

// --- Provider Implementation ---

// Provider implements provider.EmailProvider for AWS Simple Email Service (SES).
type Provider struct {
	client      SESClient
	ruleSetName string
}

var _ provider.EmailProvider = (*Provider)(nil)

// Option allows configuring the SES Provider.
type Option func(*Provider)

// WithClient configures a custom SESClient implementation (e.g. AWS SDK or testdouble).
func WithClient(client SESClient) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// WithRuleSetName configures the SES receipt rule set name.
func WithRuleSetName(name string) Option {
	return func(p *Provider) {
		p.ruleSetName = name
	}
}

// New creates a new AWS SES EmailProvider adapter.
func New(opts ...Option) *Provider {
	p := &Provider{
		ruleSetName: DefaultRuleSetName,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.client == nil {
		p.client = NewInMemoryClient()
	}
	return p
}

// NewProvider is an alias for New.
func NewProvider(opts ...Option) *Provider {
	return New(opts...)
}

// Client returns the underlying SESClient.
func (p *Provider) Client() SESClient {
	return p.client
}

// RuleSetName returns the configured receipt rule set name.
func (p *Provider) RuleSetName() string {
	return p.ruleSetName
}

func (p *Provider) ensureRuleSet(ctx context.Context) error {
	_, err := p.client.DescribeReceiptRuleSet(ctx, p.ruleSetName)
	if err != nil {
		if createErr := p.client.CreateReceiptRuleSet(ctx, p.ruleSetName); createErr != nil && !errors.Is(createErr, domain.ErrAlreadyExists) {
			return createErr
		}
	}
	return nil
}

// --- Zone Methods (SES Domain Identities) ---

// ListZones returns domain identities registered and verified in SES.
func (p *Provider) ListZones(ctx context.Context) ([]domain.Zone, error) {
	domains, err := p.client.ListIdentities(ctx, "Domain")
	if err != nil {
		return nil, domain.NewProviderError("failed to list SES domain identities", err)
	}
	if len(domains) == 0 {
		return []domain.Zone{}, nil
	}

	attrs, err := p.client.GetIdentityVerificationAttributes(ctx, domains)
	if err != nil {
		return nil, domain.NewProviderError("failed to get SES identity attributes", err)
	}

	activeRuleSet, err := p.client.DescribeActiveReceiptRuleSet(ctx)
	routingEnabled := false
	if err == nil && activeRuleSet != nil && activeRuleSet.Name != "" {
		routingEnabled = true
	}

	now := time.Now().UTC()
	zones := make([]domain.Zone, 0, len(domains))
	for _, d := range domains {
		status := "pending"
		if attr, ok := attrs[d]; ok {
			if attr.VerificationStatus == "Success" {
				status = "active"
			} else if attr.VerificationStatus != "" {
				status = strings.ToLower(attr.VerificationStatus)
			}
		}
		zones = append(zones, domain.Zone{
			ProviderZoneID:      d,
			Name:                d,
			Status:              status,
			EmailRoutingEnabled: routingEnabled,
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return zones, nil
}

// --- Email Routing Settings (SES Active Receipt Rule Set) ---

// GetEmailRoutingSettings returns the active receipt rule set status for email routing.
func (p *Provider) GetEmailRoutingSettings(ctx context.Context, zoneID string) (provider.RoutingSettings, error) {
	activeRuleSet, err := p.client.DescribeActiveReceiptRuleSet(ctx)
	if err != nil {
		return provider.RoutingSettings{}, domain.NewProviderError("failed to describe active receipt rule set", err)
	}

	if activeRuleSet != nil && activeRuleSet.Name != "" {
		return provider.RoutingSettings{
			Enabled: true,
			Status:  "active",
		}, nil
	}

	return provider.RoutingSettings{
		Enabled: false,
		Status:  "inactive",
	}, nil
}

// UpdateEmailRoutingSettings activates or deactivates the SES receipt rule set.
func (p *Provider) UpdateEmailRoutingSettings(ctx context.Context, zoneID string, settings provider.RoutingSettingsUpdate) error {
	if settings.Enabled {
		if err := p.ensureRuleSet(ctx); err != nil {
			return domain.NewProviderError("failed to ensure receipt rule set exists", err)
		}
		if err := p.client.SetActiveReceiptRuleSet(ctx, p.ruleSetName); err != nil {
			return domain.NewProviderError("failed to activate receipt rule set", err)
		}
	} else {
		if err := p.client.SetActiveReceiptRuleSet(ctx, ""); err != nil {
			return domain.NewProviderError("failed to deactivate receipt rule set", err)
		}
	}
	return nil
}

// --- Destination Addresses (SES Email Identities) ---

// ListDestinationAddresses returns SES verified email identities.
func (p *Provider) ListDestinationAddresses(ctx context.Context, accountID string) ([]domain.DestinationAddress, error) {
	emails, err := p.client.ListIdentities(ctx, "EmailAddress")
	if err != nil {
		return nil, domain.NewProviderError("failed to list SES email identities", err)
	}
	if len(emails) == 0 {
		return []domain.DestinationAddress{}, nil
	}

	attrs, err := p.client.GetIdentityVerificationAttributes(ctx, emails)
	if err != nil {
		return nil, domain.NewProviderError("failed to get SES email verification attributes", err)
	}

	now := time.Now().UTC()
	dests := make([]domain.DestinationAddress, 0, len(emails))
	for _, email := range emails {
		status := domain.DestinationStatusPending
		var verifiedAt *time.Time
		if attr, ok := attrs[email]; ok {
			if attr.VerificationStatus == "Success" {
				status = domain.DestinationStatusVerified
				verifiedAt = attr.VerifiedAt
				if verifiedAt == nil {
					verifiedAt = &now
				}
			} else if attr.VerificationStatus == "Pending" {
				status = domain.DestinationStatusPending
			} else {
				status = domain.DestinationStatusUnverified
			}
		}
		dests = append(dests, domain.DestinationAddress{
			ProviderAccountID: accountID,
			ProviderAddressID: email,
			Email:             email,
			VerifiedAt:        verifiedAt,
			Status:            status,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}
	return dests, nil
}

// CreateDestinationAddress sends an SES email verification request and registers identity.
func (p *Provider) CreateDestinationAddress(ctx context.Context, accountID string, email string) (domain.DestinationAddress, error) {
	if err := p.client.VerifyEmailIdentity(ctx, email); err != nil {
		return domain.DestinationAddress{}, domain.NewProviderError("failed to verify email identity", err)
	}

	attrs, _ := p.client.GetIdentityVerificationAttributes(ctx, []string{email})
	now := time.Now().UTC()
	status := domain.DestinationStatusPending
	var verifiedAt *time.Time
	if attr, ok := attrs[email]; ok && attr.VerificationStatus == "Success" {
		status = domain.DestinationStatusVerified
		verifiedAt = attr.VerifiedAt
		if verifiedAt == nil {
			verifiedAt = &now
		}
	}

	return domain.DestinationAddress{
		ProviderAccountID: accountID,
		ProviderAddressID: email,
		Email:             email,
		VerifiedAt:        verifiedAt,
		Status:            status,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// UpdateDestinationAddress updates an SES email identity (deleting old identity and registering new if address changed).
func (p *Provider) UpdateDestinationAddress(ctx context.Context, accountID, addressID string, email string) (domain.DestinationAddress, error) {
	attrs, err := p.client.GetIdentityVerificationAttributes(ctx, []string{addressID})
	if err != nil {
		return domain.DestinationAddress{}, domain.NewProviderError("failed to query destination address", err)
	}
	if _, ok := attrs[addressID]; !ok {
		return domain.DestinationAddress{}, domain.NewNotFoundError("destination", addressID)
	}

	if email != addressID {
		if err := p.client.DeleteIdentity(ctx, addressID); err != nil {
			return domain.DestinationAddress{}, domain.NewProviderError("failed to delete old destination identity", err)
		}
		if err := p.client.VerifyEmailIdentity(ctx, email); err != nil {
			return domain.DestinationAddress{}, domain.NewProviderError("failed to verify new destination identity", err)
		}
	}

	now := time.Now().UTC()
	status := domain.DestinationStatusPending
	var verifiedAt *time.Time
	newAttrs, _ := p.client.GetIdentityVerificationAttributes(ctx, []string{email})
	if attr, ok := newAttrs[email]; ok && attr.VerificationStatus == "Success" {
		status = domain.DestinationStatusVerified
		verifiedAt = attr.VerifiedAt
		if verifiedAt == nil {
			verifiedAt = &now
		}
	}

	return domain.DestinationAddress{
		ProviderAccountID: accountID,
		ProviderAddressID: email,
		Email:             email,
		VerifiedAt:        verifiedAt,
		Status:            status,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// DeleteDestinationAddress deletes an SES email identity.
func (p *Provider) DeleteDestinationAddress(ctx context.Context, accountID, addressID string) error {
	attrs, err := p.client.GetIdentityVerificationAttributes(ctx, []string{addressID})
	if err != nil {
		return domain.NewProviderError("failed to query destination address", err)
	}
	if _, ok := attrs[addressID]; !ok {
		return domain.NewNotFoundError("destination", addressID)
	}

	if err := p.client.DeleteIdentity(ctx, addressID); err != nil {
		return domain.NewProviderError("failed to delete destination identity", err)
	}
	return nil
}

// --- Rules Methods (SES Receipt Rules) ---

// ListRules returns explicit SES receipt rules for a specified zone.
func (p *Provider) ListRules(ctx context.Context, zoneID string) ([]domain.RoutingRule, error) {
	ruleSet, err := p.client.DescribeReceiptRuleSet(ctx, p.ruleSetName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return []domain.RoutingRule{}, nil
		}
		return nil, domain.NewProviderError("failed to describe receipt rule set", err)
	}
	if ruleSet == nil {
		return []domain.RoutingRule{}, nil
	}

	now := time.Now().UTC()
	var rules []domain.RoutingRule
	for _, r := range ruleSet.Rules {
		if isAnyCatchAllRule(r) {
			continue
		}
		if !belongsToZone(r, zoneID) {
			continue
		}

		matcherType := r.MatcherType
		if matcherType == "" {
			matcherType = domain.MatcherTypeLiteral
		}
		matcherField := r.MatcherField
		if matcherField == "" {
			matcherField = domain.MatcherFieldTo
		}
		matcherValue := ""
		if len(r.Recipients) > 0 {
			matcherValue = r.Recipients[0]
		}

		actionType := ""
		destination := ""
		if len(r.Actions) > 0 {
			actionType = r.Actions[0].Type
			destination = r.Actions[0].Destination
		}

		rules = append(rules, domain.RoutingRule{
			ZoneID:         zoneID,
			ProviderRuleID: r.Name,
			Name:           r.Name,
			MatcherType:    matcherType,
			MatcherField:   matcherField,
			MatcherValue:   matcherValue,
			ActionType:     actionType,
			Destination:    destination,
			Enabled:        r.Enabled,
			Source:         "provider",
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	return rules, nil
}

// CreateRule creates a new receipt rule in the SES receipt rule set.
func (p *Provider) CreateRule(ctx context.Context, zoneID string, rule provider.CreateRoutingRule) (domain.RoutingRule, error) {
	if err := p.ensureRuleSet(ctx); err != nil {
		return domain.RoutingRule{}, domain.NewProviderError("failed to ensure receipt rule set", err)
	}

	ruleName := rule.Name
	if ruleName == "" {
		ruleName = fmt.Sprintf("rule_%s_%d", strings.ReplaceAll(zoneID, ".", "_"), time.Now().UnixNano())
	}

	matcherType := rule.MatcherType
	if matcherType == "" {
		matcherType = domain.MatcherTypeLiteral
	}
	matcherField := rule.MatcherField
	if matcherField == "" {
		matcherField = domain.MatcherFieldTo
	}
	actionType := rule.ActionType
	if actionType == "" {
		actionType = domain.ActionTypeForward
	}

	sesRule := ReceiptRule{
		Name:         ruleName,
		Enabled:      rule.Enabled,
		Recipients:   []string{rule.MatcherValue},
		MatcherType:  matcherType,
		MatcherField: matcherField,
		Actions: []ReceiptAction{
			{
				Type:        actionType,
				Destination: rule.Destination,
			},
		},
	}

	if err := p.client.CreateReceiptRule(ctx, p.ruleSetName, sesRule, ""); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return domain.RoutingRule{}, domain.NewConflictError(fmt.Sprintf("rule '%s' already exists", ruleName))
		}
		return domain.RoutingRule{}, domain.NewProviderError("failed to create SES receipt rule", err)
	}

	now := time.Now().UTC()
	return domain.RoutingRule{
		ZoneID:         zoneID,
		ProviderRuleID: ruleName,
		Name:           rule.Name,
		MatcherType:    matcherType,
		MatcherField:   matcherField,
		MatcherValue:   rule.MatcherValue,
		ActionType:     actionType,
		Destination:    rule.Destination,
		Enabled:        rule.Enabled,
		Source:         "provider",
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// UpdateRule updates an existing receipt rule in the SES receipt rule set.
func (p *Provider) UpdateRule(ctx context.Context, zoneID, ruleID string, rule provider.UpdateRoutingRule) (domain.RoutingRule, error) {
	matcherType := rule.MatcherType
	if matcherType == "" {
		matcherType = domain.MatcherTypeLiteral
	}
	matcherField := rule.MatcherField
	if matcherField == "" {
		matcherField = domain.MatcherFieldTo
	}
	actionType := rule.ActionType
	if actionType == "" {
		actionType = domain.ActionTypeForward
	}

	sesRule := ReceiptRule{
		Name:         ruleID,
		Enabled:      rule.Enabled,
		Recipients:   []string{rule.MatcherValue},
		MatcherType:  matcherType,
		MatcherField: matcherField,
		Actions: []ReceiptAction{
			{
				Type:        actionType,
				Destination: rule.Destination,
			},
		},
	}

	if err := p.client.UpdateReceiptRule(ctx, p.ruleSetName, sesRule); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.RoutingRule{}, domain.NewNotFoundError("rule", ruleID)
		}
		return domain.RoutingRule{}, domain.NewProviderError("failed to update SES receipt rule", err)
	}

	ruleName := rule.Name
	if ruleName == "" {
		ruleName = ruleID
	}

	now := time.Now().UTC()
	return domain.RoutingRule{
		ZoneID:         zoneID,
		ProviderRuleID: ruleID,
		Name:           ruleName,
		MatcherType:    matcherType,
		MatcherField:   matcherField,
		MatcherValue:   rule.MatcherValue,
		ActionType:     actionType,
		Destination:    rule.Destination,
		Enabled:        rule.Enabled,
		Source:         "provider",
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// DeleteRule removes a receipt rule from the SES receipt rule set.
func (p *Provider) DeleteRule(ctx context.Context, zoneID, ruleID string) error {
	if err := p.client.DeleteReceiptRule(ctx, p.ruleSetName, ruleID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NewNotFoundError("rule", ruleID)
		}
		return domain.NewProviderError("failed to delete SES receipt rule", err)
	}
	return nil
}

// --- Catch-All Methods (SES Catch-All Receipt Rule) ---

// GetCatchAll retrieves the default catch-all rule in the SES receipt rule set for the given zone.
func (p *Provider) GetCatchAll(ctx context.Context, zoneID string) (domain.CatchAllRule, error) {
	ruleSet, err := p.client.DescribeReceiptRuleSet(ctx, p.ruleSetName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.CatchAllRule{
				ZoneID:         zoneID,
				ProviderRuleID: "",
				ActionType:     domain.ActionTypeDrop,
				Enabled:        false,
			}, nil
		}
		return domain.CatchAllRule{}, domain.NewProviderError("failed to describe receipt rule set", err)
	}
	if ruleSet == nil {
		return domain.CatchAllRule{
			ZoneID:         zoneID,
			ProviderRuleID: "",
			ActionType:     domain.ActionTypeDrop,
			Enabled:        false,
		}, nil
	}

	for _, r := range ruleSet.Rules {
		if isCatchAllForZone(r, zoneID) {
			actionType := ""
			destination := ""
			if len(r.Actions) > 0 {
				actionType = r.Actions[0].Type
				destination = r.Actions[0].Destination
			}
			now := time.Now().UTC()
			return domain.CatchAllRule{
				ZoneID:         zoneID,
				ProviderRuleID: r.Name,
				ActionType:     actionType,
				Destination:    destination,
				Enabled:        r.Enabled,
				Source:         "provider",
				LastSyncedAt:   &now,
				CreatedAt:      now,
				UpdatedAt:      now,
			}, nil
		}
	}

	return domain.CatchAllRule{
		ZoneID:         zoneID,
		ProviderRuleID: "",
		ActionType:     domain.ActionTypeDrop,
		Enabled:        false,
	}, nil
}

// UpdateCatchAll creates or updates the default catch-all rule in the SES receipt rule set for the given zone.
func (p *Provider) UpdateCatchAll(ctx context.Context, zoneID string, rule provider.UpdateCatchAll) (domain.CatchAllRule, error) {
	if err := p.ensureRuleSet(ctx); err != nil {
		return domain.CatchAllRule{}, domain.NewProviderError("failed to ensure receipt rule set", err)
	}

	ruleName := catchAllRuleName(zoneID)
	actionType := rule.ActionType
	if actionType == "" {
		actionType = domain.ActionTypeDrop
	}

	sesRule := ReceiptRule{
		Name:         ruleName,
		Enabled:      rule.Enabled,
		Recipients:   []string{zoneID},
		MatcherType:  domain.MatcherTypeAll,
		MatcherField: "",
		Actions: []ReceiptAction{
			{
				Type:        actionType,
				Destination: rule.Destination,
			},
		},
	}

	// Try update first; if rule does not exist, create it
	err := p.client.UpdateReceiptRule(ctx, p.ruleSetName, sesRule)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			if createErr := p.client.CreateReceiptRule(ctx, p.ruleSetName, sesRule, ""); createErr != nil {
				return domain.CatchAllRule{}, domain.NewProviderError("failed to create SES catch-all rule", createErr)
			}
		} else {
			return domain.CatchAllRule{}, domain.NewProviderError("failed to update SES catch-all rule", err)
		}
	}

	now := time.Now().UTC()
	return domain.CatchAllRule{
		ZoneID:         zoneID,
		ProviderRuleID: ruleName,
		ActionType:     actionType,
		Destination:    rule.Destination,
		Enabled:        rule.Enabled,
		Source:         "provider",
		LastSyncedAt:   &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// --- Helpers ---

func catchAllRuleName(zoneID string) string {
	return fmt.Sprintf("catchall-%s", zoneID)
}

func isAnyCatchAllRule(r ReceiptRule) bool {
	nameLower := strings.ToLower(r.Name)
	if strings.HasPrefix(nameLower, "catchall-") || strings.HasPrefix(nameLower, "catch-all") {
		return true
	}
	if r.MatcherType == domain.MatcherTypeAll {
		return true
	}
	for _, rec := range r.Recipients {
		if !strings.Contains(rec, "@") {
			return true
		}
	}
	return false
}

func isCatchAllForZone(r ReceiptRule, zoneID string) bool {
	targetName := catchAllRuleName(zoneID)
	if strings.EqualFold(r.Name, targetName) {
		return true
	}
	zoneLower := strings.ToLower(zoneID)
	for _, rec := range r.Recipients {
		recLower := strings.ToLower(rec)
		if recLower == zoneLower || recLower == "@"+zoneLower || recLower == "*@"+zoneLower {
			return true
		}
	}
	return false
}

func belongsToZone(r ReceiptRule, zoneID string) bool {
	if zoneID == "" {
		return true
	}
	zoneLower := strings.ToLower(zoneID)
	for _, rec := range r.Recipients {
		recLower := strings.ToLower(rec)
		if strings.HasSuffix(recLower, "@"+zoneLower) || recLower == zoneLower {
			return true
		}
	}
	return strings.Contains(strings.ToLower(r.Name), zoneLower)
}
