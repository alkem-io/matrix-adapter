package queue

import (
	"strings"

	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// NewInvalidPayloadError creates an error response for JSON unmarshal errors.
func NewInvalidPayloadError(err error) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrorCodeInvalidPayload, err.Error())
}

// NewValidationError creates an error response for validation errors (e.g., UUID parse errors).
func NewValidationError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrorCodeValidationError, msg)
}

// MapServiceError maps service layer errors to appropriate error responses.
// It checks error message content to determine the error code.
// Matrix SDK-specific error extraction should be handled in internal/infrastructure/matrix.
func MapServiceError(err error) dto.BaseResponse {
	if err == nil {
		return dto.NewSuccessResponse()
	}

	msg := err.Error()
	msgLower := strings.ToLower(msg)

	// Map by error message patterns
	switch {
	case strings.Contains(msgLower, "not found"):
		return dto.NewErrorResponse(dto.ErrorCodeNotFound, msg)
	case strings.Contains(msgLower, "forbidden") || strings.Contains(msgLower, "permission"):
		return dto.NewErrorResponse(dto.ErrorCodePermissionDenied, msg)
	default:
		return dto.NewErrorResponse(dto.ErrorCodeMatrixError, msg)
	}
}
