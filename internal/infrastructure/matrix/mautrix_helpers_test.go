//nolint:unparam // test helpers use fixed values
package matrix

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

// newTestAdapter creates a minimal MautrixAdapter with only the idMapper set,
// suitable for testing pure helper methods that don't require an appservice.
func newTestAdapter(homeserverDomain string) *MautrixAdapter {
	return &MautrixAdapter{
		idMapper: domain.NewIDMapper(homeserverDomain),
	}
}

// ============================================================================
// safePrefix
// ============================================================================

func TestSafePrefix_LongerThanN(t *testing.T) {
	got := safePrefix("abcdefgh", 3)
	assert.Equal(t, "abc", got)
}

func TestSafePrefix_ShorterThanN(t *testing.T) {
	got := safePrefix("ab", 5)
	assert.Equal(t, "ab", got)
}

func TestSafePrefix_ExactlyN(t *testing.T) {
	got := safePrefix("abcde", 5)
	assert.Equal(t, "abcde", got)
}

func TestSafePrefix_EmptyString(t *testing.T) {
	got := safePrefix("", 5)
	assert.Equal(t, "", got)
}

func TestSafePrefix_ZeroN(t *testing.T) {
	got := safePrefix("hello", 0)
	assert.Equal(t, "", got)
}

func TestSafePrefix_EmptyStringZeroN(t *testing.T) {
	got := safePrefix("", 0)
	assert.Equal(t, "", got)
}

// ============================================================================
// selectPreferredAlias
// ============================================================================

func TestSelectPreferredAlias_EmptyAliases(t *testing.T) {
	adapter := newTestAdapter("test.local")
	got := adapter.selectPreferredAlias([]string{})
	assert.Equal(t, "", got)
}

func TestSelectPreferredAlias_NilAliases(t *testing.T) {
	adapter := newTestAdapter("test.local")
	got := adapter.selectPreferredAlias(nil)
	assert.Equal(t, "", got)
}

func TestSelectPreferredAlias_WithAlkemioAlias(t *testing.T) {
	adapter := newTestAdapter("test.local")
	aliases := []string{
		"#some-random-alias:test.local",
		"#550e8400-e29b-41d4-a716-446655440000:test.local",
		"#another-alias:test.local",
	}
	got := adapter.selectPreferredAlias(aliases)
	assert.Equal(t, "#550e8400-e29b-41d4-a716-446655440000:test.local", got)
}

func TestSelectPreferredAlias_NoAlkemioAlias_FallbackToFirst(t *testing.T) {
	adapter := newTestAdapter("test.local")
	aliases := []string{
		"#random-alias:test.local",
		"#another-alias:test.local",
	}
	got := adapter.selectPreferredAlias(aliases)
	assert.Equal(t, "#random-alias:test.local", got)
}

func TestSelectPreferredAlias_AlkemioAliasFirst(t *testing.T) {
	adapter := newTestAdapter("test.local")
	aliases := []string{
		"#550e8400-e29b-41d4-a716-446655440000:test.local",
		"#random:test.local",
	}
	got := adapter.selectPreferredAlias(aliases)
	assert.Equal(t, "#550e8400-e29b-41d4-a716-446655440000:test.local", got)
}

func TestSelectPreferredAlias_WrongDomain_FallbackToFirst(t *testing.T) {
	adapter := newTestAdapter("test.local")
	// UUID alias belongs to a different domain, so IDMapper won't match it.
	aliases := []string{
		"#550e8400-e29b-41d4-a716-446655440000:other.server",
		"#fallback:test.local",
	}
	got := adapter.selectPreferredAlias(aliases)
	assert.Equal(t, "#550e8400-e29b-41d4-a716-446655440000:other.server", got)
}

func TestSelectPreferredAlias_MultipleAlkemioAliases_PicksFirst(t *testing.T) {
	adapter := newTestAdapter("test.local")
	aliases := []string{
		"#random:test.local",
		"#a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11:test.local",
		"#550e8400-e29b-41d4-a716-446655440000:test.local",
	}
	got := adapter.selectPreferredAlias(aliases)
	// Should return the first Alkemio-patterned alias encountered.
	assert.Equal(t, "#a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11:test.local", got)
}

// ============================================================================
// parseMessageEvent
// ============================================================================

