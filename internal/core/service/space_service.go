package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

// SpaceService handles operations related to Matrix Spaces.
type SpaceService struct {
	matrix   ports.MatrixPort
	logger   ports.Logger
	idMapper *domain.IDMapper

	// Hierarchy convergence write budgets — see hierarchy_budget.go.
	stateEventsPerSecond         float64
	parentPointerEventsPerSecond float64
	parentPointerBudgetPerCall   int
	maxWriteOperationsPerCall    int
}

// NewSpaceService creates a new instance of SpaceService.
func NewSpaceService(matrix ports.MatrixPort, logger ports.Logger, idMapper *domain.IDMapper, cfg *config.Config) *SpaceService {
	return &SpaceService{
		matrix:                       matrix,
		logger:                       logger,
		idMapper:                     idMapper,
		stateEventsPerSecond:         cfg.Hierarchy.StateEventsPerSecond,
		parentPointerEventsPerSecond: cfg.Hierarchy.ParentPointerEventsPerSecond,
		parentPointerBudgetPerCall:   cfg.Hierarchy.ParentPointerBudgetPerCall,
		maxWriteOperationsPerCall:    cfg.Hierarchy.MaxWriteOperationsPerCall,
	}
}

// ============================================================================
// Space CRUD Operations (communication.space.*)
// ============================================================================

// CreateSpace creates a new Matrix Space with idempotent alias lookup.
// If the space already exists (alias resolves), it returns success.
func (s *SpaceService) CreateSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
	name, topic, avatarURL string,
	joinRule string,
	isPublic *bool,
	customState map[string]map[string]interface{},
	parentContextID *uuid.UUID,
	initialMembers []domain.Actor,
) error {
	s.logger.Info("Creating space with Alkemio Context ID",
		"alkemio_context_id", alkemioContextID,
		"name", name,
		"join_rule", joinRule)

	// Build alias for idempotency check
	alias := s.idMapper.SpaceAlias(alkemioContextID)

	// Check if space already exists (idempotency)
	existingRoomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err == nil {
		// Space already exists - idempotent success
		s.logger.Info("Space already exists (idempotent)",
			"alkemio_context_id", alkemioContextID,
			"existing_room_id", existingRoomID)
		return nil
	}

	// If error is not "not found", return it
	if !domain.IsNotFoundError(err) {
		return fmt.Errorf("failed to check space alias: %w", err)
	}

	// Determine effective join rule (default to invite)
	effectiveJoinRule := joinRule
	if effectiveJoinRule == "" {
		effectiveJoinRule = "invite"
	}

	// Create the space
	spaceRoomID, err := s.matrix.CreateSpace(ctx, alkemioContextID, name, topic, avatarURL, effectiveJoinRule, initialMembers)
	if err != nil {
		return fmt.Errorf("failed to create space: %w", err)
	}

	// Set directory visibility if specified
	if isPublic != nil {
		if err := s.matrix.SetRoomDirectoryVisibility(ctx, spaceRoomID, *isPublic); err != nil {
			s.logger.Warn("Failed to set space directory visibility",
				"alkemio_context_id", alkemioContextID, "is_public", *isPublic, "error", err)
		}
	}

	// Set custom io.alkemio.* state events if specified
	if len(customState) > 0 {
		if err := s.matrix.SetCustomState(ctx, spaceRoomID, customState); err != nil {
			s.logger.Warn("Failed to set custom state on space",
				"alkemio_context_id", alkemioContextID, "error", err)
		}
	}

	// If parent context is specified, set up hierarchy
	if parentContextID != nil {
		parentAlias := s.idMapper.SpaceAlias(*parentContextID)
		parentRoomID, err := s.matrix.ResolveAlias(ctx, parentAlias)
		if err != nil {
			s.logger.Warn("Failed to resolve parent space, skipping hierarchy setup",
				"parent_context_id", parentContextID,
				"error", err)
		} else {
			// Add this space as a child of the parent
			if err := s.matrix.AddSpaceChild(ctx, parentRoomID, spaceRoomID, "", false); err != nil {
				s.logger.Warn("Failed to add space as child of parent",
					"parent_room_id", parentRoomID,
					"child_room_id", spaceRoomID,
					"error", err)
			}
			// Set parent relationship on this space
			if err := s.matrix.SetSpaceParent(ctx, spaceRoomID, parentRoomID); err != nil {
				s.logger.Warn("Failed to set space parent",
					"child_room_id", spaceRoomID,
					"parent_room_id", parentRoomID,
					"error", err)
			}
		}
	}

	s.logger.Info("Space created successfully", "alkemio_context_id", alkemioContextID, "room_id", spaceRoomID)
	return nil
}

