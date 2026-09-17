package domain

import (
	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// RoomClass is the governance class of a governed room (spec data-model E1).
type RoomClass int

const (
	// ClassSpace is an m.space room owned by an Alkemio space.
	ClassSpace RoomClass = iota
	// ClassThread is a space-anchored thread room (updates, comments).
	ClassThread
	// ClassConversation is a direct or group conversation room.
	ClassConversation
)

// LadderOptions carries the per-room user entries the ladder preserves or
// projects: share-link guests (power 0, preserved on repair) and, for class S
// only, the elevated admin/lead entries (power 75, recomputed every repair).
type LadderOptions struct {
	Guests   []id.UserID
	Elevated []id.UserID
}

// EntityMarker is the content of the io.alkemio.entity state event: the
// Alkemio entity that owns a governed room. ParentID is the owning space id
// for space-anchored rooms, nil for conversation rooms and top-level spaces.
type EntityMarker struct {
	EntityID   string  `json:"entityId"`
	EntityType string  `json:"entityType"` // "thread" | "space"
	ParentID   *string `json:"parentId"`
}

// GovernanceMarker is the content of the io.alkemio.governance state event:
// the governance state of a room as last applied by the control plane.
type GovernanceMarker struct {
	LadderVersion  int    `json:"ladderVersion"`
	MembershipMode string `json:"membershipMode"` // "space" | "platform" | "projected"
	AppliedAt      int64  `json:"appliedAt"`      // epoch ms
	RoomVersion    string `json:"roomVersion"`
}

// Membership modes of io.alkemio.governance.membershipMode.
const (
	// MembershipModeSpace admits members by space-room membership (restricted join rule).
	MembershipModeSpace = "space"
	// MembershipModePlatform admits members only by control-plane invitation.
	MembershipModePlatform = "platform"
	// MembershipModeProjected is the space room itself: membership projected from authorization.
	MembershipModeProjected = "projected"
)

// BotPresence is the outcome of establishing the control-plane bot in a room
// (contract governed-operations-authority §4).
type BotPresence struct {
	Joined           bool
	PowerOK          bool
	UnresolvedReason string // non-empty when the bot could not be established, e.g. "bot-unreachable"
}

// SweepReport is the counted outcome of one idle-device sweep run.
type SweepReport struct {
	ScannedUsers      int  `json:"scanned_users"`
	ScannedDevices    int  `json:"scanned_devices"`
	Deleted           int  `json:"deleted"`
	SkippedNoLastSeen int  `json:"skipped_no_last_seen"`
	Failed            int  `json:"failed"`
	DryRun            bool `json:"dry_run"`
}

// CreateRoomParams carries a governed room creation (port CreateRoomWithAlias).
type CreateRoomParams struct {
	AlkemioRoomID   uuid.UUID
	RoomType        string // "direct" | "community"
	Name            string
	Topic           string
	AvatarURL       string
	JoinRule        string // declared membership mode: "restricted" | "invite" | ""
	ParentContextID *uuid.UUID
	WorldReadable   bool // history_visibility world_readable (room under a public space)
	CustomState     map[string]map[string]interface{}
	InitialMembers  []Actor
}

// CreateSpaceParams carries a governed space creation (port CreateSpace).
type CreateSpaceParams struct {
	AlkemioContextID uuid.UUID
	Name             string
	Topic            string
	AvatarURL        string
	JoinRule         string
	ParentContextID  *uuid.UUID
	CustomState      map[string]map[string]interface{}
	InitialMembers   []Actor
}

// RoomGovernanceState is the current governed state of a room as read from
// the homeserver — the compare half of every compare-before-write in repair.
type RoomGovernanceState struct {
	JoinRule          string
	JoinRuleAllowRoom string // the m.space room id of a restricted rule's allow entry
	HistoryVisibility string
	GuestAccess       string
	Entity            *EntityMarker
	Governance        *GovernanceMarker
	Aliases           []string
	SpaceParents      []string // state keys of m.space.parent events
	IsDirect          bool     // room's create content marks it a DM? (not readable — derived by caller)
}

// RoomAccessState is the desired access shape a repair writes (nil = no change).
type RoomAccessState struct {
	JoinRule          *string
	JoinRuleAllowRoom string // used only with JoinRule "restricted"
	HistoryVisibility *string
	GuestAccess       *string
}

// RepairOutcome is the counted outcome of one repair invocation (data-model E7).
type RepairOutcome struct {
	Scanned           int
	Repaired          int
	Unresolved        []RepairProblem
	Failed            []RepairProblem
	SkippedPreVersion []string
	DryRun            bool
	Writes            int
	BudgetRemaining   int
}

// RepairProblem names one room or entity a repair could not converge or failed on.
type RepairProblem struct {
	ID     string
	Reason string
}
