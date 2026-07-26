package matrix

import (
	"math"
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

// rawInt64 coerces JSON numbers for size/w/h, which are non-negative; a NaN/Inf,
// negative, or int64-overflowing float (e.g. a hostile info.size of 1e30) must be
// rejected rather than surfacing an implementation-defined garbage/negative int64.
func TestRawInt64_RangeAndSignGuards(t *testing.T) {
	// Overflow: 1e30 far exceeds math.MaxInt64.
	if v, ok := rawInt64(1e30); ok {
		t.Errorf("rawInt64(1e30) must be rejected, got (%d, true)", v)
	}
	// Boundary: float64(math.MaxInt64) rounds UP to exactly 2^63, which overflows
	// int64 (wraps to MinInt64); it must be rejected, not accepted.
	if v, ok := rawInt64(float64(math.MaxInt64)); ok {
		t.Errorf("rawInt64(2^63) must be rejected, got (%d, true)", v)
	}
	// Negative floats are not valid sizes/dimensions.
	if v, ok := rawInt64(float64(-5)); ok {
		t.Errorf("rawInt64(-5) must be rejected, got (%d, true)", v)
	}
	// NaN / +Inf / -Inf are rejected.
	if _, ok := rawInt64(math.NaN()); ok {
		t.Error("rawInt64(NaN) must be rejected")
	}
	if _, ok := rawInt64(math.Inf(1)); ok {
		t.Error("rawInt64(+Inf) must be rejected")
	}
	if _, ok := rawInt64(math.Inf(-1)); ok {
		t.Error("rawInt64(-Inf) must be rejected")
	}
	// A normal in-range value still coerces.
	got, ok := rawInt64(float64(9))
	if !ok || got != 9 {
		t.Errorf("rawInt64(9.0) = (%d, %v), want (9, true)", got, ok)
	}
	// Non-float numeric kinds still work.
	if got, ok := rawInt64(int(42)); !ok || got != 42 {
		t.Errorf("rawInt64(int 42) = (%d, %v), want (42, true)", got, ok)
	}
}

// isOwnAppserviceUser trusts only UUID-localpart users on our own homeserver.
func TestIsOwnAppserviceUser(t *testing.T) {
	a := newTestAdapter("test.local")

	ghost := expectedUserID(testActorID) // @<uuid>:test.local
	assert.True(t, a.isOwnAppserviceUser(ghost), "own-domain UUID ghost is trusted")

	assert.False(t, a.isOwnAppserviceUser("@element:test.local"),
		"non-UUID localpart on our domain is not an appservice ghost")
	assert.False(t, a.isOwnAppserviceUser(id.UserID("@"+testActorID.String()+":evil.server")),
		"UUID localpart on a foreign homeserver is not ours")
}

// M3 — a forged io.alkemio.document_id from a sender outside our appservice
// namespace is NOT surfaced as an authoritative DocumentID; only the MediaID is
// surfaced (Element-origin / re-home path).
func TestParseMessageEvent_ForgedDocumentID_FromNonGhost_Dropped(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$forged"),
		Sender:    id.UserID("@attacker:test.local"), // not a UUID ghost
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype":                "m.image",
				"body":                   "evil.png",
				"url":                    "mxc://test.local/realmedia",
				"info":                   map[string]any{"mimetype": "image/png"},
				"io.alkemio.document_id": "victim-doc-id",
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	require.Len(t, msg.Attachments, 1)
	assert.Empty(t, msg.Attachments[0].DocumentID, "forged document_id must be dropped")
	assert.Equal(t, "realmedia", msg.Attachments[0].MediaID, "MediaID still surfaced (re-home path)")
}

// The same event from one of our own ghosts is a genuine echo, so the
// DocumentID is trusted and surfaced alongside the MediaID (coalesce path).
func TestParseMessageEvent_DocumentID_FromGhost_Surfaced(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$echo"),
		Sender:    expectedUserID(testActorID), // own-appservice ghost
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype":                "m.image",
				"body":                   "photo.png",
				"url":                    "mxc://test.local/mediaX",
				"info":                   map[string]any{"mimetype": "image/png"},
				"io.alkemio.document_id": "doc-real",
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "doc-real", msg.Attachments[0].DocumentID)
	assert.Equal(t, "mediaX", msg.Attachments[0].MediaID)
}

// A legacy media event without MSC2530 filename metadata uses body as the
// attachment display name without duplicating it as message Content.
func TestParseMessageEvent_MediaEvent_NoFilenameDoesNotDuplicateBody(t *testing.T) {
	a := newTestAdapter("test.local")

	evt := &event.Event{
		ID:        id.EventID("$m"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype": "m.file",
				"body":    "report.pdf",
				"url":     "mxc://test.local/f",
				"info":    map[string]any{"mimetype": "application/pdf"},
			},
		},
	}

	msg := a.parseMessageEvent(evt, "!room:test.local")
	require.NotNil(t, msg)
	assert.Empty(t, msg.Content)
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "report.pdf", msg.Attachments[0].DisplayName)
}