// GetSpace retrieves space details including members and children.
func (s *SpaceService) GetSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
) (*domain.Space, error) {
	s.logger.Info("Getting space", "alkemio_context_id", alkemioContextID)

	// Build alias and resolve to Matrix room ID
	alias := s.idMapper.SpaceAlias(alkemioContextID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return nil, domain.NewSpaceNotFoundError(alkemioContextID.String())
	}

	// Get space details
	space, err := s.matrix.GetSpaceDetails(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get space details: %w", err)
	}
	space.AlkemioContextID = alkemioContextID

	// Get space members and map to Alkemio actor IDs
	members, err := s.matrix.GetSpaceMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get space members: %w", err)
	}

	space.MemberIDs = make([]uuid.UUID, 0, len(members))
	for _, memberID := range members {
		// Extract UUID from Matrix user ID (@uuid:domain)
		if actorUUID := s.idMapper.AlkemioActorID(memberID); actorUUID != uuid.Nil {
			space.MemberIDs = append(space.MemberIDs, actorUUID)
		}
	}

	// Get space children
	children, err := s.matrix.GetSpaceChildren(ctx, roomID)
	if err != nil {
		s.logger.Warn("Failed to get space children", "room_id", roomID, "error", err)
		children = []domain.SpaceChild{}
	}
	space.Children = children

	return space, nil
}

// UpdateSpace updates space metadata.
func (s *SpaceService) UpdateSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
	name, topic, avatarURL *string,
	joinRule *string,
	isPublic *bool,
	customState map[string]map[string]interface{},
) error {
	s.logger.Info("Updating space", "alkemio_context_id", alkemioContextID)

	// Resolve alias to get Matrix room ID
	alias := s.idMapper.SpaceAlias(alkemioContextID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return domain.NewSpaceNotFoundError(alkemioContextID.String())
	}

	err = s.matrix.UpdateSpaceState(ctx, roomID, name, topic, avatarURL, joinRule)
	if err != nil {
		return fmt.Errorf("failed to update space: %w", err)
	}

	// Set directory visibility if specified
	if isPublic != nil {
		if err := s.matrix.SetRoomDirectoryVisibility(ctx, roomID, *isPublic); err != nil {
			s.logger.Warn("Failed to set space directory visibility",
				"alkemio_context_id", alkemioContextID, "is_public", *isPublic, "error", err)
		}
	}

	// Set custom io.alkemio.* state events if specified
	if len(customState) > 0 {
		if err := s.matrix.SetCustomState(ctx, roomID, customState); err != nil {
			s.logger.Warn("Failed to set custom state on space",
				"alkemio_context_id", alkemioContextID, "error", err)
		}
	}

	return nil
}

// DeleteSpace kicks all members, leaves the space, and removes the alias.
func (s *SpaceService) DeleteSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
	reason string,
) error {
	s.logger.Info("Deleting space", "alkemio_context_id", alkemioContextID, "reason", reason)

	// Resolve alias to get Matrix room ID
	alias := s.idMapper.SpaceAlias(alkemioContextID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		// Space doesn't exist - idempotent success
		if domain.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to resolve space alias: %w", err)
	}

	// Get space members to kick
	members, err := s.matrix.GetSpaceMembers(ctx, roomID)
	if err != nil {
		s.logger.Warn("Failed to get space members for kick", "room_id", roomID, "error", err)
	} else {
		// Kick all members
		for _, memberID := range members {
			if err := s.matrix.KickFromSpace(ctx, roomID, memberID, reason); err != nil {
				s.logger.Warn("Failed to kick user from space", "user_id", memberID, "room_id", roomID, "error", err)
			}
		}
	}

	// Delete the alias
	if err := s.matrix.DeleteAlias(ctx, alias); err != nil {
		s.logger.Warn("Failed to delete space alias", "alias", alias, "error", err)
	}

	s.logger.Info("Space deleted successfully", "alkemio_context_id", alkemioContextID)
	return nil
}

// ListSpaces returns a paginated list of Alkemio context IDs.
func (s *SpaceService) ListSpaces(
	ctx context.Context,
	cursor string,
) ([]uuid.UUID, string, error) {
	s.logger.Info("Listing spaces", "cursor", cursor)

	// Get all joined rooms
	rooms, err := s.matrix.GetAllJoinedRooms(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list spaces: %w", err)
	}

	// Extract Alkemio context IDs from space aliases
	alkemioContextIDs := s.extractAlkemioContextIDs(ctx, rooms)

	return alkemioContextIDs, "", nil
}

// extractAlkemioContextIDs extracts Alkemio context UUIDs from Matrix spaces.
func (s *SpaceService) extractAlkemioContextIDs(ctx context.Context, rooms []id.RoomID) []uuid.UUID {
	alkemioContextIDs := make([]uuid.UUID, 0)

	for _, roomID := range rooms {
		space, err := s.matrix.GetSpaceDetails(ctx, roomID)
		if err != nil {
			continue
		}

		contextID := s.idMapper.AlkemioContextID(space.Alias)
		if contextID != uuid.Nil {
			alkemioContextIDs = append(alkemioContextIDs, contextID)
		}
	}

	return alkemioContextIDs
}

