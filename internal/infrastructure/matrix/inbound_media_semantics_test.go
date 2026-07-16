package matrix

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// ============================================================================
// F2 — MSC2530 captions preserved
// ============================================================================

// A modern media event sets body=caption and a separate top-level `filename`.
// The caption must be KEPT as Content, and the filename must be the attachment's
// DisplayName.
func TestParseMessageEvent_MSC2530Caption_Preserved(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$cap"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype":  "m.image",
				"body":     "Look at this sunset!", // caption
				"filename": "sunset.jpg",           // real filename
				"url":      "mxc://test.local/capmedia",
				"info":     map[string]any{"mimetype": "image/jpeg"},
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	assert.Equal(t, "Look at this sunset!", msg.Content, "caption must be preserved as Content")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "sunset.jpg", msg.Attachments[0].DisplayName, "DisplayName must be the filename, not the caption")
	assert.Equal(t, "capmedia", msg.Attachments[0].MediaID)
}

// Legacy media has no separate filename field, so its body is the combined
// name/caption and must retain develop's behavior of being forwarded as Content.
func TestParseMessageEvent_LegacyBody_PreservedAsContent(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$legacy"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype": "m.file",
				"body":    "report.pdf", // filename as body, no `filename` field
				"url":     "mxc://test.local/legacymedia",
				"info":    map[string]any{"mimetype": "application/pdf"},
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	assert.Equal(t, "report.pdf", msg.Content, "legacy media body must be forwarded as Content")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "report.pdf", msg.Attachments[0].DisplayName)
}

// A3 — when a top-level `filename` field is present, body is a CAPTION and is
// preserved as Content even if it happens to equal the filename. The presence of
// the field (not DisplayName==body) drives the decision, so a caption that
// coincides with the filename is no longer dropped.
func TestParseMessageEvent_FilenameEqualsBody_CaptionPreserved(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$eq"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype":  "m.file",
				"body":     "report.pdf",
				"filename": "report.pdf",
				"url":      "mxc://test.local/eqmedia",
				"info":     map[string]any{"mimetype": "application/pdf"},
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	assert.Equal(t, "report.pdf", msg.Content,
		"a present filename field means body is a caption — preserved even when it equals the filename")
	assert.Equal(t, "report.pdf", msg.Attachments[0].DisplayName)
}

// The live-sync path (handleMessageEvent) applies the same caption semantics as
// the read path — a caption is delivered as Content, the filename as DisplayName.
func TestHandleMessageEvent_MSC2530Caption_Delivered(t *testing.T) {
	roomUUID := uuid.New()
	alias := "#" + roomUUID.String() + ":test.local"
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{Aliases: []id.RoomAlias{id.RoomAlias(alias)}},
	}
	a := newFullTestAdapter(newMockAS(botIntent, nil), &mockAdminAPI{})

	got := make(chan domain.Message, 1)
	a.eventHandlers = EventHandlers{
		OnMessage: func(m domain.Message) error { got <- m; return nil },
	}

	evt := &event.Event{
		ID:        id.EventID("$e2"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		RoomID:    "!room:test.local",
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype":  "m.image",
				"body":     "My holiday photo", // caption
				"filename": "beach.png",
				"url":      "mxc://test.local/mid2",
				"info":     map[string]any{"mimetype": "image/png", "size": float64(3)},
			},
		},
	}

	a.handleMessageEvent(evt)

	select {
	case m := <-got:
		assert.Equal(t, "My holiday photo", m.Content, "caption preserved on live-sync path")
		require.Len(t, m.Attachments, 1)
		assert.Equal(t, "beach.png", m.Attachments[0].DisplayName)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a message delivered to OnMessage")
	}
}

// ============================================================================
// F3 — present-but-empty body is forwarded (not dropped)
// ============================================================================

