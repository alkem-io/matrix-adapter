package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// ============================================================================
// Room-side pointer repair: dual-canonical clearing and convergence
// ============================================================================

func TestSetChildren_PointerRepairClearsStalePointerAndConverges(t *testing.T) {
	newParentID := uuid.New()
	child := uuid.New()
	oldParentRoomID := id.RoomID("!old-parent:test.local")
	newParentRoomID := id.RoomID("!new-parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(newParentID): newParentRoomID,
			spaceIDMapper.RoomAlias(child):        childRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(childRoomID)}, // already converged on the space side
		getSpaceParentsResults: map[id.RoomID][]id.RoomID{
			childRoomID: {oldParentRoomID}, // stale, pre-existing pointer to the OLD category
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        newParentID,
		DesiredChildContextIDs: []string{child.String()},
		SyncChildParent:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ParentPointersRepaired) != 1 {
		t.Fatalf("expected the child repaired, got repaired=%+v deferred=%+v", result.ParentPointersRepaired, result.ParentPointersDeferred)
	}
	if len(matrix.clearSpaceParentCalls) != 1 {
		t.Fatalf("expected exactly one ClearSpaceParent call for the stale pointer, got %+v", matrix.clearSpaceParentCalls)
	}
	if matrix.clearSpaceParentCalls[0].staleParentID != oldParentRoomID {
		t.Errorf("expected the stale OLD pointer cleared, got %+v", matrix.clearSpaceParentCalls[0])
	}
	if matrix.setSpaceParentCount != 1 {
		t.Errorf("expected exactly one SetSpaceParent call for the new canonical pointer, got %d", matrix.setSpaceParentCount)
	}

	// Simulate the writes having actually landed: the child now carries only
	// the new, correct pointer.
	matrix.getSpaceParentsResults[childRoomID] = []id.RoomID{newParentRoomID}
	matrix.clearSpaceParentCalls = nil
	matrix.setSpaceParentCount = 0

	result2, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        newParentID,
		DesiredChildContextIDs: []string{child.String()},
		SyncChildParent:        true,
	})
	if err != nil {
		t.Fatalf("pass 2: unexpected error: %v", err)
	}
	if len(result2.ParentPointersRepaired) != 0 || len(result2.ParentPointersDeferred) != 0 {
		t.Errorf("pass 2: expected zero pointer accounting once converged, got repaired=%+v deferred=%+v",
			result2.ParentPointersRepaired, result2.ParentPointersDeferred)
	}
	if matrix.setSpaceParentCount != 0 || len(matrix.clearSpaceParentCalls) != 0 {
		t.Errorf("pass 2: expected zero writes of any kind, got sets=%d clears=%+v",
			matrix.setSpaceParentCount, matrix.clearSpaceParentCalls)
	}
}

func TestSetChildren_PointerRepairDoesNotRewriteAChildWithOnlyTheDesiredParentLive(t *testing.T) {
	parentID := uuid.New()
	child := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child):     childRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(childRoomID)},
		getSpaceParentsResults: map[id.RoomID][]id.RoomID{
			childRoomID: {parentRoomID},
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child.String()},
		SyncChildParent:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.setSpaceParentCount != 0 || len(matrix.clearSpaceParentCalls) != 0 {
		t.Errorf("expected zero writes for a child whose only live pointer already names the desired parent, got sets=%d clears=%+v",
			matrix.setSpaceParentCount, matrix.clearSpaceParentCalls)
	}
	if len(result.ParentPointersRepaired) != 0 || len(result.ParentPointersDeferred) != 0 {
		t.Errorf("expected no repair accounting, got repaired=%+v deferred=%+v", result.ParentPointersRepaired, result.ParentPointersDeferred)
	}
}

// TestSetChildren_DryRunSyncChildParentIssuesZeroMembershipAffectingCalls
// asserts the US5 dry-run path never performs a write that could force the
// bot to join a room a person actually reads: GetSpaceParents (the only read
// this path performs) is a Synapse-admin-API read, and ClearSpaceParent /
// SetSpaceParent — the only calls that can trigger an admin-join — must never
// be invoked under dry_run regardless of how much drift the read finds.
func TestSetChildren_DryRunSyncChildParentIssuesZeroMembershipAffectingCalls(t *testing.T) {
	parentID := uuid.New()
	child := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child):     childRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(childRoomID)},
		getSpaceParentsResults: map[id.RoomID][]id.RoomID{
			childRoomID: {"!old-parent:test.local"}, // stale — would need both a clear and a set
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child.String()},
		SyncChildParent:        true,
		DryRun:                 true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.setSpaceParentCount != 0 || len(matrix.clearSpaceParentCalls) != 0 {
		t.Errorf("expected zero SetSpaceParent/ClearSpaceParent calls under dry_run, got sets=%d clears=%+v",
			matrix.setSpaceParentCount, matrix.clearSpaceParentCalls)
	}
	if len(result.ParentPointersRepaired) != 1 {
		t.Errorf("expected dry_run to still report the would-be repair, got %+v", result.ParentPointersRepaired)
	}
}

// TestSetChildren_PointerRepairDefersAndFailsWhenTheParentReadFails asserts a
// failed GetSpaceParents never becomes a blind SetSpaceParent. That write
// carries canonical=true, so issuing it over an unread — and therefore
// uncleared — stale pointer would manufacture the very dual-canonical state
// (R-7) this repair exists to remove, and, because the write itself would
// succeed, the call would report success=true on pointer state it never
// verified, terminating the operator's "repeat until failed==0" loop on an
// unread. The child must instead be reported deferred with success=false; the
// candidate set is the whole resolved desired set on every pass, so the next
// pass picks it up for free.
func TestSetChildren_PointerRepairDefersAndFailsWhenTheParentReadFails(t *testing.T) {
	parentID := uuid.New()
	child := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child):     childRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(childRoomID)}, // already converged space-side
		getSpaceParentErr:            errors.New("synapse admin API unavailable"),
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child.String()},
		SyncChildParent:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.setSpaceParentCount != 0 || len(matrix.clearSpaceParentCalls) != 0 {
		t.Errorf("expected zero room-side writes when the pointer read failed, got sets=%d clears=%+v",
			matrix.setSpaceParentCount, matrix.clearSpaceParentCalls)
	}
	if len(result.ParentPointersRepaired) != 0 {
		t.Errorf("expected no repair claimed on an unread child, got %+v", result.ParentPointersRepaired)
	}
	if len(result.ParentPointersDeferred) != 1 || result.ParentPointersDeferred[0] != string(childRoomID) {
		t.Errorf("expected the unread child reported deferred, got %+v", result.ParentPointersDeferred)
	}
	if result.Success {
		t.Error("expected success=false so the pass is repeated rather than read as convergence")
	}
	if result.WriteFailed {
		t.Error("expected write_failed=false: the failure was a read, not a rejected Matrix write")
	}
	if result.DeadlineExceeded {
		t.Error("expected deadline_exceeded=false: the call was not aborted by its own deadline")
	}
}