// ============================================================================
// Hierarchy Operations (communication.hierarchy.*)
// ============================================================================

// SetParent establishes parent-child relationship for a room or subspace.
func (s *SpaceService) SetParent(
	ctx context.Context,
	childID string,
	isSpace bool,
	parentContextID uuid.UUID,
	order string,
	suggested bool,
) error {
	s.logger.Info("Setting parent relationship",
		"child_id", childID,
		"is_space", isSpace,
		"parent_context_id", parentContextID)

	// Resolve parent space
	parentAlias := s.idMapper.SpaceAlias(parentContextID)
	parentRoomID, err := s.matrix.ResolveAlias(ctx, parentAlias)
	if err != nil {
		return domain.NewParentNotFoundError(parentContextID.String())
	}

	// Resolve child (either room or space)
	var childRoomID id.RoomID
	if isSpace {
		// Parse as UUID and resolve space alias
		childUUID, err := uuid.Parse(childID)
		if err != nil {
			return fmt.Errorf("invalid child space ID: %w", err)
		}
		childAlias := s.idMapper.SpaceAlias(childUUID)
		childRoomID, err = s.matrix.ResolveAlias(ctx, childAlias)
		if err != nil {
			return domain.NewChildNotFoundError(childID)
		}
	} else {
		// Parse as UUID and resolve room alias
		childUUID, err := uuid.Parse(childID)
		if err != nil {
			return fmt.Errorf("invalid child room ID: %w", err)
		}
		roomAlias := s.idMapper.RoomAlias(childUUID)
		childRoomID, err = s.matrix.ResolveAlias(ctx, roomAlias)
		if err != nil {
			return domain.NewChildNotFoundError(childID)
		}
	}

	// Add child to parent space
	if err := s.matrix.AddSpaceChild(ctx, parentRoomID, childRoomID, order, suggested); err != nil {
		return fmt.Errorf("failed to add child to space: %w", err)
	}

	// Set parent on child
	if err := s.matrix.SetSpaceParent(ctx, childRoomID, parentRoomID); err != nil {
		return fmt.Errorf("failed to set parent on child: %w", err)
	}

	s.logger.Info("Parent relationship set successfully",
		"child_room_id", childRoomID,
		"parent_room_id", parentRoomID)
	return nil
}

// SetChildrenParams is the decoded form of a set_children request: the parent
// space plus the desired child set and the flags that control how far
// convergence goes.
type SetChildrenParams struct {
	ParentContextID        uuid.UUID
	DesiredChildContextIDs []string
	ChildrenAreSpaces      bool
	ApplyRemovals          bool
	PruneUnknown           bool
	SyncChildParent        bool
	DryRun                 bool
}

// SetChildrenResult is the in-memory outcome of SetChildren, mirroring
// dto.SetChildrenResponse without the wire envelope. Every slice reports what
// happened (or, under DryRun, would have happened) — never a full identifier
// list, only the ones actually touched.
type SetChildrenResult struct {
	Added                  []string
	Removed                []string
	PrunedUnknown          []string
	UnknownKept            []string
	Unresolved             []string
	ParentPointersRepaired []string
	ParentPointersDeferred []string
	Changed                bool
	Success                bool
	// DeadlineExceeded reports whether the call's own execution deadline (see
	// config.Hierarchy.SetChildrenTimeoutSeconds) was reached before every
	// attempted write could be issued. Set independently of WriteFailed so a
	// caller can distinguish "ran out of time" from "Matrix rejected a
	// write" even though both leave Success=false.
	DeadlineExceeded bool
	// WriteFailed reports whether at least one Matrix write actually
	// attempted by this call returned an error (as opposed to never being
	// attempted because the deadline or write-count budget was reached
	// first).
	WriteFailed bool
}

