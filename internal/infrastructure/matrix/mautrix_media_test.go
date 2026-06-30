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
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
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
			DocumentID:  "doc-123",
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
	assert.Equal(t, "/internal/file/doc-123/content", gotPath)

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
	assert.Equal(t, "doc-123", content["io.alkemio.document_id"])

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
		sendTextResult:         &mautrix.RespSendEvent{EventID: "$text1"},
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/file9")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media9"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	eventID, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"see attached",
		[]domain.Attachment{{
			DocumentID:  "doc-9",
			DisplayName: "report.pdf",
			MimeType:    "application/pdf",
			Size:        3,
		}},
		"", // no idempotency key
	)
	require.NoError(t, err)
	assert.Equal(t, id.EventID("$text1"), eventID, "primary event is the text event")
	assert.Equal(t, 1, intent.sendTextCalled)
	require.Equal(t, 1, intent.sendMessageEventCalled)

	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"], "non-image MIME maps to m.file")
	assert.Equal(t, "doc-9", content["io.alkemio.document_id"])
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
		[]domain.Attachment{{DocumentID: "missing", DisplayName: "x", MimeType: "image/png"}},
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
				{DocumentID: "doc-1", DisplayName: "a.png", MimeType: "image/png"},
				{DocumentID: "doc-2", DisplayName: "b.png", MimeType: "image/png"},
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

// Without an idempotency key, behaviour is unchanged: text uses SendText and no
// deterministic transaction IDs are attached to attachment events.
func TestSendMessage_NoIdempotencyKey_NoTxnIDs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		sendTextResult:         &mautrix.RespSendEvent{EventID: "$text"},
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/a")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media"},
	}
	a := newMediaTestAdapter(t, ts.URL, intent)
	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"caption",
		[]domain.Attachment{{DocumentID: "doc-1", DisplayName: "a.png", MimeType: "image/png"}},
		"", // no key
	)
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendTextCalled, "text still uses SendText without a key")
	// only the single attachment event went through SendMessageEvent, with no txn.
	require.Equal(t, []string{""}, intent.sendMsgEventTxns)
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
		[]domain.Attachment{{DocumentID: "big", DisplayName: "big.bin", MimeType: "application/octet-stream"}},
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
		[]domain.Attachment{{DocumentID: "slow", DisplayName: "x", MimeType: "image/png"}},
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
		[]domain.Attachment{{DocumentID: "doc", DisplayName: "a.png", MimeType: "image/png", Size: 999999}},
		"",
	)
	require.NoError(t, err)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(len(body)), info["size"], "info.size must be the uploaded byte count")
}

// UploadMedia / DownloadMedia thin wrappers delegate to the bot client.
func TestUploadAndDownloadMedia(t *testing.T) {
	intent := &mockIntentAPI{
		uploadBytesResult:   &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/up1")},
		downloadBytesResult: []byte("DATA"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	mxc, err := a.UploadMedia(context.Background(), []byte("DATA"), "image/png")
	require.NoError(t, err)
	assert.Equal(t, "mxc://test.local/up1", mxc.String())
	assert.Equal(t, "image/png", intent.lastUploadBytesType)

	data, err := a.DownloadMedia(context.Background(), id.MustParseContentURI("mxc://test.local/up1"))
	require.NoError(t, err)
	assert.Equal(t, []byte("DATA"), data)
	assert.Equal(t, 1, intent.downloadBytesCalled)
}
