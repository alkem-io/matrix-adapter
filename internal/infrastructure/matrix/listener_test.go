package matrix

import (
	"encoding/json"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/google/uuid"
	"math"
	"maunium.net/go/mautrix"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// adapter returns a zero-value MautrixAdapter suitable for testing pure helper
// methods that do not touch any adapter state (isMessageEdit,
// extractRedactedEventID, extractRedactionReason, extractThreadIDFromMessage).
func adapter() *MautrixAdapter { return &MautrixAdapter{} }

// ---------------------------------------------------------------------------
// extractAttachment (T010 — inbound media translation)
// ---------------------------------------------------------------------------

func mediaEvent(raw map[string]any) *event.Event {
	return &event.Event{Content: event.Content{Raw: raw}}
}

// Inbound m.image carrying io.alkemio.document_id (our own outbound echo) →
// both DocumentID and MediaID are surfaced.
func TestExtractAttachment_ImageWithDocumentID(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.image",
		"body":    "photo.jpg",
		"url":     "mxc://test.local/media123",
		"info": map[string]any{
			"mimetype": "image/jpeg",
			"size":     float64(12345), // JSON numbers decode to float64
			"w":        float64(1920),
			"h":        float64(1080),
		},
		"io.alkemio.document_id": "doc-abc",
	}), testIDMapper)
	if att == nil {
		t.Fatal("expected non-nil attachment")
		return
	}
	// This is our own outbound echo (it carries io.alkemio.document_id). Both refs
	// are surfaced: the server routes echoes by DocumentID presence to the coalesce
	// path, which stamps externalReference=media_id on the doc and drops the staging
	// twin — so it needs the MediaID too.
	if att.DocumentID != "doc-abc" {
		t.Errorf("expected DocumentID 'doc-abc', got %q", att.DocumentID)
	}
	if att.MediaID != "media123" {
		t.Errorf("expected MediaID 'media123' surfaced alongside DocumentID, got %q", att.MediaID)
	}
	if att.MimeType != "image/jpeg" {
		t.Errorf("expected MimeType 'image/jpeg', got %q", att.MimeType)
	}
	if att.Size != 12345 {
		t.Errorf("expected Size 12345, got %d", att.Size)
	}
	if att.DisplayName != "photo.jpg" {
		t.Errorf("expected DisplayName 'photo.jpg', got %q", att.DisplayName)
	}
	if att.Width == nil || *att.Width != 1920 {
		t.Errorf("expected Width 1920, got %v", att.Width)
	}
	if att.Height == nil || *att.Height != 1080 {
		t.Errorf("expected Height 1080, got %v", att.Height)
	}
}

// Inbound m.image from Element on OUR homeserver (no io.alkemio.document_id) →
// MediaID set, DocumentID empty (server will re-home by media_id).
func TestExtractAttachment_ImageWithoutDocumentID(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.image",
		"body":    "element.png",
		"url":     "mxc://test.local/xyz789",
		"info": map[string]any{
			"mimetype": "image/png",
			"size":     float64(42),
		},
	}), testIDMapper)
	if att == nil {
		t.Fatal("expected non-nil attachment")
		return
	}
	if att.MediaID != "xyz789" {
		t.Errorf("expected MediaID 'xyz789', got %q", att.MediaID)
	}
	if att.DocumentID != "" {
		t.Errorf("expected empty DocumentID, got %q", att.DocumentID)
	}
	if att.Width != nil || att.Height != nil {
		t.Errorf("expected nil dims, got w=%v h=%v", att.Width, att.Height)
	}
}

