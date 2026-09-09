package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

// ============================================================================
// SetChildren — contract semantics table
//
// Each test below is one row of the hierarchy-set-children contract's
// semantics table (specs/061-forum-matrix-hierarchy-sync/contracts/
// hierarchy-set-children.md). Row numbers in test names refer to that table.
// ============================================================================

func TestSetChildren_Row1_ParentNotFound(t *testing.T) {
	matrix := &mockSpaceMatrixPort{} // no resolveAliasResults entries — parent alias unresolved
	svc := newSpaceService(matrix)

	_, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID: uuid.New(),
	})

	if err == nil {
		t.Fatal("expected an error when the parent space does not resolve")
	}
	if !errors.Is(err, domain.ErrSpaceNotFound) {
		t.Errorf("expected a SPACE_NOT_FOUND error, got: %v", err)
	}
	if matrix.getSpaceChildStateKeysCount != 0 {
		t.Error("expected the children never to be read once the parent fails to resolve")
	}
	if matrix.addSpaceChildCount != 0 || matrix.removeSpaceChildCount != 0 {
		t.Error("expected zero writes when the parent does not resolve — and it is never lazily created")
	}
}

func TestSetChildren_Row2_ConvergedIsZeroWrites(t *testing.T) {
	parentID := uuid.New()
	childA := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!childA:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(childA):    childRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(childRoomID)},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{childA.String()},
		ApplyRemovals:          true, // removals in scope, but nothing to do
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || result.Changed {
		t.Errorf("expected success=true changed=false, got success=%v changed=%v", result.Success, result.Changed)
	}
	if len(result.Added) != 0 || len(result.Removed) != 0 || len(result.PrunedUnknown) != 0 ||
		len(result.UnknownKept) != 0 || len(result.Unresolved) != 0 {
		t.Errorf("expected all arrays empty when already converged, got %+v", result)
	}
	if matrix.addSpaceChildCount != 0 || matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero state writes (call-count asserted), got adds=%d removes=%d",
			matrix.addSpaceChildCount, matrix.removeSpaceChildCount)
	}
}

func TestSetChildren_Row3_MissingChildAddedAndAddsPrecedeRemoves(t *testing.T) {
	parentID := uuid.New()
	newChild := uuid.New()
	staleChild := uuid.New() // resolves to a live room but is no longer desired

	parentRoomID := id.RoomID("!parent:test.local")
	newChildRoomID := id.RoomID("!new:test.local")
	staleRoomID := id.RoomID("!stale:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(newChild):  newChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(staleRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: staleChild,
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{newChild.String()},
		ApplyRemovals:          true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != string(newChildRoomID) {
		t.Errorf("expected the missing child added, got %+v", result.Added)
	}
	if len(result.Removed) != 1 || result.Removed[0] != string(staleRoomID) {
		t.Errorf("expected the stale child removed, got %+v", result.Removed)
	}
	wantOrder := []string{"add:" + string(newChildRoomID), "remove:" + string(staleRoomID)}
	if len(matrix.callOrder) != 2 || matrix.callOrder[0] != wantOrder[0] || matrix.callOrder[1] != wantOrder[1] {
		t.Errorf("expected add to strictly precede remove within the call, got order: %v", matrix.callOrder)
	}
}

func TestSetChildren_Row4_ApplyRemovalsFalseAddsOnlyEvenWithExtras(t *testing.T) {
	parentID := uuid.New()
	newChild := uuid.New()

	parentRoomID := id.RoomID("!parent:test.local")
	newChildRoomID := id.RoomID("!new:test.local")
	extraRoomID := id.RoomID("!extra:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(newChild):  newChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(extraRoomID)},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{newChild.String()},
		ApplyRemovals:          false, // phase A of the two-phase sweep
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.addSpaceChildCount != 1 {
		t.Errorf("expected the missing child still added, got %d add call(s)", matrix.addSpaceChildCount)
	}
	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero removal writes when apply_removals=false, even with an extra present, got %d",
			matrix.removeSpaceChildCount)
	}
	if len(result.Removed) != 0 || len(result.PrunedUnknown) != 0 || len(result.UnknownKept) != 0 {
		t.Errorf("expected no removal accounting when apply_removals=false, got %+v", result)
	}
}

