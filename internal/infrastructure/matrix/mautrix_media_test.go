package matrix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// Valid UUID document ids — fetchDocumentContent validates the shape before
// building the internal file-service URL.
const (
	docID1 = "11111111-1111-4111-8111-111111111111"
	docID2 = "22222222-2222-4222-8222-222222222222"
)

// newMediaTestAdapter builds an adapter wired to a stub file-service and a mock
// intent that records media uploads + sent events.
func newMediaTestAdapter(t *testing.T, fileServiceURL string, intent *mockIntentAPI) *MautrixAdapter {
	t.Helper()
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})
	cfg := &config.Config{}
	cfg.FileService.URL = fileServiceURL
	a.cfg = cfg
	return a
}

// T007 — send with an image attachment emits an m.image event carrying
// url(mxc) + info + io.alkemio.document_id, with the bytes fetched from
// file-service and uploaded via the mautrix client (both mocked).
func TestSendMessage_WithImageAttachment(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("JPEGBYTES"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/abc123")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media1"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	w, h := 1920, 1080
	eventID, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"", // attachment-only message
		[]domain.Attachment{{
			DocumentID:  docID1,
			DisplayName: "photo.jpg",
			MimeType:    "image/jpeg",
			Size:        9,
			Width:       &w,
			Height:      &h,
		}},
		"", // no idempotency key
	)
	require.NoError(t, err)
	assert.Equal(t, id.EventID("$media1"), eventID)

	// file-service was hit on the internal content endpoint.
	assert.Equal(t, "/internal/file/"+docID1+"/content", gotPath)

	// bytes were uploaded to the homeserver media repo with the right type.
	assert.Equal(t, 1, intent.uploadBytesCalled)
	assert.Equal(t, []byte("JPEGBYTES"), intent.lastUploadBytesData)
	assert.Equal(t, "image/jpeg", intent.lastUploadBytesType)

	// no text event was sent (content was empty).
	assert.Equal(t, 0, intent.sendTextCalled)

	// exactly one media event was sent with the expected content.
	require.Equal(t, 1, intent.sendMessageEventCalled)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok, "media event content should be a raw map")
	assert.Equal(t, "m.image", content["msgtype"])
	assert.Equal(t, "photo.jpg", content["body"])
	assert.Equal(t, "mxc://test.local/abc123", content["url"])
	assert.Equal(t, docID1, content["io.alkemio.document_id"])

	info, ok := content["info"].(map[string]any)
	require.True(t, ok, "info should be a raw map")
	assert.Equal(t, "image/jpeg", info["mimetype"])
	assert.Equal(t, int64(9), info["size"])
	assert.Equal(t, 1920, info["w"])
	assert.Equal(t, 1080, info["h"])
}

// Text + attachment → 1 m.text event + 1 media event; the returned event ID is
// the text event.
func TestSendMessage_TextPlusAttachment(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("PDF"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/file9")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$text1"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	eventID, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"see attached",
		[]domain.Attachment{{
			DocumentID:  docID1,
			DisplayName: "report.pdf",
			MimeType:    "application/pdf",
			Size:        3,
		}},
		"", // no idempotency key
	)
	require.NoError(t, err)
	assert.Equal(t, id.EventID("$text1"), eventID, "primary event is the text event")
	// Text now always goes through SendMessageEvent (unified with the media path),
	// so both the text and the media event are SendMessageEvent calls.
	assert.Equal(t, 0, intent.sendTextCalled, "text no longer uses the SendText helper")
	require.Equal(t, 2, intent.sendMessageEventCalled)

	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"], "non-image MIME maps to m.file")
	assert.Equal(t, docID1, content["io.alkemio.document_id"])
}

// A non-OK response from file-service surfaces as an error and no event is sent.
func TestSendMessage_AttachmentFetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, ts.URL, intent)

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "x", MimeType: "image/png"}},
		"", // no idempotency key
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
	assert.Equal(t, 0, intent.sendMessageEventCalled)
}

