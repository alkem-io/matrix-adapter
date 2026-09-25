package matrix

import (
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// LadderVersion is the version of the governance power-level ladder this
// adapter applies (recorded in io.alkemio.governance.ladderVersion).
const LadderVersion = 1

// RoomClass is the governance class of a governed room (domain.RoomClass).
type RoomClass = domain.RoomClass

// Room class constants re-exported beside the ladder they parameterize.
const (
	ClassSpace        = domain.ClassSpace
	ClassThread       = domain.ClassThread
	ClassConversation = domain.ClassConversation
)

// LadderOptions carries the per-room user entries the ladder preserves or
// projects (domain.LadderOptions).
type LadderOptions = domain.LadderOptions

// Power levels of the governance ladder (data-model E2). Ordinary members
// (users_default 50) can only participate in the timeline; every
// administrative capability requires power only the bot (100) holds.
const (
	ladderUsersDefault  = 50
	ladderEventsDefault = 50
	ladderStateDefault  = 100
	ladderInvite        = 75
	ladderKick          = 100
	ladderBan           = 100
	ladderRedact        = 75
	ladderNotifyRoom    = 75
	ladderBotPower      = 100
	ladderElevatedPower = 75
	ladderGuestPower    = 0
	ladderTimelinePower = 50
	ladderGovernedState = 100
)

// ladderEvents is the explicit per-event-type power map of ladder version 1.
// Synapse seeds new rooms with m.room.name/avatar/canonical_alias at 50 and
// power_level_content_override merges at the TOP level, so omitting `events`
// would let members rename rooms even with state_default 100 (research R-5).
func ladderEvents() map[string]int {
	return map[string]int{
		"m.room.message": ladderTimelinePower,
		"m.reaction":     ladderTimelinePower,

		"m.room.name":               ladderGovernedState,
		"m.room.topic":              ladderGovernedState,
		"m.room.avatar":             ladderGovernedState,
		"m.room.canonical_alias":    ladderGovernedState,
		"m.room.power_levels":       ladderGovernedState,
		"m.room.join_rules":         ladderGovernedState,
		"m.room.history_visibility": ladderGovernedState,
		"m.room.guest_access":       ladderGovernedState,
		"m.room.pinned_events":      ladderGovernedState,
		"m.room.tombstone":          ladderGovernedState,
		"m.room.server_acl":         ladderGovernedState,
		"m.room.encryption":         ladderGovernedState,

		"m.space.child":  ladderGovernedState,
		"m.space.parent": ladderGovernedState,

		StateAlkemioEntity.Type:     ladderGovernedState,
		StateAlkemioGovernance.Type: ladderGovernedState,
		StateAlkemioVisibility.Type: ladderGovernedState,
		StateAlkemioPending.Type:    ladderGovernedState,
	}
}

// BuildLadder builds the exact m.room.power_levels content of ladder version 1
// for a room class (data-model E2). Guests keep explicit 0-entries; elevated
// entries (75) are applied for ClassSpace only; no other users entry exists.
func BuildLadder(class RoomClass, bot id.UserID, opts LadderOptions) *event.PowerLevelsEventContent {
	users := map[id.UserID]int{bot: ladderBotPower}
	for _, guest := range opts.Guests {
		if guest == bot {
			continue
		}
		users[guest] = ladderGuestPower
	}
	if class == ClassSpace {
		for _, elevated := range opts.Elevated {
			if elevated == bot {
				continue
			}
			users[elevated] = ladderElevatedPower
		}
	}

	stateDefault := ladderStateDefault
	invite := ladderInvite
	kick := ladderKick
	ban := ladderBan
	redact := ladderRedact
	notifyRoom := ladderNotifyRoom

	return &event.PowerLevelsEventContent{
		Users:           users,
		UsersDefault:    ladderUsersDefault,
		Events:          ladderEvents(),
		EventsDefault:   ladderEventsDefault,
		StateDefaultPtr: &stateDefault,
		InvitePtr:       &invite,
		KickPtr:         &kick,
		BanPtr:          &ban,
		RedactPtr:       &redact,
		Notifications:   &event.NotificationPowerLevels{RoomPtr: &notifyRoom},
	}
}

// LadderDiff reports whether two power-level contents differ semantically:
// absent pointers compare by their Matrix-spec default values, and nil maps
// equal empty maps, so a converged room produces zero writes.
func LadderDiff(current, want *event.PowerLevelsEventContent) bool {
	if current == nil || want == nil {
		return current != want
	}
	return ladderScalarsDiffer(current, want) ||
		usersDiffer(current.Users, want.Users) ||
		eventsDiffer(current.Events, want.Events)
}

// ladderScalarsDiffer compares the scalar levels by their effective values
// (accessors substitute the Matrix-spec defaults for absent pointers).
func ladderScalarsDiffer(current, want *event.PowerLevelsEventContent) bool {
	return current.UsersDefault != want.UsersDefault ||
		current.EventsDefault != want.EventsDefault ||
		current.StateDefault() != want.StateDefault() ||
		current.Invite() != want.Invite() ||
		current.Kick() != want.Kick() ||
		current.Ban() != want.Ban() ||
		current.Redact() != want.Redact() ||
		current.Notifications.Room() != want.Notifications.Room()
}

func usersDiffer(current, want map[id.UserID]int) bool {
	if len(current) != len(want) {
		return true
	}
	for user, level := range want {
		got, ok := current[user]
		if !ok || got != level {
			return true
		}
	}
	return false
}

func eventsDiffer(current, want map[string]int) bool {
	if len(current) != len(want) {
		return true
	}
	for eventType, level := range want {
		got, ok := current[eventType]
		if !ok || got != level {
			return true
		}
	}
	return false
}

// PreserveGuestEntries returns the share-link guest entries of a current
// power-level content: the user ids sitting at exactly power 0. They are the
// only per-user entries repair carries over (contract room-governance-ladder §2.2).
func PreserveGuestEntries(current *event.PowerLevelsEventContent) []id.UserID {
	if current == nil {
		return nil
	}
	var guests []id.UserID
	for user, level := range current.Users {
		if level == ladderGuestPower {
			guests = append(guests, user)
		}
	}
	return guests
}
