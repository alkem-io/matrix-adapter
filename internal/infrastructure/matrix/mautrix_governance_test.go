package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

var govTestRoomID = uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")

// stateEventTypeSeen reports whether the intent sent a state event of the type.
func stateEventTypeSeen(intent *mockIntentAPI, eventType event.Type) bool {
	for _, sent := range intent.sendStateEventTypes {
		if sent == eventType {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Creation carries the ladder + markers + atomic alias (contract §3b)
// ----------------------------------------------------------------------------

func TestCreateRoomWithAlias_Ladder(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!governed:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{getRoomVersionResult: "10"})

	_, err := a.CreateRoomWithAlias(
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: govTestRoomID, RoomType: "community", Name: "Room"},
	)
	require.NoError(t, err)
	require.NotNil(t, botIntent.lastCreateRoomReq)

	// The create request's PowerLevelOverride IS the ladder — asserted for
	// the first time in this feature (research R-1: the old override was
	// only {users_default: 50}).
	want := BuildLadder(ClassConversation, "@bot:test.local", LadderOptions{})
	got := botIntent.lastCreateRoomReq.PowerLevelOverride
	require.NotNil(t, got)
	assert.False(t, LadderDiff(got, want), "PowerLevelOverride must equal ladder v1 for the class")

	// Initial state carries join rules, history visibility, guest access
	// forbidden and the (synthesized) identity marker.
	types := make([]string, 0, len(botIntent.lastCreateRoomReq.InitialState))
	for _, evt := range botIntent.lastCreateRoomReq.InitialState {
		types = append(types, evt.Type.Type)
	}
	assert.Contains(t, types, "m.room.join_rules")
	assert.Contains(t, types, "m.room.history_visibility")
	assert.Contains(t, types, "m.room.guest_access")
	assert.Contains(t, types, "io.alkemio.entity")

	// io.alkemio.governance is written after creation (bot state event).
	assert.True(t, stateEventTypeSeen(botIntent, StateAlkemioGovernance), "governance marker written last")
}

func TestCreateSpace_Ladder(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!space:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{getRoomVersionResult: "10"})

	_, err := a.CreateSpace(
		context.Background(),
		domain.CreateSpaceParams{AlkemioContextID: govTestRoomID, Name: "Space", JoinRule: "invite"},
	)
	require.NoError(t, err)
	require.NotNil(t, botIntent.lastCreateRoomReq)

	want := BuildLadder(ClassSpace, "@bot:test.local", LadderOptions{})
	got := botIntent.lastCreateRoomReq.PowerLevelOverride
	require.NotNil(t, got)
	assert.False(t, LadderDiff(got, want), "space PowerLevelOverride must equal ladder v1")

	types := make([]string, 0, len(botIntent.lastCreateRoomReq.InitialState))
	for _, evt := range botIntent.lastCreateRoomReq.InitialState {
		types = append(types, evt.Type.Type)
	}
	assert.Contains(t, types, "io.alkemio.entity")
	assert.Contains(t, types, "m.room.guest_access")
	assert.True(t, stateEventTypeSeen(botIntent, StateAlkemioGovernance))
}

func TestCreateRoom_AtomicAlias(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!atomic:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{getRoomVersionResult: "10"})

	_, err := a.CreateRoomWithAlias(
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: govTestRoomID, RoomType: "community", Name: "Room"},
	)
	require.NoError(t, err)
	require.NotNil(t, botIntent.lastCreateRoomReq)

	// The canonical alias is written atomically with creation (FR-020) …
	assert.Equal(t, govTestRoomID.String(), botIntent.lastCreateRoomReq.RoomAliasName)
	// … so CreateAlias runs exactly once, for the #t_ alias only.
	assert.Equal(t, 1, botIntent.createAliasCalled)
	assert.Equal(t, id.RoomAlias("#t_"+govTestRoomID.String()+":test.local"), botIntent.lastCreateAliasAlias)
}