// M1 — with an idempotency key, the text event and each attachment event get a
// distinct, deterministic Matrix transaction ID, so a retry of the same logical
// send is de-duplicated by the homeserver rather than producing duplicates.
func TestSendMessage_IdempotencyKey_DeterministicTxnIDs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG"))
	}))
	defer ts.Close()

	newIntent := func() *mockIntentAPI {
		return &mockIntentAPI{
			sendTextResult:         &mautrix.RespSendEvent{EventID: "$text"},
			uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/a")},
			sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media"},
		}
	}

	send := func(intent *mockIntentAPI) {
		a := newMediaTestAdapter(t, ts.URL, intent)
		_, err := a.SendMessage(
			context.Background(),
			"!room:test.local",
			testActor(testActorID, "Alice"),
			"caption",
			[]domain.Attachment{
				{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png"},
				{DocumentID: docID2, DisplayName: "b.png", MimeType: "image/png"},
			},
			"req-key-42", // idempotency key
		)
		require.NoError(t, err)
	}

	intent1 := newIntent()
	send(intent1)
	// text routed through SendMessageEvent (so it can carry a txn) + 2 attachments.
	require.Equal(t, []string{"req-key-42:text", "req-key-42:att0", "req-key-42:att1"}, intent1.sendMsgEventTxns)
	// text did NOT use the txn-less SendText helper when a key is present.
	assert.Equal(t, 0, intent1.sendTextCalled)

	// A retry with the same key recomputes byte-for-byte identical txn IDs, so
	// the homeserver de-duplicates each event.
	intent2 := newIntent()
	send(intent2)
	assert.Equal(t, intent1.sendMsgEventTxns, intent2.sendMsgEventTxns)
}

// Without an idempotency key, no deterministic transaction IDs are attached:
// both the text and the attachment event go through SendMessageEvent with an
// empty transaction ID (mautrix mints a random one per call).
func TestSendMessage_NoIdempotencyKey_NoTxnIDs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/a")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)
	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"caption",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png"}},
		"", // no key
	)
	require.NoError(t, err)
	assert.Equal(t, 0, intent.sendTextCalled, "text no longer uses the SendText helper")
	// text + single attachment, both via SendMessageEvent, neither with a txn.
	require.Equal(t, []string{"", ""}, intent.sendMsgEventTxns)
}

// M2 — fetchDocumentContent rejects a body larger than the configured max
// attachment size instead of buffering it all into memory.
func TestFetchDocumentContent_RejectsOversizedBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 100)) // 100 bytes
	}))
	defer ts.Close()

	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, ts.URL, intent)
	a.cfg.FileService.MaxAttachmentBytes = 10 // cap below the 100-byte body

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "big.bin", MimeType: "application/octet-stream"}},
		"",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds max attachment size")
	assert.Equal(t, 0, intent.sendMessageEventCalled, "no event sent when fetch is rejected")
}

// M2 — fetchDocumentContent uses a client with a timeout, so a hung file-service
// surfaces as an error rather than blocking forever.
func TestFetchDocumentContent_TimesOut(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release // block until the test releases it
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	defer close(release)

	// Swap in a short-timeout client for the duration of the test.
	orig := fileServiceHTTPClient
	fileServiceHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	defer func() { fileServiceHTTPClient = orig }()

	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, ts.URL, intent)

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "x", MimeType: "image/png"}},
		"",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch document")
	assert.Equal(t, 0, intent.sendMessageEventCalled)
}

// LOW(b) — info.size reflects the bytes actually uploaded, not the caller's
// (possibly stale/spoofed) att.Size.
func TestSendMessage_InfoSizeIsActualBytes(t *testing.T) {
	body := []byte("ACTUAL-BYTES") // 12 bytes
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/a")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$m"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		// att.Size deliberately disagrees with the real body length.
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png", Size: 999999}},
		"",
	)
	require.NoError(t, err)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(len(body)), info["size"], "info.size must be the uploaded byte count")
}

// M6 — a threaded media reply (SendReply + attachment) carries the thread
// relation on the media event: m.thread rel_type, event_id == threadID, and the
// m.in_reply_to fallback.
func TestSendReply_WithAttachment_CarriesThreadRelation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/m")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	const threadID = id.EventID("$thread-root")
	_, err := a.SendReply(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"", // attachment-only reply
		threadID,
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png"}},
		"",
	)
	require.NoError(t, err)

	require.Equal(t, 1, intent.sendMessageEventCalled, "only the media event (no text)")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)

	relates, ok := content["m.relates_to"].(map[string]any)
	require.True(t, ok, "media event must carry m.relates_to")
	assert.Equal(t, "m.thread", relates["rel_type"])
	assert.Equal(t, threadID.String(), relates["event_id"])
	assert.Equal(t, true, relates["is_falling_back"])

	inReplyTo, ok := relates["m.in_reply_to"].(map[string]any)
	require.True(t, ok, "thread relation must include the m.in_reply_to fallback")
	assert.Equal(t, threadID.String(), inReplyTo["event_id"])
}

