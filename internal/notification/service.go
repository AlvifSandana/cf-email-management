package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
)

// Supported webhook types
const (
	WebhookTypeGeneric  = "generic"
	WebhookTypeSlack    = "slack"
	WebhookTypeDiscord  = "discord"
	WebhookTypeTelegram = "telegram"
)

// Sentinel errors for notification operations
var (
	ErrWebhookURLMissing      = errors.New("webhook URL is required")
	ErrUnsupportedWebhookType = errors.New("unsupported webhook type")
)

// Config configures the notification service.
type Config struct {
	WebhookURL     string
	WebhookType    string // "generic", "slack", "discord", "telegram"
	TelegramChatID string
	Timeout        time.Duration
	HTTPClient     *http.Client
}

// Service delivers webhook notifications and alerts.
type Service struct {
	WebhookURL     string
	WebhookType    string
	TelegramChatID string
	Timeout        time.Duration
	Client         *http.Client
}

// NewService creates a new configured notification Service.
func NewService(cfg Config) *Service {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: timeout,
		}
	}
	webhookType := strings.ToLower(strings.TrimSpace(cfg.WebhookType))
	if webhookType == "" {
		webhookType = WebhookTypeGeneric
	}
	return &Service{
		WebhookURL:     strings.TrimSpace(cfg.WebhookURL),
		WebhookType:    webhookType,
		TelegramChatID: strings.TrimSpace(cfg.TelegramChatID),
		Timeout:        timeout,
		Client:         client,
	}
}

// GenericDriftPayload defines the generic JSON payload for drift alerts.
type GenericDriftPayload struct {
	Event     string            `json:"event"`
	Timestamp string            `json:"timestamp"`
	Zone      string            `json:"zone"`
	Diffs     []domain.DiffItem `json:"diffs"`
}

// GenericDestinationPayload defines the generic JSON payload for verified destinations.
type GenericDestinationPayload struct {
	Event     string `json:"event"`
	Timestamp string `json:"timestamp"`
	Email     string `json:"email"`
}

// SlackTextBlock represents text in a Slack block.
type SlackTextBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

// SlackBlock represents a Slack Block Kit block.
type SlackBlock struct {
	Type   string            `json:"type"`
	Text   *SlackTextBlock   `json:"text,omitempty"`
	Fields []*SlackTextBlock `json:"fields,omitempty"`
}

// SlackField represents a field inside a Slack attachment.
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackAttachment represents a Slack attachment.
type SlackAttachment struct {
	Color  string       `json:"color,omitempty"`
	Title  string       `json:"title,omitempty"`
	Text   string       `json:"text,omitempty"`
	Fields []SlackField `json:"fields,omitempty"`
}