// Media hosted on a FOREIGN homeserver must NOT be surfaced as a bare media_id.
// MediaID is the server's re-home key and is resolved by externalReference
// against media OUR Synapse stored, so a foreign media id would either miss
// (attachment silently lost) or COLLIDE with an unrelated local media id and
// resolve to the wrong document. With no document id either, the event carries
// no reference the server can act on, so no attachment is surfaced at all —
// exactly like a non-mxc url.
func TestExtractAttachment_ForeignHomeserverMediaNotSurfaced(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.image",
		"body":    "federated.png",
		"url":     "mxc://other.server/xyz789",
		"info":    map[string]any{"mimetype": "image/png", "size": float64(42)},
	}), testIDMapper)
	if att != nil {
		t.Errorf("foreign-homeserver media must not surface a re-homable ref, got %+v", att)
	}
}

// A foreign-homeserver url on an ECHO of our own outbound media (trusted sender,
// io.alkemio.document_id present) still surfaces the DocumentID — that ref is
// ours and resolvable — but never the foreign media id, which would send the
// coalesce path at the wrong (or a colliding) document.
func TestExtractAttachment_ForeignHomeserverKeepsDocumentIDDropsMediaID(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype":                "m.image",
		"body":                   "federated.png",
		"url":                    "mxc://other.server/xyz789",
		"io.alkemio.document_id": "doc-abc",
	}), testIDMapper)
	if att == nil {
		t.Fatal("expected the document id to still be surfaced")
		return
	}
	if att.MediaID != "" {
		t.Errorf("foreign media id must not be surfaced, got %q", att.MediaID)
	}
	if att.DocumentID != "doc-abc" {
		t.Errorf("expected DocumentID 'doc-abc', got %q", att.DocumentID)
	}
}

// m.file inbound maps to an attachment too.
func TestExtractAttachment_File(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.file",
		"body":    "doc.pdf",
		"url":     "mxc://test.local/fileabc",
		"info":    map[string]any{"mimetype": "application/pdf", "size": float64(1000)},
	}), testIDMapper)
	if att == nil {
		t.Fatal("expected non-nil attachment")
		return
	}
	if att.MediaID != "fileabc" {
		t.Errorf("expected MediaID 'fileabc', got %q", att.MediaID)
	}
}

// A plain text message yields no attachment.
func TestExtractAttachment_NonMedia(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.text",
		"body":    "hello",
	}), testIDMapper)
	if att != nil {
		t.Errorf("expected nil attachment for m.text, got %+v", att)
	}
}

// An event with no raw content yields no attachment.
func TestExtractAttachment_NilRaw(t *testing.T) {
	if att := extractAttachment(&event.Event{}, testIDMapper); att != nil {
		t.Errorf("expected nil attachment for empty event, got %+v", att)
	}
}

// ---------------------------------------------------------------------------
// parseStateChange
// ---------------------------------------------------------------------------

func TestParseStateChange_RoomName(t *testing.T) {
	evt := &event.Event{
		Type: event.StateRoomName,
		Content: event.Content{
			Parsed: &event.RoomNameEventContent{Name: "My Room"},
		},
	}
	sc := parseStateChange(evt)
	if sc == nil {
		t.Fatal("expected non-nil stateChange")
	}
	if sc.DisplayName == nil || *sc.DisplayName != "My Room" {
		t.Errorf("expected DisplayName 'My Room', got %v", sc.DisplayName)
	}
	if sc.Topic != nil {
		t.Errorf("expected Topic nil, got %v", sc.Topic)
	}
	if sc.AvatarURL != nil {
		t.Errorf("expected AvatarURL nil, got %v", sc.AvatarURL)
	}
}

func TestParseStateChange_RoomTopic(t *testing.T) {
	evt := &event.Event{
		Type: event.StateTopic,
		Content: event.Content{
			Parsed: &event.TopicEventContent{Topic: "Discussion about Go"},
		},
	}
	sc := parseStateChange(evt)
	if sc == nil {
		t.Fatal("expected non-nil stateChange")
	}
	if sc.Topic == nil || *sc.Topic != "Discussion about Go" {
		t.Errorf("expected Topic 'Discussion about Go', got %v", sc.Topic)
	}
	if sc.DisplayName != nil {
		t.Errorf("expected DisplayName nil, got %v", sc.DisplayName)
	}
}