func TestParseMessageEvent_ValidMessage(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC).UnixMilli()

	evt := &event.Event{
		ID:        id.EventID("$event1"),
		Sender:    id.UserID("@alice:test.local"),
		Type:      event.EventMessage,
		Timestamp: ts,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "Hello, world!",
			},
		},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	require.NotNil(t, msg)
	assert.Equal(t, "$event1", msg.ID)
	assert.Equal(t, roomID.String(), msg.RoomID)
	assert.Equal(t, "Hello, world!", msg.Content)
	assert.Equal(t, "@alice:test.local", msg.SenderMatrixID)
	assert.Equal(t, time.UnixMilli(ts), msg.Timestamp)
	assert.Empty(t, msg.ThreadID)
}

func TestParseMessageEvent_WithThreadRelation(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")

	evt := &event.Event{
		ID:        id.EventID("$reply1"),
		Sender:    id.UserID("@bob:test.local"),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "Thread reply",
				RelatesTo: &event.RelatesTo{
					Type:    event.RelThread,
					EventID: id.EventID("$thread-root"),
					InReplyTo: &event.InReplyTo{
						EventID: id.EventID("$thread-root"),
					},
					IsFallingBack: true,
				},
			},
		},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	require.NotNil(t, msg)
	assert.Equal(t, "Thread reply", msg.Content)
	assert.Equal(t, "$thread-root", msg.ThreadID)
}

func TestParseMessageEvent_WithInReplyToFallback(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")

	evt := &event.Event{
		ID:        id.EventID("$reply2"),
		Sender:    id.UserID("@carol:test.local"),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "Reply fallback",
				RelatesTo: &event.RelatesTo{
					InReplyTo: &event.InReplyTo{
						EventID: id.EventID("$parent-msg"),
					},
				},
			},
		},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	require.NotNil(t, msg)
	assert.Equal(t, "$parent-msg", msg.ThreadID)
}

func TestParseMessageEvent_EmptyBody_ReturnsNil(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")

	evt := &event.Event{
		ID:        id.EventID("$evt1"),
		Sender:    id.UserID("@alice:test.local"),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "",
			},
		},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	assert.Nil(t, msg)
}

func TestParseMessageEvent_NilContent_ReturnsNil(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")

	evt := &event.Event{
		ID:        id.EventID("$evt2"),
		Sender:    id.UserID("@alice:test.local"),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content:   event.Content{},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	assert.Nil(t, msg)
}

func TestParseMessageEvent_RawJSONFallback(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")
	ts := int64(1700000000000)

	evt := &event.Event{
		ID:        id.EventID("$raw1"),
		Sender:    id.UserID("@dave:test.local"),
		Type:      event.EventMessage,
		Timestamp: ts,
		Content: event.Content{
			Raw: map[string]interface{}{
				"body":    "Raw body fallback",
				"msgtype": "m.text",
			},
		},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	require.NotNil(t, msg)
	assert.Equal(t, "Raw body fallback", msg.Content)
	assert.Equal(t, "$raw1", msg.ID)
}

// ============================================================================
// parseReactionEvent
// ============================================================================

func TestParseReactionEvent_ValidReaction(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC).UnixMilli()

	evt := &event.Event{
		ID:        id.EventID("$reaction1"),
		Sender:    id.UserID("@alice:test.local"),
		Type:      event.EventReaction,
		Timestamp: ts,
		Content: event.Content{
			Parsed: &event.ReactionEventContent{
				RelatesTo: event.RelatesTo{
					EventID: id.EventID("$target-msg"),
					Key:     "\U0001F44D",
					Type:    event.RelAnnotation,
				},
			},
		},
	}

	reaction := adapter.parseReactionEvent(evt, roomID)
	require.NotNil(t, reaction)
	assert.Equal(t, id.EventID("$reaction1"), reaction.ID)
	assert.Equal(t, roomID, reaction.RoomID)
	assert.Equal(t, id.EventID("$target-msg"), reaction.MessageID)
	assert.Equal(t, "\U0001F44D", reaction.Emoji)
	assert.Equal(t, "@alice:test.local", reaction.SenderMatrixID)
	assert.Equal(t, time.UnixMilli(ts), reaction.Timestamp)
}

func TestParseReactionEvent_MissingEventID_ReturnsNil(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")

	evt := &event.Event{
		ID:        id.EventID("$reaction2"),
		Sender:    id.UserID("@bob:test.local"),
		Type:      event.EventReaction,
		Timestamp: 1700000000000,
		Content: event.Content{
			Parsed: &event.ReactionEventContent{
				RelatesTo: event.RelatesTo{
					EventID: "", // Empty event ID
					Key:     "\U0001F44D",
					Type:    event.RelAnnotation,
				},
			},
		},
	}

	reaction := adapter.parseReactionEvent(evt, roomID)
	assert.Nil(t, reaction)
}

func TestParseReactionEvent_NoContent_ReturnsNil(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room123:test.local")

	evt := &event.Event{
		ID:        id.EventID("$reaction3"),
		Sender:    id.UserID("@carol:test.local"),
		Type:      event.EventReaction,
		Timestamp: 1700000000000,
		Content:   event.Content{},
	}

	reaction := adapter.parseReactionEvent(evt, roomID)
	assert.Nil(t, reaction)
}

// ============================================================================
// extractMessageBody
// ============================================================================

func TestExtractMessageBody_ParsedContent(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "Parsed body content",
			},
		},
	}

	body := adapter.extractMessageBody(evt)
	assert.Equal(t, "Parsed body content", body)
}

