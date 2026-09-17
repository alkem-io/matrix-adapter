package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// The removal half of set_children used to infer its work: anything present in
// the parent that was absent from the caller's desired set was a removal
// candidate. That inference is unsound, because "absent from the desired set"
// and "should not be here" are not the same statement. The desired set is a
// snapshot of the caller's database taken before the call; a discussion created
// or recategorised after that read is missing from it while being entirely
// correct in Matrix. Removing on that basis deletes a live discussion's only
// route into the hierarchy, and — because the removal succeeds — reports
// success while doing it.
//
// Removal is therefore authorized rather than inferred: the caller names the
// children it has positively established belong elsewhere, and only those are
// eligible. These tests pin that, including the case the old behaviour got
// wrong.

func TestSetChildren_UnauthorizedLiveExtraIsKeptNotRemoved(t *testing.T) {
	// The race the authorization gate exists for: a discussion created in this
	// category after the caller read its snapshot. It is live, it belongs
	// exactly where it is, and it is missing from `desired` purely because it
	// did not exist when `desired` was built.
	parentID := uuid.New()
	desiredChild := uuid.New()
	racedChild := uuid.New() // created after the caller's snapshot — correct, unnamed

	parentRoomID := id.RoomID("!parent:test.local")
	desiredRoomID := id.RoomID("!desired:test.local")
	racedRoomID := id.RoomID("!raced:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID):    parentRoomID,
			spaceIDMapper.RoomAlias(desiredChild): desiredRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(desiredRoomID), string(racedRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			racedRoomID: racedChild, // reverse-resolves to a live Alkemio room
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{desiredChild.String()},
		ApplyRemovals:          true,
		// Nothing authorized: the caller established no child as belonging
		// elsewhere, so nothing may be removed no matter what the diff says.
		RemovableChildContextIDs: nil,
		PruneUnknown:             true, // must not reach a live room either
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero removal writes for an unauthorized live extra, got %d", matrix.removeSpaceChildCount)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected the raced child NOT removed — it is live and correctly placed, got %+v", result.Removed)
	}
	if len(result.PrunedUnknown) != 0 {
		t.Errorf("prune is for edges with no Alkemio identity left; a live room must never reach it, got %+v",
			result.PrunedUnknown)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(racedRoomID) {
		t.Errorf("expected the kept edge surfaced so the drift is visible rather than silent, got %+v",
			result.UnknownKept)
	}
	if result.Converged {
		t.Error("expected converged=false — an extra edge is still outstanding, whatever the caller chose to authorize")
	}
	if !result.Success {
		t.Error("expected success=true — declining to remove an unauthorized edge is the contract, not a failure")
	}
}

func TestSetChildren_OnlyAuthorizedExtrasAreRemoved(t *testing.T) {
	// Two live extras, one authorized. The gate must be per-child, not a
	// blanket on/off: authorizing one removal cannot license the other.
	parentID := uuid.New()
	authorizedChild := uuid.New()
	unauthorizedChild := uuid.New()

	parentRoomID := id.RoomID("!parent:test.local")
	authorizedRoomID := id.RoomID("!authorized:test.local")
	unauthorizedRoomID := id.RoomID("!unauthorized:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(authorizedRoomID), string(unauthorizedRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			authorizedRoomID:   authorizedChild,
			unauthorizedRoomID: unauthorizedChild,
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:          parentID,
		ApplyRemovals:            true,
		RemovableChildContextIDs: []string{authorizedChild.String()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Removed) != 1 || result.Removed[0] != string(authorizedRoomID) {
		t.Errorf("expected exactly the authorized extra removed, got %+v", result.Removed)
	}
	if matrix.removeSpaceChildCount != 1 {
		t.Errorf("expected exactly one removal write, got %d", matrix.removeSpaceChildCount)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(unauthorizedRoomID) {
		t.Errorf("expected the unauthorized extra kept and reported, got %+v", result.UnknownKept)
	}
}

func TestSetChildren_AuthorizationOfAnAbsentChildRemovesNothing(t *testing.T) {
	// Authorization is permission, never instruction. A caller naming a child
	// that is not actually attached here must not cause a write — the adapter
	// removes the intersection of what is authorized with what is present, so
	// a stale authorization list is inert rather than destructive.
	parentID := uuid.New()
	attachedChild := uuid.New()
	notAttachedChild := uuid.New()

	parentRoomID := id.RoomID("!parent:test.local")
	attachedRoomID := id.RoomID("!attached:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID):     parentRoomID,
			spaceIDMapper.RoomAlias(attachedChild): attachedRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(attachedRoomID)},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:          parentID,
		DesiredChildContextIDs:   []string{attachedChild.String()},
		ApplyRemovals:            true,
		RemovableChildContextIDs: []string{notAttachedChild.String()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero writes — the authorized child is not attached here, got %d",
			matrix.removeSpaceChildCount)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected nothing reported removed, got %+v", result.Removed)
	}
	if !result.Converged {
		t.Error("expected converged=true — the desired child is attached and nothing is outstanding")
	}
}

func TestSetChildren_UnparseableAuthorizationEntryAuthorizesNothing(t *testing.T) {
	// The authorization set is the only thing between a live edge and
	// deletion, so an id the adapter cannot parse must authorize nothing
	// rather than be matched loosely or treated as a wildcard.
	parentID := uuid.New()
	liveChild := uuid.New()

	parentRoomID := id.RoomID("!parent:test.local")
	liveRoomID := id.RoomID("!live:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(liveRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			liveRoomID: liveChild,
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:          parentID,
		ApplyRemovals:            true,
		RemovableChildContextIDs: []string{"not-a-uuid", ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero removal writes from an unparseable authorization, got %d",
			matrix.removeSpaceChildCount)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(liveRoomID) {
		t.Errorf("expected the live edge kept and reported, got %+v", result.UnknownKept)
	}
}

func TestSetChildren_GhostPruneStillWorksWithoutAuthorization(t *testing.T) {
	// Guard against over-correction. The authorization gate governs live rooms;
	// a ghost has no Alkemio identity left, so it can never appear in an
	// authorization list, and gating it there would make PruneUnknown dead
	// code. Prune keeps its own separate opt-in.
	parentID := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	ghostRoomID := id.RoomID("!ghost:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(ghostRoomID)},
		// ghostRoomID absent from resolveAlkemioIDResults -> reverse-resolves
		// to nothing, the real ghost case.
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:          parentID,
		ApplyRemovals:            true,
		RemovableChildContextIDs: nil, // no live room authorized, and none needed
		PruneUnknown:             true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.PrunedUnknown) != 1 || result.PrunedUnknown[0] != string(ghostRoomID) {
		t.Errorf("expected the ghost still prunable under its own opt-in, got %+v", result.PrunedUnknown)
	}
	if len(result.UnknownKept) != 0 {
		t.Errorf("expected nothing left kept once the ghost was pruned, got %+v", result.UnknownKept)
	}
}
