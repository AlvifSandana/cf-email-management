package domain

import (
	"testing"
	"time"
)

func TestDestinationAddress_IsVerified(t *testing.T) {
	dest1 := DestinationAddress{
		Status: DestinationStatusPending,
	}
	if dest1.IsVerified() {
		t.Errorf("expected pending destination to be unverified")
	}

	dest2 := DestinationAddress{
		Status: DestinationStatusVerified,
	}
	if !dest2.IsVerified() {
		t.Errorf("expected verified status to be verified")
	}

	now := time.Now()
	dest3 := DestinationAddress{
		Status:     DestinationStatusPending,
		VerifiedAt: &now,
	}
	if !dest3.IsVerified() {
		t.Errorf("expected destination with VerifiedAt to be verified")
	}
}

func TestAppError(t *testing.T) {
	err := NewNotFoundError("rule", "rul_123")
	if err.HTTPStatus != 404 {
		t.Errorf("expected 404, got %d", err.HTTPStatus)
	}
	if err.Code != ErrCodeNotFound {
		t.Errorf("expected %s, got %s", ErrCodeNotFound, err.Code)
	}

	destErr := NewDestinationNotVerifiedError("ops@example.com")
	if destErr.HTTPStatus != 400 {
		t.Errorf("expected 400, got %d", destErr.HTTPStatus)
	}
	if destErr.Code != ErrCodeDestinationNotVerified {
		t.Errorf("expected %s, got %s", ErrCodeDestinationNotVerified, destErr.Code)
	}
}
