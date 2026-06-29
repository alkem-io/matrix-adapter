package matrix

import (
	"encoding/json"
	"testing"

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
	}))
	if att == nil {
		t.Fatal("expected non-nil attachment")
		return
	}
	if att.MediaID != "media123" {
		t.Errorf("expected MediaID 'media123', got %q", att.MediaID)
	}
	if att.DocumentID != "doc-abc" {
		t.Errorf("expected DocumentID 'doc-abc', got %q", att.DocumentID)
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

// Inbound m.image from Element (no io.alkemio.document_id) → MediaID set,
// DocumentID empty (server will re-home by media_id).
func TestExtractAttachment_ImageWithoutDocumentID(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.image",
		"body":    "element.png",
		"url":     "mxc://other.server/xyz789",
		"info": map[string]any{
			"mimetype": "image/png",
			"size":     float64(42),
		},
	}))
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

// m.file inbound maps to an attachment too.
func TestExtractAttachment_File(t *testing.T) {
	att := extractAttachment(mediaEvent(map[string]any{
		"msgtype": "m.file",
		"body":    "doc.pdf",
		"url":     "mxc://test.local/fileabc",
		"info":    map[string]any{"mimetype": "application/pdf", "size": float64(1000)},
	}))
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
	}))
	if att != nil {
		t.Errorf("expected nil attachment for m.text, got %+v", att)
	}
}

// An event with no raw content yields no attachment.
func TestExtractAttachment_NilRaw(t *testing.T) {
	if att := extractAttachment(&event.Event{}); att != nil {
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