// SetChildren converges a parent space's m.space.child edges toward a desired
// child set: it reads the parent's actual children once, adds every missing
// edge, and — only when ApplyRemovals is set — removes edges that no longer
// belong (keyed on their raw Matrix state_key, since a deleted discussion's
// ghost edge has no Alkemio identifier left to name it by). It never creates
// the parent space, never creates or deletes any room, space, or alias — the
// only writes it can ever issue are m.space.child edges and, opted in via
// SyncChildParent, m.space.parent pointers on children in the resolved
// desired set that are not already canonical to this parent.
func (s *SpaceService) SetChildren(ctx context.Context, params SetChildrenParams) (*SetChildrenResult, error) {
	s.logger.Info("Converging space children",
		"parent_context_id", params.ParentContextID,
		"desired_count", len(params.DesiredChildContextIDs),
		"children_are_spaces", params.ChildrenAreSpaces,
		"apply_removals", params.ApplyRemovals,
		"prune_unknown", params.PruneUnknown,
		"sync_child_parent", params.SyncChildParent,
		"dry_run", params.DryRun)

	// Resolve the parent. A confirmed absence ⇒ SPACE_NOT_FOUND, zero writes,
	// never created — that is the expected, non-error "space was never
	// created" skip a full-vocabulary sweep relies on. Any other resolution
	// error (timeout, 5xx, rate limit) is a transient fault and must NOT be
	// reported the same way: doing so would let a degraded Synapse make a
	// category with real drift look like a clean, expected skip.
	parentAlias := s.idMapper.SpaceAlias(params.ParentContextID)
	parentRoomID, err := s.matrix.ResolveAlias(ctx, parentAlias)
	if err != nil {
		if domain.IsNotFoundError(err) {
			return nil, domain.NewSpaceNotFoundError(params.ParentContextID.String())
		}
		return nil, fmt.Errorf("failed to resolve parent alias: %w", err)
	}

	// One children read for the whole call.
	actualStateKeys, err := s.matrix.GetSpaceChildStateKeys(ctx, parentRoomID)
	if err != nil {
		return nil, fmt.Errorf("failed to read space children: %w", err)
	}
	actual := toKeySet(actualStateKeys)

	desired, unresolved, resolveFailed := s.resolveDesiredChildren(ctx, params)
	result := &SetChildrenResult{Unresolved: unresolved}

	toAdd := diffKeys(desired, actual)

	extras := s.planRemovals(ctx, diffKeys(actual, desired), params, resolveFailed, len(unresolved) > 0)

	outcome := &writeOutcome{maxWrites: s.maxWriteOperationsPerCall}
	if extras.incompleteRead {
		outcome.failed = true
	}
	stateBudget := newStateWriteRateLimiter(s.stateEventsPerSecond, time.Now, time.Sleep)

	// Phase A: all adds precede all removes, inside this single call.
	result.Added = s.applyAdds(ctx, parentRoomID, toAdd, params.DryRun, stateBudget, outcome)

	if params.ApplyRemovals {
		result.Removed, result.PrunedUnknown, result.UnknownKept = s.applyRemovalsAndPrune(
			ctx, parentRoomID, extras, params.DryRun, stateBudget, outcome)
	}

	// Room-side canonical-parent repair: opt-in, scoped to the whole resolved
	// desired set (not just this call's toAdd) so a repair deferred by budget
	// on one pass is still a candidate on the next — otherwise, once a child is
	// converged, it drops out of every future toAdd and its deferred pointer
	// can never be picked up again. Left false, SetSpaceParent (and so the
	// admin-join it can trigger) is never called.
	if params.SyncChildParent {
		result.ParentPointersRepaired, result.ParentPointersDeferred =
			s.applyParentPointerRepair(ctx, parentRoomID, sortedKeys(desired), params.DryRun, outcome)
	}

	if params.DryRun {
		result.Changed = len(result.Added) > 0 || len(result.Removed) > 0 ||
			len(result.PrunedUnknown) > 0 || len(result.ParentPointersRepaired) > 0
	} else {
		result.Changed = outcome.succeeded
	}
	// Honest partial converge: any write failure makes the whole call
	// success=false even though everything it could still attempt was attempted
	// and whatever succeeded is reported. No compensating rollback exists — the
	// next convergent pass, run again until failed==0, is the compensation.
	result.Success = !outcome.failed
	result.DeadlineExceeded = outcome.deadlineExceeded
	result.WriteFailed = outcome.writeFailed

	s.logger.Info("Space children convergence complete",
		"parent_context_id", params.ParentContextID,
		"added", len(result.Added), "removed", len(result.Removed),
		"pruned_unknown", len(result.PrunedUnknown), "unknown_kept", len(result.UnknownKept),
		"unresolved", len(result.Unresolved),
		"parent_pointers_repaired", len(result.ParentPointersRepaired),
		"parent_pointers_deferred", len(result.ParentPointersDeferred),
		"changed", result.Changed, "success", result.Success)

	return result, nil
}