func TestSetChildren_Row5_KnownExtraRemovedUnderApplyRemovals(t *testing.T) {
	parentID := uuid.New()
	staleChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	staleRoomID := id.RoomID("!stale:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(staleRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: staleChild,
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
	if len(result.Removed) != 1 || result.Removed[0] != string(staleRoomID) {
		t.Errorf("expected the extra edge removed (recategorisation leftover), got %+v", result.Removed)
	}
	if matrix.removeSpaceChildCount != 1 {
		t.Errorf("expected exactly one removal write, got %d", matrix.removeSpaceChildCount)
	}
}

func TestSetChildren_Row6_GhostKeptByDefaultPrunedWhenRequested(t *testing.T) {
	parentID := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	ghostRoomID := id.RoomID("!ghost:test.local")

	newFixture := func() *mockSpaceMatrixPort {
		return &mockSpaceMatrixPort{
			resolveAliasResults: map[string]id.RoomID{
				spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			},
			getSpaceChildStateKeysResult: []string{string(ghostRoomID)},
			// No resolveAlkemioIDResults entry for ghostRoomID: it reverse-resolves
			// to uuid.Nil, exactly like a deleted discussion's alias-less edge.
		}
	}

	t.Run("kept by default", func(t *testing.T) {
		matrix := newFixture()
		svc := newSpaceService(matrix)

		result, err := svc.SetChildren(context.Background(), SetChildrenParams{
			ParentContextID: parentID,
			ApplyRemovals:   true,
			PruneUnknown:    false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(ghostRoomID) {
			t.Errorf("expected the ghost edge reported in unknown_kept, got %+v", result.UnknownKept)
		}
		if matrix.removeSpaceChildCount != 0 {
			t.Errorf("expected zero writes for a kept ghost edge, got %d", matrix.removeSpaceChildCount)
		}
	})

	t.Run("pruned when requested", func(t *testing.T) {
		matrix := newFixture()
		svc := newSpaceService(matrix)

		result, err := svc.SetChildren(context.Background(), SetChildrenParams{
			ParentContextID: parentID,
			ApplyRemovals:   true,
			PruneUnknown:    true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.PrunedUnknown) != 1 || result.PrunedUnknown[0] != string(ghostRoomID) {
			t.Errorf("expected the ghost edge reported in pruned_unknown, got %+v", result.PrunedUnknown)
		}
		if matrix.removeSpaceChildCount != 1 {
			t.Errorf("expected exactly one removal write for the pruned ghost, got %d", matrix.removeSpaceChildCount)
		}
	})
}

func TestSetChildren_Row7_UnresolvableDesiredIsReportedNotCreated(t *testing.T) {
	parentID := uuid.New()
	missingChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			// missingChild's room alias is deliberately absent.
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{missingChild.String()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0] != missingChild.String() {
		t.Errorf("expected the unresolvable desired child reported in unresolved, got %+v", result.Unresolved)
	}
	if matrix.addSpaceChildCount != 0 {
		t.Error("expected no add write for an unresolvable desired child")
	}
	if matrix.createSpaceCalled {
		t.Error("expected no room/space creation for an unresolvable desired child — reported, never guessed at")
	}
}

// TestSetChildren_Row7_TransientResolveFailureBlocksRemoval covers the
// destructive interaction the plain Row7 test (ApplyRemovals left false)
// cannot: a desired child whose alias lookup fails transiently — not
// confirmed absent — must never authorize deleting its still-correct,
// already-converged edge. Distinguishing this from Row6/Row7's confirmed
// "child does not exist" cases is the whole point: a transient failure gives
// no guarantee the child is actually gone.
func TestSetChildren_Row7_TransientResolveFailureBlocksRemoval(t *testing.T) {
	parentID := uuid.New()
	liveChild := uuid.New() // desired, already converged, but its lookup will fail transiently this call
	parentRoomID := id.RoomID("!parent:test.local")
	liveChildRoomID := id.RoomID("!livechild:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			// liveChild's room alias is deliberately absent from resolveAliasResults.
		},
		resolveAliasErrs: map[string]error{
			// A transient failure (not "not found") on the desired child's alias.
			spaceIDMapper.RoomAlias(liveChild): errors.New("429 Too Many Requests"),
		},
		getSpaceChildStateKeysResult: []string{string(liveChildRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			liveChildRoomID: liveChild, // the extra reverse-resolves as live — it IS still desired
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{liveChild.String()},
		ApplyRemovals:          true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected ZERO removal writes when a desired child's resolution failed transiently, got %d "+
			"(the still-correct edge for %s must never be deleted on an incomplete picture of the desired set)",
			matrix.removeSpaceChildCount, liveChildRoomID)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected nothing reported removed, got %+v", result.Removed)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(liveChildRoomID) {
		t.Errorf("expected the extra reported kept (removal phase skipped entirely), got unknown_kept=%+v", result.UnknownKept)
	}
	if result.Success {
		t.Error("expected success=false so the repeat-until-failed==0 protocol retries this call")
	}
	// Not a confirmed absence: the transient failure must not land in
	// unresolved[], which is reserved for report-only, genuinely-absent
	// children (FR-005).
	if len(result.Unresolved) != 0 {
		t.Errorf("expected a transient failure NOT reported in unresolved (that is confirmed-absence only), got %+v",
			result.Unresolved)
	}
}

// TestSetChildren_Row7_TransientResolveFailureAlsoBlocksPruning is the whole-
// call-empty-desired-set degenerate case: if Synapse is down for the entire
// alias-resolution phase, EVERY desired child fails transiently, so a naive
// implementation would compute desired=∅ and delete every existing edge in
// one call while still reporting success. The fail-safe gate must stop that
// regardless of how many desired children are affected.
func TestSetChildren_Row7_TransientResolveFailureAlsoBlocksPruning(t *testing.T) {
	parentID := uuid.New()
	child := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
		},
		resolveAliasErr:              errors.New("503 Service Unavailable"), // every desired lookup fails transiently
		getSpaceChildStateKeysResult: []string{string(childRoomID)},
		// No resolveAlkemioIDResults entry: the extra would reverse-resolve to
		// uuid.Nil, i.e. classify as an "unknown ghost" if classification ran —
		// it must not even get that far.
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero removal writes when every desired child failed to resolve, got %d", matrix.removeSpaceChildCount)
	}
	if len(result.PrunedUnknown) != 0 {
		t.Errorf("expected nothing pruned, got %+v", result.PrunedUnknown)
	}
	if len(result.UnknownKept) != 1 || result.UnknownKept[0] != string(childRoomID) {
		t.Errorf("expected the sole existing edge reported kept rather than deleted, got %+v", result.UnknownKept)
	}
	if result.Success {
		t.Error("expected success=false, not a falsely-reported convergent success")
	}
}

func TestSetChildren_Row8_RemovalFailureAfterAddIsHonestPartial(t *testing.T) {
	parentID := uuid.New()
	newChild := uuid.New()
	staleChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	newChildRoomID := id.RoomID("!new:test.local")
	staleRoomID := id.RoomID("!stale:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(newChild):  newChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(staleRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: staleChild,
		},
		removeSpaceChildErr: errors.New("synapse rejected the removal"),
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{newChild.String()},
		ApplyRemovals:          true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected success=false when a removal fails after a successful add")
	}
	if !result.Changed {
		t.Error("expected changed=true — the add did succeed, so this call did change state")
	}
	if len(result.Added) != 1 {
		t.Errorf("expected the successful add still reported, got %+v", result.Added)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected the failed removal NOT reported as removed (it stays a detectable extra edge), got %+v",
			result.Removed)
	}
	if !result.WriteFailed {
		t.Error("expected WriteFailed=true — Matrix actually rejected the removal write")
	}
	if result.DeadlineExceeded {
		t.Error("expected DeadlineExceeded=false — this call ran to completion, it never hit its own deadline")
	}
}

