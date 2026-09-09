package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// ============================================================================
// Parent resolution: transient failure vs. confirmed absence
//
// A confirmed-absent parent (SPACE_NOT_FOUND) is the expected, non-error
// "space was never created" skip a full-vocabulary sweep relies on. Any other
// resolution error must be reported differently, so a degraded Synapse can
// never be counted as that same, harmless skip.
// ============================================================================

func TestSetChildren_TransientParentResolutionFailureIsNotReportedAsSpaceNotFound(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr: errors.New("connection reset by peer"), // not a "not found" error
	}
	svc := newSpaceService(matrix)

	_, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected an error when parent resolution fails transiently")
	}
	if errors.Is(err, domain.ErrSpaceNotFound) {
		t.Errorf("a transient parent resolution failure must not be reported as SPACE_NOT_FOUND (the confirmed-absence skip), got: %v", err)
	}
	if matrix.getSpaceChildStateKeysCount != 0 {
		t.Error("expected children never read once parent resolution fails")
	}
}

func TestSetChildren_ConfirmedAbsentParentIsStillReportedAsSpaceNotFound(t *testing.T) {
	// No resolveAliasResults entry and no resolveAliasErr — the mock's default
	// fallback is domain.NewSpaceNotFoundError, exactly a confirmed absence.
	matrix := &mockSpaceMatrixPort{}
	svc := newSpaceService(matrix)

	_, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrSpaceNotFound) {
		t.Errorf("expected a confirmed-absent parent still reported as SPACE_NOT_FOUND, got: %v", err)
	}
}
