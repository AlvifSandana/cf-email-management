package destination

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
	"github.com/bariskode/email-management-service/internal/storage"
)

// Service provides destination address business operations.
type Service struct {
	repo     storage.DestinationRepository
	provider provider.EmailProvider
	audit    *audit.Service
}

// NewService creates a new Destination Service.
func NewService(repo storage.DestinationRepository, provider provider.EmailProvider, audit *audit.Service) *Service {
	return &Service{
		repo:     repo,
		provider: provider,
		audit:    audit,
	}
}

// List returns all destinations for a given provider account.
func (s *Service) List(ctx context.Context, accountID string) ([]domain.DestinationAddress, error) {
	if accountID != "" {
		return s.repo.ListByAccount(ctx, accountID)
	}
	return s.repo.List(ctx)
}

// Get returns a single destination by its local ID.
func (s *Service) Get(ctx context.Context, id string) (*domain.DestinationAddress, error) {
	return s.repo.Get(ctx, id)
}

// Create creates a destination address in provider and stores it locally.
func (s *Service) Create(ctx context.Context, accountID, email, requestID string) (*domain.DestinationAddress, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid email address format: %s", email))
	}

	// Check if already exists locally
	existing, err := s.repo.GetByEmail(ctx, accountID, email)
	if err == nil && existing != nil {
		return existing, nil
	}

	// Call provider
	provDest, err := s.provider.CreateDestinationAddress(ctx, accountID, email)
	if err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "CREATE_DESTINATION",
			ResourceType:     "destination",
			ResourceID:       email,
			RequestID:        requestID,
			After:            map[string]string{"email": email, "account_id": accountID},
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return nil, err
	}

	now := time.Now().UTC()
	dest := &domain.DestinationAddress{
		ID:                generateID("dst"),
		ProviderAccountID: accountID,
		ProviderAddressID: provDest.ProviderAddressID,
		Email:             email,
		VerifiedAt:        provDest.VerifiedAt,
		Status:            provDest.Status,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.repo.Save(ctx, dest); err != nil {
		return nil, domain.NewInternalError("failed to save destination locally", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:        "CREATE_DESTINATION",
		ResourceType:     "destination",
		ResourceID:       dest.ID,
		RequestID:        requestID,
		After:            dest,
		ProviderResponse: provDest,
		Status:           "SUCCESS",
	})

	return dest, nil
}

// Update updates an existing destination.
func (s *Service) Update(ctx context.Context, id, email, requestID string) (*domain.DestinationAddress, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid email address format: %s", email))
	}

	dest, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	before := *dest
	provDest, err := s.provider.UpdateDestinationAddress(ctx, dest.ProviderAccountID, dest.ProviderAddressID, email)
	if err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "UPDATE_DESTINATION",
			ResourceType:     "destination",
			ResourceID:       id,
			RequestID:        requestID,
			Before:           before,
			After:            map[string]string{"email": email},
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return nil, err
	}

	dest.Email = email
	dest.Status = provDest.Status
	dest.VerifiedAt = provDest.VerifiedAt
	dest.UpdatedAt = time.Now().UTC()

	if err := s.repo.Save(ctx, dest); err != nil {
		return nil, domain.NewInternalError("failed to update destination locally", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:        "UPDATE_DESTINATION",
		ResourceType:     "destination",
		ResourceID:       dest.ID,
		RequestID:        requestID,
		Before:           before,
		After:            dest,
		ProviderResponse: provDest,
		Status:           "SUCCESS",
	})

	return dest, nil
}

// Delete deletes a destination address from provider and local storage.
func (s *Service) Delete(ctx context.Context, id, requestID string) error {
	dest, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.provider.DeleteDestinationAddress(ctx, dest.ProviderAccountID, dest.ProviderAddressID); err != nil {
		_ = s.audit.Record(ctx, audit.RecordParams{
			Operation:        "DELETE_DESTINATION",
			ResourceType:     "destination",
			ResourceID:       id,
			RequestID:        requestID,
			Before:           dest,
			Status:           "FAILED",
			ErrorCode:        domain.ErrCodeProviderError,
			ProviderResponse: err.Error(),
		})
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return domain.NewInternalError("failed to delete destination locally", err)
	}

	_ = s.audit.Record(ctx, audit.RecordParams{
		Operation:    "DELETE_DESTINATION",
		ResourceType: "destination",
		ResourceID:   id,
		RequestID:    requestID,
		Before:       dest,
		Status:       "SUCCESS",
	})

	return nil
}

// VerifyDestination checks if an email is verified for an account.
func (s *Service) VerifyDestination(ctx context.Context, accountID, email string) (bool, error) {
	d, err := s.repo.GetByEmail(ctx, accountID, email)
	if err != nil {
		return false, nil
	}
	return d.IsVerified(), nil
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