// TestSetChildren_DeadlineExceededDistinctFromWriteFailure asserts that a call
// aborted by its own execution deadline (config.Hierarchy.
// SetChildrenTimeoutSeconds, applied by the queue handler as the ctx it
// passes in) is reported distinctly from an actual Matrix write rejection:
// DeadlineExceeded=true, WriteFailed=false, and — because the abort happens
// before the loop's first write attempt — zero writes are ever issued. This
// is what lets a caller tell "the adapter ran out of time, repeat the call"
// apart from "Matrix rejected a write, investigate" (both otherwise report
// Success=false).
func TestSetChildren_DeadlineExceededDistinctFromWriteFailure(t *testing.T) {
	parentID := uuid.New()
	childA := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!childA:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(childA):    childRoomID,
		},
		// getSpaceChildStateKeysResult left empty: childA is missing, so it is
		// a genuine toAdd candidate the loop would otherwise write.
	}
	svc := newSpaceService(matrix)

	// A deadline already in the past reproduces exactly what the queue
	// handler's context.WithTimeout(ctx, SetChildrenTimeout) produces once
	// that timeout fires mid-call — ctx.Err() is observed on the very first
	// loop iteration, before any write is attempted.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	result, err := svc.SetChildren(ctx, SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{childA.String()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected success=false once the call's own deadline has passed")
	}
	if !result.DeadlineExceeded {
		t.Error("expected DeadlineExceeded=true")
	}
	if result.WriteFailed {
		t.Error("expected WriteFailed=false — no write was actually attempted, let alone rejected by Matrix")
	}
	if len(result.Added) != 0 {
		t.Errorf("expected zero adds once the deadline has passed, got %+v", result.Added)
	}
	if matrix.addSpaceChildCount != 0 {
		t.Error("expected zero AddSpaceChild calls once ctx.Err() is observed")
	}
}