func TestExtractMessageBody_RawJSONFallback(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Raw: map[string]interface{}{
				"body":    "Raw fallback body",
				"msgtype": "m.text",
			},
		},
	}

	body := adapter.extractMessageBody(evt)
	assert.Equal(t, "Raw fallback body", body)
}

func TestExtractMessageBody_NoBodyAtAll(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Type:    event.EventMessage,
		Content: event.Content{},
	}

	body := adapter.extractMessageBody(evt)
	assert.Equal(t, "", body)
}

func TestExtractMessageBody_RawWithNonStringBody(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Raw: map[string]interface{}{
				"body": 12345, // Not a string
			},
		},
	}

	body := adapter.extractMessageBody(evt)
	assert.Equal(t, "", body)
}

// ============================================================================
// extractThreadIDFromRaw
// ============================================================================

func TestExtractThreadIDFromRaw_ValidThreadID(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"body": "some text",
				"m.relates_to": map[string]interface{}{
					"m.in_reply_to": map[string]interface{}{
						"event_id": "$thread-root-event",
					},
				},
			},
		},
	}

	threadID := adapter.extractThreadIDFromRaw(evt)
	assert.Equal(t, "$thread-root-event", threadID)
}

func TestExtractThreadIDFromRaw_NoRelatesTo(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"body": "no relates_to",
			},
		},
	}

	threadID := adapter.extractThreadIDFromRaw(evt)
	assert.Equal(t, "", threadID)
}

func TestExtractThreadIDFromRaw_NoInReplyTo(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"m.relates_to": map[string]interface{}{
					"rel_type": "m.annotation",
				},
			},
		},
	}

	threadID := adapter.extractThreadIDFromRaw(evt)
	assert.Equal(t, "", threadID)
}

func TestExtractThreadIDFromRaw_EmptyContent(t *testing.T) {
	adapter := newTestAdapter("test.local")

	evt := &event.Event{
		Content: event.Content{},
	}

	threadID := adapter.extractThreadIDFromRaw(evt)
	assert.Equal(t, "", threadID)
}

// ============================================================================
// parseEventContent (generic helper)
// ============================================================================

func TestParseEventContent_ParsedContentAvailable(t *testing.T) {
	parsed := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    "test body",
	}
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: parsed,
		},
	}

	got, ok := parseEventContent[event.MessageEventContent](evt)
	require.True(t, ok)
	assert.Equal(t, "test body", got.Body)
}

func TestParseEventContent_ParseFromVeryRaw(t *testing.T) {
	raw, _ := json.Marshal(event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    "from VeryRaw",
	})

	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			VeryRaw: raw,
		},
	}

	got, ok := parseEventContent[event.MessageEventContent](evt)
	require.True(t, ok)
	assert.Equal(t, "from VeryRaw", got.Body)
}

func TestParseEventContent_FailsForWrongType(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.ReactionEventContent{}, // Wrong type
		},
	}

	_, ok := parseEventContent[event.MessageEventContent](evt)
	assert.False(t, ok)
}

