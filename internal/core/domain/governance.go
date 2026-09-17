package domain

import (
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