func TestCreateRoom_RestrictedNeedsParent(t *testing.T) {
	parentID := uuid.MustParse("990e8400-e29b-41d4-a716-446655440004")

	t.Run("parent resolves: restricted with allow entry", func(t *testing.T) {
		botIntent := &mockIntentAPI{
			createRoomResult:   &mautrix.RespCreateRoom{RoomID: "!thread:test.local"},
			resolveAliasResult: &mautrix.RespAliasResolve{RoomID: "!parentspace:test.local"},
		}
		as := newMockAS(botIntent, nil)
		a := newFullTestAdapter(as, &mockAdminAPI{getRoomVersionResult: "10"})

		_, err := a.CreateRoomWithAlias(context.Background(), domain.CreateRoomParams{
			AlkemioRoomID: govTestRoomID, RoomType: "community", Name: "Thread",
			JoinRule: "restricted", ParentContextID: &parentID,
		})
		require.NoError(t, err)
		joinRules := findInitialState(t, botIntent.lastCreateRoomReq, event.StateJoinRules)
		content, ok := joinRules.Content.Parsed.(*event.JoinRulesEventContent)
		require.True(t, ok)
		assert.Equal(t, event.JoinRuleRestricted, content.JoinRule)
		require.Len(t, content.Allow, 1)
		assert.Equal(t, id.RoomID("!parentspace:test.local"), content.Allow[0].RoomID)
	})

	t.Run("parent does not resolve: stays invite (platform-driven)", func(t *testing.T) {
		botIntent := &mockIntentAPI{
			createRoomResult: &mautrix.RespCreateRoom{RoomID: "!thread:test.local"},
			resolveAliasErr:  errors.New("M_NOT_FOUND"),
		}
		as := newMockAS(botIntent, nil)
		a := newFullTestAdapter(as, &mockAdminAPI{getRoomVersionResult: "10"})

		_, err := a.CreateRoomWithAlias(context.Background(), domain.CreateRoomParams{
			AlkemioRoomID: govTestRoomID, RoomType: "community", Name: "Thread",
			JoinRule: "restricted", ParentContextID: &parentID,
		})
		require.NoError(t, err)
		joinRules := findInitialState(t, botIntent.lastCreateRoomReq, event.StateJoinRules)
		content, ok := joinRules.Content.Parsed.(*event.JoinRulesEventContent)
		require.True(t, ok)
		assert.Equal(t, event.JoinRuleInvite, content.JoinRule)
	})
}

func findInitialState(t *testing.T, req *mautrix.ReqCreateRoom, eventType event.Type) *event.Event {
	t.Helper()
	require.NotNil(t, req)
	for _, evt := range req.InitialState {
		if evt.Type == eventType {
			return evt
		}
	}
	t.Fatalf("initial state lacks %s", eventType.Type)
	return nil
}

// ----------------------------------------------------------------------------
// ApplyLadder: repair preserves guests, converged rooms write nothing
// ----------------------------------------------------------------------------

func plContentJSON(t *testing.T, pl *event.PowerLevelsEventContent) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(pl)
	require.NoError(t, err)
	var content map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &content))
	return content
}

func TestRepair_GuestEntriesPreserved(t *testing.T) {
	guest := id.UserID("@guest-7:test.local")
	current := BuildLadder(ClassThread, "@bot:test.local", LadderOptions{Guests: []id.UserID{guest}})
	// drift something so a write happens
	current.Events["m.room.name"] = 50

	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{getStateEventContentResult: plContentJSON(t, current)}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	changed, err := a.ApplyLadder(context.Background(), "!room:test.local", ClassThread, LadderOptions{}, false)
	require.NoError(t, err)
	assert.True(t, changed)

	written, ok := botIntent.lastSendStateEventContent.(*event.PowerLevelsEventContent)
	require.True(t, ok)
	assert.Equal(t, 0, written.Users[guest], "guest 0-entry preserved")
	assert.Equal(t, 100, written.Users["@bot:test.local"])
	assert.Len(t, written.Users, 2, "no other users entry survives")
}