// A video/audio MIME maps to the m.video / m.audio msgtype (outbound), both via
// the pure mapper and end-to-end through a send.
func TestMediaMsgType_VideoAudio(t *testing.T) {
	assert.Equal(t, "m.video", mediaMsgType("video/mp4"))
	assert.Equal(t, "m.audio", mediaMsgType("audio/mpeg"))
	assert.Equal(t, "m.image", mediaMsgType("image/gif"))
	assert.Equal(t, "m.file", mediaMsgType("application/zip"))
	assert.Equal(t, "m.file", mediaMsgType(""))
}

func TestSendMessage_VideoAttachment_MapsToVideo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("MP4"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/v")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$v"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "clip.mp4", MimeType: "video/mp4"}}, "")
	require.NoError(t, err)

	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.video", content["msgtype"])
}

// When the attachment carries no MIME, the file-service response Content-Type is
// used both for the upload and the derived msgtype/info.mimetype.
func TestSendMessage_MimeFallbackToResponseContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/ogg")
		_, _ = w.Write([]byte("OGG"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/au")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$au"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "voice", MimeType: ""}}, "")
	require.NoError(t, err)

	assert.Equal(t, "audio/ogg", intent.lastUploadBytesType, "upload uses the response Content-Type")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.audio", content["msgtype"], "msgtype derived from response Content-Type")
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "audio/ogg", info["mimetype"])
}

// Each attachment in a multi-attachment send gets its own event with a distinct
// body (filename) and io.alkemio.document_id.
func TestSendMessage_MultiAttachment_DistinctPerEvent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/x")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$x"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "first.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "second.png", MimeType: "image/png"},
		}, "")
	require.NoError(t, err)

	require.Len(t, intent.sendMsgEventContents, 2)
	c0, ok := intent.sendMsgEventContents[0].(map[string]any)
	require.True(t, ok)
	c1, ok := intent.sendMsgEventContents[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "first.png", c0["body"])
	assert.Equal(t, docID1, c0["io.alkemio.document_id"])
	assert.Equal(t, "second.png", c1["body"])
	assert.Equal(t, docID2, c1["io.alkemio.document_id"])
}

// A non-positive configured max attachment size falls back to the built-in
// default rather than capping reads at zero.
func TestMaxAttachmentBytes_NonPositiveFallsBackToDefault(t *testing.T) {
	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, "", intent)

	a.cfg.FileService.MaxAttachmentBytes = -1
	assert.Equal(t, config.DefaultMaxAttachmentBytes, a.cfg.MaxAttachmentBytes())

	a.cfg.FileService.MaxAttachmentBytes = 0
	assert.Equal(t, config.DefaultMaxAttachmentBytes, a.cfg.MaxAttachmentBytes())

	a.cfg.FileService.MaxAttachmentBytes = 1234
	assert.Equal(t, int64(1234), a.cfg.MaxAttachmentBytes())
}

// fetchDocumentContent rejects a non-UUID document id before any HTTP call.
func TestFetchDocumentContent_RejectsNonUUID(t *testing.T) {
	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, "http://file-service:4000", intent)

	_, _, err := a.fetchDocumentContent(context.Background(), "not-a-uuid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid document id")
}

// M1 — GetMessage on a media event returns the attachment (mxc→MediaID,
// io.alkemio.document_id→DocumentID for our own ghost), and does not surface the
// filename as message Content.
func TestGetMessage_MediaEvent_ReturnsAttachment(t *testing.T) {
	ghost := expectedUserID(testActorID) // @<uuid>:test.local — an own-appservice ghost
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$m1"),
			Sender:    ghost,
			Type:      event.EventMessage,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"msgtype":                "m.image",
					"body":                   "photo.jpg",
					"url":                    "mxc://test.local/media123",
					"info":                   map[string]any{"mimetype": "image/jpeg", "size": float64(9)},
					"io.alkemio.document_id": docID1,
				},
			},
		},
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$m1")
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Empty(t, msg.Content, "media filename must not be surfaced as Content")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "media123", msg.Attachments[0].MediaID)
	assert.Equal(t, docID1, msg.Attachments[0].DocumentID)
	assert.Equal(t, "photo.jpg", msg.Attachments[0].DisplayName)
}

// UploadMedia thin wrapper delegates to the bot client.
func TestUploadMedia(t *testing.T) {
	intent := &mockIntentAPI{
		uploadBytesResult: &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/up1")},
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	mxc, err := a.UploadMedia(context.Background(), []byte("DATA"), "image/png")
	require.NoError(t, err)
	assert.Equal(t, "mxc://test.local/up1", mxc.String())
	assert.Equal(t, "image/png", intent.lastUploadBytesType)
	assert.Equal(t, 1, intent.uploadBytesCalled)
}