// resolveDesiredChildren resolves every desired child id to its Matrix room id
// (via RoomAlias, or SpaceAlias when ChildrenAreSpaces). It returns the
// resolved set, the ids confirmed absent (report-only, never created, never
// guessed — FR-005), and whether any resolution failed for a reason other than
// confirmed absence (a rate limit, a 5xx, a transport error, a timeout).
//
// The two failure modes are kept apart deliberately: a confirmed-absent child
// genuinely does not exist, so it was never in `actual` either and dropping it
// from `desired` changes nothing. A transient failure gives no such guarantee
// — the child may be a live, previously-converged room that Synapse simply
// could not resolve just now — so the caller must never let a transient
// failure drive a removal decision.
func (s *SpaceService) resolveDesiredChildren(ctx context.Context, params SetChildrenParams) (desired map[string]struct{}, unresolved []string, resolveFailed bool) {
	desired = make(map[string]struct{}, len(params.DesiredChildContextIDs))

	for _, desiredIDStr := range params.DesiredChildContextIDs {
		desiredUUID, parseErr := uuid.Parse(desiredIDStr)
		if parseErr != nil {
			unresolved = append(unresolved, desiredIDStr)
			continue
		}

		alias := s.idMapper.RoomAlias(desiredUUID)
		if params.ChildrenAreSpaces {
			alias = s.idMapper.SpaceAlias(desiredUUID)
		}

		childRoomID, resolveErr := s.matrix.ResolveAlias(ctx, alias)
		if resolveErr != nil {
			if domain.IsNotFoundError(resolveErr) {
				unresolved = append(unresolved, desiredIDStr)
			} else {
				s.logger.Warn("Desired child alias resolution failed transiently — not treated as absence",
					"desired_child_id", desiredIDStr, "error", resolveErr)
				resolveFailed = true
			}
			continue
		}
		desired[string(childRoomID)] = struct{}{}
	}

	sort.Strings(unresolved)
	return desired, unresolved, resolveFailed
}

// removalPlan is one call's decision about the parent's extra edges: which of
// them this call has actually earned the right to remove, which it may only
// report, and whether the reads those decisions rest on were complete.
type removalPlan struct {
	// removableKnown are extras confirmed to reverse-resolve to a different,
	// still-live Alkemio room — recategorisation leftovers, removed whenever
	// ApplyRemovals is set.
	removableKnown []string
	// unknownExtras are extras this call may not remove unless prune is still
	// permitted: confirmed ghosts, plus any extra whose classification read
	// failed. Whatever is not pruned is reported as unknown_kept — drift class
	// (b), the report an operator reviews before opting into the prune.
	unknownExtras []string
	// prunePermitted is PruneUnknown narrowed by what this call actually
	// established. Prune is the one destructive step whose contract (D-16) is
	// report-before-destroy, so it is withheld whenever the picture is
	// incomplete.
	prunePermitted bool
	// incompleteRead reports that some read this call's decisions depend on
	// did not answer, so the call must report success=false and be repeated —
	// a broken read is never convergence.
	incompleteRead bool
}

// planRemovals decides how far this call's removal phase may go, given the
// parent's extra edges and how trustworthy this call's picture of `desired`
// turned out to be.
//
// A transient resolution failure (a rate limit, a 5xx, a timeout) gives no
// guarantee about the id it hit: the desired child it failed to resolve might
// be a live, previously-converged room Synapse just couldn't resolve this
// instant, so its still-correct edge must never be read as an extra. Whenever
// that happens the whole removal phase is skipped for this call — every
// current extra is reported as unknown_kept, exactly as if PruneUnknown were
// off — and the call is reported failed so "repeat until failed==0" retries it
// rather than treating a broken read as convergence.
//
// A confirmed absence (report-only — never created, never guessed) carries no
// such ambiguity: the id genuinely does not exist, so it was never in `actual`
// under its own identity either. Extras are still classified and known extras
// are still removed; only the prune opt-in is held back for this call, because
// the unresolved child's own dangling edge, if it is the thing still sitting in
// `actual`, would otherwise be indistinguishable here from a ghost this same
// call would prune, and the unresolved report exists precisely so an operator
// reviews that case before it is pruned. An extra whose own classification read
// failed is held back for the same reason and additionally fails the call,
// since — unlike a confirmed absence — nothing was actually established.
func (s *SpaceService) planRemovals(
	ctx context.Context, extras []string, params SetChildrenParams,
	resolveFailed, unresolvedPresent bool,
) removalPlan {
	if !params.ApplyRemovals {
		return removalPlan{incompleteRead: resolveFailed}
	}
	if resolveFailed {
		return removalPlan{unknownExtras: extras, incompleteRead: true}
	}

	removableKnown, unknownExtras, classifyFailed := s.classifyExtras(ctx, extras)
	return removalPlan{
		removableKnown: removableKnown,
		unknownExtras:  unknownExtras,
		prunePermitted: params.PruneUnknown && !unresolvedPresent && !classifyFailed,
		incompleteRead: classifyFailed,
	}
}

// classifyExtras reverse-resolves each extra edge's state_key: one that still
// names a live Alkemio room is a recategorisation leftover (removableKnown);
// one confirmed to carry no Alkemio alias is a ghost left by a deleted
// discussion (unknownExtras). Cost is per extra edge, bounded by drift count.
//
// An extra whose reverse resolution *failed* is neither: its identity is
// unknown, not confirmed absent. It is reported in unknownExtras — so it is
// kept and surfaced, never removed on this call — and classifyFailed is
// returned so the caller both withholds the prune opt-in (a ghost is defined
// by a read that answered, not by one that broke) and reports the call failed,
// which is what makes "repeat until failed==0" terminate on a complete
// classification rather than on a homeserver hiccup.
func (s *SpaceService) classifyExtras(ctx context.Context, extras []string) (removableKnown, unknownExtras []string, classifyFailed bool) {
	for _, extraKey := range extras {
		alkemioID, resolveErr := s.matrix.ResolveAlkemioID(ctx, id.RoomID(extraKey))
		if resolveErr != nil {
			s.logger.Warn("Extra child edge could not be classified — kept, never pruned, on this call",
				"child_state_key", extraKey, "error", resolveErr)
			classifyFailed = true
			unknownExtras = append(unknownExtras, extraKey)
			continue
		}
		if alkemioID != uuid.Nil {
			removableKnown = append(removableKnown, extraKey)
		} else {
			unknownExtras = append(unknownExtras, extraKey)
		}
	}
	return removableKnown, unknownExtras, classifyFailed
}

