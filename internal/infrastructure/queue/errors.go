package queue

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// ============================================================================
// Error Response Helpers
// ============================================================================

// NewInvalidParamError creates an error response for invalid parameter errors.
func NewInvalidParamError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeInvalidParam, msg)
}

// NewRoomNotFoundError creates an error response for room not found errors.
func NewRoomNotFoundError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, msg)
}

// NewMessageNotFoundError creates an error response for message not found errors.
func NewMessageNotFoundError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeMessageNotFound, msg)
}

// NewReactionNotFoundError creates an error response for reaction not found errors.
func NewReactionNotFoundError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeReactionNotFound, msg)
}

// NewInvalidPayloadError creates an error response for JSON unmarshal errors.
func NewInvalidPayloadError(err error) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeInvalidParam, "invalid payload: "+err.Error())
}

// ============================================================================
// UUID Validation Helpers
// ============================================================================

// UUIDValidator is an interface for types that can return a UUID.
type UUIDValidator interface {
	// UUID returns the UUID value of the implementing type.
	UUID() uuid.UUID
}

// RequireUUID validates that a UUID field is not nil/zero and returns an error response if invalid.
// Returns nil if validation passes, or a pointer to an error response if validation fails.
func RequireUUID(val UUIDValidator, fieldName string) *dto.BaseResponse {
	if val.UUID() == uuid.Nil {
		resp := NewInvalidParamError(fmt.Sprintf("%s is required", fieldName))
		return &resp
	}
	return nil
}

// RequireNonEmpty validates that a string field is not empty.
// Returns nil if validation passes, or a pointer to an error response if validation fails.
func RequireNonEmpty(val, fieldName string) *dto.BaseResponse {
	if val == "" {
		resp := NewInvalidParamError(fmt.Sprintf("%s is required", fieldName))
		return &resp
	}
	return nil
}

// ============================================================================
// Service Error Mapping
// ============================================================================

// MapServiceError maps service layer errors to appropriate error responses.
// Uses typed domain errors first, falls back to string matching for legacy/Matrix SDK errors.
func MapServiceError(err error) dto.BaseResponse {
	if err == nil {
		return dto.NewSuccessResponse()
	}

	// Check typed domain errors first (preferred)
	switch {
	case errors.Is(err, domain.ErrSpaceNotFound):
		return dto.NewErrorResponse(dto.ErrCodeSpaceNotFound, "Space not found")
	case errors.Is(err, domain.ErrParentNotFound):
		return dto.NewErrorResponse(dto.ErrCodeSpaceNotFound, "Parent space not found")
	case errors.Is(err, domain.ErrChildNotFound):
		return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, "Child room or space not found")
	case errors.Is(err, domain.ErrRoomNotFound):
		return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, "Room not found")
	case errors.Is(err, domain.ErrForbidden):
		return dto.NewErrorResponse(dto.ErrCodeNotAllowed, err.Error())
	case errors.Is(err, domain.ErrInvalidParam):
		return dto.NewErrorResponse(dto.ErrCodeInvalidParam, err.Error())
	}

	// Fall back to string matching for Matrix SDK errors and legacy patterns
	msg := err.Error()
	msgLower := strings.ToLower(msg)

	switch {
	// Permission patterns
	case domain.IsForbiddenError(err):
		return dto.NewErrorResponse(dto.ErrCodeNotAllowed, msg)
	// Generic not found (catch-all)
	case domain.IsNotFoundError(err):
		return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, msg)
	// Validation patterns
	case strings.Contains(msgLower, "invalid"):
		return dto.NewErrorResponse(dto.ErrCodeInvalidParam, msg)
	// Default to Matrix error
	default:
		return dto.NewErrorResponse(dto.ErrCodeMatrixError, msg)
	}
}

// ============================================================================
// Batch Operation Result Mapping
// ============================================================================

// MapToBatchResult maps an error to a BaseResponse for batch operations.
// This is an alias for MapServiceError, provided for semantic clarity in batch contexts.
func MapToBatchResult(err error) dto.BaseResponse {
	return MapServiceError(err)
}
