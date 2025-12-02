package queue

import (
	"errors"
	"testing"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

func TestMapServiceError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedCode  dto.ErrorCode
		expectSuccess bool
	}{
		{
			name:          "nil error returns success",
			err:           nil,
			expectSuccess: true,
		},
		{
			name:         "ErrSpaceNotFound maps to SPACE_NOT_FOUND",
			err:          domain.ErrSpaceNotFound,
			expectedCode: dto.ErrCodeSpaceNotFound,
		},
		{
			name:         "ErrParentNotFound maps to SPACE_NOT_FOUND",
			err:          domain.ErrParentNotFound,
			expectedCode: dto.ErrCodeSpaceNotFound,
		},
		{
			name:         "ErrChildNotFound maps to ROOM_NOT_FOUND",
			err:          domain.ErrChildNotFound,
			expectedCode: dto.ErrCodeRoomNotFound,
		},
		{
			name:         "ErrRoomNotFound maps to ROOM_NOT_FOUND",
			err:          domain.ErrRoomNotFound,
			expectedCode: dto.ErrCodeRoomNotFound,
		},
		{
			name:         "ErrActorNotFound maps to ACTOR_NOT_FOUND",
			err:          domain.ErrActorNotFound,
			expectedCode: dto.ErrCodeActorNotFound,
		},
		{
			name:         "ErrForbidden maps to NOT_ALLOWED",
			err:          domain.ErrForbidden,
			expectedCode: dto.ErrCodeNotAllowed,
		},
		{
			name:         "ErrInvalidParam maps to INVALID_PARAM",
			err:          domain.ErrInvalidParam,
			expectedCode: dto.ErrCodeInvalidParam,
		},
		{
			name:         "forbidden string matches",
			err:          errors.New("forbidden: you cannot do this"),
			expectedCode: dto.ErrCodeNotAllowed,
		},
		{
			name:         "not found string matches",
			err:          errors.New("resource not found"),
			expectedCode: dto.ErrCodeRoomNotFound,
		},
		{
			name:         "unknown error maps to MATRIX_ERROR",
			err:          errors.New("connection timeout"),
			expectedCode: dto.ErrCodeMatrixError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := MapServiceError(tc.err)

			if tc.expectSuccess {
				if !result.Success {
					t.Errorf("expected success=true, got false")
				}
				return
			}

			if result.Success {
				t.Errorf("expected success=false, got true")
			}
			if result.Error == nil {
				t.Fatalf("expected error!=nil")
			}
			if result.Error.Code != tc.expectedCode {
				t.Errorf("expected code=%s, got %s", tc.expectedCode, result.Error.Code)
			}
		})
	}
}

func TestMapToBatchResult(t *testing.T) {
	successResult := MapToBatchResult(nil)
	if !successResult.Success {
		t.Error("nil error should return success=true")
	}

	errorResult := MapToBatchResult(domain.ErrRoomNotFound)
	if errorResult.Success {
		t.Error("error should return success=false")
	}
	if errorResult.Error == nil {
		t.Fatal("error should return error!=nil")
	}
	if errorResult.Error.Code != dto.ErrCodeRoomNotFound {
		t.Errorf("expected code=%s, got %s", dto.ErrCodeRoomNotFound, errorResult.Error.Code)
	}

	spaceResult := MapToBatchResult(domain.ErrSpaceNotFound)
	if spaceResult.Success {
		t.Error("error should return success=false")
	}
	if spaceResult.Error.Code != dto.ErrCodeSpaceNotFound {
		t.Errorf("expected code=%s, got %s", dto.ErrCodeSpaceNotFound, spaceResult.Error.Code)
	}
}