func TestParseStateChange_RoomAvatar(t *testing.T) {
	evt := &event.Event{
		Type: event.StateRoomAvatar,
		Content: event.Content{
			Parsed: &event.RoomAvatarEventContent{
				URL: id.ContentURIString("mxc://example.com/abc123"),
			},
		},
	}
	sc := parseStateChange(evt)
	if sc == nil {
		t.Fatal("expected non-nil stateChange")
	}
	if sc.AvatarURL == nil {
		t.Fatal("expected AvatarURL non-nil")
	}
	if *sc.AvatarURL != "mxc://example.com/abc123" {
		t.Errorf("expected 'mxc://example.com/abc123', got %q", *sc.AvatarURL)
	}
	if sc.DisplayName != nil {
		t.Errorf("expected DisplayName nil, got %v", sc.DisplayName)
	}
}

func TestParseStateChange_UnknownEventType(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.MessageEventContent{Body: "hello"},
		},
	}
	sc := parseStateChange(evt)
	if sc != nil {
		t.Errorf("expected nil for unknown event type, got %+v", sc)
	}
}

func TestParseStateChange_NilContent(t *testing.T) {
	evt := &event.Event{
		Type:    event.StateRoomName,
		Content: event.Content{},
	}
	sc := parseStateChange(evt)
	if sc != nil {
		t.Errorf("expected nil for nil content, got %+v", sc)
	}
}

func TestParseStateChange_EmptyName(t *testing.T) {
	evt := &event.Event{
		Type: event.StateRoomName,
		Content: event.Content{
			Parsed: &event.RoomNameEventContent{Name: ""},
		},
	}
	sc := parseStateChange(evt)
	if sc == nil {
		t.Fatal("expected non-nil stateChange for empty name")
	}
	if sc.DisplayName == nil || *sc.DisplayName != "" {
		t.Errorf("expected empty DisplayName, got %v", sc.DisplayName)
	}
}

// ---------------------------------------------------------------------------
// isMessageEdit
// ---------------------------------------------------------------------------

func TestIsMessageEdit_WithReplace(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Raw: map[string]interface{}{
				"body": "edited message",
				"m.relates_to": map[string]interface{}{
					"rel_type": "m.replace",
					"event_id": "$original123",
				},
			},
		},
	}
	if !adapter().isMessageEdit(evt) {
		t.Error("expected isMessageEdit to return true for m.replace relation")
	}
}

func TestIsMessageEdit_RegularMessage(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Raw: map[string]interface{}{
				"body":    "hello world",
				"msgtype": "m.text",
			},
		},
	}
	if adapter().isMessageEdit(evt) {
		t.Error("expected isMessageEdit to return false for regular message")
	}
}

func TestIsMessageEdit_NilContent(t *testing.T) {
	evt := &event.Event{
		Type:    event.EventMessage,
		Content: event.Content{},
	}
	if adapter().isMessageEdit(evt) {
		t.Error("expected isMessageEdit to return false when Content.Raw is nil")
	}
}

func TestIsMessageEdit_ThreadRelation(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Raw: map[string]interface{}{
				"body": "thread reply",
				"m.relates_to": map[string]interface{}{
					"rel_type": "m.thread",
					"event_id": "$thread_root",
				},
			},
		},
	}
	if adapter().isMessageEdit(evt) {
		t.Error("expected isMessageEdit to return false for m.thread relation")
	}
}

func TestIsMessageEdit_MissingRelType(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Raw: map[string]interface{}{
				"body": "message",
				"m.relates_to": map[string]interface{}{
					"event_id": "$some_event",
				},
			},
		},
	}
	if adapter().isMessageEdit(evt) {
		t.Error("expected isMessageEdit to return false when rel_type is missing")
	}
}

