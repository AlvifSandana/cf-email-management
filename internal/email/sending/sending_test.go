package sending

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
)

func TestSend_Success(t *testing.T) {
	accountID := "acc_123"
	expectedToken := "test_token_secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		expectedPath := "/accounts/" + accountID + "/email/sending/send"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+expectedToken {
			t.Errorf("expected Authorization header 'Bearer %s', got '%s'", expectedToken, auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var payload cfSendPayload
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}

		if payload.From != "sender@example.com" {
			t.Errorf("expected from sender@example.com, got %s", payload.From)
		}
		if len(payload.To) != 2 || payload.To[0] != "to1@example.com" || payload.To[1] != "to2@example.com" {
			t.Errorf("unexpected to recipients: %v", payload.To)
		}
		if payload.Subject != "Test Email" {
			t.Errorf("expected subject 'Test Email', got '%s'", payload.Subject)
		}
		if payload.Text != "Hello text" {
			t.Errorf("expected text 'Hello text', got '%s'", payload.Text)
		}
		if payload.HTML != "<p>Hello HTML</p>" {
			t.Errorf("expected html '<p>Hello HTML</p>', got '%s'", payload.HTML)
		}
		if payload.Headers["X-Custom-ID"] != "custom_123" {
			t.Errorf("expected header X-Custom-ID 'custom_123', got '%s'", payload.Headers["X-Custom-ID"])
		}

		successResp := cfResponse{
			Success:  boolPtr(true),
			Errors:   []cfError{},
			Messages: []string{},
			Result: json.RawMessage(`{
				"message_id": "msg_success_001",
				"delivered": ["to1@example.com", "to2@example.com"],
				"permanent_bounces": [],
				"queued": []
			}`),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(successResp)
	}))
	defer server.Close()

	client := NewClient(expectedToken, WithBaseURL(server.URL))
	req := SendEmailRequest{
		From:     "sender@example.com",
		To:       []string{"to1@example.com", "to2@example.com"},
		Subject:  "Test Email",
		TextBody: "Hello text",
		HTMLBody: "<p>Hello HTML</p>",
		Headers: map[string]string{
			"X-Custom-ID": "custom_123",
		},
	}

	resp, err := client.Send(context.Background(), accountID, req)
	if err != nil {
		t.Fatalf("expected send to succeed, got error: %v", err)
	}

	if resp.MessageID != "msg_success_001" {
		t.Errorf("expected MessageID 'msg_success_001', got '%s'", resp.MessageID)
	}
	if resp.Status != "delivered" {
		t.Errorf("expected Status 'delivered', got '%s'", resp.Status)
	}
	if len(resp.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", resp.Errors)
	}
}

