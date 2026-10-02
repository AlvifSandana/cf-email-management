package sending

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
)

const (
	// DefaultBaseURL is the Cloudflare v4 API base URL.
	DefaultBaseURL = "https://api.cloudflare.com/client/v4"
)

// SendEmailRequest contains parameters for sending an outbound email via Cloudflare.
type SendEmailRequest struct {
	From     string            `json:"from"`
	To       []string          `json:"to"`
	Subject  string            `json:"subject"`
	TextBody string            `json:"text_body,omitempty"`
	HTMLBody string            `json:"html_body,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// UnmarshalJSON allows SendEmailRequest to deserialize from either text/html or text_body/html_body fields.
func (r *SendEmailRequest) UnmarshalJSON(data []byte) error {
	type Alias SendEmailRequest
	aux := struct {
		Text string `json:"text"`
		HTML string `json:"html"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if r.TextBody == "" && aux.Text != "" {
		r.TextBody = aux.Text
	}
	if r.HTMLBody == "" && aux.HTML != "" {
		r.HTMLBody = aux.HTML
	}
	return nil
}

// SendEmailResponse contains the result of sending an outbound email.
type SendEmailResponse struct {
	MessageID string   `json:"message_id,omitempty"`
	Status    string   `json:"status,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

// Client interacts with the Cloudflare Outbound Email Sending REST API.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

// Option configures a Client instance.
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
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// NewClient creates a new Cloudflare email sending client.
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

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	if c.baseURL == "" {
		return DefaultBaseURL
	}
	return c.baseURL
}

// APIToken returns the configured API token.
func (c *Client) APIToken() string {
	return c.apiToken
}

// HTTPClient returns the configured HTTP client.
func (c *Client) HTTPClient() *http.Client {
	if c.httpClient == nil {
		return http.DefaultClient
	}
	return c.httpClient
}

// Cloudflare envelope structures
type cfResponse struct {
	Success  *bool           `json:"success,omitempty"`
	Errors   []cfError       `json:"errors,omitempty"`
	Messages []string        `json:"messages,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`

	// Direct response fallback fields
	MessageID string `json:"message_id,omitempty"`
	Status    string `json:"status,omitempty"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfSendPayload struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	Subject string            `json:"subject"`
	Text    string            `json:"text,omitempty"`
	HTML    string            `json:"html,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type cfSendResult struct {
	ID                   string   `json:"id,omitempty"`
	MessageID            string   `json:"message_id,omitempty"`
	Status               string   `json:"status,omitempty"`
	Delivered            []string `json:"delivered,omitempty"`
	PermanentBounces     []string `json:"permanent_bounces,omitempty"`
	Queued               []string `json:"queued,omitempty"`
	SuppressedRecipients []string `json:"suppressed_recipients,omitempty"`
}

// Send sends an email via Cloudflare Email Sending REST API:
// POST https://api.cloudflare.com/client/v4/accounts/{account_id}/email/sending/send
func (c *Client) Send(ctx context.Context, accountID string, req SendEmailRequest) (*SendEmailResponse, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, domain.NewValidationError("accountID is required")
	}
	if strings.TrimSpace(req.From) == "" {
		return nil, domain.NewValidationError("from address is required")
	}
	if len(req.To) == 0 {
		return nil, domain.NewValidationError("at least one recipient (to) is required")
	}
	for _, to := range req.To {
		if strings.TrimSpace(to) == "" {
			return nil, domain.NewValidationError("recipient address cannot be empty")
		}
	}

	payload := cfSendPayload{
		From:    req.From,
		To:      req.To,
		Subject: req.Subject,
		Text:    req.TextBody,
		HTML:    req.HTMLBody,
		Headers: req.Headers,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, domain.NewInternalError("failed to marshal request body", err)
	}

	baseURL := c.BaseURL()
	httpClient := c.HTTPClient()

	reqURL := fmt.Sprintf("%s/accounts/%s/email/sending/send", baseURL, accountID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(data))
	if err != nil {
		return nil, domain.NewInternalError("failed to create http request", err)
	}

	if c.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, domain.NewProviderTimeoutError("provider request timed out", err)
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return nil, domain.NewInternalError("request context canceled", err)
		}
		return nil, domain.NewProviderError("provider connection error", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, domain.NewProviderError("failed to read provider response", err)
	}

	var envelope cfResponse
	unmarshalErr := json.Unmarshal(respBytes, &envelope)

	if resp.StatusCode >= 400 || (envelope.Success != nil && !*envelope.Success) {
		var cfErrors []cfError
		if unmarshalErr == nil {
			cfErrors = envelope.Errors
		}
		return nil, c.normalizeError(resp.StatusCode, cfErrors, respBytes)
	}

	if unmarshalErr != nil {
		return nil, domain.NewProviderError(
			fmt.Sprintf("failed to parse provider response: %s", string(respBytes)),
			unmarshalErr,
		)
	}

	var result cfSendResult
	if len(envelope.Result) > 0 && string(envelope.Result) != "null" {
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return nil, domain.NewProviderError("failed to parse send result", err)
		}
	}

	messageID := result.MessageID
	if messageID == "" {
		messageID = result.ID
	}
	if messageID == "" {
		messageID = envelope.MessageID
	}

	status := result.Status
	if status == "" {
		status = envelope.Status
	}
	if status == "" {
		if len(result.Delivered) > 0 {
			status = "delivered"
		} else if len(result.Queued) > 0 {
			status = "queued"
		} else {
			status = "sent"
		}
	}

	errorsList := make([]string, 0)
	for _, b := range result.PermanentBounces {
		errorsList = append(errorsList, fmt.Sprintf("permanent bounce: %s", b))
	}
	for _, s := range result.SuppressedRecipients {
		errorsList = append(errorsList, fmt.Sprintf("suppressed recipient: %s", s))
	}

	return &SendEmailResponse{
		MessageID: messageID,
		Status:    status,
		Errors:    errorsList,
	}, nil
}

