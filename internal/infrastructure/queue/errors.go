package queue

import (
	"strings"

	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// NewInvalidParamError creates an error response for invalid parameter errors.
func NewInvalidParamError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeInvalidParam, msg)
}

// NewRoomNotFoundError creates an error response for room not found errors.
func NewRoomNotFoundError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, msg)
}

// NewActorNotFoundError creates an error response for actor not found errors.
func NewActorNotFoundError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeActorNotFound, msg)
}

// NewMatrixError creates an error response for Matrix SDK/homeserver errors.
func NewMatrixError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeMatrixError, msg)
}

// NewNotAllowedError creates an error response for not allowed operations.
func NewNotAllowedError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeNotAllowed, msg)
}

// NewInternalError creates an error response for internal errors.
func NewInternalError(msg string) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeInternalError, msg)
}

// NewInvalidPayloadError creates an error response for JSON unmarshal errors.
func NewInvalidPayloadError(err error) dto.BaseResponse {
	return dto.NewErrorResponse(dto.ErrCodeInvalidParam, "invalid payload: "+err.Error())
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
	case strings.Contains(msgLower, "room not found") || strings.Contains(msgLower, "resolve alias"):
		return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, msg)
	case strings.Contains(msgLower, "actor not found") || strings.Contains(msgLower, "user not found"):
		return dto.NewErrorResponse(dto.ErrCodeActorNotFound, msg)
	case strings.Contains(msgLower, "not found"):
		return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, msg)
	case strings.Contains(msgLower, "forbidden") || strings.Contains(msgLower, "permission") || strings.Contains(msgLower, "not allowed"):
		return dto.NewErrorResponse(dto.ErrCodeNotAllowed, msg)
	case strings.Contains(msgLower, "invalid"):
		return dto.NewErrorResponse(dto.ErrCodeInvalidParam, msg)
	default:
		return dto.NewErrorResponse(dto.ErrCodeMatrixError, msg)
	}
}

// MapToRoomOperationResult maps an error to a RoomOperationResult for batch operations.
func MapToRoomOperationResult(err error) dto.RoomOperationResult {
	if err == nil {
		return dto.RoomOperationResult{Success: true}
	}

	resp := MapServiceError(err)
	return dto.RoomOperationResult{
		Success: false,
		Error:   resp.Error,
	}
}