// ---------------------------------------------------------------------------
// extractRedactedEventID
// ---------------------------------------------------------------------------

func TestExtractRedactedEventID_FromRedactsField(t *testing.T) {
	evt := &event.Event{
		Redacts: "$redacted_event_123",
	}
	got := adapter().extractRedactedEventID(evt)
	if got != "$redacted_event_123" {
		t.Errorf("expected '$redacted_event_123', got %q", got)
	}
}

func TestExtractRedactedEventID_FromRawContent(t *testing.T) {
	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"redacts": "$from_content_456",
			},
		},
	}
	got := adapter().extractRedactedEventID(evt)
	if got != "$from_content_456" {
		t.Errorf("expected '$from_content_456', got %q", got)
	}
}

func TestExtractRedactedEventID_PreferRedactsField(t *testing.T) {
	// When both Redacts field and raw content are set, Redacts field wins.
	evt := &event.Event{
		Redacts: "$from_field",
		Content: event.Content{
			Raw: map[string]interface{}{
				"redacts": "$from_content",
			},
		},
	}
	got := adapter().extractRedactedEventID(evt)
	if got != "$from_field" {
		t.Errorf("expected '$from_field', got %q", got)
	}
}

func TestExtractRedactedEventID_Empty(t *testing.T) {
	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"reason": "spam",
			},
		},
	}
	got := adapter().extractRedactedEventID(evt)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractRedactedEventID_NilContent(t *testing.T) {
	evt := &event.Event{}
	got := adapter().extractRedactedEventID(evt)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractRedactionReason
// ---------------------------------------------------------------------------

func TestExtractRedactionReason_Present(t *testing.T) {
	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"reason": "violates policy",
			},
		},
	}
	got := adapter().extractRedactionReason(evt)
	if got != "violates policy" {
		t.Errorf("expected 'violates policy', got %q", got)
	}
}

func TestExtractRedactionReason_Missing(t *testing.T) {
	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"other_field": "value",
			},
		},
	}
	got := adapter().extractRedactionReason(evt)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractRedactionReason_NilContent(t *testing.T) {
	evt := &event.Event{}
	got := adapter().extractRedactionReason(evt)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractRedactionReason_NonStringValue(t *testing.T) {
	evt := &event.Event{
		Content: event.Content{
			Raw: map[string]interface{}{
				"reason": 42,
			},
		},
	}
	got := adapter().extractRedactionReason(evt)
	if got != "" {
		t.Errorf("expected empty string for non-string reason, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractThreadIDFromMessage
// ---------------------------------------------------------------------------

func TestExtractThreadIDFromMessage_ThreadRelation(t *testing.T) {
	threadRoot := id.EventID("$thread_root_001")
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				Body: "thread reply",
				RelatesTo: &event.RelatesTo{
					Type:    event.RelThread,
					EventID: threadRoot,
				},
			},
		},
	}
	got := adapter().extractThreadIDFromMessage(evt)
	if got == nil {
		t.Fatal("expected non-nil thread ID")
	}
	if *got != threadRoot {
		t.Errorf("expected %q, got %q", threadRoot, *got)
	}
}

func TestExtractThreadIDFromMessage_RawThreadRelation(t *testing.T) {
	threadRoot := id.EventID("$thread_root_from_get_event")
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			// GetEvent-style content: Parsed is nil and the relation exists only
			// in Raw.
			Raw: map[string]any{
				"m.relates_to": map[string]any{
					"rel_type": "m.thread",
					"event_id": threadRoot.String(),
				},
			},
		},
	}
	if evt.Content.Parsed != nil {
		t.Fatal("expected GetEvent-style event to have nil Parsed content")
	}

	got := adapter().extractThreadIDFromMessage(evt)
	if got == nil {
		t.Fatal("expected non-nil thread ID from raw relation")
	}
	if *got != threadRoot {
		t.Errorf("expected %q, got %q", threadRoot, *got)
	}
}

