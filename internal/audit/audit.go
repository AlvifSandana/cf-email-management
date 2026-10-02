package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/auth"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/storage"
)

// Service handles recording and querying audit events.
type Service struct {
	repo storage.AuditRepository
}

// NewService creates an Audit Service.
func NewService(repo storage.AuditRepository) *Service {
	return &Service{repo: repo}
}

// RecordParams contains values for an audit log entry.
type RecordParams struct {
	Operation        string
	ResourceType     string
	ResourceID       string
	RequestID        string
	Before           any
	After            any
	ProviderResponse any
	Status           string
	ErrorCode        string
}

// Record captures and persists a sanitized audit event.
func (s *Service) Record(ctx context.Context, p RecordParams) error {
	actorType, actorID := auth.GetActor(ctx)

	beforeJSON := sanitizeAndJSON(p.Before)
	afterJSON := sanitizeAndJSON(p.After)
	providerRespJSON := sanitizeAndJSON(p.ProviderResponse)

	status := p.Status
	if status == "" {
		status = "SUCCESS"
	}

	event := &domain.AuditEvent{
		ID:                   generateID("aud"),
		ActorType:            actorType,
		ActorID:              actorID,
		Operation:            p.Operation,
		ResourceType:         p.ResourceType,
		ResourceID:           p.ResourceID,
		RequestID:            p.RequestID,
		BeforeJSON:           beforeJSON,
		AfterJSON:            afterJSON,
		ProviderResponseJSON: providerRespJSON,
		Status:               status,
		ErrorCode:            p.ErrorCode,
		CreatedAt:            time.Now().UTC(),
	}

	return s.repo.Record(ctx, event)
}

// List returns recent audit events.
func (s *Service) List(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	return s.repo.List(ctx, limit)
}

func sanitizeAndJSON(v any) string {
	if v == nil {
		return ""
	}
	bytes, err := json.Marshal(v)
	if err != nil {
		return ""
	}

	var parsed any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		return ""
	}

	sanitized := sanitizeNode(parsed)
	out, err := json.Marshal(sanitized)
	if err != nil {
		return ""
	}
	return string(out)
}

func sanitizeNode(node any) any {
	switch v := node.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(v))
		for k, val := range v {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "token") ||
				strings.Contains(lower, "secret") ||
				strings.Contains(lower, "credential") ||
				strings.Contains(lower, "password") ||
				strings.Contains(lower, "key") {
				cleaned[k] = "[REDACTED]"
			} else {
				cleaned[k] = sanitizeNode(val)
			}
		}
		return cleaned
	case []any:
		cleaned := make([]any, len(v))
		for i, item := range v {
			cleaned[i] = sanitizeNode(item)
		}
		return cleaned
	default:
		return v
	}
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