func TestApplyLadder_Converged_NoWrite(t *testing.T) {
	current := BuildLadder(ClassConversation, "@bot:test.local", LadderOptions{})
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{getStateEventContentResult: plContentJSON(t, current)}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	changed, err := a.ApplyLadder(context.Background(), "!room:test.local", ClassConversation, LadderOptions{}, false)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, 0, botIntent.sendStateEventCalled, "a converged room produces zero writes")
}

func TestApplyLadder_DryRun_NoWrite(t *testing.T) {
	current := BuildLadder(ClassConversation, "@bot:test.local", LadderOptions{})
	current.Events["m.room.name"] = 50 // drifted
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{getStateEventContentResult: plContentJSON(t, current)}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	changed, err := a.ApplyLadder(context.Background(), "!room:test.local", ClassConversation, LadderOptions{}, true)
	require.NoError(t, err)
	assert.True(t, changed, "dry run reports the drift")
	assert.Equal(t, 0, botIntent.sendStateEventCalled, "dry run writes nothing")
}

// ----------------------------------------------------------------------------
// Reconciliation of Element-created rooms (T016)
// ----------------------------------------------------------------------------

const reconcileCreatorUUID = "550e8400-e29b-41d4-a716-446655440000"

func reconcileRoomInfo(extended bool) []byte {
	info := map[string]interface{}{
		"alkemio_room_id": govTestRoomID.String(),
		"type":            "direct",
		"is_direct":       false,
		"members": []map[string]string{
			{"actor_id": reconcileCreatorUUID, "display_name": "Creator"},
		},
	}
	if extended {
		info["entity_type"] = "thread"
		info["parent_context_id"] = "990e8400-e29b-41d4-a716-446655440004"
		info["join_rule"] = "invite"
		info["visibility"] = "shared"
	}
	raw, _ := json.Marshal(info)
	return raw
}

func newReconcileAdapter(t *testing.T, botIntent *mockIntentAPI, creatorIntent *mockIntentAPI, admin *mockAdminAPI, roomInfo []byte) *MautrixAdapter {
	t.Helper()
	creator := expectedUserID(uuid.MustParse(reconcileCreatorUUID))
	as := newMockAS(botIntent, map[id.UserID]intentAPI{creator: creatorIntent})
	a := newFullTestAdapter(as, admin)
	a.SetQueuePort(&testutil.MockQueuePort{PublishAndWaitResponse: roomInfo})
	return a
}

func TestReconcile_MakeRoomAdmin(t *testing.T) {
	creator := expectedUserID(uuid.MustParse(reconcileCreatorUUID))
	botIntent := &mockIntentAPI{}
	creatorIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		// The creator ghost is joined; the bot is not (Element-created room).
		getRoomMemberIDsResult: []id.UserID{creator},
		getRoomVersionResult:   "10",
		getStateEventContentQueue: []map[string]interface{}{
			// EnsureBotAdmin power check: bot absent from users → make_room_admin
			{"users": map[string]interface{}{creator.String(): float64(100)}},
			// re-read after promotion: bot at 100
			{"users": map[string]interface{}{creator.String(): float64(100), "@bot:test.local": float64(100)}},
			// ApplyLadder read: same (drifted vs ladder → rewrite)
			{"users": map[string]interface{}{creator.String(): float64(100), "@bot:test.local": float64(100)}},
		},
	}
	a := newReconcileAdapter(t, botIntent, creatorIntent, admin, reconcileRoomInfo(false))

	err := a.ReconcileRoom(context.Background(), "!element:test.local", govTestRoomID, creator)
	require.NoError(t, err)
	require.Len(t, admin.makeRoomAdminCalls, 1, "the bot is promoted via make_room_admin")
	assert.Equal(t, id.UserID("@bot:test.local"), admin.makeRoomAdminCalls[0].UserID)
}

