package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/observability"
)

type SuccessEnvelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

type Meta struct {
	RequestID string `json:"request_id"`
}

type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func respondJSON(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
	reqID := observability.GetRequestID(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := SuccessEnvelope{
		Data: data,
		Meta: Meta{
			RequestID: reqID,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func respondError(w http.ResponseWriter, r *http.Request, err error) {
	reqID := observability.GetRequestID(r.Context())
	w.Header().Set("Content-Type", "application/json")

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		w.WriteHeader(appErr.HTTPStatus)
		_ = json.NewEncoder(w).Encode(ErrorEnvelope{
			Error: ErrorDetail{
				Code:      appErr.Code,
				Message:   appErr.Message,
				RequestID: reqID,
			},
		})
		return
	}

	// Default unmapped error
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorDetail{
			Code:      domain.ErrCodeInternal,
			Message:   err.Error(),
			RequestID: reqID,
		},
	})
}
