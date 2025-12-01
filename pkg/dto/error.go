package dto

// ErrorCode represents categorized error types for programmatic handling.
type ErrorCode string

const (
	// ErrorCodeInvalidPayload indicates JSON parsing or schema validation failed.
	ErrorCodeInvalidPayload ErrorCode = "INVALID_PAYLOAD"
	// ErrorCodeValidationError indicates business validation failed (e.g., invalid UUID format).
	ErrorCodeValidationError ErrorCode = "VALIDATION_ERROR"
	// ErrorCodeNotFound indicates the referenced entity does not exist.
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrorCodePermissionDenied indicates the operation is not permitted.
	ErrorCodePermissionDenied ErrorCode = "PERMISSION_DENIED"
	// ErrorCodeMatrixError indicates a Matrix SDK/homeserver error.
	ErrorCodeMatrixError ErrorCode = "MATRIX_ERROR"
	// ErrorCodeInternalError indicates an unexpected system error.
	ErrorCodeInternalError ErrorCode = "INTERNAL_ERROR"
)

// ErrorResponse contains structured error information.
type ErrorResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
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

// NewSuccessResponse creates a success response.
func NewSuccessResponse() BaseResponse {
	return BaseResponse{Success: true}
}
