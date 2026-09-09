package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

// ============================================================================
// Availability: a call bounds its own work rather than depending on a
// caller-side timeout the queue transport cannot propagate.
// ============================================================================

func TestSetChildren_CancelledContextStopsTheAddLoopAndReportsFailure(t *testing.T) {
	parentID := uuid.New()
	child1 := uuid.New()
	child2 := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	child1RoomID := id.RoomID("!c1:test.local")
	child2RoomID := id.RoomID("!c2:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child1):    child1RoomID,
			spaceIDMapper.RoomAlias(child2):    child2RoomID,
		},
	}
	svc := newSpaceService(matrix)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the call ever begins

	result, err := svc.SetChildren(ctx, SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child1.String(), child2.String()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.addSpaceChildCount != 0 {
		t.Errorf("expected zero adds once the context is already cancelled, got %d", matrix.addSpaceChildCount)
	}
	if result.Success {
		t.Error("expected success=false — the call did not get to attempt its own work")
	}
}

func TestSetChildren_MaxWriteOperationsPerCallStopsTheCallHonestly(t *testing.T) {
	parentID := uuid.New()
	child1 := uuid.New()
	child2 := uuid.New()
	child3 := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	c1 := id.RoomID("!c1:test.local")
	c2 := id.RoomID("!c2:test.local")
	c3 := id.RoomID("!c3:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child1):    c1,
			spaceIDMapper.RoomAlias(child2):    c2,
			spaceIDMapper.RoomAlias(child3):    c3,
		},
	}
	cfg := fastHierarchyConfig()
	cfg.Hierarchy.MaxWriteOperationsPerCall = 2 // fewer than the 3 adds needed
	svc := NewSpaceService(matrix, &testutil.MockLogger{}, domain.NewIDMapper("test.local"), cfg)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child1.String(), child2.String(), child3.String()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.addSpaceChildCount != 2 {
		t.Errorf("expected exactly the capped 2 writes attempted, got %d", matrix.addSpaceChildCount)
	}
	if len(result.Added) != 2 {
		t.Errorf("expected exactly 2 reported added, got %+v", result.Added)
	}
	if result.Success {
		t.Error("expected success=false — the call could not finish everything it needed to under its own write cap")
	}
}

func TestSetChildren_UnboundedByDefaultMaxWriteOperationsPerCall(t *testing.T) {
	// The zero value (as produced by fastHierarchyConfig, and by any existing
	// deployment that never sets the new knob) must mean unbounded, matching
	// every pre-existing behavior in this file.
	parentID := uuid.New()
	child := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child):     childRoomID,
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child.String()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || len(result.Added) != 1 {
		t.Errorf("expected the single add to succeed normally, got success=%v added=%+v", result.Success, result.Added)
	}
}
