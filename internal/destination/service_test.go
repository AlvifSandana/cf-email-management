package destination

import (
	"context"
	"testing"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider/mock"
	"github.com/bariskode/email-management-service/internal/storage/memory"
)

func TestDestinationService(t *testing.T) {
	ctx := context.Background()
	repos := memory.New()
	prov := mock.New()
	auditService := audit.NewService(repos.Audit)
	destService := NewService(repos.Destinations, prov, auditService)

	accountID := "acc_123"

	// 1. Invalid email
	_, err := destService.Create(ctx, accountID, "not-an-email", "req_1")
	if err == nil {
		t.Fatalf("expected validation error for invalid email")
	}

	// 2. Valid create
	dest, err := destService.Create(ctx, accountID, "forward@example.com", "req_2")
	if err != nil {
		t.Fatalf("failed to create destination: %v", err)
	}
	if dest.Email != "forward@example.com" {
		t.Fatalf("expected email forward@example.com, got %s", dest.Email)
	}

	// 3. List
	list, err := destService.List(ctx, accountID)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 destination in list, got %d", len(list))
	}

	// 4. Update
	updated, err := destService.Update(ctx, dest.ID, "updated@example.com", "req_3")
	if err != nil {
		t.Fatalf("failed to update destination: %v", err)
	}
	if updated.Email != "updated@example.com" {
		t.Fatalf("expected updated@example.com, got %s", updated.Email)
	}

	// 5. Verification status
	// Initially pending
	verified, err := destService.VerifyDestination(ctx, accountID, "updated@example.com")
	if err != nil || verified {
		t.Fatalf("expected destination to not be verified initially")
	}

	// Mark verified
	updated.Status = domain.DestinationStatusVerified
	_ = repos.Destinations.Save(ctx, updated)

	verified, err = destService.VerifyDestination(ctx, accountID, "updated@example.com")
	if err != nil || !verified {
		t.Fatalf("expected destination to be verified now")
	}

	// 6. Delete
	err = destService.Delete(ctx, dest.ID, "req_4")
	if err != nil {
		t.Fatalf("failed to delete destination: %v", err)
	}

	_, err = destService.Get(ctx, dest.ID)
	if err == nil {
		t.Fatalf("expected destination to be deleted")
	}
}