func TestExtractThreadIDFromMessage_InReplyToFallback(t *testing.T) {
	replyTo := id.EventID("$reply_target_002")
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				Body: "reply message",
				RelatesTo: &event.RelatesTo{
					InReplyTo: &event.InReplyTo{
						EventID: replyTo,
					},
				},
			},
		},
	}
	got := adapter().extractThreadIDFromMessage(evt)
	if got == nil {
		t.Fatal("expected non-nil thread ID from in_reply_to fallback")
	}
	if *got != replyTo {
		t.Errorf("expected %q, got %q", replyTo, *got)
	}
}

func TestExtractThreadIDFromMessage_NoRelations(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				Body: "plain message",
			},
		},
	}
	got := adapter().extractThreadIDFromMessage(evt)
	if got != nil {
		t.Errorf("expected nil thread ID, got %q", *got)
	}
}

func TestExtractThreadIDFromMessage_NilRelatesTo(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				Body:      "message",
				RelatesTo: nil,
			},
		},
	}
	got := adapter().extractThreadIDFromMessage(evt)
	if got != nil {
		t.Errorf("expected nil thread ID, got %q", *got)
	}
}

func TestExtractThreadIDFromMessage_WrongContentType(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.RoomNameEventContent{Name: "not a message"},
		},
	}
	got := adapter().extractThreadIDFromMessage(evt)
	if got != nil {
		t.Errorf("expected nil for wrong content type, got %q", *got)
	}
}

func TestExtractThreadIDFromMessage_NilParsed(t *testing.T) {
	evt := &event.Event{
		Type:    event.EventMessage,
		Content: event.Content{},
	}
	got := adapter().extractThreadIDFromMessage(evt)
	if got != nil {
		t.Errorf("expected nil for nil parsed content, got %q", *got)
	}
}

func TestExtractThreadIDFromMessage_ThreadPreferredOverReply(t *testing.T) {
	// When both thread relation and in_reply_to are present, thread wins.
	threadRoot := id.EventID("$thread_root_003")
	replyTo := id.EventID("$reply_target_003")
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{
			Parsed: &event.MessageEventContent{
				Body: "threaded reply",
				RelatesTo: &event.RelatesTo{
					Type:    event.RelThread,
					EventID: threadRoot,
					InReplyTo: &event.InReplyTo{
						EventID: replyTo,
					},
				},
			},
		},
	}
	got := adapter().extractThreadIDFromMessage(evt)
	if got == nil {
		t.Fatal("expected non-nil thread ID")
	}
	if *got != threadRoot {
		t.Errorf("expected thread root %q, got %q", threadRoot, *got)
	}
}

// ---------------------------------------------------------------------------
// parseStateChange — VeryRaw fallback path
// ---------------------------------------------------------------------------

func TestParseStateChange_RoomNameFromVeryRaw(t *testing.T) {
	// When Parsed is nil but VeryRaw JSON is present, parseEventContent falls
	// back to ParseRaw which reads VeryRaw. Verify that path works.
	evt := &event.Event{
		Type: event.StateRoomName,
		Content: event.Content{
			VeryRaw: json.RawMessage(`{"name":"From Raw"}`),
		},
	}
	sc := parseStateChange(evt)
	if sc == nil {
		t.Fatal("expected non-nil stateChange from VeryRaw")
	}
	if sc.DisplayName == nil || *sc.DisplayName != "From Raw" {
		t.Errorf("expected 'From Raw', got %v", sc.DisplayName)
	}
}

func TestParseStateChange_TopicFromVeryRaw(t *testing.T) {
	evt := &event.Event{
		Type: event.StateTopic,
		Content: event.Content{
			VeryRaw: json.RawMessage(`{"topic":"Raw Topic"}`),
		},
	}
	sc := parseStateChange(evt)
	if sc == nil {
		t.Fatal("expected non-nil stateChange from VeryRaw")
	}
	if sc.Topic == nil || *sc.Topic != "Raw Topic" {
		t.Errorf("expected 'Raw Topic', got %v", sc.Topic)
	}
}