func TestSetChildren_Row9_DryRunComputesWithZeroWrites(t *testing.T) {
	parentID := uuid.New()
	newChild := uuid.New()
	staleChild := uuid.New()

	parentRoomID := id.RoomID("!parent:test.local")
	newChildRoomID := id.RoomID("!new:test.local")
	staleRoomID := id.RoomID("!stale:test.local")
	ghostRoomID := id.RoomID("!ghost:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(newChild):  newChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(staleRoomID), string(ghostRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: staleChild,
			// ghostRoomID intentionally absent.
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{newChild.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
		DryRun:                 true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.addSpaceChildCount != 0 || matrix.removeSpaceChildCount != 0 {
		t.Errorf("expected zero writes under dry_run, got adds=%d removes=%d",
			matrix.addSpaceChildCount, matrix.removeSpaceChildCount)
	}
	if len(result.Added) != 1 || len(result.Removed) != 1 || len(result.PrunedUnknown) != 1 {
		t.Errorf("expected dry_run to still report the would-be actions computed with the call's flags, got %+v", result)
	}
	if !result.Changed {
		t.Error("expected changed=true (would-change) under dry_run")
	}
	if !result.Success {
		t.Error("expected success=true — nothing was attempted, so nothing could fail")
	}
}

func TestSetChildren_Row10_ParentPointerRepairIsOptInAndBudgeted(t *testing.T) {
	parentID := uuid.New()
	child1 := uuid.New()
	child2 := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	child1RoomID := id.RoomID("!c1:test.local")
	child2RoomID := id.RoomID("!c2:test.local")

	newFixture := func() *mockSpaceMatrixPort {
		return &mockSpaceMatrixPort{
			resolveAliasResults: map[string]id.RoomID{
				spaceIDMapper.SpaceAlias(parentID): parentRoomID,
				spaceIDMapper.RoomAlias(child1):    child1RoomID,
				spaceIDMapper.RoomAlias(child2):    child2RoomID,
			},
		}
	}

	t.Run("default off — zero child-room writes", func(t *testing.T) {
		matrix := newFixture()
		svc := newSpaceService(matrix)

		result, err := svc.SetChildren(context.Background(), SetChildrenParams{
			ParentContextID:        parentID,
			DesiredChildContextIDs: []string{child1.String(), child2.String()},
			// SyncChildParent left false.
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matrix.setSpaceParentCount != 0 {
			t.Errorf("expected SetSpaceParent (and so any admin-join it can trigger) never called by default, got %d call(s)",
				matrix.setSpaceParentCount)
		}
		if len(result.ParentPointersRepaired) != 0 || len(result.ParentPointersDeferred) != 0 {
			t.Errorf("expected no pointer accounting when sync_child_parent=false, got %+v", result)
		}
	})

	t.Run("opted in — budget caps repairs and defers the rest, never silently", func(t *testing.T) {
		matrix := newFixture()
		cfg := fastHierarchyConfig()
		cfg.Hierarchy.ParentPointerBudgetPerCall = 1 // force exhaustion after one repair
		svc := NewSpaceService(matrix, &testutil.MockLogger{}, domain.NewIDMapper("test.local"), cfg)

		result, err := svc.SetChildren(context.Background(), SetChildrenParams{
			ParentContextID:        parentID,
			DesiredChildContextIDs: []string{child1.String(), child2.String()},
			SyncChildParent:        true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matrix.setSpaceParentCount != 1 {
			t.Errorf("expected exactly one pointer write under a capacity-1 budget, got %d", matrix.setSpaceParentCount)
		}
		if len(result.ParentPointersRepaired) != 1 {
			t.Errorf("expected exactly one repaired pointer, got %+v", result.ParentPointersRepaired)
		}
		if len(result.ParentPointersDeferred) != 1 {
			t.Errorf("expected the second touched child deferred (reported), not silently dropped, got %+v",
				result.ParentPointersDeferred)
		}
	})
}

// TestSetChildren_Row10_DeferredPointerIsPickedUpByALaterConvergentPass
// asserts US5's convergence property: a pointer deferred by a capacity-1
// budget on pass 1 is repaired by pass 2, even though pass 2's `toAdd` is
// empty (the space side already converged on pass 1) — because pointer-repair
// candidates are scoped to the whole resolved desired set, not to toAdd.
func TestSetChildren_Row10_DeferredPointerIsPickedUpByALaterConvergentPass(t *testing.T) {
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
	cfg := fastHierarchyConfig()
	cfg.Hierarchy.ParentPointerBudgetPerCall = 1 // force exhaustion after one repair
	svc := NewSpaceService(matrix, &testutil.MockLogger{}, domain.NewIDMapper("test.local"), cfg)

	params := SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{child1.String(), child2.String()},
		SyncChildParent:        true,
	}

	// Pass 1: both children added; the pointer budget caps repair at one and
	// defers the other.
	result1, err := svc.SetChildren(context.Background(), params)
	if err != nil {
		t.Fatalf("pass 1: unexpected error: %v", err)
	}
	if len(result1.ParentPointersRepaired) != 1 || len(result1.ParentPointersDeferred) != 1 {
		t.Fatalf("pass 1: expected one repaired and one deferred, got repaired=%+v deferred=%+v",
			result1.ParentPointersRepaired, result1.ParentPointersDeferred)
	}
	// Simulate the two adds and the one pointer repair having actually landed,
	// since this fixture-mock (unlike real Synapse) does not update its own
	// read results from the writes made against it.
	matrix.getSpaceChildStateKeysResult = []string{string(child1RoomID), string(child2RoomID)}
	matrix.getSpaceParentResults = map[id.RoomID]id.RoomID{
		id.RoomID(result1.ParentPointersRepaired[0]): parentRoomID,
	}

	// Pass 2: the space side is already converged (toAdd is empty), but the
	// deferred pointer from pass 1 must still be a repair candidate.
	result2, err := svc.SetChildren(context.Background(), params)
	if err != nil {
		t.Fatalf("pass 2: unexpected error: %v", err)
	}
	if len(result2.Added) != 0 {
		t.Fatalf("pass 2: expected zero adds (already converged), got %+v", result2.Added)
	}
	if len(result2.ParentPointersRepaired) != 1 {
		t.Errorf("pass 2: expected the previously-deferred pointer to be repaired now, got repaired=%+v deferred=%+v",
			result2.ParentPointersRepaired, result2.ParentPointersDeferred)
	}
	if len(result2.ParentPointersDeferred) != 0 {
		t.Errorf("pass 2: expected nothing deferred (only one candidate remained, budget is 1), got %+v",
			result2.ParentPointersDeferred)
	}
}

// TestSetChildren_Row10_AlreadyCanonicalPointerIsSkippedNotRewritten asserts
// that a child whose m.space.parent already names this parent is neither
// written nor counted — pointer-repair budget is spent only on genuine drift,
// and a converged pointer does not keep reappearing in parent_pointers_repaired
// on every subsequent pass.
func TestSetChildren_Row10_AlreadyCanonicalPointerIsSkippedNotRewritten(t *testing.T) {
	parentID := uuid.New()
	child := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	childRoomID := id.RoomID("!child:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(child):     childRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(childRoomID)}, // already converged on the space side
		getSpaceParentResults: map[id.RoomID]id.RoomID{
			childRoomID: parentRoomID, // already canonical to this parent
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
	if matrix.setSpaceParentCount != 0 {
		t.Errorf("expected zero SetSpaceParent writes for an already-canonical child, got %d", matrix.setSpaceParentCount)
	}
	if len(result.ParentPointersRepaired) != 0 || len(result.ParentPointersDeferred) != 0 {
		t.Errorf("expected an already-canonical child neither repaired nor deferred, got repaired=%+v deferred=%+v",
			result.ParentPointersRepaired, result.ParentPointersDeferred)
	}
}

func TestSetChildren_Row11_ChildrenAreSpacesResolvesTheForumLevelCall(t *testing.T) {
	// RoomAlias and SpaceAlias share the identical #<uuid>:<domain> format by
	// design (disjoint Alkemio UUID space, idmapper.go) — children_are_spaces is
	// wire-contract semantics (which id space the server is asking about, room
	// vs. subspace) rather than a different literal alias string in this
	// adapter. This test exercises the forum-level shape of the call: a forum
	// parent with a category space as its desired child.
	forumID := uuid.New()
	categorySpaceID := uuid.New()
	forumRoomID := id.RoomID("!forum:test.local")
	categoryRoomID := id.RoomID("!category:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(forumID):         forumRoomID,
			spaceIDMapper.SpaceAlias(categorySpaceID): categoryRoomID,
		},
	}
	svc := newSpaceService(matrix)

	result, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        forumID,
		DesiredChildContextIDs: []string{categorySpaceID.String()},
		ChildrenAreSpaces:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != string(categoryRoomID) {
		t.Errorf("expected the category space resolved and added under the forum, got added=%+v unresolved=%+v",
			result.Added, result.Unresolved)
	}
}

func TestSetChildren_Row12_NoDeleteOrCreatePathReachableBehaviorally(t *testing.T) {
	parentID := uuid.New()
	newChild := uuid.New()
	staleChild := uuid.New()
	parentRoomID := id.RoomID("!parent:test.local")
	newChildRoomID := id.RoomID("!new:test.local")
	staleRoomID := id.RoomID("!stale:test.local")
	ghostRoomID := id.RoomID("!ghost:test.local")

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceIDMapper.SpaceAlias(parentID): parentRoomID,
			spaceIDMapper.RoomAlias(newChild):  newChildRoomID,
		},
		getSpaceChildStateKeysResult: []string{string(staleRoomID), string(ghostRoomID)},
		resolveAlkemioIDResults: map[id.RoomID]uuid.UUID{
			staleRoomID: staleChild,
		},
	}
	svc := newSpaceService(matrix)

	// A maximally destructive-looking call: full converge plus prune.
	_, err := svc.SetChildren(context.Background(), SetChildrenParams{
		ParentContextID:        parentID,
		DesiredChildContextIDs: []string{newChild.String()},
		ApplyRemovals:          true,
		PruneUnknown:           true,
		SyncChildParent:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if matrix.createSpaceCalled {
		t.Error("expected SetChildren to never create a space or room, however destructive its flags")
	}
	if matrix.deleteAliasCalled {
		t.Error("expected SetChildren to never delete an alias — only m.space.child/m.space.parent state events")
	}
}

// TestSetChildren_SourceNeverReferencesADeleteCall is a static reachability
// guard, independent of any mock behavior: it reads the source text spanning
// SetChildren and every private helper it calls (resolveDesiredChildren,
// classifyExtras, applyAdds, applyRemovalsAndPrune, removeChildren,
// applyParentPointerRepair — the whole call graph, up to the next unrelated
// section of the file) and asserts none of it contains a "Delete" identifier.
// No pass, mode, or flag combination can reach a delete call if the call site
// does not exist in that source at all.
func TestSetChildren_SourceNeverReferencesADeleteCall(t *testing.T) {
	src, err := os.ReadFile("space_service.go")
	if err != nil {
		t.Fatalf("failed to read space_service.go: %v", err)
	}

	startMarker := []byte("func (s *SpaceService) SetChildren(")
	start := bytes.Index(src, startMarker)
	if start == -1 {
		t.Fatal("could not locate the SetChildren function in space_service.go")
	}

	endMarker := []byte("\n// ============================================================================\n// Batch Membership Operations")
	end := bytes.Index(src[start:], endMarker)
	if end == -1 {
		t.Fatal("could not locate the end of the SetChildren call-graph section (Batch Membership marker moved?)")
	}
	body := src[start : start+end]

	if bytes.Contains(body, []byte("Delete")) {
		t.Errorf("the SetChildren call graph must never reference a Delete call — no delete path may be reachable "+
			"from the reconciler; found \"Delete\" in:\n%s", body)
	}
}
