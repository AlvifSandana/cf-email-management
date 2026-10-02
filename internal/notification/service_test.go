package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/notification"
)

func sampleDiffs() []domain.DiffItem {
	return []domain.DiffItem{
		{
			Status:       domain.DiffStatusChanged,
			ResourceType: "routing_rule",
			Identifier:   "support@bariskode.com",
			Details:      "destination changed from local to remote",
		},
		{
			Status:       domain.DiffStatusRemoteOnly,
			ResourceType: "routing_rule",
			Identifier:   "sales@bariskode.com",
			Details:      "rule exists only on Cloudflare",
		},
		{
			Status:       domain.DiffStatusLocalOnly,
			ResourceType: "catch_all",
			Identifier:   "catch-all",
			Details:      "catch-all enabled locally but disabled on Cloudflare",
		},
	}
}

func TestNotifyDrift_Generic(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string
	var receivedMethod string
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: notification.WebhookTypeGeneric,
	})

	diffs := sampleDiffs()
	err := svc.NotifyDrift(context.Background(), "bariskode.com", diffs)
	if err != nil {
		t.Fatalf("unexpected error from NotifyDrift: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST method, got %s", receivedMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected application/json Content-Type, got %s", receivedContentType)
	}

	var payload notification.GenericDriftPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal received body: %v", err)
	}

	if payload.Event != "drift_detected" {
		t.Errorf("expected event 'drift_detected', got '%s'", payload.Event)
	}
	if payload.Zone != "bariskode.com" {
		t.Errorf("expected zone 'bariskode.com', got '%s'", payload.Zone)
	}
	if payload.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
	if len(payload.Diffs) != len(diffs) {
		t.Fatalf("expected %d diffs, got %d", len(diffs), len(payload.Diffs))
	}
	if payload.Diffs[0].Identifier != "support@bariskode.com" {
		t.Errorf("expected first diff identifier 'support@bariskode.com', got '%s'", payload.Diffs[0].Identifier)
	}
}

func TestNotifyDrift_Slack(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: notification.WebhookTypeSlack,
	})

	diffs := sampleDiffs()
	err := svc.NotifyDrift(context.Background(), "bariskode.com", diffs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload notification.SlackPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal Slack payload: %v", err)
	}

	if !strings.Contains(payload.Text, "bariskode.com") {
		t.Errorf("expected text to mention zone, got: %s", payload.Text)
	}
	if len(payload.Blocks) == 0 {
		t.Errorf("expected blocks to be populated")
	}
	if len(payload.Attachments) == 0 {
		t.Errorf("expected attachments to be populated")
	}

	// Verify attachment fields contain our diffs
	fields := payload.Attachments[0].Fields
	fieldMap := make(map[string]string)
	for _, f := range fields {
		fieldMap[f.Title] = f.Value
	}

	if _, ok := fieldMap["Changed Rules"]; !ok {
		t.Error("expected 'Changed Rules' field in Slack attachment")
	}
	if _, ok := fieldMap["Remote-Only Rules"]; !ok {
		t.Error("expected 'Remote-Only Rules' field in Slack attachment")
	}
	if _, ok := fieldMap["Local-Only Rules"]; !ok {
		t.Error("expected 'Local-Only Rules' field in Slack attachment")
	}
}

func TestNotifyDrift_Discord(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: notification.WebhookTypeDiscord,
	})

	diffs := sampleDiffs()
	err := svc.NotifyDrift(context.Background(), "bariskode.com", diffs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload notification.DiscordPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal Discord payload: %v", err)
	}

	if len(payload.Embeds) == 0 {
		t.Fatal("expected Discord embeds to be present")
	}

	embed := payload.Embeds[0]
	if !strings.Contains(embed.Title, "bariskode.com") {
		t.Errorf("expected title to contain zone name, got: %s", embed.Title)
	}
	if embed.Color == 0 {
		t.Error("expected embed color to be set")
	}

	fieldMap := make(map[string]string)
	for _, f := range embed.Fields {
		fieldMap[f.Name] = f.Value
	}

	if val, ok := fieldMap["Changed Rules"]; !ok || !strings.Contains(val, "support@bariskode.com") {
		t.Errorf("expected 'Changed Rules' field with support@bariskode.com, got: %s", val)
	}
	if val, ok := fieldMap["Remote Rules (Remote-Only)"]; !ok || !strings.Contains(val, "sales@bariskode.com") {
		t.Errorf("expected 'Remote Rules (Remote-Only)' field with sales@bariskode.com, got: %s", val)
	}
	if val, ok := fieldMap["Local Rules (Local-Only)"]; !ok || !strings.Contains(val, "catch-all") {
		t.Errorf("expected 'Local Rules (Local-Only)' field with catch-all, got: %s", val)
	}
}

