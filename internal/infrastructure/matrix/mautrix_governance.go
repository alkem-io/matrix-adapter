package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// ============================================================================
// Governance Operations
// ============================================================================

// clock returns the adapter's time source (injectable for tests).
func (m *MautrixAdapter) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// getPowerLevels reads the room's current m.room.power_levels content via the
// admin API (works regardless of bot membership). Returns nil when absent.
func (m *MautrixAdapter) getPowerLevels(ctx context.Context, roomID id.RoomID) (*event.PowerLevelsEventContent, error) {
	content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.power_levels")
	if err != nil {
		return nil, fmt.Errorf("failed to read power levels of %s: %w", roomID, err)
	}
	if content == nil {
		return nil, nil
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal power levels of %s: %w", roomID, err)
	}
	var pl event.PowerLevelsEventContent
	if err := json.Unmarshal(raw, &pl); err != nil {
		return nil, fmt.Errorf("failed to parse power levels of %s: %w", roomID, err)
	}
	return &pl, nil
}

// ApplyLadder rewrites the room's power levels wholesale from the governance
// ladder as the bot, preserving guest 0-entries from the current event and
// applying opts (class-S elevated entries). A converged room produces zero
// writes; a dry run computes the diff and writes nothing.
func (m *MautrixAdapter) ApplyLadder(
	ctx context.Context, roomID id.RoomID, class domain.RoomClass, opts domain.LadderOptions, dryRun bool,
) (bool, error) {
	current, err := m.getPowerLevels(ctx, roomID)
	if err != nil {
		return false, err
	}

	merged := domain.LadderOptions{
		Guests:   append(append([]id.UserID{}, opts.Guests...), PreserveGuestEntries(current)...),
		Elevated: opts.Elevated,
	}
	want := BuildLadder(class, m.as.BotMXID(), merged)

	if current != nil && !LadderDiff(current, want) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	if _, err := m.as.BotIntent().SendStateEvent(ctx, roomID, event.StatePowerLevels, "", want); err != nil {
		return false, fmt.Errorf("failed to apply ladder to %s: %w", roomID, err)
	}
	return true, nil
}

// GetRoomGovernanceState reads the room's current governed state in one pass
// via the admin API (works regardless of bot membership).
func (m *MautrixAdapter) GetRoomGovernanceState(
	ctx context.Context, roomID id.RoomID,
) (*domain.RoomGovernanceState, error) {
	stateEvents, err := m.admin.GetRoomState(ctx, roomID, "")
	if err != nil {
		return nil, fmt.Errorf("failed to read state of %s: %w", roomID, err)
	}

	state := &domain.RoomGovernanceState{}
	for _, raw := range stateEvents {
		var evt struct {
			Type     string                 `json:"type"`
			StateKey string                 `json:"state_key"`
			Content  map[string]interface{} `json:"content"`
		}
		if err := json.Unmarshal(raw, &evt); err != nil {
			continue
		}
		m.applyGovernanceStateEvent(state, evt.Type, evt.StateKey, evt.Content)
	}

	if aliases, err := m.GetRoomAliases(ctx, roomID); err == nil {
		state.Aliases = aliases
	}
	return state, nil
}

// applyGovernanceStateEvent folds one state event into a RoomGovernanceState.
func (m *MautrixAdapter) applyGovernanceStateEvent(
	state *domain.RoomGovernanceState, eventType, stateKey string, content map[string]interface{},
) {
	switch eventType {
	case "m.room.join_rules":
		applyJoinRulesContent(state, content)
	case "m.room.history_visibility":
		if visibility, ok := content["history_visibility"].(string); ok {
			state.HistoryVisibility = visibility
		}
	case "m.room.guest_access":
		if access, ok := content["guest_access"].(string); ok {
			state.GuestAccess = access
		}
	case "m.space.parent":
		state.SpaceParents = append(state.SpaceParents, stateKey)
	case StateAlkemioEntity.Type:
		state.Entity = decodeMarker[domain.EntityMarker](content)
	case StateAlkemioGovernance.Type:
		state.Governance = decodeMarker[domain.GovernanceMarker](content)
	}
}

