package domain

import (
	"errors"
	"fmt"
)

// Standard EMS Error Codes
const (
	ErrCodeNotFound               = "NOT_FOUND"
	ErrCodeAlreadyExists          = "ALREADY_EXISTS"
	ErrCodeDestinationNotVerified = "DESTINATION_NOT_VERIFIED"
	ErrCodeValidationFailed       = "VALIDATION_FAILED"
	ErrCodeProviderError          = "PROVIDER_ERROR"
	ErrCodeProviderTimeout        = "PROVIDER_TIMEOUT"
	ErrCodeUnauthorized           = "UNAUTHORIZED"
	ErrCodeForbidden              = "FORBIDDEN"
	ErrCodeConflict               = "CONFLICT"
	ErrCodeInternal               = "INTERNAL_ERROR"
)

// AppError represents a structured, client-facing application error.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Err        error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Common error constructors
func NewNotFoundError(resource string, id string) *AppError {
	return &AppError{
		Code:       ErrCodeNotFound,
		Message:    fmt.Sprintf("%s with id '%s' was not found", resource, id),
		HTTPStatus: 404,
	}
}

func NewValidationError(msg string) *AppError {
	return &AppError{
		Code:       ErrCodeValidationFailed,
		Message:    msg,
		HTTPStatus: 400,
	}
}

func NewDestinationNotVerifiedError(email string) *AppError {
	return &AppError{
		Code:       ErrCodeDestinationNotVerified,
		Message:    fmt.Sprintf("destination address '%s' has not been verified", email),
		HTTPStatus: 400,
	}
}

func NewProviderError(msg string, cause error) *AppError {
	return &AppError{
		Code:       ErrCodeProviderError,
		Message:    msg,
		HTTPStatus: 502,
		Err:        cause,
	}
}

func NewProviderTimeoutError(msg string, cause error) *AppError {
	return &AppError{
		Code:       ErrCodeProviderTimeout,
		Message:    msg,
		HTTPStatus: 504,
		Err:        cause,
	}
}

func NewUnauthorizedError(msg string) *AppError {
	return &AppError{
		Code:       ErrCodeUnauthorized,
		Message:    msg,
		HTTPStatus: 401,
	}
}

func NewForbiddenError(msg string) *AppError {
	return &AppError{
		Code:       ErrCodeForbidden,
		Message:    msg,
		HTTPStatus: 403,
	}
}

func NewConflictError(msg string) *AppError {
	return &AppError{
		Code:       ErrCodeConflict,
		Message:    msg,
		HTTPStatus: 409,
	}
}

func NewInternalError(msg string, cause error) *AppError {
	return &AppError{
		Code:       ErrCodeInternal,
		Message:    msg,
		HTTPStatus: 500,
		Err:        cause,
	}
}

// Sentinel domain errors for simple wrapping
var (
	ErrNotFound               = errors.New("resource not found")
	ErrAlreadyExists          = errors.New("resource already exists")
	ErrDestinationNotVerified = errors.New("destination address not verified")
	ErrZoneSyncLocked         = errors.New("zone synchronization already in progress")
)