// applyAdds writes every missing child edge (paced by budget unless dryRun),
// recording each write's outcome, and returns the ids added or, under dryRun,
// that would be added.
func (s *SpaceService) applyAdds(
	ctx context.Context, parentRoomID id.RoomID, toAdd []string, dryRun bool,
	budget *stateWriteRateLimiter, outcome *writeOutcome,
) []string {
	var added []string
	for _, childKey := range toAdd {
		if ctx.Err() != nil {
			outcome.failed = true
			outcome.deadlineExceeded = true
			break
		}
		if !dryRun {
			if !outcome.allowWrite() {
				break
			}
			budget.pace()
			writeErr := s.matrix.AddSpaceChild(ctx, parentRoomID, id.RoomID(childKey), "", false)
			outcome.record(writeErr)
			if writeErr != nil {
				s.logger.Warn("Failed to add space child",
					"parent_room_id", parentRoomID, "child_room_id", childKey, "error", writeErr)
				continue
			}
		}
		added = append(added, childKey)
	}
	return added
}

// applyRemovalsAndPrune executes the plan: it removes every confirmed
// known-live extra edge, and — only when the plan still permits the prune —
// every ghost extra edge too. Whatever is not pruned is returned as
// unknownKept, the report of drift class (b).
func (s *SpaceService) applyRemovalsAndPrune(
	ctx context.Context, parentRoomID id.RoomID, plan removalPlan, dryRun bool,
	budget *stateWriteRateLimiter, outcome *writeOutcome,
) (removed, prunedUnknown, unknownKept []string) {
	removed = s.removeChildren(ctx, parentRoomID, plan.removableKnown, dryRun, budget, outcome,
		"Failed to remove space child")

	if !plan.prunePermitted {
		return removed, nil, plan.unknownExtras
	}

	prunedUnknown = s.removeChildren(ctx, parentRoomID, plan.unknownExtras, dryRun, budget, outcome,
		"Failed to prune unknown space child")
	return removed, prunedUnknown, nil
}

// removeChildren issues a RemoveSpaceChild write (paced by budget unless
// dryRun) for every given state_key, and returns the ones removed or, under
// dryRun, that would be removed.
func (s *SpaceService) removeChildren(
	ctx context.Context, parentRoomID id.RoomID, childKeys []string, dryRun bool,
	budget *stateWriteRateLimiter, outcome *writeOutcome, warnMsg string,
) []string {
	var result []string
	for _, childKey := range childKeys {
		if ctx.Err() != nil {
			outcome.failed = true
			outcome.deadlineExceeded = true
			break
		}
		if !dryRun {
			if !outcome.allowWrite() {
				break
			}
			budget.pace()
			writeErr := s.matrix.RemoveSpaceChild(ctx, parentRoomID, childKey)
			outcome.record(writeErr)
			if writeErr != nil {
				s.logger.Warn(warnMsg, "parent_room_id", parentRoomID, "child_state_key", childKey, "error", writeErr)
				continue
			}
		}
		result = append(result, childKey)
	}
	return result
}