func TestSend_Success_Queued(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse{
			Success: boolPtr(true),
			Result: json.RawMessage(`{
				"message_id": "msg_queued_002",
				"delivered": [],
				"queued": ["queued@example.com"]
			}`),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	req := SendEmailRequest{
		From:     "sender@example.com",
		To:       []string{"queued@example.com"},
		Subject:  "Queued Subject",
		TextBody: "Queued text",
	}

	resp, err := client.Send(context.Background(), "acc_123", req)
	if err != nil {
		t.Fatalf("expected send to succeed, got: %v", err)
	}
	if resp.Status != "queued" {
		t.Errorf("expected Status 'queued', got '%s'", resp.Status)
	}
	if resp.MessageID != "msg_queued_002" {
		t.Errorf("expected MessageID 'msg_queued_002', got '%s'", resp.MessageID)
	}
}

func TestSend_Success_WithBouncesAndSuppression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cfResponse{
			Success: boolPtr(true),
			Result: json.RawMessage(`{
				"message_id": "msg_partial_003",
				"delivered": ["valid@example.com"],
				"permanent_bounces": ["bounce@example.com"],
				"suppressed_recipients": ["suppressed@example.com"]
			}`),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	req := SendEmailRequest{
		From:    "sender@example.com",
		To:      []string{"valid@example.com", "bounce@example.com", "suppressed@example.com"},
		Subject: "Mixed delivery",
	}

	resp, err := client.Send(context.Background(), "acc_123", req)
	if err != nil {
		t.Fatalf("expected success with bounce details, got error: %v", err)
	}

	if resp.MessageID != "msg_partial_003" {
		t.Errorf("expected MessageID 'msg_partial_003', got '%s'", resp.MessageID)
	}
	if resp.Status != "delivered" {
		t.Errorf("expected Status 'delivered', got '%s'", resp.Status)
	}
	if len(resp.Errors) != 2 {
		t.Fatalf("expected 2 errors in response, got %d: %v", len(resp.Errors), resp.Errors)
	}
}

func TestSend_LocalValidation(t *testing.T) {
	client := NewClient("token", WithBaseURL("http://localhost"))

	tests := []struct {
		name      string
		accountID string
		req       SendEmailRequest
		errMsg    string
	}{
		{
			name:      "missing accountID",
			accountID: "",
			req: SendEmailRequest{
				From: "a@b.com",
				To:   []string{"c@d.com"},
			},
			errMsg: "accountID is required",
		},
		{
			name:      "missing from",
			accountID: "acc_123",
			req: SendEmailRequest{
				From: "",
				To:   []string{"c@d.com"},
			},
			errMsg: "from address is required",
		},
		{
			name:      "missing to",
			accountID: "acc_123",
			req: SendEmailRequest{
				From: "a@b.com",
				To:   []string{},
			},
			errMsg: "at least one recipient (to) is required",
		},
		{
			name:      "empty to recipient string",
			accountID: "acc_123",
			req: SendEmailRequest{
				From: "a@b.com",
				To:   []string{""},
			},
			errMsg: "recipient address cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Send(context.Background(), tt.accountID, tt.req)
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			var appErr *domain.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected error to be *domain.AppError, got %T (%v)", err, err)
			}
			if appErr.Code != domain.ErrCodeValidationFailed {
				t.Errorf("expected code %s, got %s", domain.ErrCodeValidationFailed, appErr.Code)
			}
			if appErr.HTTPStatus != 400 {
				t.Errorf("expected HTTPStatus 400, got %d", appErr.HTTPStatus)
			}
		})
	}
}

