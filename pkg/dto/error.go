package dto

// ErrorCode represents categorized error types for programmatic handling.
type ErrorCode string

const (
	// ErrCodeInvalidParam indicates request validation failed (invalid payload or parameters).
	ErrCodeInvalidParam ErrorCode = "INVALID_PARAM"
	// ErrCodeRoomNotFound indicates the referenced room does not exist.
	ErrCodeRoomNotFound ErrorCode = "ROOM_NOT_FOUND"
	// ErrCodeSpaceNotFound indicates the referenced space does not exist.
	ErrCodeSpaceNotFound ErrorCode = "SPACE_NOT_FOUND"
	// ErrCodeActorNotFound indicates the referenced actor does not exist.
	ErrCodeActorNotFound ErrorCode = "ACTOR_NOT_FOUND"
	// ErrCodeMatrixError indicates a Matrix SDK/homeserver error.
	ErrCodeMatrixError ErrorCode = "MATRIX_ERROR"
	// ErrCodeInternalError indicates an unexpected system error.
	ErrCodeInternalError ErrorCode = "INTERNAL_ERROR"
	// ErrCodeNotAllowed indicates the operation is not permitted.
	ErrCodeNotAllowed ErrorCode = "NOT_ALLOWED"
)

// ErrorResponse contains structured error information.
type ErrorResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"` // Optional technical details
}

// BaseResponse is the standard response structure for all command responses.
// Operations that return only status use this directly.
// Operations with additional data embed this struct.
type BaseResponse struct {
	Success bool           `json:"success"`
	Error   *ErrorResponse `json:"error,omitempty"`
}

// NewErrorResponse creates an error response with the given code and message.
func NewErrorResponse(code ErrorCode, message string) BaseResponse {
	return BaseResponse{
		Success: false,
		Error: &ErrorResponse{
			Code:    code,
			Message: message,
		},
	}
}

// NewErrorResponseWithDetails creates an error response with code, message, and details.
func NewErrorResponseWithDetails(code ErrorCode, message, details string) BaseResponse {
	return BaseResponse{
		Success: false,
		Error: &ErrorResponse{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}

// NewSuccessResponse creates a success response.
func NewSuccessResponse() BaseResponse {
	return BaseResponse{Success: true}
}
