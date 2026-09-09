package dto

// ============================================================================
// Hierarchy DTOs - V3 Protocol
// ============================================================================

// SetParentRequest establishes parent-child relationship for a room or subspace.
// Topic: communication.hierarchy.set_parent
type SetParentRequest struct {
	// ChildID is the AlkemioRoomID (for rooms) or AlkemioContextID (for subspaces) to add as child.
	ChildID         string           `json:"child_id"`
	IsSpace         bool             `json:"is_space"`
	ParentContextID AlkemioContextID `json:"parent_context_id"`
	Order           string           `json:"order,omitempty"`
	Suggested       bool             `json:"suggested,omitempty"`
}

// ============================================================================
// SetChildren (communication.hierarchy.set_children)
// ============================================================================

// SetChildrenRequest declares the full desired child set of a parent space (a
// category space, or the forum space itself) and converges the space's actual
// m.space.child edges toward it in one call: missing edges are added, and —
// only when requested — edges that no longer belong are removed.
//
// Unlike SetParentRequest, this operation can remove edges, which is why
// removal is always keyed on the raw Matrix child state_key read back from the
// parent's own state, never on an Alkemio identifier: once a discussion is
// deleted its room alias is gone for good, and no Alkemio UUID can ever name
// that child edge again. The parent itself is resolved by alias exactly like
// every other space operation and is never created if it is missing.
//
// Topic: communication.hierarchy.set_children
type SetChildrenRequest struct {
	// ParentContextID is the Alkemio context of the parent space. If it does not
	// resolve to an existing space the call fails with SPACE_NOT_FOUND — this
	// operation never lazily creates a space.
	ParentContextID AlkemioContextID `json:"parent_context_id"`
	// DesiredChildContextIDs are the Alkemio context/room ids that must end up as
	// children of the parent. An id that cannot be resolved to a room is reported
	// in the response's unresolved list rather than guessed at or created.
	DesiredChildContextIDs []string `json:"desired_child_context_ids"`
	// ChildrenAreSpaces selects which alias namespace DesiredChildContextIDs are
	// resolved through: false resolves each id as a discussion room (the normal
	// category-level call), true resolves each id as a subspace (the forum-level
	// call that converges category spaces under the forum).
	ChildrenAreSpaces bool `json:"children_are_spaces"`
	// ApplyRemovals enables the removal half of convergence. When false, only
	// missing edges are added and no extra edge is inspected or touched — the
	// shape a two-phase sweep uses for its add-only first pass.
	ApplyRemovals bool `json:"apply_removals"`
	// PruneUnknown additionally removes extra edges whose state_key no longer
	// resolves to any live Alkemio room (a deleted discussion's ghost edge).
	// Ignored unless ApplyRemovals is also set; extras that resolve to a live
	// room are always eligible for removal regardless of this flag.
	PruneUnknown bool `json:"prune_unknown"`
	// SyncChildParent opts into repairing the room-side m.space.parent pointer on
	// children that were just added, under its own separate, lower write budget.
	// Left false, no room-side state is ever touched by this call.
	SyncChildParent bool `json:"sync_child_parent"`
	// DryRun computes and reports the full classification with zero writes.
	DryRun bool `json:"dry_run"`
}

// SetChildrenResponse reports what a set_children call did (or, under DryRun,
// would have done) to converge one parent space's children toward the desired
// set. Every count a caller derives from this operation must come from these
// arrays, never from iterating a request list — a disabled or unreachable
// adapter never produces one of these at all.
type SetChildrenResponse struct {
	BaseResponse `tstype:",extends"`
	// Added holds the state_keys of edges added (or, under DryRun, that would be
	// added) because a desired child was missing from the actual set.
	Added []string `json:"added"`
	// Removed holds the state_keys of extra edges removed (or would be removed)
	// because they no longer belong and still resolve to a live Alkemio room —
	// the recategorisation-leftover class.
	Removed []string `json:"removed"`
	// PrunedUnknown holds the state_keys of extra edges removed (or would be
	// removed) under PruneUnknown because they resolve to no Alkemio room at all
	// — the ghost-edge class left behind by a deleted discussion.
	PrunedUnknown []string `json:"pruned_unknown"`
	// UnknownKept holds the state_keys of extra edges that resolve to no Alkemio
	// room and were left in place because PruneUnknown was not set. This is the
	// drift a report-only pass surfaces without ever touching it.
	UnknownKept []string `json:"unknown_kept"`
	// Unresolved holds the desired child ids that did not resolve to any room —
	// reported only, never fabricated and never created.
	Unresolved []string `json:"unresolved"`
	// ParentPointersRepaired holds the state_keys of children whose room-side
	// m.space.parent was corrected (only ever populated when SyncChildParent).
	ParentPointersRepaired []string `json:"parent_pointers_repaired"`
	// ParentPointersDeferred holds the state_keys of children whose parent-pointer
	// repair was skipped because the separate pointer-repair budget was spent —
	// reported so a deferral is never mistaken for a completed repair.
	ParentPointersDeferred []string `json:"parent_pointers_deferred"`
	// Changed is true if any write was performed (or, under DryRun, would be).
	Changed bool `json:"changed"`
	// DryRun echoes the request flag.
	DryRun bool `json:"dry_run"`
}