func TestReconcile_UsesBotForPowerLevels(t *testing.T) {
	creator := expectedUserID(uuid.MustParse(reconcileCreatorUUID))
	botIntent := &mockIntentAPI{}
	creatorIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMemberIDsResult:     []id.UserID{creator, "@bot:test.local"},
		getRoomVersionResult:       "10",
		getStateEventContentResult: map[string]interface{}{"users": map[string]interface{}{creator.String(): float64(100), "@bot:test.local": float64(100)}},
	}
	a := newReconcileAdapter(t, botIntent, creatorIntent, admin, reconcileRoomInfo(false))

	err := a.ReconcileRoom(context.Background(), "!element:test.local", govTestRoomID, creator)
	require.NoError(t, err)

	assert.True(t, stateEventTypeSeen(botIntent, event.StatePowerLevels), "the ladder is written by the bot")
	assert.Equal(t, 0, creatorIntent.sendStateEventCalled, "the creator intent never writes state")

	// The wholesale ladder rewrite drops the creator's 100 by omission.
	for _, content := range botIntent.sendStateEventContents {
		if pl, ok := content.(*event.PowerLevelsEventContent); ok {
			_, creatorKept := pl.Users[creator]
			assert.False(t, creatorKept, "the Element creator's power entry is dropped")
		}
	}
}

func TestReconcile_DefaultsWhenRoomInfoLacksFields(t *testing.T) {
	creator := expectedUserID(uuid.MustParse(reconcileCreatorUUID))
	botIntent := &mockIntentAPI{}
	creatorIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMemberIDsResult:     []id.UserID{creator, "@bot:test.local"},
		getRoomVersionResult:       "10",
		getStateEventContentResult: map[string]interface{}{"users": map[string]interface{}{"@bot:test.local": float64(100)}},
	}
	a := newReconcileAdapter(t, botIntent, creatorIntent, admin, reconcileRoomInfo(false))

	err := a.ReconcileRoom(context.Background(), "!element:test.local", govTestRoomID, creator)
	require.NoError(t, err)

	// The old server's reply lacks entity_type/parent/join_rule/visibility:
	// the markers fall back to thread / platform-driven.
	var entity *domain.EntityMarker
	var governance *domain.GovernanceMarker
	for _, content := range botIntent.sendStateEventContents {
		if marker, ok := content.(*domain.EntityMarker); ok {
			entity = marker
		}
		if marker, ok := content.(*domain.GovernanceMarker); ok {
			governance = marker
		}
	}
	require.NotNil(t, entity, "identity marker written")
	assert.Equal(t, "thread", entity.EntityType)
	assert.Nil(t, entity.ParentID)
	require.NotNil(t, governance, "governance marker written")
	assert.Equal(t, domain.MembershipModePlatform, governance.MembershipMode)
}

// ----------------------------------------------------------------------------
// Device revocation + sweep (T022/T023, adapter half)
// ----------------------------------------------------------------------------

func TestRevokeActorDevices(t *testing.T) {
	actorID := uuid.MustParse(reconcileCreatorUUID)

	t.Run("all devices deleted in one call", func(t *testing.T) {
		lastSeen := int64(1757000000000)
		admin := &mockAdminAPI{listDevicesResult: []AdminDevice{
			{DeviceID: "AAA", LastSeenTS: &lastSeen},
			{DeviceID: "BBB"},
		}}
		a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

		deleted, err := a.RevokeActorDevices(context.Background(), actorID)
		require.NoError(t, err)
		assert.Equal(t, []string{"AAA", "BBB"}, deleted)
		require.Len(t, admin.deleteDevicesCalls, 1, "one delete_devices call for all devices")
		assert.Equal(t, []string{"AAA", "BBB"}, admin.deleteDevicesCalls[0].DeviceIDs)
	})

	t.Run("zero devices is a success with an empty result", func(t *testing.T) {
		admin := &mockAdminAPI{listDevicesResult: []AdminDevice{}}
		a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

		deleted, err := a.RevokeActorDevices(context.Background(), actorID)
		require.NoError(t, err)
		assert.Empty(t, deleted)
		assert.Empty(t, admin.deleteDevicesCalls)
	})

	t.Run("an admin error is an error, never a false success", func(t *testing.T) {
		lastSeen := int64(1757000000000)
		admin := &mockAdminAPI{
			listDevicesResult: []AdminDevice{{DeviceID: "AAA", LastSeenTS: &lastSeen}},
			deleteDevicesErr:  errors.New("admin api down"),
		}
		a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

		_, err := a.RevokeActorDevices(context.Background(), actorID)
		require.Error(t, err)
	})
}