// applyJoinRulesContent folds an m.room.join_rules content into the state.
func applyJoinRulesContent(state *domain.RoomGovernanceState, content map[string]interface{}) {
	if rule, ok := content["join_rule"].(string); ok {
		state.JoinRule = rule
	}
	allow, ok := content["allow"].([]interface{})
	if !ok || len(allow) == 0 {
		return
	}
	if entry, ok := allow[0].(map[string]interface{}); ok {
		if allowRoom, ok := entry["room_id"].(string); ok {
			state.JoinRuleAllowRoom = allowRoom
		}
	}
}

// decodeMarker round-trips a raw state content into a typed marker (nil on failure).
func decodeMarker[T any](content map[string]interface{}) *T {
	raw, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	var marker T
	if json.Unmarshal(raw, &marker) != nil {
		return nil
	}
	return &marker
}

// SetRoomAccessState writes the desired join rule / history visibility / guest
// access as the bot. Nil fields are left untouched.
func (m *MautrixAdapter) SetRoomAccessState(
	ctx context.Context, roomID id.RoomID, access domain.RoomAccessState,
) error {
	botIntent := m.as.BotIntent()

	if access.JoinRule != nil {
		joinRules := &event.JoinRulesEventContent{JoinRule: event.JoinRule(*access.JoinRule)}
		if *access.JoinRule == "restricted" && access.JoinRuleAllowRoom != "" {
			joinRules.Allow = []event.JoinRuleAllow{{
				RoomID: id.RoomID(access.JoinRuleAllowRoom),
				Type:   event.JoinRuleAllowRoomMembership,
			}}
		}
		if _, err := botIntent.SendStateEvent(ctx, roomID, event.StateJoinRules, "", joinRules); err != nil {
			return fmt.Errorf("failed to set join rules on %s: %w", roomID, err)
		}
	}
	if access.HistoryVisibility != nil {
		content := &event.HistoryVisibilityEventContent{
			HistoryVisibility: event.HistoryVisibility(*access.HistoryVisibility),
		}
		if _, err := botIntent.SendStateEvent(ctx, roomID, event.StateHistoryVisibility, "", content); err != nil {
			return fmt.Errorf("failed to set history visibility on %s: %w", roomID, err)
		}
	}
	if access.GuestAccess != nil {
		content := &event.GuestAccessEventContent{GuestAccess: event.GuestAccess(*access.GuestAccess)}
		if _, err := botIntent.SendStateEvent(ctx, roomID, event.StateGuestAccess, "", content); err != nil {
			return fmt.Errorf("failed to set guest access on %s: %w", roomID, err)
		}
	}
	return nil
}

// EnsureDirectRoomMarked ensures both participants of a direct room carry it
// in their m.direct account data. Idempotent — existing entries are kept.
func (m *MautrixAdapter) EnsureDirectRoomMarked(ctx context.Context, roomID id.RoomID) error {
	members, err := m.admin.GetRoomMemberIDs(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to read members of %s: %w", roomID, err)
	}
	botMXID := m.as.BotMXID()
	var participants []id.UserID
	for _, member := range members {
		if member == botMXID {
			continue
		}
		if m.idMapper.AlkemioActorID(member) != uuid.Nil {
			participants = append(participants, member)
		}
	}
	m.registerDirectRoomParticipants(ctx, roomID, true, participants)
	return nil
}

// botUnreachable is the unresolved reason of contract governed-operations-authority §4.
const botUnreachable = "bot-unreachable"