func TestParseEventContent_EmptyContent(t *testing.T) {
	evt := &event.Event{
		Type:    event.EventMessage,
		Content: event.Content{},
	}

	_, ok := parseEventContent[event.MessageEventContent](evt)
	assert.False(t, ok)
}

func TestParseEventContent_ReactionContent(t *testing.T) {
	parsed := &event.ReactionEventContent{
		RelatesTo: event.RelatesTo{
			EventID: id.EventID("$target"),
			Key:     "\u2764\uFE0F",
			Type:    event.RelAnnotation,
		},
	}
	evt := &event.Event{
		Type: event.EventReaction,
		Content: event.Content{
			Parsed: parsed,
		},
	}

	got, ok := parseEventContent[event.ReactionEventContent](evt)
	require.True(t, ok)
	assert.Equal(t, id.EventID("$target"), got.RelatesTo.EventID)
	assert.Equal(t, "\u2764\uFE0F", got.RelatesTo.Key)
}

// ============================================================================
// extractSynapseNotificationCount
// ============================================================================

func TestExtractSynapseNotificationCount_Nil(t *testing.T) {
	count := extractSynapseNotificationCount(nil)
	assert.Equal(t, 0, count)
}

func TestExtractSynapseNotificationCount_NilNotifications(t *testing.T) {
	room := &mautrix.SyncJoinedRoom{}
	count := extractSynapseNotificationCount(room)
	assert.Equal(t, 0, count)
}

func TestExtractSynapseNotificationCount_NotificationCount(t *testing.T) {
	room := &mautrix.SyncJoinedRoom{
		UnreadNotifications: &mautrix.UnreadNotificationCounts{
			NotificationCount: 5,
		},
	}
	count := extractSynapseNotificationCount(room)
	assert.Equal(t, 5, count)
}

func TestExtractSynapseNotificationCount_MSC2654Preferred(t *testing.T) {
	msc := 3
	room := &mautrix.SyncJoinedRoom{
		UnreadNotifications: &mautrix.UnreadNotificationCounts{
			NotificationCount: 5,
		},
		MSC2654UnreadCount: &msc,
	}
	// MSC2654 should override the standard count
	count := extractSynapseNotificationCount(room)
	assert.Equal(t, 3, count)
}

func TestExtractSynapseNotificationCount_MSC2654Zero(t *testing.T) {
	msc := 0
	room := &mautrix.SyncJoinedRoom{
		UnreadNotifications: &mautrix.UnreadNotificationCounts{
			NotificationCount: 7,
		},
		MSC2654UnreadCount: &msc,
	}
	// Even when MSC2654 is 0, it should override (it's a valid value).
	count := extractSynapseNotificationCount(room)
	assert.Equal(t, 0, count)
}

func TestExtractSynapseNotificationCount_OnlyMSC2654(t *testing.T) {
	msc := 10
	room := &mautrix.SyncJoinedRoom{
		MSC2654UnreadCount: &msc,
	}
	count := extractSynapseNotificationCount(room)
	assert.Equal(t, 10, count)
}

// ============================================================================
// lastMessageStats.record
// ============================================================================

func TestLastMessageStats_Record_Buckets(t *testing.T) {
	logger := &testutil.MockLogger{}

	tests := []struct {
		name          string
		eventsNeeded  int
		found         bool
		expectBucket5 int64
	}{
		{"bucket 0-5", 3, true, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &lastMessageStats{logInterval: 1000}
			stats.record(tt.eventsNeeded, tt.found, logger)
			assert.Equal(t, tt.expectBucket5, stats.bucket5)
		})
	}
}

func TestLastMessageStats_Record_AllBuckets(t *testing.T) {
	logger := &testutil.MockLogger{}
	stats := &lastMessageStats{logInterval: 1000}

	// Record values into each bucket
	stats.record(1, true, logger)   // bucket5
	stats.record(7, true, logger)   // bucket10
	stats.record(15, true, logger)  // bucket20
	stats.record(30, true, logger)  // bucket50
	stats.record(100, true, logger) // bucket200
	stats.record(0, false, logger)  // notFound

	assert.Equal(t, int64(1), stats.bucket5)
	assert.Equal(t, int64(1), stats.bucket10)
	assert.Equal(t, int64(1), stats.bucket20)
	assert.Equal(t, int64(1), stats.bucket50)
	assert.Equal(t, int64(1), stats.bucket200)
	assert.Equal(t, int64(1), stats.notFound)
	assert.Equal(t, int64(6), stats.totalCalls)
}