// applyParentPointerRepair corrects m.space.parent on every candidate child —
// the whole resolved desired set for this parent, not just children just
// added — under its own separate and lower budget. Scoping to the full
// desired set (rather than only this call's toAdd) is what makes the flag
// convergent under repetition: a child converged by an earlier pass, or one
// whose repair was deferred by a prior call's exhausted budget, is still a
// candidate here, so a later pass with SyncChildParent set keeps making
// progress instead of finding an empty toAdd and repairing nothing.
//
// A child can carry more than one live m.space.parent pointer — a
// pre-existing dual-canonical-parent violation from before this child was
// recategorised. Repair reads every live pointer (unbudgeted, so a candidate
// check never itself needs a write) and, when the desired parent is not the
// child's *only* live pointer, both clears every stale one and sets the
// desired one, so a single run actually converges to one canonical parent
// instead of leaving the old pointer standing and flapping between "repaired"
// and "skipped" on whichever pointer a later read happens to see. A child
// whose only live pointer already names the desired parent is skipped with no
// write and no accounting, so budget is spent only on genuine drift. A
// candidate whose full repair (which can take more than one write) does not
// fit the budget's remaining capacity is reported deferred rather than
// half-applied.
func (s *SpaceService) applyParentPointerRepair(
	ctx context.Context, parentRoomID id.RoomID, candidates []string, dryRun bool, outcome *writeOutcome,
) (repaired, deferred []string) {
	budget := newPointerRepairBudget(s.parentPointerEventsPerSecond, s.parentPointerBudgetPerCall, time.Now, time.Sleep)

	for _, childKey := range candidates {
		if ctx.Err() != nil {
			outcome.failed = true
			outcome.deadlineExceeded = true
			break
		}

		liveParents, readErr := s.matrix.GetSpaceParents(ctx, id.RoomID(childKey))
		if readErr != nil {
			// The read failed, so this child's pointer state is unknown, and
			// writing blind is not the conservative option it looks like:
			// SetSpaceParent writes canonical=true, so setting the desired
			// pointer over an unread — and therefore uncleared — stale one
			// manufactures exactly the dual-canonical violation (R-7) this
			// repair exists to resolve. Worse, the write succeeding would
			// leave outcome.failed false, so the pass would report success
			// while its pointer state was never actually verified, and
			// "repeat until failed==0" would terminate on an unread. Defer
			// the child and fail the call instead: the repair candidate set
			// is the whole resolved desired set on every pass, so a deferred
			// child is picked up by the next one at no cost.
			s.logger.Warn("Failed to read room-side parent pointers — deferring repair for this child",
				"child_room_id", childKey, "error", readErr)
			outcome.failed = true
			deferred = append(deferred, childKey)
			continue
		}
		plan := planParentPointerRepair(liveParents, parentRoomID)
		if plan.alreadyConverged() {
			continue // the desired parent is this child's only live pointer — nothing to repair.
		}

		writesNeeded := plan.writesNeeded()
		if !budget.canReserve(writesNeeded) {
			deferred = append(deferred, childKey)
			continue
		}
		if !dryRun && !outcome.canWrite(writesNeeded) {
			// Distinct from the pointer-repair budget's own, by-design deferral
			// above: this is the call's hard overall write ceiling, so running
			// into it means the call could not finish everything it needed to.
			outcome.failed = true
			deferred = append(deferred, childKey)
			continue
		}

		if s.executeParentPointerRepair(ctx, childKey, parentRoomID, plan, dryRun, budget, outcome) {
			repaired = append(repaired, childKey)
		}
	}
	return repaired, deferred
}

// parentPointerPlan is what one candidate child's pointer repair needs:
// which of its live m.space.parent pointers are stale and must be cleared,
// and whether the desired parent still needs to be set (false when it is
// already one of the child's live pointers).
type parentPointerPlan struct {
	stalePointers    []id.RoomID
	setDesiredNeeded bool
}

// planParentPointerRepair classifies a child's live parent pointers against
// the desired parent: every pointer that is not the desired one is stale and
// must be cleared, and the desired one only needs setting if it is not
// already among them.
func planParentPointerRepair(liveParents []id.RoomID, desiredParent id.RoomID) parentPointerPlan {
	plan := parentPointerPlan{setDesiredNeeded: true}
	for _, p := range liveParents {
		if p == desiredParent {
			plan.setDesiredNeeded = false
			continue
		}
		plan.stalePointers = append(plan.stalePointers, p)
	}
	return plan
}

// writesNeeded is the total write count one child's full plan takes: one
// clear per stale pointer, plus one set if the desired parent isn't live yet.
func (p parentPointerPlan) writesNeeded() int {
	n := len(p.stalePointers)
	if p.setDesiredNeeded {
		n++
	}
	return n
}

// alreadyConverged reports whether the child needs no write at all: the
// desired parent is its only live pointer.
func (p parentPointerPlan) alreadyConverged() bool {
	return !p.setDesiredNeeded && len(p.stalePointers) == 0
}

// executeParentPointerRepair performs (or, under dryRun, pretends to
// perform — pacing and counting nothing) one child's already-budgeted
// pointer plan: clearing every stale pointer and, if needed, setting the
// desired one. It reports whether every write in the plan succeeded.
func (s *SpaceService) executeParentPointerRepair(
	ctx context.Context, childKey string, parentRoomID id.RoomID, plan parentPointerPlan,
	dryRun bool, budget *pointerRepairBudget, outcome *writeOutcome,
) bool {
	ok := true
	for _, stalePointer := range plan.stalePointers {
		budget.reserve(!dryRun)
		if dryRun {
			continue
		}
		outcome.written++
		writeErr := s.matrix.ClearSpaceParent(ctx, id.RoomID(childKey), stalePointer)
		outcome.record(writeErr)
		if writeErr != nil {
			s.logger.Warn("Failed to clear stale room-side parent pointer",
				"child_room_id", childKey, "stale_parent_room_id", stalePointer, "error", writeErr)
			ok = false
		}
	}
	if plan.setDesiredNeeded {
		budget.reserve(!dryRun)
		if !dryRun {
			outcome.written++
			writeErr := s.matrix.SetSpaceParent(ctx, id.RoomID(childKey), parentRoomID)
			outcome.record(writeErr)
			if writeErr != nil {
				s.logger.Warn("Failed to repair room-side parent pointer",
					"child_room_id", childKey, "parent_room_id", parentRoomID, "error", writeErr)
				ok = false
			}
		}
	}
	return ok
}

