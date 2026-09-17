package dto

// ============================================================================
// Governance Repair DTOs (communication.room.governance.repair,
// communication.space.governance.repair, communication.space.member.revoke)
// 069-matrix-governance-hardening
// ============================================================================

// RoomVisibility declares the history-visibility class of a governed room.
type RoomVisibility string

const (
	// RoomVisibilityShared maps to m.room.history_visibility "shared".
	RoomVisibilityShared RoomVisibility = "shared"
	// RoomVisibilityWorldReadable maps to m.room.history_visibility "world_readable" (rooms under public spaces).
	RoomVisibilityWorldReadable RoomVisibility = "world_readable"
)

// RepairRoomGovernanceRequest asks the adapter to converge one room to the
// governance ladder: power levels, join rules, history visibility, guest
// access, identity/governance markers, aliases and bot presence.
// Topic: communication.room.governance.repair
type RepairRoomGovernanceRequest struct {
	AlkemioRoomID   AlkemioRoomID                     `json:"alkemio_room_id"`
	JoinRule        JoinRule                          `json:"join_rule"`
	ParentContextID *AlkemioContextID                 `json:"parent_context_id,omitempty"`
	CustomState     map[string]map[string]interface{} `json:"custom_state,omitempty"`
	Visibility      RoomVisibility                    `json:"visibility"`
	IsDirect        bool                              `json:"is_direct"`
	DryRun          bool                              `json:"dry_run"`
}

// RepairSpaceGovernanceRequest asks the adapter to converge one space room to
// the governance ladder, recomputing the elevated (PL 75) entries.
// Topic: communication.space.governance.repair
type RepairSpaceGovernanceRequest struct {
	AlkemioContextID AlkemioContextID                  `json:"alkemio_context_id"`
	CustomState      map[string]map[string]interface{} `json:"custom_state,omitempty"`
	ElevatedActorIDs []AlkemioActorID                  `json:"elevated_actor_ids"`
	DryRun           bool                              `json:"dry_run"`
}

// RepairIssue names one room or entity a repair run could not converge.
type RepairIssue struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// RepairFailure names one room or entity a repair run failed on.
type RepairFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// RepairReport is the counted outcome of a repair run. Counts derive from
// actual writes; a dry run computes everything and writes nothing.
type RepairReport struct {
	BaseResponse      `tstype:",extends"`
	Scanned           int             `json:"scanned"`
	Repaired          int             `json:"repaired"`
	Unresolved        []RepairIssue   `json:"unresolved,omitempty"`
	Failed            []RepairFailure `json:"failed,omitempty"`
	SkippedPreVersion []string        `json:"skipped_pre_version,omitempty"`
	DryRun            bool            `json:"dry_run"`
	Writes            int             `json:"writes"`
	BudgetRemaining   int             `json:"budget_remaining"`
}

// RevokeSpaceMemberRequest kicks an actor from the space rooms named by the
// context ids AND from every child room of each of those spaces.
// Topic: communication.space.member.revoke
type RevokeSpaceMemberRequest struct {
	ActorID           AlkemioActorID     `json:"actor_id"`
	AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
	Reason            string             `json:"reason,omitempty"`
}

// RevokeSpaceMemberResponse returns per-context results plus the number of
// child rooms the actor was kicked from.
type RevokeSpaceMemberResponse struct {
	BaseResponse `tstype:",extends"`
	// Results maps AlkemioContextID (string) to the per-space outcome.
	Results          map[string]BaseResponse `json:"results,omitempty"`
	ChildRoomsKicked int                     `json:"child_rooms_kicked"`
}