// botIntentForGoverned establishes the bot in the room and returns its intent.
// Every governed write goes through here — there is no ghost fallback
// (governed-operations-authority §1).
func (m *MautrixAdapter) botIntentForGoverned(ctx context.Context, roomID id.RoomID) (intentAPI, error) {
	presence, err := m.EnsureBotAdmin(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if presence.UnresolvedReason != "" {
		return nil, fmt.Errorf("bot could not be established in room %s: %s", roomID, presence.UnresolvedReason)
	}
	return m.as.BotIntent(), nil
}

// EnsureBotAdmin establishes the bot as a joined, power-100 member of the room
// in the §4 order: already joined → join → ghost-invite then join →
// make_room_admin for power. An unrecoverable room yields UnresolvedReason,
// never an error.
func (m *MautrixAdapter) EnsureBotAdmin(ctx context.Context, roomID id.RoomID) (domain.BotPresence, error) {
	botMXID := m.as.BotMXID()

	joined, err := m.isBotJoined(ctx, roomID)
	if err != nil {
		return domain.BotPresence{}, err
	}

	if !joined {
		if !m.joinBot(ctx, roomID) {
			return domain.BotPresence{UnresolvedReason: botUnreachable}, nil
		}
	}

	// Power: the bot must sit at 100 in the users map.
	pl, err := m.getPowerLevels(ctx, roomID)
	if err != nil {
		return domain.BotPresence{Joined: true}, err
	}
	if pl != nil && pl.Users[botMXID] == 100 {
		return domain.BotPresence{Joined: true, PowerOK: true}, nil
	}

	if err := m.admin.MakeRoomAdmin(ctx, roomID, botMXID); err != nil {
		m.logger.Warn("EnsureBotAdmin: make_room_admin failed", "room_id", roomID, "error", err)
		return domain.BotPresence{Joined: true, UnresolvedReason: botUnreachable}, nil
	}
	pl, err = m.getPowerLevels(ctx, roomID)
	if err != nil {
		return domain.BotPresence{Joined: true}, err
	}
	if pl == nil || pl.Users[botMXID] < 100 {
		return domain.BotPresence{Joined: true, UnresolvedReason: botUnreachable}, nil
	}
	return domain.BotPresence{Joined: true, PowerOK: true}, nil
}

// isBotJoined checks the bot's membership through the admin API.
func (m *MautrixAdapter) isBotJoined(ctx context.Context, roomID id.RoomID) (bool, error) {
	members, err := m.admin.GetRoomMemberIDs(ctx, roomID)
	if err != nil {
		return false, fmt.Errorf("failed to read members of %s: %w", roomID, err)
	}
	botMXID := m.as.BotMXID()
	for _, member := range members {
		if member == botMXID {
			return true, nil
		}
	}
	return false, nil
}

// joinBot joins the bot to the room: direct join first (public or restricted
// rule), then one impersonated act — a joined ghost invites the bot (the
// module explicitly allows an invite whose invitee is the bot) — and a second
// join. Returns false when neither path succeeds (e.g. an empty invite-only room).
func (m *MautrixAdapter) joinBot(ctx context.Context, roomID id.RoomID) bool {
	botMXID := m.as.BotMXID()
	botIntent := m.as.BotIntent()

	if err := botIntent.EnsureJoined(ctx, roomID); err == nil {
		m.syncBotMembership(ctx, roomID)
		return true
	}

	ghost := m.findJoinedGhost(ctx, roomID)
	if ghost == "" {
		return false
	}
	if _, err := m.as.Intent(ghost).InviteUser(ctx, roomID, &mautrix.ReqInviteUser{UserID: botMXID}); err != nil {
		m.logger.Warn("EnsureBotAdmin: ghost invite of the bot failed",
			"room_id", roomID, "ghost", ghost, "error", err)
		return false
	}
	if err := botIntent.EnsureJoined(ctx, roomID); err != nil {
		m.logger.Warn("EnsureBotAdmin: join after ghost invite failed",
			"room_id", roomID, "error", err)
		return false
	}
	m.syncBotMembership(ctx, roomID)
	return true
}

// syncBotMembership updates the StateStore so EnsureJoined callers see the
// bot as joined and don't create duplicate join events.
func (m *MautrixAdapter) syncBotMembership(ctx context.Context, roomID id.RoomID) {
	if err := m.as.SetMembership(ctx, roomID, m.as.BotMXID(), event.MembershipJoin); err != nil {
		m.logger.Warn("EnsureBotAdmin: failed to sync StateStore after join",
			"room_id", roomID, "error", err)
	}
}

// findJoinedGhost returns one joined ghost user (UUID localpart) of the room,
// or empty when none exists.
func (m *MautrixAdapter) findJoinedGhost(ctx context.Context, roomID id.RoomID) id.UserID {
	members, err := m.admin.GetRoomMemberIDs(ctx, roomID)
	if err != nil {
		return ""
	}
	botMXID := m.as.BotMXID()
	for _, member := range members {
		if member == botMXID {
			continue
		}
		if m.idMapper.AlkemioActorID(member) != uuid.Nil {
			return member
		}
	}
	return ""
}

// GetRoomVersion returns the room's Matrix room version.
func (m *MautrixAdapter) GetRoomVersion(ctx context.Context, roomID id.RoomID) (string, error) {
	return m.admin.GetRoomVersion(ctx, roomID)
}

// SetGovernanceState writes the platform identity and/or governance markers as
// the bot. Nil markers are skipped.
func (m *MautrixAdapter) SetGovernanceState(
	ctx context.Context, roomID id.RoomID, entity *domain.EntityMarker, governance *domain.GovernanceMarker,
) error {
	botIntent := m.as.BotIntent()
	if entity != nil {
		if _, err := botIntent.SendStateEvent(ctx, roomID, StateAlkemioEntity, "", entity); err != nil {
			return fmt.Errorf("failed to set io.alkemio.entity on %s: %w", roomID, err)
		}
	}
	if governance != nil {
		if _, err := botIntent.SendStateEvent(ctx, roomID, StateAlkemioGovernance, "", governance); err != nil {
			return fmt.Errorf("failed to set io.alkemio.governance on %s: %w", roomID, err)
		}
	}
	return nil
}

// RevokeActorDevices deletes ALL of the actor's Matrix devices in one call.
// Zero devices is a success with an empty result (idempotent revocation).
func (m *MautrixAdapter) RevokeActorDevices(ctx context.Context, actorID uuid.UUID) ([]string, error) {
	userID := m.idMapper.UserID(actorID)
	devices, err := m.admin.ListDevices(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices of %s: %w", userID, err)
	}
	if len(devices) == 0 {
		return []string{}, nil
	}
	deviceIDs := make([]string, len(devices))
	for i, device := range devices {
		deviceIDs[i] = device.DeviceID
	}
	if err := m.admin.DeleteDevices(ctx, userID, deviceIDs); err != nil {
		return nil, fmt.Errorf("failed to delete devices of %s: %w", userID, err)
	}
	return deviceIDs, nil
}

// sweepUsersPageSize is the admin user-list page size of the device sweep.
const sweepUsersPageSize = 100

// SweepDevices deletes devices whose last recorded use is older than idle.
// Devices without a recorded last use are skipped and counted; the bot is
// never touched; per-user failures are counted and the sweep continues.
func (m *MautrixAdapter) SweepDevices(
	ctx context.Context, idle time.Duration, dryRun bool,
) (domain.SweepReport, error) {
	report := domain.SweepReport{DryRun: dryRun}
	cutoff := m.clock().Add(-idle)
	botMXID := m.as.BotMXID()

	from := ""
	for {
		users, next, err := m.admin.ListUsers(ctx, from, sweepUsersPageSize)
		if err != nil {
			return report, fmt.Errorf("failed to list users: %w", err)
		}
		for _, user := range users {
			if user.Name == botMXID {
				continue
			}
			report.ScannedUsers++
			m.sweepUserDevices(ctx, user.Name, cutoff, dryRun, &report)
		}
		if next == "" {
			break
		}
		from = next
	}
	return report, nil
}

// sweepUserDevices sweeps one user's devices into the report.
func (m *MautrixAdapter) sweepUserDevices(
	ctx context.Context, userID id.UserID, cutoff time.Time, dryRun bool, report *domain.SweepReport,
) {
	devices, err := m.admin.ListDevices(ctx, userID)
	if err != nil {
		m.logger.Warn("SweepDevices: failed to list devices", "user_id", userID, "error", err)
		report.Failed++
		return
	}

	var toDelete []string
	for _, device := range devices {
		report.ScannedDevices++
		if device.LastSeenTS == nil {
			report.SkippedNoLastSeen++
			continue
		}
		if time.UnixMilli(*device.LastSeenTS).Before(cutoff) {
			toDelete = append(toDelete, device.DeviceID)
		}
	}
	if len(toDelete) == 0 {
		return
	}
	if !dryRun {
		if err := m.admin.DeleteDevices(ctx, userID, toDelete); err != nil {
			m.logger.Warn("SweepDevices: failed to delete devices", "user_id", userID, "error", err)
			report.Failed++
			return
		}
	}
	report.Deleted += len(toDelete)
}
