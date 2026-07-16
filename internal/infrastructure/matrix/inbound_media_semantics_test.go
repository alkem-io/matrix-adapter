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

// Legacy media has no separate filename field, so its body is the filename and
// must not also surface as a redundant text line.
func TestParseMessageEvent_LegacyBody_UsedOnlyAsDisplayName(t *testing.T) {
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
	assert.Empty(t, msg.Content, "legacy media filename must not be duplicated as Content")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "report.pdf", msg.Attachments[0].DisplayName)
}

// A body equal to the resolved filename is not a genuine caption, even when a
// top-level filename field is present, so it is not duplicated as Content.
// MSC2530: a caption exists only when body differs from the filename. A genuine
// caption (body != filename) is surfaced as Content, with the filename as the
// attachment display name.
func TestParseMessageEvent_GenuineCaption_Preserved(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$cap"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype":  "m.file",
				"body":     "please review this",
				"filename": "report.pdf",
				"url":      "mxc://test.local/capmedia",
				"info":     map[string]any{"mimetype": "application/pdf"},
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	assert.Equal(t, "please review this", msg.Content, "a caption differing from the filename is surfaced")
	assert.Equal(t, "report.pdf", msg.Attachments[0].DisplayName)
}

// MSC2530: when body == filename there is NO caption (body is just the filename),
// even with an explicit filename field present, so it is dropped — otherwise a
// spec-compliant captionless upload would render its filename as a duplicate text
// line.
func TestParseMessageEvent_FilenameEqualsBody_NoCaption(t *testing.T) {
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
	assert.Empty(t, msg.Content, "body equal to the filename is not a caption")
	assert.Equal(t, "report.pdf", msg.Attachments[0].DisplayName)
}

// Legacy media (no `filename` field): `body` IS the filename, so a body equal to
// the display name is a redundant filename and is dropped (no duplicate text).
func TestParseMessageEvent_LegacyBodyIsFilename_NotDuplicated(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$legacy"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype": "m.image",
				"body":    "photo.jpg",
				"url":     "mxc://test.local/legacymedia",
				"info":    map[string]any{"mimetype": "image/jpeg"},
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	assert.Empty(t, msg.Content, "legacy body equal to the filename is a redundant name, not a caption")
	assert.Equal(t, "photo.jpg", msg.Attachments[0].DisplayName)
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

// A message event whose body is PRESENT but not a string is malformed; GetMessage
// must error (matching develop) rather than silently return a blank Message. This
// is distinct from an absent body (bodyless/redacted → empty-content Message).
func TestGetMessage_MalformedBody_ReturnsError(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$bad"),
			Sender:    id.UserID("@user:test.local"),
			Type:      event.EventMessage,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{"body": 12345, "msgtype": "m.text"},
			},
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$bad")
	require.Error(t, err, "a non-string body is malformed and must error")
	assert.Nil(t, msg)
}

// A MEDIA event (valid mxc url + msgtype) is a valid attachment regardless of its
// body: the timeline scans (GetRoomMessages/GetLastMessage) return it via the url,
// so GetMessage must too. A corrupt/non-string body must NOT make GetMessage error
// on it — that would be an inconsistent read path. Only a non-string body with NO
// url is malformed.
func TestGetMessage_MediaEventNonStringBody_ReturnsAttachment(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$mbad"),
			Sender:    id.UserID("@user:test.local"),
			Type:      event.EventMessage,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"msgtype": "m.image",
					"body":    12345, // corrupt, non-string body
					"url":     "mxc://test.local/x",
				},
			},
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$mbad")
	require.NoError(t, err, "a media event is valid regardless of its body")
	require.NotNil(t, msg)
	require.Len(t, msg.Attachments, 1, "the media attachment must be surfaced")
	assert.Equal(t, "x", msg.Attachments[0].MediaID)
}

// The media-event body short-circuit must require a VALID mxc url — the same
// parser extractAttachment uses. A non-mxc url (e.g. http://) does not surface an
// attachment, so a non-string body alongside it is genuinely malformed: GetMessage
// must error rather than return a blank Message.
func TestGetMessage_NonMxcUrlNonStringBody_ReturnsError(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$evil"),
			Sender:    id.UserID("@user:test.local"),
			Type:      event.EventMessage,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"msgtype": "m.image",
					"body":    12345,           // corrupt, non-string body
					"url":     "http://evil/x", // NOT an mxc URI
				},
			},
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$evil")
	require.Error(t, err, "a non-mxc url with a non-string body is malformed and must error")
	assert.Nil(t, msg)
}
