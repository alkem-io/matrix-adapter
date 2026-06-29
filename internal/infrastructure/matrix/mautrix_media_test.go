package matrix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
	assert.Equal(t, 0, intent.sendMessageEventCalled)
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