func TestSend_Error_InvalidSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := cfResponse{
			Success: boolPtr(false),
			Errors: []cfError{
				{Code: 10001, Message: "email.sending.error.invalid_request_schema"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeValidationFailed {
		t.Errorf("expected code %s, got %s", domain.ErrCodeValidationFailed, appErr.Code)
	}
	if appErr.HTTPStatus != 400 {
		t.Errorf("expected HTTPStatus 400, got %d", appErr.HTTPStatus)
	}
}

func TestSend_Error_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := cfResponse{
			Success: boolPtr(false),
			Errors: []cfError{
				{Code: 10101, Message: "email.sending.error.authentication.unauthorized"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("bad_token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeUnauthorized {
		t.Errorf("expected code %s, got %s", domain.ErrCodeUnauthorized, appErr.Code)
	}
	if appErr.HTTPStatus != 401 {
		t.Errorf("expected HTTPStatus 401, got %d", appErr.HTTPStatus)
	}
}

func TestSend_Error_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		resp := cfResponse{
			Success: boolPtr(false),
			Errors: []cfError{
				{Code: 10102, Message: "email.sending.error.authentication.forbidden"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("limited_token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeUnauthorized {
		t.Errorf("expected code %s, got %s", domain.ErrCodeUnauthorized, appErr.Code)
	}
}

func TestSend_Error_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		resp := cfResponse{
			Success: boolPtr(false),
			Errors: []cfError{
				{Code: 10000, Message: "email.sending.error.not_found"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "invalid_acc", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeNotFound {
		t.Errorf("expected code %s, got %s", domain.ErrCodeNotFound, appErr.Code)
	}
	if appErr.HTTPStatus != 404 {
		t.Errorf("expected HTTPStatus 404, got %d", appErr.HTTPStatus)
	}
}

func TestSend_Error_DestinationNotVerified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := cfResponse{
			Success: boolPtr(false),
			Errors: []cfError{
				{Code: 10202, Message: "destination address unverified: external@example.com not verified"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"external@example.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeDestinationNotVerified {
		t.Errorf("expected code %s, got %s", domain.ErrCodeDestinationNotVerified, appErr.Code)
	}
	if appErr.HTTPStatus != 400 {
		t.Errorf("expected HTTPStatus 400, got %d", appErr.HTTPStatus)
	}
}

func TestSend_Error_ProviderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		resp := cfResponse{
			Success: boolPtr(false),
			Errors: []cfError{
				{Code: 504, Message: "gateway timeout from upstream"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeProviderTimeout {
		t.Errorf("expected code %s, got %s", domain.ErrCodeProviderTimeout, appErr.Code)
	}
	if appErr.HTTPStatus != 504 {
		t.Errorf("expected HTTPStatus 504, got %d", appErr.HTTPStatus)
	}
}

func TestSend_Error_ProviderInternalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		resp := cfResponse{
			Success: boolPtr(false),
			Errors: []cfError{
				{Code: 10002, Message: "email.sending.error.internal_server"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeProviderError {
		t.Errorf("expected code %s, got %s", domain.ErrCodeProviderError, appErr.Code)
	}
	if appErr.HTTPStatus != 502 {
		t.Errorf("expected HTTPStatus 502, got %d", appErr.HTTPStatus)
	}
}

func TestSend_Error_ContextDeadlineExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Send(ctx, "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected context deadline error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T (%v)", err, err)
	}
	if appErr.Code != domain.ErrCodeProviderTimeout {
		t.Errorf("expected code %s, got %s", domain.ErrCodeProviderTimeout, appErr.Code)
	}
}

func TestSend_Error_NonJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway from Cloudflare Edge</body></html>"))
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	_, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.ErrCodeProviderError {
		t.Errorf("expected code %s, got %s", domain.ErrCodeProviderError, appErr.Code)
	}
}

func TestSend_DirectDTOResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message_id": "direct_msg_999", "status": "sent"}`))
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	resp, err := client.Send(context.Background(), "acc_123", SendEmailRequest{
		From: "a@b.com",
		To:   []string{"c@d.com"},
	})
	if err != nil {
		t.Fatalf("expected direct response to succeed, got: %v", err)
	}
	if resp.MessageID != "direct_msg_999" {
		t.Errorf("expected MessageID 'direct_msg_999', got '%s'", resp.MessageID)
	}
	if resp.Status != "sent" {
		t.Errorf("expected Status 'sent', got '%s'", resp.Status)
	}
}

func TestClient_OptionsAndGetters(t *testing.T) {
	customHC := &http.Client{Timeout: 5 * time.Second}
	c := NewClient("my_token",
		WithBaseURL("https://custom.api.cloudflare.com/"),
		WithHTTPClient(customHC),
	)

	if c.BaseURL() != "https://custom.api.cloudflare.com" {
		t.Errorf("expected base URL https://custom.api.cloudflare.com, got %s", c.BaseURL())
	}
	if c.APIToken() != "my_token" {
		t.Errorf("expected token my_token, got %s", c.APIToken())
	}
	if c.HTTPClient() != customHC {
		t.Errorf("expected custom HTTP client")
	}

	defaultClient := NewClient("tok")
	if defaultClient.BaseURL() != DefaultBaseURL {
		t.Errorf("expected default base URL %s, got %s", DefaultBaseURL, defaultClient.BaseURL())
	}
	if defaultClient.HTTPClient() == nil {
		t.Errorf("expected default HTTPClient to be initialized")
	}
}

func TestSendEmailRequest_UnmarshalJSON(t *testing.T) {
	jsonData := `{"from":"a@b.com","to":["c@d.com"],"subject":"Hello","text":"plain body","html":"<h1>html body</h1>"}`
	var req SendEmailRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.TextBody != "plain body" {
		t.Errorf("expected TextBody 'plain body', got '%s'", req.TextBody)
	}
	if req.HTMLBody != "<h1>html body</h1>" {
		t.Errorf("expected HTMLBody '<h1>html body</h1>', got '%s'", req.HTMLBody)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
