package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// ============================================================================
// Removal gate: a confirmed-absent desired child must not disable removal of
// unrelated, genuinely-live recategorisation leftovers, nor mislabel them as
// unknown_kept — only the prune of ghost extras is held back for this call.
// ============================================================================

func TestSetChildren_UnresolvedDesiredChildDoesNotSuppressRemovalOfLiveExtras(t *testing.T) {
	parentID := uuid.New()
	liveChild := uuid.New()     // resolves fine
	goneChild := uuid.New()     // confirmed absent -> unresolved[]
	staleLeftover := uuid.New() // recategorised away from this parent; still live elsewhere
	parentRoomID := id.RoomID("!parent:test.local")
	liveChildRoomID := id.RoomID("!live:test.local")
	staleRoomID := id.RoomID("!stale:test.local")
	ghostRoomID := id.RoomID("!ghost:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(liveChild): liveChildRoomID,
			// goneChild's alias deliberately absent -> confirmed absence (unresolved[]).
		},
		getSpaceChildStateKeysResult: []string{string(liveChildRoomID), string(staleRoomID), string(ghostRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: staleLeftover, // reverse-resolves to a different, still-live Alkemio room
			// ghostRoomID intentionally absent -> reverse-resolves to nothing (a real ghost).
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{liveChild.String(), goneChild.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0] != goneChild.String() {
		t.Fatalf("expected the confirmed-absent child reported unresolved, got %+v", result.Unresolved)
	}

	if len(result.Removed) != 1 || result.Removed[0] != string(staleRoomID) {
		t.Errorf("expected the recategorisation leftover still removed despite the unresolved sibling, got removed=%+v",
			result.Removed)
	}
	if len(result.PrunedUnknown) != 0 {
		t.Errorf("expected the ghost NOT pruned while a desired child in this call is unresolved, got %+v",
			result.PrunedUnknown)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(ghostRoomID) {
		t.Errorf("expected the ghost reported unknown_kept instead of silently dropped, got %+v", result.UnknownKept)
	}
	if !result.Success {
		t.Error("expected success=true — a confirmed absence is not a failure, unlike a transient resolution error")
	}
}

func TestSetChildren_TransientResolveFailureStillSuppressesTheWholeRemovalPhase(t *testing.T) {
	// The transient case keeps the original, fully conservative behavior: the
	// whole picture of `desired` is untrustworthy, so nothing is removed or
	// pruned and the call is reported failed.
	parentID := uuid.New()
	liveChild := uuid.New()
	flakyChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	liveChildRoomID := id.RoomID("!live:test.local")
	staleRoomID := id.RoomID("!stale:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(liveChild): liveChildRoomID,
		},
		resolveAliasErrs: map[string]error{
			spaceIDMapper.RoomAlias(flakyChild): errors.New("upstream 502"), // transient, not "not found"
		},
		getSpaceChildStateKeysResult: []string{string(liveChildRoomID), string(staleRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: uuid.New(),
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{liveChild.String(), flakyChild.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Removed) != 0 || len(result.PrunedUnknown) != 0 {
		t.Errorf("expected zero removals of any kind on a transient resolution failure, got removed=%+v pruned=%+v",
			result.Removed, result.PrunedUnknown)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(staleRoomID) {
		t.Errorf("expected the extra reported unknown_kept unchanged, got %+v", result.UnknownKept)
	}
	if result.Success {
		t.Error("expected success=false so the repeat-until-failed==0 protocol retries this call")
	}
}