// SlackPayload defines the payload for Slack webhooks.
type SlackPayload struct {
	Text        string            `json:"text,omitempty"`
	Blocks      []SlackBlock      `json:"blocks,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// DiscordEmbedField represents an embed field for Discord.
type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// DiscordEmbed represents an embed object for Discord.
type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

// DiscordPayload defines the payload for Discord webhooks.
type DiscordPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []DiscordEmbed `json:"embeds,omitempty"`
}

// TelegramPayload defines the payload for Telegram Bot API sendMessage.
type TelegramPayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// NotifyDrift formats and delivers drift detection alerts to the configured webhook.
func (s *Service) NotifyDrift(ctx context.Context, zoneName string, diffs []domain.DiffItem) error {
	if strings.TrimSpace(s.WebhookURL) == "" {
		return ErrWebhookURLMissing
	}

	if diffs == nil {
		diffs = []domain.DiffItem{}
	}

	switch s.getWebhookType() {
	case WebhookTypeGeneric:
		payload := GenericDriftPayload{
			Event:     "drift_detected",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Zone:      zoneName,
			Diffs:     diffs,
		}
		return s.send(ctx, payload)

	case WebhookTypeSlack:
		payload := s.buildSlackDriftPayload(zoneName, diffs)
		return s.send(ctx, payload)

	case WebhookTypeDiscord:
		payload := s.buildDiscordDriftPayload(zoneName, diffs)
		return s.send(ctx, payload)

	case WebhookTypeTelegram:
		payload := s.buildTelegramDriftPayload(zoneName, diffs)
		return s.send(ctx, payload)

	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedWebhookType, s.WebhookType)
	}
}

// NotifyDestinationVerified formats and delivers verified destination alerts to the configured webhook.
func (s *Service) NotifyDestinationVerified(ctx context.Context, email string) error {
	if strings.TrimSpace(s.WebhookURL) == "" {
		return ErrWebhookURLMissing
	}

	switch s.getWebhookType() {
	case WebhookTypeGeneric:
		payload := GenericDestinationPayload{
			Event:     "destination_verified",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Email:     email,
		}
		return s.send(ctx, payload)

	case WebhookTypeSlack:
		payload := s.buildSlackDestinationPayload(email)
		return s.send(ctx, payload)

	case WebhookTypeDiscord:
		payload := s.buildDiscordDestinationPayload(email)
		return s.send(ctx, payload)

	case WebhookTypeTelegram:
		payload := s.buildTelegramDestinationPayload(email)
		return s.send(ctx, payload)

	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedWebhookType, s.WebhookType)
	}
}

func (s *Service) getWebhookType() string {
	t := strings.ToLower(strings.TrimSpace(s.WebhookType))
	if t == "" {
		return WebhookTypeGeneric
	}
	return t
}

func (s *Service) getTimeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return 10 * time.Second
}

func (s *Service) getHTTPClient() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: s.getTimeout()}
}

func (s *Service) getTelegramChatID() string {
	if s.TelegramChatID != "" {
		return s.TelegramChatID
	}
	if parsed, err := url.Parse(s.WebhookURL); err == nil {
		if q := parsed.Query().Get("chat_id"); q != "" {
			return q
		}
	}
	return "default"
}

func (s *Service) buildSlackDriftPayload(zoneName string, diffs []domain.DiffItem) SlackPayload {
	var changed, remoteRules, localRules []string
	for _, d := range diffs {
		desc := fmt.Sprintf("• `%s` (%s)", d.Identifier, d.ResourceType)
		if d.Details != "" {
			desc += fmt.Sprintf(": %s", d.Details)
		}
		switch d.Status {
		case domain.DiffStatusChanged:
			changed = append(changed, desc)
		case domain.DiffStatusRemoteOnly:
			remoteRules = append(remoteRules, desc)
		case domain.DiffStatusLocalOnly:
			localRules = append(localRules, desc)
		default:
			changed = append(changed, fmt.Sprintf("• [%s] `%s`", d.Status, d.Identifier))
		}
	}

	summaryText := fmt.Sprintf("⚠️ Drift detected for zone *%s*: %d difference(s) found.", zoneName, len(diffs))

	blocks := []SlackBlock{
		{
			Type: "header",
			Text: &SlackTextBlock{
				Type:  "plain_text",
				Text:  fmt.Sprintf("⚠️ Drift Detected: %s", zoneName),
				Emoji: true,
			},
		},
		{
			Type: "section",
			Text: &SlackTextBlock{
				Type: "mrkdwn",
				Text: summaryText,
			},
		},
	}

	var fields []SlackField
	if len(changed) > 0 {
		fields = append(fields, SlackField{
			Title: "Changed Rules",
			Value: strings.Join(changed, "\n"),
			Short: false,
		})
	}
	if len(remoteRules) > 0 {
		fields = append(fields, SlackField{
			Title: "Remote-Only Rules",
			Value: strings.Join(remoteRules, "\n"),
			Short: false,
		})
	}
	if len(localRules) > 0 {
		fields = append(fields, SlackField{
			Title: "Local-Only Rules",
			Value: strings.Join(localRules, "\n"),
			Short: false,
		})
	}

	attachments := []SlackAttachment{
		{
			Color:  "#E74C3C",
			Title:  "Drift Details",
			Fields: fields,
		},
	}

	return SlackPayload{
		Text:        summaryText,
		Blocks:      blocks,
		Attachments: attachments,
	}
}

func (s *Service) buildSlackDestinationPayload(email string) SlackPayload {
	text := fmt.Sprintf("✅ Destination verified: %s", email)
	return SlackPayload{
		Text: text,
		Blocks: []SlackBlock{
			{
				Type: "header",
				Text: &SlackTextBlock{
					Type:  "plain_text",
					Text:  "✅ Destination Verified",
					Emoji: true,
				},
			},
			{
				Type: "section",
				Text: &SlackTextBlock{
					Type: "mrkdwn",
					Text: fmt.Sprintf("Destination address *%s* has been successfully verified.", email),
				},
			},
		},
		Attachments: []SlackAttachment{
			{
				Color: "#2ECC71",
				Title: "Destination Verified",
				Text:  email,
			},
		},
	}
}

func (s *Service) buildDiscordDriftPayload(zoneName string, diffs []domain.DiffItem) DiscordPayload {
	var changed, remoteRules, localRules []domain.DiffItem

	for _, d := range diffs {
		switch d.Status {
		case domain.DiffStatusChanged:
			changed = append(changed, d)
		case domain.DiffStatusRemoteOnly:
			remoteRules = append(remoteRules, d)
		case domain.DiffStatusLocalOnly:
			localRules = append(localRules, d)
		default:
			changed = append(changed, d)
		}
	}

	fields := []DiscordEmbedField{
		{
			Name:   "Changed Rules",
			Value:  formatDiscordDiffItems(changed),
			Inline: false,
		},
		{
			Name:   "Remote Rules (Remote-Only)",
			Value:  formatDiscordDiffItems(remoteRules),
			Inline: false,
		},
		{
			Name:   "Local Rules (Local-Only)",
			Value:  formatDiscordDiffItems(localRules),
			Inline: false,
		},
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("⚠️ Drift Detected: %s", zoneName),
		Description: fmt.Sprintf("%d difference(s) detected between local configuration and Cloudflare.", len(diffs)),
		Color:       0xE74C3C, // Red
		Fields:      fields,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	return DiscordPayload{
		Content: fmt.Sprintf("⚠️ Drift detected for zone **%s**", zoneName),
		Embeds:  []DiscordEmbed{embed},
	}
}

func formatDiscordDiffItems(items []domain.DiffItem) string {
	if len(items) == 0 {
		return "None"
	}
	var lines []string
	for _, item := range items {
		line := fmt.Sprintf("• **%s** (`%s`)", item.Identifier, item.ResourceType)
		if item.Details != "" {
			line += fmt.Sprintf(": %s", item.Details)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (s *Service) buildDiscordDestinationPayload(email string) DiscordPayload {
	embed := DiscordEmbed{
		Title:       "✅ Destination Verified",
		Description: fmt.Sprintf("Destination address **%s** has been successfully verified.", email),
		Color:       0x2ECC71, // Green
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	return DiscordPayload{
		Content: fmt.Sprintf("✅ Destination verified: **%s**", email),
		Embeds:  []DiscordEmbed{embed},
	}
}

func (s *Service) buildTelegramDriftPayload(zoneName string, diffs []domain.DiffItem) TelegramPayload {
	var changed, remoteRules, localRules []string
	for _, d := range diffs {
		desc := fmt.Sprintf("• `%s` (%s)", d.Identifier, d.ResourceType)
		if d.Details != "" {
			desc += fmt.Sprintf(": %s", d.Details)
		}
		switch d.Status {
		case domain.DiffStatusChanged:
			changed = append(changed, desc)
		case domain.DiffStatusRemoteOnly:
			remoteRules = append(remoteRules, desc)
		case domain.DiffStatusLocalOnly:
			localRules = append(localRules, desc)
		default:
			changed = append(changed, fmt.Sprintf("• [%s] `%s`", d.Status, d.Identifier))
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*⚠️ Drift Detected for Zone:* `%s`\n", zoneName))
	sb.WriteString(fmt.Sprintf("*Total Differences:* %d\n\n", len(diffs)))

	sb.WriteString("*Changed Rules:*\n")
	if len(changed) > 0 {
		sb.WriteString(strings.Join(changed, "\n") + "\n\n")
	} else {
		sb.WriteString("None\n\n")
	}

	sb.WriteString("*Remote Rules (Remote-Only):*\n")
	if len(remoteRules) > 0 {
		sb.WriteString(strings.Join(remoteRules, "\n") + "\n\n")
	} else {
		sb.WriteString("None\n\n")
	}

	sb.WriteString("*Local Rules (Local-Only):*\n")
	if len(localRules) > 0 {
		sb.WriteString(strings.Join(localRules, "\n"))
	} else {
		sb.WriteString("None")
	}

	return TelegramPayload{
		ChatID:    s.getTelegramChatID(),
		Text:      strings.TrimSpace(sb.String()),
		ParseMode: "Markdown",
	}
}

func (s *Service) buildTelegramDestinationPayload(email string) TelegramPayload {
	text := fmt.Sprintf("✅ *Destination Verified*\nDestination address `%s` has been verified successfully.", email)
	return TelegramPayload{
		ChatID:    s.getTelegramChatID(),
		Text:      text,
		ParseMode: "Markdown",
	}
}

func (s *Service) send(ctx context.Context, payload any) error {
	if strings.TrimSpace(s.WebhookURL) == "" {
		return ErrWebhookURLMissing
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	timeout := s.getTimeout()
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, s.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "EMS-Webhook-Alerting/1.0")

	resp, err := s.getHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook endpoint responded with HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}