func TestNotifyDrift_Telegram(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:     ts.URL,
		WebhookType:    notification.WebhookTypeTelegram,
		TelegramChatID: "-100123456789",
	})

	diffs := sampleDiffs()
	err := svc.NotifyDrift(context.Background(), "bariskode.com", diffs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload notification.TelegramPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal Telegram payload: %v", err)
	}

	if payload.ChatID != "-100123456789" {
		t.Errorf("expected chat_id '-100123456789', got '%s'", payload.ChatID)
	}
	if payload.ParseMode != "Markdown" {
		t.Errorf("expected parse_mode 'Markdown', got '%s'", payload.ParseMode)
	}
	if !strings.Contains(payload.Text, "bariskode.com") {
		t.Errorf("expected text to contain zone name, got: %s", payload.Text)
	}
	if !strings.Contains(payload.Text, "support@bariskode.com") {
		t.Errorf("expected text to contain changed rule identifier, got: %s", payload.Text)
	}
	if !strings.Contains(payload.Text, "sales@bariskode.com") {
		t.Errorf("expected text to contain remote rule identifier, got: %s", payload.Text)
	}
	if !strings.Contains(payload.Text, "catch-all") {
		t.Errorf("expected text to contain local rule identifier, got: %s", payload.Text)
	}
}

func TestNotifyDestinationVerified(t *testing.T) {
	tests := []struct {
		webhookType string
		verifyBody  func(t *testing.T, body []byte)
	}{
		{
			webhookType: notification.WebhookTypeGeneric,
			verifyBody: func(t *testing.T, body []byte) {
				var p notification.GenericDestinationPayload
				if err := json.Unmarshal(body, &p); err != nil {
					t.Fatalf("failed to unmarshal generic payload: %v", err)
				}
				if p.Event != "destination_verified" {
					t.Errorf("expected event 'destination_verified', got '%s'", p.Event)
				}
				if p.Email != "alerts@bariskode.com" {
					t.Errorf("expected email 'alerts@bariskode.com', got '%s'", p.Email)
				}
				if p.Timestamp == "" {
					t.Error("expected non-empty timestamp")
				}
			},
		},
		{
			webhookType: notification.WebhookTypeSlack,
			verifyBody: func(t *testing.T, body []byte) {
				var p notification.SlackPayload
				if err := json.Unmarshal(body, &p); err != nil {
					t.Fatalf("failed to unmarshal slack payload: %v", err)
				}
				if !strings.Contains(p.Text, "alerts@bariskode.com") {
					t.Errorf("expected slack text to mention email, got '%s'", p.Text)
				}
				if len(p.Blocks) == 0 {
					t.Error("expected slack blocks to be present")
				}
			},
		},
		{
			webhookType: notification.WebhookTypeDiscord,
			verifyBody: func(t *testing.T, body []byte) {
				var p notification.DiscordPayload
				if err := json.Unmarshal(body, &p); err != nil {
					t.Fatalf("failed to unmarshal discord payload: %v", err)
				}
				if len(p.Embeds) == 0 {
					t.Fatal("expected discord embeds")
				}
				if !strings.Contains(p.Embeds[0].Description, "alerts@bariskode.com") {
					t.Errorf("expected discord embed to mention email, got '%s'", p.Embeds[0].Description)
				}
			},
		},
		{
			webhookType: notification.WebhookTypeTelegram,
			verifyBody: func(t *testing.T, body []byte) {
				var p notification.TelegramPayload
				if err := json.Unmarshal(body, &p); err != nil {
					t.Fatalf("failed to unmarshal telegram payload: %v", err)
				}
				if p.ChatID != "chat_123" {
					t.Errorf("expected chat_id 'chat_123', got '%s'", p.ChatID)
				}
				if p.ParseMode != "Markdown" {
					t.Errorf("expected parse_mode 'Markdown', got '%s'", p.ParseMode)
				}
				if !strings.Contains(p.Text, "alerts@bariskode.com") {
					t.Errorf("expected telegram text to mention email, got '%s'", p.Text)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.webhookType, func(t *testing.T) {
			var receivedBody []byte
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			svc := notification.NewService(notification.Config{
				WebhookURL:     ts.URL,
				WebhookType:    tc.webhookType,
				TelegramChatID: "chat_123",
			})

			err := svc.NotifyDestinationVerified(context.Background(), "alerts@bariskode.com")
			if err != nil {
				t.Fatalf("unexpected error from NotifyDestinationVerified: %v", err)
			}

			tc.verifyBody(t, receivedBody)
		})
	}
}

func TestNotifyDrift_NilDiffs(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: notification.WebhookTypeGeneric,
	})

	err := svc.NotifyDrift(context.Background(), "example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload notification.GenericDriftPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if payload.Diffs == nil || len(payload.Diffs) != 0 {
		t.Errorf("expected empty non-nil slice, got: %v", payload.Diffs)
	}
}

