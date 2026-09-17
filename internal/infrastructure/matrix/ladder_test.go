package matrix

import (
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const testBot = id.UserID("@00000000-0000-0000-0000-000000000000:example.com")

// TestLadder is the executable half of contract room-governance-ladder §3a:
// it asserts, for each class, every value of ladder version 1 (data-model E2),
// guest preservation, elevated recompute, and that no other users entry survives.
func TestLadder(t *testing.T) {
	guest := id.UserID("@guest-1:example.com")
	admin := id.UserID("@11111111-1111-4111-8111-111111111111:example.com")
	lead := id.UserID("@22222222-2222-4222-8222-222222222222:example.com")

	governedStateTypes := []string{
		"m.room.name", "m.room.topic", "m.room.avatar", "m.room.canonical_alias",
		"m.room.power_levels", "m.room.join_rules", "m.room.history_visibility",
		"m.room.guest_access", "m.room.pinned_events", "m.room.tombstone",
		"m.room.server_acl", "m.room.encryption",
		"m.space.child", "m.space.parent",
		"io.alkemio.entity", "io.alkemio.governance",
		"io.alkemio.visibility", "io.alkemio.pending",
	}

	cases := []struct {
		name        string
		class       RoomClass
		opts        LadderOptions
		wantUsers   map[id.UserID]int
		extraForbid []id.UserID // must NOT appear in users
	}{
		{
			name:      "space with elevated and guest",
			class:     ClassSpace,
			opts:      LadderOptions{Guests: []id.UserID{guest}, Elevated: []id.UserID{admin, lead}},
			wantUsers: map[id.UserID]int{testBot: 100, guest: 0, admin: 75, lead: 75},
		},
		{
			name:        "thread ignores elevated, keeps guests",
			class:       ClassThread,
			opts:        LadderOptions{Guests: []id.UserID{guest}, Elevated: []id.UserID{admin}},
			wantUsers:   map[id.UserID]int{testBot: 100, guest: 0},
			extraForbid: []id.UserID{admin},
		},
		{
			name:      "conversation bare",
			class:     ClassConversation,
			opts:      LadderOptions{},
			wantUsers: map[id.UserID]int{testBot: 100},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pl := BuildLadder(tc.class, testBot, tc.opts)
			assertLadderScalars(t, pl)
			assertLadderEvents(t, pl, governedStateTypes)
			assertLadderUsers(t, pl, tc.wantUsers, tc.extraForbid)
		})
	}
}

func assertLadderScalars(t *testing.T, pl *event.PowerLevelsEventContent) {
	t.Helper()
	if pl.UsersDefault != 50 {
		t.Errorf("users_default = %d, want 50", pl.UsersDefault)
	}
	if pl.EventsDefault != 50 {
		t.Errorf("events_default = %d, want 50", pl.EventsDefault)
	}
	var notificationsRoom *int
	if pl.Notifications != nil {
		notificationsRoom = pl.Notifications.RoomPtr
	}
	pointerLevels := []struct {
		name string
		got  *int
		want int
	}{
		{"state_default", pl.StateDefaultPtr, 100},
		{"invite", pl.InvitePtr, 75},
		{"kick", pl.KickPtr, 100},
		{"ban", pl.BanPtr, 100},
		{"redact", pl.RedactPtr, 75},
		{"notifications.room", notificationsRoom, 75},
	}
	for _, level := range pointerLevels {
		if level.got == nil || *level.got != level.want {
			t.Errorf("%s = %v, want explicit %d", level.name, level.got, level.want)
		}
	}
}

