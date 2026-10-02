package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
)

const (
	DefaultBaseURL = "https://api.cloudflare.com/client/v4"
)

// Client implements provider.EmailProvider for Cloudflare.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

// Option allows configuring the Cloudflare client.
type Option func(*Client)

// WithBaseURL overrides the default Cloudflare API base URL.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// NewClient creates a new Cloudflare provider client.
func NewClient(apiToken string, opts ...Option) *Client {
	c := &Client{
		baseURL:  DefaultBaseURL,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Cloudflare standard response envelope
type cfResponse[T any] struct {
	Success  bool      `json:"success"`
	Errors   []cfError `json:"errors"`
	Messages []string  `json:"messages"`
	Result   T         `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any, resultTarget any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return domain.NewInternalError("failed to marshal request body", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return domain.NewInternalError("failed to create http request", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return domain.NewProviderTimeoutError("provider request timed out", err)
		}
		return domain.NewProviderError("provider connection error", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProviderError("failed to read provider response", err)
	}

	if resultTarget != nil {
		if err := json.Unmarshal(respBytes, resultTarget); err != nil {
			return domain.NewProviderError(fmt.Sprintf("failed to parse provider response: %s", string(respBytes)), err)
		}
	}

	if resp.StatusCode >= 400 {
		return domain.NewProviderError(fmt.Sprintf("provider returned status %d: %s", resp.StatusCode, string(respBytes)), nil)
	}

	return nil
}

// --- Zone endpoints ---

type cfZone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (c *Client) ListZones(ctx context.Context) ([]domain.Zone, error) {
	var resp cfResponse[[]cfZone]
	if err := c.doRequest(ctx, http.MethodGet, "/zones", nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, c.formatCFError(resp.Errors)
	}

	zones := make([]domain.Zone, len(resp.Result))
	for i, z := range resp.Result {
		zones[i] = domain.Zone{
			ProviderZoneID: z.ID,
			Name:           z.Name,
			Status:         z.Status,
		}
	}
	return zones, nil
}

// --- Routing Settings ---

type cfRoutingSettings struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Tag     string `json:"tag"`
}

func (c *Client) GetEmailRoutingSettings(ctx context.Context, zoneID string) (provider.RoutingSettings, error) {
	var resp cfResponse[cfRoutingSettings]
	path := fmt.Sprintf("/zones/%s/email/routing", zoneID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return provider.RoutingSettings{}, err
	}
	if !resp.Success {
		return provider.RoutingSettings{}, c.formatCFError(resp.Errors)
	}

	return provider.RoutingSettings{
		Enabled: resp.Result.Enabled,
		Status:  resp.Result.Status,
	}, nil
}

func (c *Client) UpdateEmailRoutingSettings(ctx context.Context, zoneID string, settings provider.RoutingSettingsUpdate) error {
	payload := map[string]any{
		"enabled": settings.Enabled,
	}
	var resp cfResponse[cfRoutingSettings]
	path := fmt.Sprintf("/zones/%s/email/routing", zoneID)
	if err := c.doRequest(ctx, http.MethodPut, path, payload, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return c.formatCFError(resp.Errors)
	}
	return nil
}

// --- Destination Addresses ---

type cfDestinationAddress struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Verified   *time.Time `json:"verified"`
	Created    time.Time  `json:"created"`
	Modified   time.Time  `json:"modified"`
	StatusText string     `json:"status"`
}

func (c *Client) ListDestinationAddresses(ctx context.Context, accountID string) ([]domain.DestinationAddress, error) {
	var resp cfResponse[[]cfDestinationAddress]
	path := fmt.Sprintf("/accounts/%s/email/routing/addresses", accountID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, c.formatCFError(resp.Errors)
	}

	dests := make([]domain.DestinationAddress, len(resp.Result))
	for i, d := range resp.Result {
		status := domain.DestinationStatusPending
		if d.Verified != nil {
			status = domain.DestinationStatusVerified
		}
		dests[i] = domain.DestinationAddress{
			ProviderAccountID: accountID,
			ProviderAddressID: d.ID,
			Email:             d.Email,
			VerifiedAt:        d.Verified,
			Status:            status,
			CreatedAt:         d.Created,
			UpdatedAt:         d.Modified,
		}
	}
	return dests, nil
}

func (c *Client) CreateDestinationAddress(ctx context.Context, accountID string, email string) (domain.DestinationAddress, error) {
	payload := map[string]string{
		"email": email,
	}
	var resp cfResponse[cfDestinationAddress]
	path := fmt.Sprintf("/accounts/%s/email/routing/addresses", accountID)
	if err := c.doRequest(ctx, http.MethodPost, path, payload, &resp); err != nil {
		return domain.DestinationAddress{}, err
	}
	if !resp.Success {
		return domain.DestinationAddress{}, c.formatCFError(resp.Errors)
	}

	d := resp.Result
	status := domain.DestinationStatusPending
	if d.Verified != nil {
		status = domain.DestinationStatusVerified
	}

	return domain.DestinationAddress{
		ProviderAccountID: accountID,
		ProviderAddressID: d.ID,
		Email:             d.Email,
		VerifiedAt:        d.Verified,
		Status:            status,
		CreatedAt:         d.Created,
		UpdatedAt:         d.Modified,
	}, nil
}

func (c *Client) UpdateDestinationAddress(ctx context.Context, accountID, addressID string, email string) (domain.DestinationAddress, error) {
	payload := map[string]string{
		"email": email,
	}
	var resp cfResponse[cfDestinationAddress]
	path := fmt.Sprintf("/accounts/%s/email/routing/addresses/%s", accountID, addressID)
	if err := c.doRequest(ctx, http.MethodPut, path, payload, &resp); err != nil {
		return domain.DestinationAddress{}, err
	}
	if !resp.Success {
		return domain.DestinationAddress{}, c.formatCFError(resp.Errors)
	}

	d := resp.Result
	status := domain.DestinationStatusPending
	if d.Verified != nil {
		status = domain.DestinationStatusVerified
	}

	return domain.DestinationAddress{
		ProviderAccountID: accountID,
		ProviderAddressID: d.ID,
		Email:             d.Email,
		VerifiedAt:        d.Verified,
		Status:            status,
		CreatedAt:         d.Created,
		UpdatedAt:         d.Modified,
	}, nil
}

func (c *Client) DeleteDestinationAddress(ctx context.Context, accountID, addressID string) error {
	var resp cfResponse[map[string]any]
	path := fmt.Sprintf("/accounts/%s/email/routing/addresses/%s", accountID, addressID)
	if err := c.doRequest(ctx, http.MethodDelete, path, nil, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return c.formatCFError(resp.Errors)
	}
	return nil
}

// --- Rules endpoints ---

type cfMatcher struct {
	Type  string `json:"type"`
	Field string `json:"field,omitempty"`
	Value string `json:"value,omitempty"`
}

type cfAction struct {
	Type  string   `json:"type"`
	Value []string `json:"value"`
}

type cfRule struct {
	ID       string      `json:"id"`
	Tag      string      `json:"tag"`
	Name     string      `json:"name"`
	Enabled  bool        `json:"enabled"`
	Matchers []cfMatcher `json:"matchers"`
	Actions  []cfAction  `json:"actions"`
}

func (c *Client) ListRules(ctx context.Context, zoneID string) ([]domain.RoutingRule, error) {
	var resp cfResponse[[]cfRule]
	path := fmt.Sprintf("/zones/%s/email/routing/rules", zoneID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, c.formatCFError(resp.Errors)
	}

	rules := make([]domain.RoutingRule, 0, len(resp.Result))
	for _, r := range resp.Result {
		matcherType := ""
		matcherField := ""
		matcherValue := ""
		if len(r.Matchers) > 0 {
			matcherType = r.Matchers[0].Type
			matcherField = r.Matchers[0].Field
			matcherValue = r.Matchers[0].Value
		}

		actionType := ""
		destination := ""
		if len(r.Actions) > 0 {
			actionType = r.Actions[0].Type
			if len(r.Actions[0].Value) > 0 {
				destination = r.Actions[0].Value[0]
			}
		}

		rules = append(rules, domain.RoutingRule{
			ProviderRuleID: r.ID,
			Name:           r.Name,
			MatcherType:    matcherType,
			MatcherField:   matcherField,
			MatcherValue:   matcherValue,
			ActionType:     actionType,
			Destination:    destination,
			Enabled:        r.Enabled,
			Source:         "provider",
		})
	}
	return rules, nil
}

func (c *Client) CreateRule(ctx context.Context, zoneID string, rule provider.CreateRoutingRule) (domain.RoutingRule, error) {
	payload := cfRule{
		Name:    rule.Name,
		Enabled: rule.Enabled,
		Matchers: []cfMatcher{
			{
				Type:  rule.MatcherType,
				Field: rule.MatcherField,
				Value: rule.MatcherValue,
			},
		},
		Actions: []cfAction{
			{
				Type:  rule.ActionType,
				Value: []string{rule.Destination},
			},
		},
	}

	var resp cfResponse[cfRule]
	path := fmt.Sprintf("/zones/%s/email/routing/rules", zoneID)
	if err := c.doRequest(ctx, http.MethodPost, path, payload, &resp); err != nil {
		return domain.RoutingRule{}, err
	}
	if !resp.Success {
		return domain.RoutingRule{}, c.formatCFError(resp.Errors)
	}

	r := resp.Result
	return domain.RoutingRule{
		ProviderRuleID: r.ID,
		Name:           r.Name,
		MatcherType:    rule.MatcherType,
		MatcherField:   rule.MatcherField,
		MatcherValue:   rule.MatcherValue,
		ActionType:     rule.ActionType,
		Destination:    rule.Destination,
		Enabled:        r.Enabled,
		Source:         "provider",
	}, nil
}

func (c *Client) UpdateRule(ctx context.Context, zoneID, ruleID string, rule provider.UpdateRoutingRule) (domain.RoutingRule, error) {
	payload := cfRule{
		Name:    rule.Name,
		Enabled: rule.Enabled,
		Matchers: []cfMatcher{
			{
				Type:  rule.MatcherType,
				Field: rule.MatcherField,
				Value: rule.MatcherValue,
			},
		},
		Actions: []cfAction{
			{
				Type:  rule.ActionType,
				Value: []string{rule.Destination},
			},
		},
	}

	var resp cfResponse[cfRule]
	path := fmt.Sprintf("/zones/%s/email/routing/rules/%s", zoneID, ruleID)
	if err := c.doRequest(ctx, http.MethodPut, path, payload, &resp); err != nil {
		return domain.RoutingRule{}, err
	}
	if !resp.Success {
		return domain.RoutingRule{}, c.formatCFError(resp.Errors)
	}

	r := resp.Result
	return domain.RoutingRule{
		ProviderRuleID: r.ID,
		Name:           r.Name,
		MatcherType:    rule.MatcherType,
		MatcherField:   rule.MatcherField,
		MatcherValue:   rule.MatcherValue,
		ActionType:     rule.ActionType,
		Destination:    rule.Destination,
		Enabled:        r.Enabled,
		Source:         "provider",
	}, nil
}

func (c *Client) DeleteRule(ctx context.Context, zoneID, ruleID string) error {
	var resp cfResponse[map[string]any]
	path := fmt.Sprintf("/zones/%s/email/routing/rules/%s", zoneID, ruleID)
	if err := c.doRequest(ctx, http.MethodDelete, path, nil, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return c.formatCFError(resp.Errors)
	}
	return nil
}

// --- Catch-All endpoints ---
// Explicitly uses dedicated /zones/{zone_id}/email/routing/rules/catch_all endpoint

type cfCatchAllRule struct {
	ID       string      `json:"id"`
	Tag      string      `json:"tag"`
	Name     string      `json:"name"`
	Enabled  bool        `json:"enabled"`
	Matchers []cfMatcher `json:"matchers"`
	Actions  []cfAction  `json:"actions"`
}

func (c *Client) GetCatchAll(ctx context.Context, zoneID string) (domain.CatchAllRule, error) {
	var resp cfResponse[cfCatchAllRule]
	path := fmt.Sprintf("/zones/%s/email/routing/rules/catch_all", zoneID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return domain.CatchAllRule{}, err
	}
	if !resp.Success {
		return domain.CatchAllRule{}, c.formatCFError(resp.Errors)
	}

	r := resp.Result
	actionType := ""
	destination := ""
	if len(r.Actions) > 0 {
		actionType = r.Actions[0].Type
		if len(r.Actions[0].Value) > 0 {
			destination = r.Actions[0].Value[0]
		}
	}

	return domain.CatchAllRule{
		ProviderRuleID: r.ID,
		ActionType:     actionType,
		Destination:    destination,
		Enabled:        r.Enabled,
		Source:         "provider",
	}, nil
}

func (c *Client) UpdateCatchAll(ctx context.Context, zoneID string, rule provider.UpdateCatchAll) (domain.CatchAllRule, error) {
	payload := cfCatchAllRule{
		Name:    "Catch-all",
		Enabled: rule.Enabled,
		Matchers: []cfMatcher{
			{Type: domain.MatcherTypeAll},
		},
		Actions: []cfAction{
			{
				Type:  rule.ActionType,
				Value: []string{rule.Destination},
			},
		},
	}

	var resp cfResponse[cfCatchAllRule]
	path := fmt.Sprintf("/zones/%s/email/routing/rules/catch_all", zoneID)
	if err := c.doRequest(ctx, http.MethodPut, path, payload, &resp); err != nil {
		return domain.CatchAllRule{}, err
	}
	if !resp.Success {
		return domain.CatchAllRule{}, c.formatCFError(resp.Errors)
	}

	r := resp.Result
	return domain.CatchAllRule{
		ProviderRuleID: r.ID,
		ActionType:     rule.ActionType,
		Destination:    rule.Destination,
		Enabled:        r.Enabled,
		Source:         "provider",
	}, nil
}

func (c *Client) formatCFError(errors []cfError) error {
	if len(errors) == 0 {
		return domain.NewProviderError("unknown provider error", nil)
	}
	msgs := make([]string, len(errors))
	for i, e := range errors {
		msgs[i] = fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return domain.NewProviderError(strings.Join(msgs, "; "), nil)
}