// Inbound m.video / m.audio events extract as attachments (parity with image/file).
func TestExtractAttachment_VideoAudio(t *testing.T) {
	video := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.video",
		"body":    "clip.mp4",
		"url":     "mxc://test.local/vid1",
		"info":    map[string]any{"mimetype": "video/mp4", "size": float64(100)},
	}), true)
	require.NotNil(t, video)
	assert.Equal(t, "vid1", video.MediaID)
	assert.Equal(t, "video/mp4", video.MimeType)

	audio := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.audio",
		"body":    "voice.ogg",
		"url":     "mxc://test.local/aud1",
		"info":    map[string]any{"mimetype": "audio/ogg"},
	}), true)
	require.NotNil(t, audio)
	assert.Equal(t, "aud1", audio.MediaID)
	assert.Equal(t, "audio/ogg", audio.MimeType)
}

// Redacting a threaded sticker preserves its thread linkage, exactly like a
// redacted m.room.message: processRedactedEvent routes EventSticker through the
// message branch (which extracts the thread id), not the default branch (which
// drops it).
func TestProcessRedactedEvent_Sticker_PreservesThreadID(t *testing.T) {
	a := newTestAdapter("test.local")

	got := make(chan domain.MessageRedactedEvent, 1)
	a.eventHandlers = EventHandlers{
		OnMessageRedacted: func(e domain.MessageRedactedEvent) error { got <- e; return nil },
	}

	redactionEvt := &event.Event{ID: id.EventID("$redaction"), Timestamp: 1700000000000}
	originalSticker := &event.Event{
		ID:   id.EventID("$sticker"),
		Type: event.EventSticker,
		Content: event.Content{
			Raw: map[string]any{
				"body": "party parrot",
				"url":  "mxc://test.local/stickerid",
				"m.relates_to": map[string]any{
					"rel_type": "m.thread",
					"event_id": "$threadroot",
				},
			},
		},
	}

	a.processRedactedEvent(redactionEvt, uuid.New(), "$sticker", uuid.New(), "spam", originalSticker)

	select {
	case e := <-got:
		require.NotNil(t, e.ThreadID, "redacted sticker must carry its thread id")
		assert.Equal(t, "$threadroot", e.ThreadID.String())
	case <-time.After(2 * time.Second):
		t.Fatal("expected a message-redaction event for the redacted sticker")
	}
}

// An m.sticker is image-like media despite carrying NO msgtype field: the
// EventSticker type bypasses the mediaMsgTypes gate and the url+info surface an
// attachment. body (alt-text) becomes the DisplayName; MediaID is the mxc id.
func TestExtractAttachment_Sticker(t *testing.T) {
	evt := mediaEvent(map[string]any{
		"body": "party parrot", // alt-text, no filename field
		"url":  "mxc://test.local/stickerid",
		"info": map[string]any{"mimetype": "image/png", "w": float64(128), "h": float64(128)},
	})
	evt.Type = event.EventSticker

	att := extractAttachment(evt, true)
	require.NotNil(t, att)
	assert.Equal(t, "stickerid", att.MediaID, "sticker mxc id is the re-home key")
	assert.Equal(t, "party parrot", att.DisplayName)
	assert.Equal(t, "image/png", att.MimeType)
	require.NotNil(t, att.Width)
	assert.Equal(t, 128, *att.Width)
}

// A sticker with no info.mimetype leaves MimeType EMPTY — the adapter must not
// guess (a sticker may be webp/gif/Lottie); the real content-type is resolved
// downstream at re-home from the stored blob.
func TestExtractAttachment_Sticker_AbsentMimeStaysEmpty(t *testing.T) {
	evt := mediaEvent(map[string]any{
		"body": "wave",
		"url":  "mxc://test.local/nomime",
	})
	evt.Type = event.EventSticker

	att := extractAttachment(evt, true)
	require.NotNil(t, att)
	assert.Equal(t, "nomime", att.MediaID)
	assert.Empty(t, att.MimeType, "sticker with absent mimetype must not be guessed")
}

