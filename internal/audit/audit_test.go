package audit

import (
	"context"
	"strings"
	"testing"

	"github.com/bariskode/email-management-service/internal/storage/memory"
)

func TestAuditService_RedactsSecrets(t *testing.T) {
	repos := memory.New()
	auditService := NewService(repos.Audit)
	ctx := context.Background()

	sensitivePayload := map[string]any{
		"name":       "Test Account",
		"api_token":  "super_secret_token_12345",
		"password":   "super_secret_password",
		"credential": "key_123",
		"safe_field": "visible_value",
	}

	err := auditService.Record(ctx, RecordParams{
		Operation:    "CREATE_ACCOUNT",
		ResourceType: "account",
		ResourceID:   "acc_123",
		RequestID:    "req_test",
		After:        sensitivePayload,
		Status:       "SUCCESS",
	})
	if err != nil {
		t.Fatalf("failed to record audit event: %v", err)
	}

	events, err := auditService.List(ctx, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("failed to list audit events: %v", err)
	}

	event := events[0]
	if strings.Contains(event.AfterJSON, "super_secret_token_12345") {
		t.Errorf("audit log leaked api_token plaintext: %s", event.AfterJSON)
	}
	if strings.Contains(event.AfterJSON, "super_secret_password") {
		t.Errorf("audit log leaked password plaintext: %s", event.AfterJSON)
	}
	if !strings.Contains(event.AfterJSON, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in audit json: %s", event.AfterJSON)
	}
	if !strings.Contains(event.AfterJSON, "visible_value") {
		t.Errorf("expected safe_field to remain visible in audit json: %s", event.AfterJSON)
	}
}