// normalizeError converts Cloudflare error envelopes into structured domain.AppError.
func (c *Client) normalizeError(statusCode int, cfErrors []cfError, rawBody []byte) *domain.AppError {
	var errMsgs []string
	hasDestinationNotVerified := false
	hasUnauthorized := false
	hasNotFound := false
	hasValidation := false
	hasConflict := false

	for _, e := range cfErrors {
		msg := strings.TrimSpace(e.Message)
		if e.Code != 0 {
			errMsgs = append(errMsgs, fmt.Sprintf("[%d] %s", e.Code, msg))
		} else if msg != "" {
			errMsgs = append(errMsgs, msg)
		}

		lowerMsg := strings.ToLower(msg)

		switch e.Code {
		case 10000:
			hasNotFound = true
		case 10001, 10200, 10201, 10202:
			hasValidation = true
		case 10101, 10102, 10103, 10105, 10203:
			hasUnauthorized = true
		}

		if strings.Contains(lowerMsg, "not verified") ||
			strings.Contains(lowerMsg, "unverified") ||
			(strings.Contains(lowerMsg, "destination") && strings.Contains(lowerMsg, "verified")) {
			hasDestinationNotVerified = true
		}
		if strings.Contains(lowerMsg, "unauthorized") ||
			strings.Contains(lowerMsg, "forbidden") ||
			strings.Contains(lowerMsg, "permission") ||
			strings.Contains(lowerMsg, "authentication") {
			hasUnauthorized = true
		}
		if strings.Contains(lowerMsg, "not found") {
			hasNotFound = true
		}
		if strings.Contains(lowerMsg, "invalid") ||
			strings.Contains(lowerMsg, "validation") ||
			strings.Contains(lowerMsg, "schema") ||
			strings.Contains(lowerMsg, "too_big") {
			hasValidation = true
		}
	}

	combinedMsg := strings.Join(errMsgs, "; ")
	if combinedMsg == "" {
		if len(rawBody) > 0 {
			combinedMsg = string(rawBody)
		} else {
			combinedMsg = fmt.Sprintf("provider returned status %d", statusCode)
		}
	}

	if hasDestinationNotVerified {
		return domain.NewDestinationNotVerifiedError(combinedMsg)
	}

	if hasUnauthorized || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return domain.NewUnauthorizedError(combinedMsg)
	}

	if hasNotFound || statusCode == http.StatusNotFound {
		return &domain.AppError{
			Code:       domain.ErrCodeNotFound,
			Message:    combinedMsg,
			HTTPStatus: http.StatusNotFound,
		}
	}

	if hasValidation || statusCode == http.StatusBadRequest {
		return domain.NewValidationError(combinedMsg)
	}

	if hasConflict || statusCode == http.StatusConflict {
		return domain.NewConflictError(combinedMsg)
	}

	if statusCode == http.StatusGatewayTimeout || statusCode == 504 {
		return domain.NewProviderTimeoutError(combinedMsg, nil)
	}

	return domain.NewProviderError(combinedMsg, nil)
}