// A sticker's body is alt-text, never message Content: a url-less sticker (e.g.
// E2EE content.file, or a non-standard url) has no extractable media and no
// surface-able text, so it is DROPPED (ok=false) — not published as a phantom
// empty message. This matters for the live-sync path, which has no
// isBlankMessage guard and drops only on ok=false.
func TestExtractInboundMessage_UrllessSticker_Dropped(t *testing.T) {
	evt := &event.Event{
		Type: event.EventSticker,
		Content: event.Content{
			Raw: map[string]any{
				"body": "party parrot", // alt-text only; no url the adapter can parse
			},
		},
	}

	content, attachment, ok := extractInboundMessage(evt, false)
	assert.False(t, ok, "a url-less sticker has nothing to surface → dropped on every path")
	assert.Empty(t, content, "sticker alt-text must never surface as Content")
	assert.Nil(t, attachment, "no parseable url → no attachment")
}

// A normal sticker (url present) still surfaces its attachment with empty Content.
func TestExtractInboundMessage_Sticker_AttachmentEmptyContent(t *testing.T) {
	evt := &event.Event{
		Type: event.EventSticker,
		Content: event.Content{
			Raw: map[string]any{
				"body": "party parrot",
				"url":  "mxc://test.local/stickerid",
				"info": map[string]any{"mimetype": "image/png"},
			},
		},
	}

	content, attachment, ok := extractInboundMessage(evt, false)
	assert.True(t, ok)
	assert.Empty(t, content)
	require.NotNil(t, attachment)
	assert.Equal(t, "stickerid", attachment.MediaID)
	assert.Equal(t, "party parrot", attachment.DisplayName)
}

// A media msgtype carrying neither a parseable mxc URL nor a document id yields
// no attachment (no dead both-empty record).
func TestExtractAttachment_NoRefs_ReturnsNil(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.image",
		"body":    "broken.png",
		// no url, no io.alkemio.document_id
	}), true)
	assert.Nil(t, att)
}

// handleMessageEvent wiring: an empty-body media event resolves the room and
// delivers exactly one attachment through OnMessage, with empty Content.
func TestHandleMessageEvent_MediaEvent_Delivered(t *testing.T) {
	roomUUID := uuid.New()
	alias := "#" + roomUUID.String() + ":test.local"
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{Aliases: []id.RoomAlias{id.RoomAlias(alias)}},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	got := make(chan domain.Message, 1)
	a.eventHandlers = EventHandlers{
		OnMessage: func(m domain.Message) error { got <- m; return nil },
	}

	evt := &event.Event{
		ID:        id.EventID("$e1"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventMessage,
		RoomID:    "!room:test.local",
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"msgtype": "m.image",
				"body":    "pic.png", // filename only, no real caption
				"url":     "mxc://test.local/mid",
				"info":    map[string]any{"mimetype": "image/png", "size": float64(3)},
			},
		},
	}

	a.handleMessageEvent(evt)

	select {
	case m := <-got:
		assert.Empty(t, m.Content, "legacy media filename is not duplicated as text")
		assert.Equal(t, roomUUID.String(), m.RoomID)
		require.Len(t, m.Attachments, 1)
		assert.Equal(t, "mid", m.Attachments[0].MediaID)
		assert.Equal(t, "pic.png", m.Attachments[0].DisplayName)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a message to be delivered to OnMessage")
	}
}

// An inbound m.sticker from a normal Element user routes through processEvent →
// handleMessageEvent and surfaces exactly ONE attachment (MediaID = the sticker
// mxc id, DisplayName = alt-text) with empty Content — the same shape as an
// inbound image, so the server's inbound-media re-home pipeline re-homes it by
// MediaID.
func TestProcessEvent_Sticker_DeliveredAsMedia(t *testing.T) {
	roomUUID := uuid.New()
	alias := "#" + roomUUID.String() + ":test.local"
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{Aliases: []id.RoomAlias{id.RoomAlias(alias)}},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	got := make(chan domain.Message, 1)
	a.eventHandlers = EventHandlers{
		OnMessage: func(m domain.Message) error { got <- m; return nil },
	}

	evt := &event.Event{
		ID:        id.EventID("$sticker1"),
		Sender:    expectedUserID(testActorID),
		Type:      event.EventSticker, // sticker has no msgtype field
		RoomID:    "!room:test.local",
		Timestamp: 1700000000000,
		Content: event.Content{
			Raw: map[string]any{
				"body": "party parrot",
				"url":  "mxc://test.local/stickerid",
				"info": map[string]any{"mimetype": "image/png", "w": float64(128), "h": float64(128)},
			},
		},
	}

	a.processEvent(evt)

	select {
	case m := <-got:
		assert.Empty(t, m.Content, "sticker alt-text is not duplicated as text")
		assert.Equal(t, roomUUID.String(), m.RoomID)
		require.Len(t, m.Attachments, 1)
		assert.Equal(t, "stickerid", m.Attachments[0].MediaID, "MediaID carries the re-home key")
		assert.Equal(t, "party parrot", m.Attachments[0].DisplayName)
	case <-time.After(2 * time.Second):
		t.Fatal("expected the sticker to be delivered to OnMessage as media")
	}
}