func TestLastMessageStats_Record_BoundaryValues(t *testing.T) {
	logger := &testutil.MockLogger{}

	tests := []struct {
		name         string
		eventsNeeded int
		found        bool
		checkField   string
		expected     int64
	}{
		{"exactly 5", 5, true, "bucket5", 1},
		{"exactly 10", 10, true, "bucket10", 1},
		{"exactly 20", 20, true, "bucket20", 1},
		{"exactly 50", 50, true, "bucket50", 1},
		{"over 50", 51, true, "bucket200", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &lastMessageStats{logInterval: 1000}
			stats.record(tt.eventsNeeded, tt.found, logger)

			switch tt.checkField {
			case "bucket5":
				assert.Equal(t, tt.expected, stats.bucket5)
			case "bucket10":
				assert.Equal(t, tt.expected, stats.bucket10)
			case "bucket20":
				assert.Equal(t, tt.expected, stats.bucket20)
			case "bucket50":
				assert.Equal(t, tt.expected, stats.bucket50)
			case "bucket200":
				assert.Equal(t, tt.expected, stats.bucket200)
			}
		})
	}
}

// ============================================================================
// Integration-style: parseMessageEvent + parseReactionEvent together
// ============================================================================

func TestParseMessageEvent_ThreadRelation_MThread(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room:test.local")

	evt := &event.Event{
		ID:        id.EventID("$child"),
		Sender:    id.UserID("@user:test.local"),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "Thread child message",
				RelatesTo: &event.RelatesTo{
					Type:    "m.thread",
					EventID: id.EventID("$root"),
				},
			},
		},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	require.NotNil(t, msg)
	assert.Equal(t, "$root", msg.ThreadID, "Should extract thread ID from m.thread relation type")
}

func TestParseMessageEvent_NoRelatesTo(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room:test.local")

	evt := &event.Event{
		ID:        id.EventID("$standalone"),
		Sender:    id.UserID("@user:test.local"),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "Standalone message",
			},
		},
	}

	msg := adapter.parseMessageEvent(evt, roomID)
	require.NotNil(t, msg)
	assert.Empty(t, msg.ThreadID, "Standalone message should have no thread ID")
}

func TestParseReactionEvent_WithRawContent(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room:test.local")

	raw, _ := json.Marshal(event.ReactionEventContent{
		RelatesTo: event.RelatesTo{
			EventID: id.EventID("$msg"),
			Key:     "\U0001F525",
			Type:    event.RelAnnotation,
		},
	})

	evt := &event.Event{
		ID:        id.EventID("$react-raw"),
		Sender:    id.UserID("@user:test.local"),
		Type:      event.EventReaction,
		Timestamp: 1700000000000,
		Content: event.Content{
			VeryRaw: raw,
		},
	}

	reaction := adapter.parseReactionEvent(evt, roomID)
	require.NotNil(t, reaction)
	assert.Equal(t, id.EventID("$msg"), reaction.MessageID)
	assert.Equal(t, "\U0001F525", reaction.Emoji)
}

func TestParseReactionEvent_MultipleEmojis(t *testing.T) {
	adapter := newTestAdapter("test.local")
	roomID := id.RoomID("!room:test.local")

	emojis := []string{"\U0001F44D", "\u2764\uFE0F", "\U0001F389", "\U0001F60A"}

	for _, emoji := range emojis {
		evt := &event.Event{
			ID:        id.EventID("$react-" + emoji),
			Sender:    id.UserID("@user:test.local"),
			Type:      event.EventReaction,
			Timestamp: 1700000000000,
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: id.EventID("$target"),
						Key:     emoji,
						Type:    event.RelAnnotation,
					},
				},
			},
		}

		reaction := adapter.parseReactionEvent(evt, roomID)
		require.NotNil(t, reaction, "Reaction with emoji %s should parse", emoji)
		assert.Equal(t, emoji, reaction.Emoji)
	}
}