// ---------------------------------------------------------------------------
// Round-3 review regressions
// ---------------------------------------------------------------------------

// A media event's body must ALWAYS reach Content. An earlier revision blanked it
// whenever it equalled the resolved display name (the MSC2530 "body == filename
// ⇒ no caption" rule), which silently emptied the message for every consumer
// reading Content without the attachments array — on the pre-attachments
// behaviour that consumer saw the filename. Both strings now travel together, so
// the renderer (not the adapter) decides whether to show one or both.
func TestExtractInboundMessage_MediaBodyIsNeverBlanked(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "legacy media: body IS the filename, no filename field",
			raw: map[string]any{
				"msgtype": "m.file",
				"body":    "report.pdf",
				"url":     "mxc://test.local/legacy",
				"info":    map[string]any{"mimetype": "application/pdf"},
			},
			want: "report.pdf",
		},
		{
			name: "MSC2530 captionless: explicit filename equal to body",
			raw: map[string]any{
				"msgtype":  "m.image",
				"body":     "photo.jpg",
				"filename": "photo.jpg",
				"url":      "mxc://test.local/eq",
				"info":     map[string]any{"mimetype": "image/jpeg"},
			},
			want: "photo.jpg",
		},
		{
			name: "MSC2530 caption: body differs from filename",
			raw: map[string]any{
				"msgtype":  "m.image",
				"body":     "look at this sunset",
				"filename": "sunset.jpg",
				"url":      "mxc://test.local/cap",
				"info":     map[string]any{"mimetype": "image/jpeg"},
			},
			want: "look at this sunset",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content, att, ok := extractInboundMessage(mediaEvent(tc.raw), testIDMapper)
			if !ok {
				t.Fatal("expected the media event to be surfaced")
			}
			if content != tc.want {
				t.Errorf("Content = %q, want %q (the body must not be dropped)", content, tc.want)
			}
			if att == nil {
				t.Fatal("expected an attachment")
			}
			// The MSC2530 signal survives: the resolved filename is on the
			// attachment, so `Content == DisplayName` still means "no caption".
			if att.DisplayName == "" {
				t.Error("DisplayName must carry the resolved filename")
			}
		})
	}
}

// info.w / info.h are asserted by the sending client and land in the Alkemio
// server's GraphQL `Int` (32-bit) width/height. Anything outside the positive
// 32-bit pixel domain must be surfaced as ABSENT rather than shipped onward.
func TestRawPixelPtr_ClampedToPositive32BitRange(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want *int
	}{
		{"in range", float64(1920), ptrInt(1920)},
		{"max int32 is accepted", float64(math.MaxInt32), ptrInt(math.MaxInt32)},
		{"one past max int32 is rejected", float64(math.MaxInt32) + 1, nil},
		{"far past max int32 is rejected", float64(1 << 40), nil},
		{"zero is absent (server treats non-positive as absent)", float64(0), nil},
		{"negative is absent", float64(-1), nil},
		{"non-numeric is absent", "1920", nil},
		{"json.Number past max int32 is rejected", json.Number("2147483648"), nil},
		{"json.Number in range is accepted", json.Number("1080"), ptrInt(1080)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rawPixelPtr(tc.in)
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("rawPixelPtr(%v) = %d, want nil", tc.in, *got)
			case tc.want != nil && got == nil:
				t.Errorf("rawPixelPtr(%v) = nil, want %d", tc.in, *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("rawPixelPtr(%v) = %d, want %d", tc.in, *got, *tc.want)
			}
		})
	}
}

func ptrInt(v int) *int { return &v }