func TestSweepDevices(t *testing.T) {
	now := time.UnixMilli(1757600000000)
	idle := 720 * time.Hour
	oldTS := now.Add(-idle - time.Hour).UnixMilli()
	freshTS := now.Add(-time.Hour).UnixMilli()

	newSweepAdapter := func(admin *mockAdminAPI) *MautrixAdapter {
		a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)
		a.now = func() time.Time { return now }
		return a
	}
	user := id.UserID("@550e8400-e29b-41d4-a716-446655440000:test.local")

	t.Run("idle deleted, active kept, no-last-seen skipped, bot never scanned", func(t *testing.T) {
		admin := &mockAdminAPI{
			listUsersResult: []AdminUser{{Name: "@bot:test.local"}, {Name: user}},
			listDevicesResult: []AdminDevice{
				{DeviceID: "IDLE", LastSeenTS: &oldTS},
				{DeviceID: "FRESH", LastSeenTS: &freshTS},
				{DeviceID: "NOTS"},
			},
		}
		report, err := newSweepAdapter(admin).SweepDevices(context.Background(), idle, false)
		require.NoError(t, err)
		assert.Equal(t, 1, report.ScannedUsers, "the bot is never scanned")
		assert.Equal(t, 3, report.ScannedDevices)
		assert.Equal(t, 1, report.Deleted)
		assert.Equal(t, 1, report.SkippedNoLastSeen)
		assert.Equal(t, 0, report.Failed)
		require.Len(t, admin.deleteDevicesCalls, 1)
		assert.Equal(t, []string{"IDLE"}, admin.deleteDevicesCalls[0].DeviceIDs)
	})

	t.Run("dry run deletes nothing", func(t *testing.T) {
		admin := &mockAdminAPI{
			listUsersResult:   []AdminUser{{Name: user}},
			listDevicesResult: []AdminDevice{{DeviceID: "IDLE", LastSeenTS: &oldTS}},
		}
		report, err := newSweepAdapter(admin).SweepDevices(context.Background(), idle, true)
		require.NoError(t, err)
		assert.True(t, report.DryRun)
		assert.Equal(t, 1, report.Deleted, "dry run reports what WOULD be deleted")
		assert.Empty(t, admin.deleteDevicesCalls, "dry run performs no deletion")
	})

	t.Run("per-user failure counted, sweep continues", func(t *testing.T) {
		admin := &mockAdminAPI{
			listUsersResult:   []AdminUser{{Name: user}},
			listDevicesResult: []AdminDevice{{DeviceID: "IDLE", LastSeenTS: &oldTS}},
			deleteDevicesErr:  errors.New("boom"),
		}
		report, err := newSweepAdapter(admin).SweepDevices(context.Background(), idle, false)
		require.NoError(t, err)
		assert.Equal(t, 1, report.Failed)
		assert.Equal(t, 0, report.Deleted)
	})
}

// ----------------------------------------------------------------------------
// Divergence record (T019)
// ----------------------------------------------------------------------------

func TestDivergence_LoggedAndCounted(t *testing.T) {
	logger := &adapterMockLogger{}
	before := domain.DivergenceCounts()[string(domain.DivergenceAlias)]

	domain.LogDivergence(logger, "!room:test.local", govTestRoomID.String(), domain.DivergenceAlias, "test detail")

	after := domain.DivergenceCounts()[string(domain.DivergenceAlias)]
	assert.Equal(t, before+1, after, "counter incremented per class")
}
