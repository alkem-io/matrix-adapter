package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ============================================================================
// Sentinel Errors - Single Source of Truth for Error Types
// ============================================================================

// ErrNotFound is the base error for all "not found" conditions.
var ErrNotFound = errors.New("not found")

// ErrRoomNotFound indicates a room does not exist.
var ErrRoomNotFound = fmt.Errorf("room %w", ErrNotFound)

// ErrSpaceNotFound indicates a space does not exist.
var ErrSpaceNotFound = fmt.Errorf("space %w", ErrNotFound)

// ErrParentNotFound indicates a parent space does not exist.
var ErrParentNotFound = fmt.Errorf("parent space %w", ErrNotFound)

// ErrChildNotFound indicates a child room/space does not exist.
var ErrChildNotFound = fmt.Errorf("child %w", ErrNotFound)

// ErrForbidden indicates an operation is not allowed.
var ErrForbidden = errors.New("forbidden")

// ErrInvalidParam indicates an invalid parameter.
var ErrInvalidParam = errors.New("invalid parameter")

// ============================================================================
// Error Detection Helpers
// ============================================================================

// IsNotFoundError checks if an error represents a "not found" condition.
// Works with both typed errors (errors.Is) and legacy string-based errors.
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check typed errors first
	if errors.Is(err, ErrNotFound) {
		return true
	}

	// Fall back to string matching for Matrix SDK errors
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "m_not_found")
}

// IsRoomNotFoundError checks if an error specifically indicates a room not found.
func IsRoomNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrRoomNotFound)
}

// IsForbiddenError checks if an error indicates a permission/access denial.
func IsForbiddenError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrForbidden) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "permission") ||
		strings.Contains(msg, "not allowed")
}

// ============================================================================
// Error Constructors - Wrap with Context
// ============================================================================

// wrapError wraps an error with an ID for context.
func wrapError(base error, id string) error {
	return fmt.Errorf("%w: %s", base, id)
}

// NewRoomNotFoundError creates a room not found error with context.
func NewRoomNotFoundError(roomID string) error {
	return wrapError(ErrRoomNotFound, roomID)
}

// NewSpaceNotFoundError creates a space not found error with context.
func NewSpaceNotFoundError(contextID string) error {
	return wrapError(ErrSpaceNotFound, contextID)
}

// NewParentNotFoundError creates a parent not found error with context.
func NewParentNotFoundError(parentID string) error {
	return wrapError(ErrParentNotFound, parentID)
}

// NewChildNotFoundError creates a child not found error with context.
func NewChildNotFoundError(childID string) error {
	return wrapError(ErrChildNotFound, childID)
}