func assertLadderEvents(t *testing.T, pl *event.PowerLevelsEventContent, governedStateTypes []string) {
	t.Helper()
	for _, stateType := range governedStateTypes {
		if got, ok := pl.Events[stateType]; !ok || got != 100 {
			t.Errorf("events[%s] = %d (present=%v), want 100", stateType, got, ok)
		}
	}
	if got := pl.Events["m.room.message"]; got != 50 {
		t.Errorf("events[m.room.message] = %d, want 50", got)
	}
	if got := pl.Events["m.reaction"]; got != 50 {
		t.Errorf("events[m.reaction] = %d, want 50", got)
	}
	if len(pl.Events) != len(governedStateTypes)+2 {
		t.Errorf("events map has %d entries, want %d", len(pl.Events), len(governedStateTypes)+2)
	}
}

func assertLadderUsers(t *testing.T, pl *event.PowerLevelsEventContent, wantUsers map[id.UserID]int, extraForbid []id.UserID) {
	t.Helper()
	if len(pl.Users) != len(wantUsers) {
		t.Errorf("users has %d entries, want %d: %v", len(pl.Users), len(wantUsers), pl.Users)
	}
	for user, level := range wantUsers {
		if got, ok := pl.Users[user]; !ok || got != level {
			t.Errorf("users[%s] = %d (present=%v), want %d", user, got, ok, level)
		}
	}
	for _, forbidden := range extraForbid {
		if _, ok := pl.Users[forbidden]; ok {
			t.Errorf("users[%s] present, must be absent for this class", forbidden)
		}
	}
}

func TestLadder_ElevatedRecomputedNotAccumulated(t *testing.T) {
	adminA := id.UserID("@aaaa1111-0000-4000-8000-000000000001:example.com")
	adminB := id.UserID("@bbbb2222-0000-4000-8000-000000000002:example.com")

	first := BuildLadder(ClassSpace, testBot, LadderOptions{Elevated: []id.UserID{adminA}})
	if _, ok := first.Users[adminA]; !ok {
		t.Fatal("adminA missing from first build")
	}
	// A later build with a different elevated set carries only that set —
	// nothing from a previous event survives (repair recomputes, never merges).
	second := BuildLadder(ClassSpace, testBot, LadderOptions{Elevated: []id.UserID{adminB}})
	if _, ok := second.Users[adminA]; ok {
		t.Error("adminA survived into the recomputed ladder")
	}
	if got := second.Users[adminB]; got != 75 {
		t.Errorf("users[adminB] = %d, want 75", got)
	}
}

func TestLadderDiff(t *testing.T) {
	base := BuildLadder(ClassConversation, testBot, LadderOptions{})

	if LadderDiff(base, BuildLadder(ClassConversation, testBot, LadderOptions{})) {
		t.Error("identical ladders reported as different")
	}

	drifted := BuildLadder(ClassConversation, testBot, LadderOptions{})
	drifted.Users[id.UserID("@stray:example.com")] = 100
	if !LadderDiff(drifted, base) {
		t.Error("stray users entry not detected as drift")
	}

	lowered := BuildLadder(ClassConversation, testBot, LadderOptions{})
	lowered.Events["m.room.name"] = 50
	if !LadderDiff(lowered, base) {
		t.Error("lowered event level not detected as drift")
	}

	// Semantic comparison: an absent kick pointer means the spec default (50),
	// which differs from the ladder's 100.
	noKick := BuildLadder(ClassConversation, testBot, LadderOptions{})
	noKick.KickPtr = nil
	if !LadderDiff(noKick, base) {
		t.Error("absent kick pointer (spec default 50) not detected as drift")
	}
}

func TestPreserveGuestEntries(t *testing.T) {
	guest := id.UserID("@guest-9:example.com")
	pl := BuildLadder(ClassThread, testBot, LadderOptions{Guests: []id.UserID{guest}})
	preserved := PreserveGuestEntries(pl)
	if len(preserved) != 1 || preserved[0] != guest {
		t.Errorf("preserved = %v, want exactly [%s]", preserved, guest)
	}
	if got := PreserveGuestEntries(nil); got != nil {
		t.Errorf("nil content should preserve nothing, got %v", got)
	}
}
