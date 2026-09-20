package matrix

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

// ---------------------------------------------------------------------------
// capturing logger
// ---------------------------------------------------------------------------

type logEntry struct {
	level  string
	msg    string
	fields []interface{}
}

// field returns the value logged under key, and whether it was present. The
// adapter logs zap-style alternating key/value pairs.
func (e logEntry) field(key string) (interface{}, bool) {
	for i := 0; i+1 < len(e.fields); i += 2 {
		if k, ok := e.fields[i].(string); ok && k == key {
			return e.fields[i+1], true
		}
	}
	return nil, false
}

// capturingLogger records structured log entries so tests can assert that an
// operator actually gets the identifying context for a silently-dropped item.
type capturingLogger struct{ entries []logEntry }

func (l *capturingLogger) Debug(msg string, fields ...interface{}) { l.record("debug", msg, fields) }
func (l *capturingLogger) Info(msg string, fields ...interface{})  { l.record("info", msg, fields) }
func (l *capturingLogger) Warn(msg string, fields ...interface{})  { l.record("warn", msg, fields) }
func (l *capturingLogger) Error(msg string, fields ...interface{}) { l.record("error", msg, fields) }
func (l *capturingLogger) With(_ ...interface{}) ports.Logger      { return l }

func (l *capturingLogger) record(level, msg string, fields []interface{}) {
	l.entries = append(l.entries, logEntry{level: level, msg: msg, fields: fields})
}

// warnContaining returns the first warn entry whose message contains sub.
func (l *capturingLogger) warnContaining(t *testing.T, sub string) logEntry {
	t.Helper()
	for _, e := range l.entries {
		if e.level == "warn" && strings.Contains(e.msg, sub) {
			return e
		}
	}
	t.Fatalf("no warn log containing %q; got %+v", sub, l.entries)
	return logEntry{}
}

// ---------------------------------------------------------------------------
// F2 — a dropped attachment must be traceable
// ---------------------------------------------------------------------------

// A partial fan-out failure is reported to the server as a SUCCESS (the primary
// event was delivered), so this warning is the ONLY trace the dropped attachment
// leaves. Without room / sender / primary-event / document ids it is untraceable
// in production: an operator cannot tell WHICH message lost WHICH document.
func TestFanOutAttachments_PartialFailureLogCarriesIdentifyingContext(t *testing.T) {
	fileServiceURL := stubFileService(t, func(r *http.Request) (*http.Response, error) {
		// The second document is unfetchable, so attachment 2 of 2 fails after the
		// text event and attachment 1 have already been delivered.
		if strings.Contains(r.URL.Path, docID2) {
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody}, nil
		}
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/ok")},
		sendTextResult:         &mautrix.RespSendEvent{EventID: "$text"},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media1"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)
	log := &capturingLogger{}
	a.logger = log

	eventID, err := a.SendMessage(context.Background(), "!room1:test.local",
		testActor(testActorID, "Alice"), "hello",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "ok.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "gone.png", MimeType: "image/png"},
		})
	require.NoError(t, err, "partial fan-out is reported as success")
	require.Equal(t, id.EventID("$text"), eventID)

	entry := log.warnContaining(t, "fan-out partial failure")

	roomID, ok := entry.field("room_id")
	assert.True(t, ok, "the warning must name the room")
	assert.Equal(t, id.RoomID("!room1:test.local"), roomID)

	sender, ok := entry.field("sender_user_id")
	assert.True(t, ok, "the warning must name the sender")
	assert.Equal(t, expectedUserID(testActorID), sender)

	primary, ok := entry.field("primary_event_id")
	assert.True(t, ok, "the warning must name the primary event that WAS delivered")
	assert.Equal(t, id.EventID("$text"), primary)

	docID, ok := entry.field("document_id")
	assert.True(t, ok, "the warning must name the document that was dropped")
	assert.Equal(t, docID2, docID)

	idx, ok := entry.field("attachment")
	assert.True(t, ok)
	assert.Equal(t, 2, idx)
	total, ok := entry.field("total")
	assert.True(t, ok)
	assert.Equal(t, 2, total)
}

// ---------------------------------------------------------------------------
// F14 — the whole message's fan-out is time-bounded
// ---------------------------------------------------------------------------

// The AMQP/watermill message context carries NO deadline and processMessage runs
// messages SEQUENTIALLY in one goroutine, so without a whole-message budget a
// single send could occupy the room-ops consumer for N x the per-attachment
// ceiling and stall all room messaging behind it. fanOutAttachments must impose
// ONE shared deadline on the entire fan-out, including the media event sends.
func TestFanOutAttachments_WholeMessageIsTimeBounded(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/ok")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	// context.Background() has no deadline, exactly like the queue-handler context.
	_, err := a.SendMessage(context.Background(), "!room1:test.local",
		testActor(testActorID, "Alice"), "",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "b.png", MimeType: "image/png"},
		})
	require.NoError(t, err)
	require.Equal(t, 2, intent.sendMessageEventCalled)

	require.False(t, intent.lastSendMsgEventDeadline.IsZero(),
		"the fan-out must bound the WHOLE message, including the media event sends — "+
			"a deadline-less context lets one send hold the sequential consumer indefinitely")
	budget := messageFanOutTimeout(a.cfg.MaxAttachmentBytes())
	assert.LessOrEqual(t, time.Until(intent.lastSendMsgEventDeadline), budget,
		"the deadline must be the shared whole-message budget")
}

// The budget must actually be a BOUND: strictly less than what an unbudgeted
// fan-out of maxAttachmentsPerMessage attachments could occupy (10 x the
// per-attachment ceiling), while still leaving a healthy send ample room.
func TestMessageFanOutTimeout_IsTighterThanTheUnboundedFanOut(t *testing.T) {
	const maxAttachments = 10 // queue.maxAttachmentsPerMessage
	for _, maxBytes := range []int64{1 << 20, 50 << 20, 1 << 40} {
		perAttachment := mediaStreamTimeout(maxBytes)
		budget := messageFanOutTimeout(maxBytes)
		assert.Greater(t, budget, perAttachment,
			"a single max-size attachment must still fit in the budget")
		assert.Less(t, budget, maxAttachments*perAttachment,
			"the budget must bound the fan-out below the unbudgeted worst case")
	}
}
