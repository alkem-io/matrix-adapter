package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"ErrNotFound", ErrNotFound, true},
		{"ErrRoomNotFound", ErrRoomNotFound, true},
		{"ErrSpaceNotFound", ErrSpaceNotFound, true},
		{"ErrActorNotFound", ErrActorNotFound, true},
		{"wrapped ErrNotFound", fmt.Errorf("wrapped: %w", ErrNotFound), true},
		{"Matrix M_NOT_FOUND", errors.New("M_NOT_FOUND: room not registered"), true},
		{"lowercase not found", errors.New("resource not found"), true},
		{"unrelated error", errors.New("connection refused"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsNotFoundError(tc.err)
			if result != tc.expected {
				t.Errorf("IsNotFoundError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestIsRoomNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"ErrRoomNotFound", ErrRoomNotFound, true},
		{"wrapped ErrRoomNotFound", fmt.Errorf("context: %w", ErrRoomNotFound), true},
		{"ErrSpaceNotFound", ErrSpaceNotFound, false},
		{"generic not found string", errors.New("room not found"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsRoomNotFoundError(tc.err)
			if result != tc.expected {
				t.Errorf("IsRoomNotFoundError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestIsForbiddenError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"ErrForbidden", ErrForbidden, true},
		{"wrapped ErrForbidden", fmt.Errorf("wrapped: %w", ErrForbidden), true},
		{"forbidden string", errors.New("Forbidden: user lacks permission"), true},
		{"permission denied string", errors.New("Permission denied for this resource"), true},
		{"not allowed string", errors.New("Operation not allowed"), true},
		{"unrelated error", errors.New("connection timeout"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsForbiddenError(tc.err)
			if result != tc.expected {
				t.Errorf("IsForbiddenError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestErrorConstructors(t *testing.T) {
	roomErr := NewRoomNotFoundError("room-123")
	if !errors.Is(roomErr, ErrRoomNotFound) {
		t.Error("NewRoomNotFoundError should wrap ErrRoomNotFound")
	}
	if !errors.Is(roomErr, ErrNotFound) {
		t.Error("NewRoomNotFoundError should be detectable as ErrNotFound")
	}

	spaceErr := NewSpaceNotFoundError("space-456")
	if !errors.Is(spaceErr, ErrSpaceNotFound) {
		t.Error("NewSpaceNotFoundError should wrap ErrSpaceNotFound")
	}

	actorErr := NewActorNotFoundError("actor-789")
	if !errors.Is(actorErr, ErrActorNotFound) {
		t.Error("NewActorNotFoundError should wrap ErrActorNotFound")
	}

	parentErr := NewParentNotFoundError("parent-abc")
	if !errors.Is(parentErr, ErrParentNotFound) {
		t.Error("NewParentNotFoundError should wrap ErrParentNotFound")
	}

	childErr := NewChildNotFoundError("child-def")
	if !errors.Is(childErr, ErrChildNotFound) {
		t.Error("NewChildNotFoundError should wrap ErrChildNotFound")
	}
}

func TestErrorHierarchy(t *testing.T) {
	specificErrors := []error{
		ErrRoomNotFound,
		ErrSpaceNotFound,
		ErrActorNotFound,
		ErrParentNotFound,
		ErrChildNotFound,
	}

	for _, err := range specificErrors {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%v should be detectable as ErrNotFound", err)
		}
	}
}