// A text message with body present but empty ("") and no attachment must be
// forwarded with empty Content (develop's `, ok` semantics), not silently
// dropped.
func TestHandleMessageEvent_PresentButEmptyBody_Forwarded(t *testing.T) {
	roomUUID := uuid.New()
	alias := "#" + roomUUID.String() + ":test.local"
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{Aliases: []id.RoomAlias{id.RoomAlias(alias)}},
	}
	a := newFullTestAdapter(newMockAS(botIntent, nil), &mockAdminAPI{})

	got := make(chan domain.Message, 1)
	a.eventHandlers = EventHandlers{
		OnMessage: func(m domain.Message) error { got <- m; return nil },
	}

	evt := &event.Event{
		ID:        id.EventID("$empty"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		RoomID:    "!room:test.local",
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"body":    "", // present but empty
				"msgtype": "m.text",
			},
		},
	}

	a.handleMessageEvent(evt)

	select {
	case m := <-got:
		assert.Empty(t, m.Content)
		assert.Equal(t, roomUUID.String(), m.RoomID)
		assert.Empty(t, m.Attachments)
	case <-time.After(2 * time.Second):
		t.Fatal("present-but-empty body was dropped; expected it to be forwarded")
	}
}

// A genuinely absent body (no "body" key) with no attachment is still dropped.
func TestExtractInboundMessage_AbsentBodyNoAttachment_Dropped(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Raw: map[string]any{"msgtype": "m.text"}, // no body key
		},
	}
	_, _, ok := extractInboundMessage(evt, false)
	assert.False(t, ok, "absent body with no attachment must be dropped")
}

// ============================================================================
// F4 — GetMessage on a present-but-empty-body message yields a Message
// ============================================================================

// A1 — GetLastMessage (a scan) skips a trailing (newest) blank preview message
// and returns the previous real one. On develop, parseMessageEvent returned nil
// for an empty body so scans never saw it; after the empty-body fix (F4) the
// scanning read paths filter blanks explicitly.
func TestGetLastMessage_SkipsTrailingBlank(t *testing.T) {
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			// dir="b" ⇒ newest first: a blank preview message, then a real one.
			Chunk: []*event.Event{
				{
					ID: "$blank", Sender: id.UserID("@user:test.local"), Type: event.EventMessage,
					Timestamp: 1700000002000,
					Content:   event.Content{Raw: map[string]any{"body": "", "msgtype": "m.text"}},
				},
				{
					ID: "$real", Sender: id.UserID("@user:test.local"), Type: event.EventMessage,
					Timestamp: 1700000001000,
					Content:   event.Content{Raw: map[string]any{"body": "hello", "msgtype": "m.text"}},
				},
			},
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	msg, err := a.GetLastMessage(context.Background(), "!room:test.local")
	require.NoError(t, err)
	require.NotNil(t, msg, "a real message exists behind the blank")
	assert.Equal(t, "$real", msg.ID, "the trailing blank must be skipped as the room's last message")
	assert.Equal(t, "hello", msg.Content)
}

// A1 — GetRoomMessages (a history scan) filters blank preview messages out of
// the returned list, matching develop.
func TestGetRoomMessages_FiltersBlank(t *testing.T) {
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					ID: "$real", Sender: id.UserID("@user:test.local"), Type: event.EventMessage,
					Timestamp: 1700000001000,
					Content:   event.Content{Raw: map[string]any{"body": "hi", "msgtype": "m.text"}},
				},
				{
					ID: "$blank", Sender: id.UserID("@user:test.local"), Type: event.EventMessage,
					Timestamp: 1700000002000,
					Content:   event.Content{Raw: map[string]any{"body": "", "msgtype": "m.text"}},
				},
			},
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	msgs, err := a.GetRoomMessages(context.Background(), "!room:test.local")
	require.NoError(t, err)
	require.Len(t, msgs, 1, "the blank message must be filtered from room history")
	assert.Equal(t, "$real", msgs[0].ID)
}

func TestGetMessage_EmptyBody_ReturnsMessage(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$e"),
			Sender:    id.UserID("@user:test.local"),
			Type:      event.EventMessage,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{"body": "", "msgtype": "m.text"},
			},
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$e")
	require.NoError(t, err, "empty-body message must not error")
	require.NotNil(t, msg)
	assert.Empty(t, msg.Content)
	assert.Equal(t, "$e", msg.ID)
}

func TestGetMessage_AbsentBody_ReturnsMessage(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$redacted"),
			Sender:    id.UserID("@user:test.local"),
			Type:      event.EventMessage,
			Timestamp: 1700000000000,
			Content:   event.Content{Raw: map[string]any{}},
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$redacted")
	require.NoError(t, err, "bodyless message events must remain readable by ID")
	require.NotNil(t, msg)
	assert.Empty(t, msg.Content)
	assert.Equal(t, "$redacted", msg.ID)
}