// End to end on the inbound path: a hostile info block naming out-of-32-bit-range
// dimensions and size must not reach the wire DTO, or the server's GraphQL
// response breaks for the whole query — not just for this attachment.
func TestExtractAttachment_OutOf32BitRangeInfoIsDropped(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.image",
		"body":    "huge.png",
		"url":     "mxc://test.local/huge",
		"info": map[string]any{
			"mimetype": "image/png",
			"size":     float64(int64(1) << 40),
			"w":        float64(math.MaxInt32) + 1,
			"h":        float64(math.MaxInt32) + 1,
		},
	}), testIDMapper)
	if att == nil {
		t.Fatal("expected an attachment (the media ref itself is still valid)")
	}
	if att.Width != nil {
		t.Errorf("Width = %d, want nil (out of GraphQL Int range)", *att.Width)
	}
	if att.Height != nil {
		t.Errorf("Height = %d, want nil (out of GraphQL Int range)", *att.Height)
	}
	if att.Size != 0 {
		t.Errorf("Size = %d, want 0/unknown (out of GraphQL Int range)", att.Size)
	}
}

// An m.thread relation whose event_id is the EMPTY string names no parent, so
// extraction must fall through to the m.in_reply_to fallback instead of
// short-circuiting with "" and losing the thread linkage.
func TestExtractThreadID_EmptyThreadEventIDFallsBackToInReplyTo(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{Raw: map[string]any{
			"msgtype": "m.text",
			"body":    "reply",
			"m.relates_to": map[string]any{
				"rel_type":      "m.thread",
				"event_id":      "",
				"m.in_reply_to": map[string]any{"event_id": "$parent"},
			},
		}},
	}
	if got := extractThreadID(evt); got != "$parent" {
		t.Errorf("extractThreadID = %q, want %q", got, "$parent")
	}
}

// A missing event_id on the m.thread relation behaves the same way.
func TestExtractThreadID_MissingThreadEventIDFallsBackToInReplyTo(t *testing.T) {
	evt := &event.Event{
		Type: event.EventMessage,
		Content: event.Content{Raw: map[string]any{
			"msgtype": "m.text",
			"body":    "reply",
			"m.relates_to": map[string]any{
				"rel_type":      "m.thread",
				"m.in_reply_to": map[string]any{"event_id": "$parent"},
			},
		}},
	}
	if got := extractThreadID(evt); got != "$parent" {
		t.Errorf("extractThreadID = %q, want %q", got, "$parent")
	}
}

// extractThreadID's contract says the shared inbound helper never triggers
// ParseRaw. The event is shared with the caller, and ParseRaw permanently
// populates Content.Parsed — so extractInboundMessage must stay observation-only
// on an event that carries only VeryRaw.
func TestExtractInboundMessage_DoesNotParseRawOnSharedEvent(t *testing.T) {
	evt := &event.Event{
		Type:    event.EventMessage,
		Content: event.Content{VeryRaw: json.RawMessage(`{"msgtype":"m.text","body":"hi"}`)},
	}

	_, _, _ = extractInboundMessage(evt, testIDMapper)

	if evt.Content.Parsed != nil {
		t.Errorf("extractInboundMessage populated Content.Parsed (%T) — it must not "+
			"ParseRaw a shared event; extractThreadID's contract depends on it", evt.Content.Parsed)
	}
}

// ---------------------------------------------------------------------------
// Bridge guarantees (T024 — contract appservice-event-bridge G2–G4)
// ---------------------------------------------------------------------------