func TestService_MissingWebhookURL(t *testing.T) {
	svc := notification.NewService(notification.Config{
		WebhookURL: "",
	})

	errDrift := svc.NotifyDrift(context.Background(), "example.com", nil)
	if !errors.Is(errDrift, notification.ErrWebhookURLMissing) {
		t.Errorf("expected ErrWebhookURLMissing, got: %v", errDrift)
	}

	errDest := svc.NotifyDestinationVerified(context.Background(), "dest@example.com")
	if !errors.Is(errDest, notification.ErrWebhookURLMissing) {
		t.Errorf("expected ErrWebhookURLMissing, got: %v", errDest)
	}
}

func TestService_UnsupportedWebhookType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: "unsupported_channel",
	})

	err := svc.NotifyDrift(context.Background(), "example.com", nil)
	if !errors.Is(err, notification.ErrUnsupportedWebhookType) {
		t.Errorf("expected ErrUnsupportedWebhookType, got: %v", err)
	}

	errDest := svc.NotifyDestinationVerified(context.Background(), "dest@example.com")
	if !errors.Is(errDest, notification.ErrUnsupportedWebhookType) {
		t.Errorf("expected ErrUnsupportedWebhookType, got: %v", errDest)
	}
}

func TestService_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server crash"))
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: notification.WebhookTypeGeneric,
	})

	err := svc.NotifyDrift(context.Background(), "example.com", sampleDiffs())
	if err == nil {
		t.Fatal("expected error on HTTP 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error message to contain '500', got: %v", err)
	}
}

func TestService_ClientTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: notification.WebhookTypeGeneric,
		Timeout:     20 * time.Millisecond,
	})

	err := svc.NotifyDrift(context.Background(), "example.com", sampleDiffs())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestService_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL,
		WebhookType: notification.WebhookTypeGeneric,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := svc.NotifyDrift(ctx, "example.com", sampleDiffs())
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

func TestService_DefaultGenericWebhookType(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL: ts.URL,
		// WebhookType omitted -> defaults to generic
	})

	err := svc.NotifyDrift(context.Background(), "example.com", sampleDiffs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p notification.GenericDriftPayload
	if err := json.Unmarshal(receivedBody, &p); err != nil {
		t.Fatalf("failed to unmarshal payload as generic: %v", err)
	}
	if p.Event != "drift_detected" {
		t.Errorf("expected event 'drift_detected', got: %s", p.Event)
	}
}

func TestService_TelegramChatIDFromURL(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc := notification.NewService(notification.Config{
		WebhookURL:  ts.URL + "?chat_id=query_chat_42",
		WebhookType: notification.WebhookTypeTelegram,
		// TelegramChatID omitted, should be extracted from query param
	})

	err := svc.NotifyDestinationVerified(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p notification.TelegramPayload
	if err := json.Unmarshal(receivedBody, &p); err != nil {
		t.Fatalf("failed to unmarshal telegram payload: %v", err)
	}
	if p.ChatID != "query_chat_42" {
		t.Errorf("expected chat_id 'query_chat_42', got: '%s'", p.ChatID)
	}
}
