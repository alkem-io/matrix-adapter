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