// writeOutcome accumulates whether any write across a whole SetChildren call
// succeeded and whether any failed, so the honest-partial-converge verdict is
// computed once at the end regardless of which phase wrote it. It also
// enforces this call's own hard ceiling on total state-event writes
// (maxWrites, 0 = unbounded): the queue transport this operation is served
// over carries no cancellation signal the caller can use to stop a call that
// is taking too long, so the call must be able to stop itself rather than
// keep writing indefinitely once its own budget is spent.
type writeOutcome struct {
	succeeded bool
	failed    bool
	maxWrites int
	written   int
	// deadlineExceeded is set when a phase loop observes ctx.Err() (the
	// handler's own SetChildrenTimeout firing) and aborts before attempting
	// every write it otherwise would have. Kept apart from writeFailed so a
	// caller can be told "the adapter ran out of time" distinctly from
	// "Matrix rejected a write".
	deadlineExceeded bool
	// writeFailed is set when a write actually attempted against Matrix
	// (add, removal, prune, or parent-pointer repair) returned an error —
	// i.e. Matrix itself rejected or failed to apply the write, as opposed
	// to the write never being attempted because a budget or the deadline
	// was reached first.
	writeFailed bool
}

// record folds one write attempt's result (nil error = success) into the
// outcome.
func (o *writeOutcome) record(err error) {
	if err != nil {
		o.failed = true
		o.writeFailed = true
		return
	}
	o.succeeded = true
}

// allowWrite reports whether one more write may be attempted under maxWrites
// and, if so, counts it. Once the cap is spent it marks the call failed
// (an honest, incomplete partial — never a silently truncated success) and
// refuses every further write for the rest of the call.
func (o *writeOutcome) allowWrite() bool {
	if !o.canWrite(1) {
		o.failed = true
		return false
	}
	o.written++
	return true
}

// canWrite reports whether n more writes fit under maxWrites without
// spending any of it — used to decide, before issuing any write for one
// unit of work that can take more than one write (a pointer repair clearing
// a stale parent and setting the new one), whether the whole unit fits
// rather than leaving it half-applied.
func (o *writeOutcome) canWrite(n int) bool {
	return o.maxWrites <= 0 || o.written+n <= o.maxWrites
}

// toKeySet builds a set from a slice of Matrix state_keys.
func toKeySet(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}

// diffKeys returns the sorted keys present in want but absent from have.
func diffKeys(want, have map[string]struct{}) []string {
	diff := make([]string, 0, len(want))
	for key := range want {
		if _, ok := have[key]; !ok {
			diff = append(diff, key)
		}
	}
	sort.Strings(diff)
	return diff
}

// sortedKeys returns the keys of a set in sorted order, for deterministic
// iteration (e.g. pointer-repair candidate ordering) over a map.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ============================================================================
// Batch Membership Operations (communication.space.member.batch.*)
// ============================================================================

// BatchAddMember adds an actor to multiple spaces.
func (s *SpaceService) BatchAddMember(
	ctx context.Context,
	actorID uuid.UUID,
	contextIDs []uuid.UUID,
) map[string]error {
	results := make(map[string]error)
	actor := domain.NewActor(actorID)

	for _, contextID := range contextIDs {
		alias := s.idMapper.SpaceAlias(contextID)
		spaceRoomID, err := s.matrix.ResolveAlias(ctx, alias)
		if err != nil {
			results[contextID.String()] = domain.ErrSpaceNotFound
			continue
		}

		err = s.matrix.InviteToSpace(ctx, spaceRoomID, actor)
		results[contextID.String()] = err
	}

	return results
}

// BatchRemoveMember removes an actor from multiple spaces.
func (s *SpaceService) BatchRemoveMember(
	ctx context.Context,
	actorID uuid.UUID,
	contextIDs []uuid.UUID,
	reason string,
) map[string]error {
	results := make(map[string]error)
	userMatrixID := s.idMapper.UserID(actorID)

	for _, contextID := range contextIDs {
		alias := s.idMapper.SpaceAlias(contextID)
		spaceRoomID, err := s.matrix.ResolveAlias(ctx, alias)
		if err != nil {
			results[contextID.String()] = domain.ErrSpaceNotFound
			continue
		}

		err = s.matrix.KickFromSpace(ctx, spaceRoomID, userMatrixID, reason)
		results[contextID.String()] = err
	}

	return results
}