// newListenerTestAdapter builds an adapter whose alias lookup resolves the
// test room to the given Alkemio uuid, with OnMemberUpdated/OnMessage wired
// to channels for synchronising with the handlers' goroutines.
func newListenerTestAdapter(alkemioRoomID string) (*MautrixAdapter, chan domain.RoomMemberUpdatedEvent, chan domain.Message) {
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{Aliases: []id.RoomAlias{id.RoomAlias("#" + alkemioRoomID + ":test.local")}},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	memberCh := make(chan domain.RoomMemberUpdatedEvent, 1)
	messageCh := make(chan domain.Message, 1)
	a.SetEventHandlers(EventHandlers{
		OnMemberUpdated: func(evt domain.RoomMemberUpdatedEvent) error {
			memberCh <- evt
			return nil
		},
		OnMessage: func(msg domain.Message) error {
			messageCh <- msg
			return nil
		},
	})
	return a, memberCh, messageCh
}

func TestListener_BotKick_EmitsMemberLeave(t *testing.T) {
	roomUUID := "880e8400-e29b-41d4-a716-446655440003"
	target := "550e8400-e29b-41d4-a716-446655440000"
	a, memberCh, _ := newListenerTestAdapter(roomUUID)

	stateKey := "@" + target + ":test.local"
	a.processEvent(&event.Event{
		Type:     event.StateMember,
		RoomID:   "!room:test.local",
		Sender:   "@bot:test.local", // the bot performed the kick
		StateKey: &stateKey,
		Content:  event.Content{Parsed: &event.MemberEventContent{Membership: event.MembershipLeave}},
	})

	select {
	case evt := <-memberCh:
		if evt.Membership != "leave" {
			t.Errorf("membership = %q, want leave", evt.Membership)
		}
		if evt.MemberID.String() != target {
			t.Errorf("member = %s, want %s", evt.MemberID, target)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a bot-performed kick must still emit a membership event (G2)")
	}
}

func TestListener_MemberEvent_BeforeOwnSenderFilter(t *testing.T) {
	// A message FROM the bot is dropped (own-sender filter) …
	roomUUID := "880e8400-e29b-41d4-a716-446655440003"
	a, memberCh, messageCh := newListenerTestAdapter(roomUUID)

	a.processEvent(&event.Event{
		Type:    event.EventMessage,
		RoomID:  "!room:test.local",
		Sender:  "@bot:test.local",
		Content: event.Content{Raw: map[string]interface{}{"body": "own message"}},
	})
	select {
	case <-messageCh:
		t.Fatal("the bot's own timeline events must be filtered")
	case <-time.After(150 * time.Millisecond):
	}

	// … but a membership event with the bot as SENDER is dispatched, because
	// membership handling precedes the own-sender filter.
	stateKey := "@550e8400-e29b-41d4-a716-446655440000:test.local"
	a.processEvent(&event.Event{
		Type:     event.StateMember,
		RoomID:   "!room:test.local",
		Sender:   "@bot:test.local",
		StateKey: &stateKey,
		Content:  event.Content{Parsed: &event.MemberEventContent{Membership: event.MembershipLeave}},
	})
	select {
	case <-memberCh:
	case <-time.After(2 * time.Second):
		t.Fatal("membership dispatch must precede the own-sender filter")
	}
}

func TestListener_UnresolvedSender_ZeroUUID(t *testing.T) {
	roomUUID := "880e8400-e29b-41d4-a716-446655440003"
	a, _, messageCh := newListenerTestAdapter(roomUUID)

	// A sender that is not a platform actor (non-UUID localpart) still
	// produces exactly one emitted event, with the zero-UUID unresolved
	// marker — never dropped, never misattributed (G3, US6-AS4).
	a.processEvent(&event.Event{
		Type:    event.EventMessage,
		RoomID:  "!room:test.local",
		Sender:  "@alice-admin:test.local",
		Content: event.Content{Raw: map[string]interface{}{"body": "hello from a non-actor"}},
	})

	select {
	case msg := <-messageCh:
		if msg.SenderID != uuid.Nil {
			t.Errorf("sender = %s, want the zero UUID (unresolved)", msg.SenderID)
		}
		if msg.Content != "hello from a non-actor" {
			t.Errorf("content = %q", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an event from a non-actor sender must be emitted, not dropped")
	}
}
