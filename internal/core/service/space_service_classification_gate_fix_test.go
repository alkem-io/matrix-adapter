package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// ============================================================================
// Classification gate: "ghost" is a read that answered, not a read that broke.
//
// classifyExtras reverse-resolves each extra edge's state_key, and the answer
// decides whether prune_unknown may destroy that edge (D-16: ghosts get an
// explicit, opt-in destructive step, and only after they are reported). A
// failed alias lookup is not the same answer as a confirmed missing alias, so
// it must never be spent as one: the edge is kept and reported, the prune
// opt-in is withheld for the call, and the call is reported failed so the
// operator's "repeat until failed==0" loop cannot terminate on a homeserver
// hiccup that was silently read as a ghost.
// ============================================================================

func TestSetChildren_UnclassifiableExtraIsNeverPrunedAndFailsTheCall(t *testing.T) {
	parentID := uuid.New()
	liveChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	liveChildRoomID := id.RoomID("!live:test.local")
	flakyExtraRoomID := id.RoomID("!flaky-extra:test.local")
	ghostRoomID := id.RoomID("!ghost:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(liveChild): liveChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{
			string(liveChildRoomID), string(flakyExtraRoomID), string(ghostRoomID),
		},
		resolveAlkemioIDErrs: map[id.RoomID]error{
			flakyExtraRoomID: errors.New("alias lookup failed: upstream 502"),
		},
		// ghostRoomID has no entry in resolveAlkemioIDResults and no injected
		// error: a confirmed no-alias ghost.
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{liveChild.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.PrunedUnknown) != 0 {
		t.Errorf("expected zero prunes while one extra could not be classified, got %+v", result.PrunedUnknown)
	}
	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero RemoveSpaceChild writes, got %d for %+v",
			matrix.removeSpaceChildCount, matrix.removeSpaceChildStateKeys)
	}
	if len(result.UnknownKept) != 2 {
		t.Errorf("expected both the unclassifiable extra and the real ghost reported unknown_kept, got %+v",
			result.UnknownKept)
	}
	if result.Success {
		t.Error("expected success=false: the classification this call's prune decision rests on was incomplete")
	}
	if result.WriteFailed {
		t.Error("expected write_failed=false: the failure was a read, not a rejected Matrix write")
	}
}

func TestSetChildren_ConfirmedExtrasAreStillRemovedWhenAnotherExtraIsUnclassifiable(t *testing.T) {
	// The withheld step is the prune, not the whole removal phase: an extra
	// that reverse-resolved successfully to a different live Alkemio room is a
	// confirmed recategorisation leftover, and a failed lookup on an unrelated
	// edge says nothing about it.
	parentID := uuid.New()
	liveChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	liveChildRoomID := id.RoomID("!live:test.local")
	staleRoomID := id.RoomID("!stale:test.local")
	flakyExtraRoomID := id.RoomID("!flaky-extra:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(liveChild): liveChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{
			string(liveChildRoomID), string(staleRoomID), string(flakyExtraRoomID),
		},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: uuid.New(),
		},
		resolveAlkemioIDErrs: map[id.RoomID]error{
			flakyExtraRoomID: errors.New("alias lookup failed: upstream 502"),
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{liveChild.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Removed) != 1 || result.Removed[0] != string(staleRoomID) {
		t.Errorf("expected the confirmed recategorisation leftover still removed, got %+v", result.Removed)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(flakyExtraRoomID) {
		t.Errorf("expected only the unclassifiable extra reported unknown_kept, got %+v", result.UnknownKept)
	}
	if len(result.PrunedUnknown) != 0 {
		t.Errorf("expected the prune opt-in withheld for this call, got %+v", result.PrunedUnknown)
	}
	if result.Success {
		t.Error("expected success=false so the pass is repeated until the classification is complete")
	}
	if !result.Changed {
		t.Error("expected changed=true — the confirmed removal did land (honest partial converge)")
	}
}

func TestSetChildren_ConfirmedGhostIsStillPrunedWhenEveryClassificationSucceeds(t *testing.T) {
	// Guard against over-correction: the new gate must fire on a failed
	// lookup only, never on the confirmed-absent alias that defines a ghost.
	parentID := uuid.New()
	liveChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	liveChildRoomID := id.RoomID("!live:test.local")
	ghostRoomID := id.RoomID("!ghost:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(liveChild): liveChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(liveChildRoomID), string(ghostRoomID)},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{liveChild.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.PrunedUnknown) != 1 || result.PrunedUnknown[0] != string(ghostRoomID) {
		t.Errorf("expected the confirmed ghost pruned, got pruned=%+v kept=%+v", result.PrunedUnknown, result.UnknownKept)
	}
	if !result.Success {
		t.Error("expected success=true: every classification answered")
	}
}
