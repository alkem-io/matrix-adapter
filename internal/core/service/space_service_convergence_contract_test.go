package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// `success` answers one question: did execution hit an error. It is not, and
// cannot be, the caller's termination condition, because a call can execute
// flawlessly and still leave the hierarchy short of the desired state — a
// desired child that resolved to no room, an extra edge kept because its
// identity or authorization could not be established, a pointer repair the
// budget could not fit. All of those return success=true. A caller that stops
// when nothing failed therefore declares reconciliation finished with required
// work still outstanding.
//
// `converged` is the flag that actually answers "is there anything left to do".

func TestSetChildren_ConvergedTrueOnlyWhenNothingIsOutstanding(t *testing.T) {
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
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child.String()},
		ApplyRemovals:          true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Converged {
		t.Errorf("expected converged=true — desired matches actual with nothing outstanding, got %+v", result)
	}
	if result.Changed {
		t.Error("expected changed=false — an already-converged parent needs no writes")
	}
}

func TestSetChildren_UnresolvedDesiredChildBlocksConvergence(t *testing.T) {
	// The desired child does not exist in Matrix. Nothing failed — a confirmed
	// absence is a legitimate answer — but the hierarchy demonstrably does not
	// match what was asked for, so the caller must not stop here.
	parentID := uuid.New()
	presentChild := uuid.New()
	missingChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	presentRoomID := id.RoomID("!present:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID):    parentRoomID,
			spaceIDMapper.RoomAlias(presentChild): presentRoomID,
			// missingChild's alias deliberately absent -> confirmed absence.
		},
		getSpaceChildStateKeysResult: []string{string(presentRoomID)},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{presentChild.String(), missingChild.String()},
		ApplyRemovals:          true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true — a confirmed absence is not an execution failure")
	}
	if result.Converged {
		t.Error("expected converged=false — a desired child is not in the hierarchy, so work remains")
	}
	if len(result.Unresolved) != 1 {
		t.Errorf("expected the absent child reported unresolved, got %+v", result.Unresolved)
	}
}

func TestSetChildren_OversizedPointerRepairIsUnprocessableNotDeferredForever(t *testing.T) {
	// A single child whose repair needs more writes than the per-call pointer
	// budget can ever reserve at once. Deferring it is not "try again later":
	// every future pass recomputes the same plan and defers it identically, so
	// a runbook that repeats until nothing is outstanding never terminates.
	// It has to be reported as its own actionable class.
	parentID := uuid.New()
	child := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	// fastHierarchyConfig caps pointer repairs at 5 writes per call. Six stale
	// pointers to clear plus the desired one to set is seven — permanently
	// oversized, not merely unlucky with what is left this call.
	stalePointers := []id.RoomID{
		"!stale1:test.local", "!stale2:test.local", "!stale3:test.local",
		"!stale4:test.local", "!stale5:test.local", "!stale6:test.local",
	}

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child):     childRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(childRoomID)},
		getSpaceParentsResults: map[id.RoomID][]id.RoomID{
			childRoomID: stalePointers,
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

	if len(result.ParentPointersUnprocessable) != 1 || result.ParentPointersUnprocessable[0] != string(childRoomID) {
		t.Errorf("expected the oversized repair reported unprocessable, got %+v", result.ParentPointersUnprocessable)
	}
	if len(result.ParentPointersDeferred) != 0 {
		t.Errorf("expected it NOT in the deferred list — deferred means a later pass can clear it, and none can, got %+v",
			result.ParentPointersDeferred)
	}
	if result.Converged {
		t.Error("expected converged=false — a requested repair is outstanding and no pass will ever complete it")
	}
	if len(matrix.clearSpaceParentCalls) != 0 || matrix.setSpaceParentCount != 0 {
		t.Errorf("expected zero pointer writes — a repair that cannot complete must not be started half-way, got %d clears and %d sets",
			len(matrix.clearSpaceParentCalls), matrix.setSpaceParentCount)
	}
}

func TestSetChildren_PointerRepairWithinBudgetStillCompletes(t *testing.T) {
	// Guard against over-correction: the unprocessable class must be scoped to
	// repairs that exceed the budget outright, never to ordinary ones.
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
			childRoomID: {"!stale1:test.local", "!stale2:test.local"}, // 2 clears + 1 set = 3 <= 5
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

	if len(result.ParentPointersUnprocessable) != 0 {
		t.Errorf("expected nothing unprocessable for a repair that fits, got %+v", result.ParentPointersUnprocessable)
	}
	if len(result.ParentPointersRepaired) != 1 {
		t.Errorf("expected the repair to complete, got %+v", result.ParentPointersRepaired)
	}
	if !result.Converged {
		t.Errorf("expected converged=true once the repair landed and nothing else is outstanding, got %+v", result)
	}
}

func TestSetChildren_AddOnlyPhaseWithExtrasIsNotConverged(t *testing.T) {
	// The add-only phase never inspects extras at all, so its UnknownKept is
	// empty for a reason that has nothing to do with the parent being clean.
	// Convergence must be judged on the edges actually left attached, or the
	// one phase that is forbidden from removing anything becomes the one that
	// reports the hierarchy finished.
	parentID := uuid.New()
	desiredChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	desiredRoomID := id.RoomID("!desired:test.local")
	extraRoomID := id.RoomID("!extra:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID):    parentRoomID,
			spaceIDMapper.RoomAlias(desiredChild): desiredRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(desiredRoomID), string(extraRoomID)},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{desiredChild.String()},
		ApplyRemovals:          false, // phase A of the two-phase sweep
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected success=true — an add-only phase that adds nothing has not failed")
	}
	if result.Converged {
		t.Error("expected converged=false — an extra edge is still attached, whether or not this phase may remove it")
	}
	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero removal writes in an add-only phase, got %d", matrix.removeSpaceChildCount)
	}
}

func TestSetChildren_KeptExtraBlocksConvergenceEvenThoughNothingFailed(t *testing.T) {
	// The case that makes `converged` necessary rather than cosmetic: an extra
	// edge the caller did not authorize is deliberately left alone. That is the
	// correct behaviour and not an error — and it is also unfinished business.
	parentID := uuid.New()
	extraChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	extraRoomID := id.RoomID("!extra:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(extraRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			extraRoomID: extraChild,
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID: parentID,
		ApplyRemovals:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected success=true — nothing was attempted and nothing failed")
	}
	if result.Converged {
		t.Error("expected converged=false — an unauthorized extra edge is still attached, so the pass is not done")
	}
}
