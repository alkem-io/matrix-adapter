package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
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

// Valid UUID document ids used by the file-service fetch path.
const (
	docID1 = "11111111-1111-4111-8111-111111111111"
	docID2 = "22222222-2222-4222-8222-222222222222"
	docID3 = "33333333-3333-4333-8333-333333333333"
)

// A multi-attachment send is a fan-out of independent events, exactly like an
// Element multi-image send. When something has ALREADY been delivered (here the
// text event), a later attachment failure is a PARTIAL success: the delivered
// primary is returned with a nil error (no rollback), so the sender's text does
// not vanish and a server retry does not duplicate it. Matrix/Element provide no
// cross-event atomicity, so the adapter must not reinvent it.
func TestSendMessage_AttachmentFailure_WithText_ReturnsPrimaryNoError(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})
	intent := &mockIntentAPI{
		sendTextResult:    &mautrix.RespSendEvent{EventID: "$text"},
		uploadBytesResult: &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/media")},
		sendMessageEventResults: []*mautrix.RespSendEvent{
			{EventID: "$attachment-1"},
			nil,
		},
		sendMessageEventErrs: []error{nil, assert.AnError},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	eventID, err := a.SendMessage(
		context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "files",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "one.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "two.png", MimeType: "image/png"},
			{DocumentID: docID3, DisplayName: "three.png", MimeType: "image/png"},
		},
	)

	require.NoError(t, err, "partial failure with an already-delivered primary must not surface an error")
	assert.Equal(t, id.EventID("$text"), eventID, "the already-delivered text event is returned as the primary")
	assert.Equal(t, 2, intent.sendMessageEventCalled, "the third attachment must not be attempted (stop on first failure)")
	assert.Empty(t, intent.redactEventIDs, "prior events are NOT rolled back (Element parity)")
}

// A text-less message whose FIRST attachment fails has delivered nothing, so the
// error IS surfaced (primaryEventID == "") — the caller must retry the whole
// send.
func TestSendMessage_FirstAttachmentFailure_NoText_ReturnsError(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})
	intent := &mockIntentAPI{
		uploadBytesResult:       &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/media")},
		sendMessageEventResults: []*mautrix.RespSendEvent{nil},
		sendMessageEventErrs:    []error{assert.AnError},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	eventID, err := a.SendMessage(
		context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "one.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "two.png", MimeType: "image/png"},
		},
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "attachment 1 of 2 failed")
	assert.Empty(t, eventID, "nothing delivered, so no primary event id")
	assert.Equal(t, 1, intent.sendMessageEventCalled, "the second attachment must not be attempted")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func stubFileService(t *testing.T, fn roundTripFunc) string {
	t.Helper()
	original := fileServiceHTTPClient
	fileServiceHTTPClient = &http.Client{Transport: fn}
	t.Cleanup(func() { fileServiceHTTPClient = original })
	return "http://file-service.test"
}

func fileServiceResponse(status int, contentType string, body []byte) *http.Response {
	header := make(http.Header)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{
		StatusCode:    status,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

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
	fileServiceURL := stubFileService(t, func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return fileServiceResponse(http.StatusOK, "image/jpeg", []byte("JPEGBYTES")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/abc123")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media1"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

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

// Media must be uploaded by the SENDER ghost, not the appservice bot, so blob
// ownership/quota/retention attribute to the acting user (a bot-account purge
// must not strip every bridged blob).
func TestSendMessage_AttachmentUploadedBySenderNotBot(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})
	botIntent := &mockIntentAPI{}
	sender := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/s")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$m"},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{expectedUserID(testActorID): sender})
	a := newFullTestAdapter(as, &mockAdminAPI{})
	a.cfg = &config.Config{}
	a.cfg.FileService.URL = fileServiceURL

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "s.png", MimeType: "image/png"}})
	require.NoError(t, err)

	assert.Equal(t, 1, sender.uploadBytesCalled, "upload must go through the sender ghost intent")
	assert.Equal(t, 0, botIntent.uploadBytesCalled, "the appservice bot must NOT upload the media")
}

// Text + attachment → 1 m.text event + 1 media event; the returned event ID is
// the text event.
func TestSendMessage_TextPlusAttachment(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "application/pdf", []byte("PDF")), nil
	})

	intent := &mockIntentAPI{
		sendTextResult:         &mautrix.RespSendEvent{EventID: "$text1"},
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/file9")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media1"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

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
	)
	require.NoError(t, err)
	assert.Equal(t, id.EventID("$text1"), eventID, "primary event is the text event")
	assert.Equal(t, 1, intent.sendTextCalled)
	require.Equal(t, 1, intent.sendMessageEventCalled)

	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"], "non-image MIME maps to m.file")
	assert.Equal(t, docID1, content["io.alkemio.document_id"])
}

// A non-OK response from file-service surfaces as an error and no event is sent.
func TestSendMessage_AttachmentFetchError(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusNotFound, "", nil), nil
	})

	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "x", MimeType: "image/png"}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
	assert.Equal(t, 0, intent.sendMessageEventCalled)
}

func TestSendMessage_AttachmentWithoutFileServiceFailsClearly(t *testing.T) {
	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, "", intent)

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "x", MimeType: "image/png"}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file-service URL not configured")
	assert.Equal(t, 0, intent.uploadBytesCalled)
	assert.Equal(t, 0, intent.sendMessageEventCalled)
}

// Text uses develop's SendText helper and media sends omit an explicit
// transaction ID, leaving the SDK to mint a fresh random transaction per event.
func TestSendMessage_UsesSDKGeneratedTransactions(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		sendTextResult:         &mautrix.RespSendEvent{EventID: "$text"},
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/a")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)
	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"caption",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png"}},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendTextCalled)
	require.Equal(t, []string{""}, intent.sendMsgEventTxns)
}

// A declared Content-Length over the cap is rejected before UploadMedia starts.
func TestSendAttachment_RejectsDeclaredOversizeBeforeUpload(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "", make([]byte, 100)), nil
	})

	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, fileServiceURL, intent)
	a.cfg.FileService.MaxAttachmentBytes = 10 // cap below the 100-byte body

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "big.bin", MimeType: "application/octet-stream"}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds max attachment size")
	assert.Equal(t, 0, intent.uploadBytesCalled, "declared oversize must not start UploadMedia")
	assert.Equal(t, 0, intent.sendMessageEventCalled, "no event sent when fetch is rejected")
}

// Response-header wait remains bounded without applying a Client.Timeout to the
// streamed response body.
func TestSendAttachment_ResponseHeaderTimesOut(t *testing.T) {
	original := fileServiceHTTPClient
	fileServiceHTTPClient = newFileServiceHTTPClient(50 * time.Millisecond)
	t.Cleanup(func() { fileServiceHTTPClient = original })
	assert.Zero(t, fileServiceHTTPClient.Timeout, "streamed body must not have a whole-request timeout")

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	transport, ok := fileServiceHTTPClient.Transport.(*http.Transport)
	require.True(t, ok)
	transport.Proxy = nil
	transport.DialContext = func(context.Context, string, string) (net.Conn, error) {
		return clientConn, nil
	}
	// Drain the request but deliberately never send response headers.
	go func() { _, _ = io.Copy(io.Discard, serverConn) }()

	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, "http://file-service.test", intent)

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "x", MimeType: "image/png"}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch document")
	assert.Equal(t, 0, intent.sendMessageEventCalled)
}

// ctxBlockingReader blocks Read until the request context is cancelled, then
// returns that error — modelling a file-service that streams headers then stalls
// mid-body. Its unblocking proves the size-proportional deadline (mediaCtx)
// bounds the streamed body read, not just the header wait.
type ctxBlockingReader struct{ ctx context.Context }

func (r *ctxBlockingReader) Read(_ []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r *ctxBlockingReader) Close() error { return nil }

// A mid-body stall from file-service must NOT hang the send forever (it would
// wedge the single sequential watermill consumer). sendAttachment bounds the
// fetch+upload with mediaStreamTimeout via the fetch context; a short parent
// deadline is inherited (WithTimeout takes the EARLIER deadline), so the stalled
// body read is cancelled quickly rather than blocking for the 60s+ floor.
func TestSendAttachment_BodyStall_DeadlineCancels(t *testing.T) {
	fileServiceURL := stubFileService(t, func(req *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Type", "image/png")
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        header,
			Body:          &ctxBlockingReader{ctx: req.Context()},
			ContentLength: -1,
		}, nil
	})
	intent := &mockIntentAPI{
		uploadBytesResult: &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/media")},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	var err error
	go func() {
		_, err = a.SendMessage(
			ctx, "!room:test.local", testActor(testActorID, "Alice"), "",
			[]domain.Attachment{{DocumentID: docID1, DisplayName: "x", MimeType: "image/png"}},
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sendAttachment hung past the media deadline; the streamed body read was not bounded")
	}
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 0, intent.sendMessageEventCalled, "no media event on a deadline-cancelled fetch")
}

// mediaStreamTimeout is the floor (fileServiceFetchTimeout) for small payloads
// and scales up proportionally with the per-attachment cap for large ones.
func TestMediaStreamTimeout_ScalesWithSize(t *testing.T) {
	assert.Equal(t, fileServiceFetchTimeout, mediaStreamTimeout(0), "zero-size stays at the floor")
	assert.Equal(t, fileServiceFetchTimeout, mediaStreamTimeout(fileServiceMinThroughputBytesPerSec-1),
		"below one throughput-second stays at the floor")

	big := int64(100) * fileServiceMinThroughputBytesPerSec // 100 MiB
	got := mediaStreamTimeout(big)
	assert.Equal(t, fileServiceFetchTimeout+100*time.Second, got, "100 MiB adds 100s at 1 MiB/s")
	assert.Greater(t, got, fileServiceFetchTimeout)

	// Absurd config must not overflow int64-ns and wrap negative: the result is
	// clamped to the floor + a 1h ceiling, and stays positive.
	clamped := mediaStreamTimeout(math.MaxInt64)
	assert.Equal(t, fileServiceFetchTimeout+3600*time.Second, clamped, "clamped to floor + 1h ceiling")
	assert.Positive(t, clamped, "clamped duration must stay positive, never wrap negative")
}

// LOW(b) — info.size reflects the bytes actually uploaded, not the caller's
// (possibly stale/spoofed) att.Size.
func TestSendMessage_InfoSizeIsActualBytes(t *testing.T) {
	body := []byte("ACTUAL-BYTES") // 12 bytes
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", body), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/a")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$m"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"",
		// att.Size deliberately disagrees with the real body length.
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png", Size: 999999}},
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
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/m")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$media"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	const threadID = id.EventID("$thread-root")
	_, err := a.SendReply(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"", // attachment-only reply
		threadID,
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "a.png", MimeType: "image/png"}},
	)
	require.NoError(t, err)

	require.Equal(t, 1, intent.sendMessageEventCalled, "only the media event (no text)")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)

	relates, ok := content["m.relates_to"].(*event.RelatesTo)
	require.True(t, ok, "media event must carry a typed m.relates_to")
	assert.Equal(t, event.RelThread, relates.Type)
	assert.Equal(t, threadID, relates.EventID)
	assert.True(t, relates.IsFallingBack)
	require.NotNil(t, relates.InReplyTo)
	assert.Equal(t, threadID, relates.InReplyTo.EventID)

	relationJSON, err := json.Marshal(relates)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"rel_type":"m.thread",
		"event_id":"$thread-root",
		"m.in_reply_to":{"event_id":"$thread-root"},
		"is_falling_back":true
	}`, string(relationJSON), "typed relation must preserve the existing wire shape")
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
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "video/mp4", []byte("MP4")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/v")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$v"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "clip.mp4", MimeType: "video/mp4"}})
	require.NoError(t, err)

	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.video", content["msgtype"])
}

// When the attachment carries no MIME, the file-service response Content-Type is
// used both for the upload and the derived msgtype/info.mimetype.
func TestSendMessage_MimeFallbackToResponseContentType(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "audio/ogg", []byte("OGG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/au")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$au"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "voice", MimeType: ""}})
	require.NoError(t, err)

	assert.Equal(t, "audio/ogg", intent.lastUploadBytesType, "upload uses the response Content-Type")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.audio", content["msgtype"], "msgtype derived from response Content-Type")
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "audio/ogg", info["mimetype"])
}

// The file-service response header is authoritative for both upload metadata
// and Matrix event classification, even when the attachment hint disagrees.
func TestSendMessage_ResponseContentTypeWinsMimeDisagreement(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/png")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$png"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "image", MimeType: "application/octet-stream"}})
	require.NoError(t, err)

	assert.Equal(t, "image/png", intent.lastUploadBytesType)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.image", content["msgtype"])
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image/png", info["mimetype"])
}

// The server-declared att.MimeType is authoritative when specific: a generic
// file-service Content-Type (application/octet-stream) must NOT override a
// precise att.MimeType, so the media still renders inline (m.image), not m.file.
func TestSendMessage_SpecificAttMimeBeatsGenericResponseType(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "application/octet-stream", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/png")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$png"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "image", MimeType: "image/png"}})
	require.NoError(t, err)

	assert.Equal(t, "image/png", intent.lastUploadBytesType)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.image", content["msgtype"], "specific att.MimeType must win over generic octet-stream")
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image/png", info["mimetype"])
}

// A mixed-/upper-case file-service Content-Type must be lowercased so the
// derived msgtype classifies as inline media (m.image) rather than m.file, and
// info.mimetype/upload Content-Type are stored lowercased. Guards [0].
func TestSendMessage_MixedCaseResponseContentTypeLowercased(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "Image/JPEG", []byte("JPG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/jpg")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$jpg"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "image", MimeType: ""}})
	require.NoError(t, err)

	assert.Equal(t, "image/jpeg", intent.lastUploadBytesType, "upload Content-Type lowercased")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.image", content["msgtype"], "mixed-case image/* still classifies as m.image")
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image/jpeg", info["mimetype"])
}

// A parameterized Content-Type keeps its parameters (charset) ONLY on the upload
// Content-Type — where charset must survive for text — but the event's
// info.mimetype and msgtype get the BARE type (params stripped), matching Matrix's
// convention that FileInfo.mimetype is an unparameterized type/subtype. Guards [8].
func TestSendMessage_ParameterizedTypeBareInEventFullOnUpload(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "text/plain; charset=utf-8", []byte("hi")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/txt")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$txt"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "note.txt", MimeType: "application/octet-stream"}})
	require.NoError(t, err)

	assert.Equal(t, "text/plain; charset=utf-8", intent.lastUploadBytesType, "charset preserved on upload Content-Type")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"], "text/* classifies as m.file")
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "text/plain", info["mimetype"], "info.mimetype is the bare type, charset stripped")
}

// A malformed/params-only att.MimeType AND response Content-Type must not leak
// into upload metadata; the resolver collapses to application/octet-stream.
// Guards [1].
func TestSendMessage_MalformedTypesCollapseToOctetStream(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "; charset=utf-8", []byte("x")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/bin")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$bin"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "blob", MimeType: "; charset=x"}})
	require.NoError(t, err)

	assert.Equal(t, "application/octet-stream", intent.lastUploadBytesType, "malformed types collapse to octet-stream")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"])
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "application/octet-stream", info["mimetype"])
}

// A malformed PARAMETER must not discard an otherwise usable base type: the
// resolver recovers "image/png" so the media still renders inline (m.image)
// rather than collapsing to m.file/octet-stream. Guards the parameter-recovery
// path.
func TestSendMessage_MalformedParamRecoversBaseType(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/png")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$png"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "image", MimeType: "image/png; =bad"}})
	require.NoError(t, err)

	assert.Equal(t, "image/png", intent.lastUploadBytesType, "base type recovered from malformed param")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.image", content["msgtype"], "recovered image/png classifies as m.image")
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image/png", info["mimetype"])
}

// A subtype-less type ("image") is not a valid media type: mediaMsgType would
// classify it m.file yet it would otherwise leak into upload Content-Type and
// info.mimetype. It must be rejected and collapse to application/octet-stream.
// Guards the subtype-less rejection path.
func TestSendMessage_SubtypeLessTypeRejected(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "", []byte("x")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/bin")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$bin"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "blob", MimeType: "image"}})
	require.NoError(t, err)

	assert.Equal(t, "application/octet-stream", intent.lastUploadBytesType, "subtype-less type rejected")
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"])
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "application/octet-stream", info["mimetype"])
}

// A malformed BASE type (internal space) is NOT recoverable: mime.ParseMediaType
// rejects "image/png bad", and the recovery path must re-validate the base
// rather than leak it. It collapses to application/octet-stream, not m.image.
func TestSendMessage_MalformedBaseTypeSpaceRejected(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "", []byte("x")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/bin")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$bin"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "blob", MimeType: "image/png bad"}})
	require.NoError(t, err)

	assert.Equal(t, "application/octet-stream", intent.lastUploadBytesType)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"])
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "application/octet-stream", info["mimetype"])
}

// A control character in the base type would, if leaked into the UploadMedia
// Content-Type header, be rejected by net/http and HARD-FAIL the send. The
// resolver must collapse it to application/octet-stream so the upload succeeds.
// This is the header-injection regression guard: assert err == nil.
func TestSendMessage_ControlCharBaseTypeCollapsesAndSendSucceeds(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "", []byte("x")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/bin")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$bin"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "blob", MimeType: "image/png\x00x"}})
	require.NoError(t, err, "control-char type must not reach the upload header and hard-fail the send")

	assert.Equal(t, "application/octet-stream", intent.lastUploadBytesType)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "application/octet-stream", info["mimetype"])
}

// An extra path segment ("image/png/x") is a malformed base that ParseMediaType
// rejects; it must not leak and collapses to application/octet-stream.
func TestSendMessage_ExtraSegmentBaseTypeRejected(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "", []byte("x")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/bin")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$bin"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "blob", MimeType: "image/png/x"}})
	require.NoError(t, err)

	assert.Equal(t, "application/octet-stream", intent.lastUploadBytesType)
	content, ok := intent.lastSendMsgEventContent.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "m.file", content["msgtype"])
	info, ok := content["info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "application/octet-stream", info["mimetype"])
}

// Each attachment in a multi-attachment send gets its own event with a distinct
// body (filename) and io.alkemio.document_id.
func TestSendMessage_MultiAttachment_DistinctPerEvent(t *testing.T) {
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})

	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/x")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$x"},
	}
	a := newMediaTestAdapter(t, fileServiceURL, intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "first.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "second.png", MimeType: "image/png"},
		})
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

// The real streaming send path rejects a non-UUID document id before any HTTP call.
func TestSendAttachment_RejectsNonUUID(t *testing.T) {
	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, "http://file-service:4000", intent)

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
		[]domain.Attachment{{DocumentID: "not-a-uuid", DisplayName: "bad.bin"}})
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
	assert.Empty(t, msg.Content, "legacy media filename must not be duplicated as Content")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "media123", msg.Attachments[0].MediaID)
	assert.Equal(t, docID1, msg.Attachments[0].DocumentID)
	assert.Equal(t, "photo.jpg", msg.Attachments[0].DisplayName)
}

// Regression: GetMessage on an m.sticker event id returns the media message WITH
// its attachment (previously errored "event is not a message" because
// parseMessageEvent rejected any non-m.room.message type). A sticker surfaces
// image media (MediaID = mxc id, DisplayName = alt-text) with empty Content.
func TestGetMessage_Sticker_ReturnsAttachment(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$sticker1"),
			Sender:    id.UserID("@element:test.local"), // a normal Element user
			Type:      event.EventSticker,               // no msgtype field on stickers
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"body": "party parrot",
					"url":  "mxc://test.local/stickerid",
					"info": map[string]any{"mimetype": "image/png", "w": float64(128), "h": float64(128)},
				},
			},
		},
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$sticker1")
	require.NoError(t, err, "GetMessage on a sticker must not error (regression)")
	require.NotNil(t, msg)
	assert.Empty(t, msg.Content, "sticker alt-text must not be duplicated as Content")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "stickerid", msg.Attachments[0].MediaID, "MediaID is the re-home key")
	assert.Equal(t, "party parrot", msg.Attachments[0].DisplayName)
	assert.Equal(t, "image/png", msg.Attachments[0].MimeType)
}

// A sticker with a present-but-non-string body and no usable media is malformed:
// malformedMessageBody now covers m.sticker (not just m.room.message), so
// GetMessage errors "event is not a message" instead of returning a blank message.
func TestGetMessage_Sticker_MalformedBody_Errors(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$badsticker"),
			Sender:    id.UserID("@element:test.local"),
			Type:      event.EventSticker,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"body": map[string]any{"not": "a string"}, // present but non-string
					// no url → no attachment
				},
			},
		},
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$badsticker")
	require.Error(t, err, "a malformed-body sticker with no media must error, not return a blank message")
	assert.Nil(t, msg)
}

// A sticker in the unread window counts toward the unread total, exactly like an
// m.room.message: countUnreadMessages uses isMessageLikeEvent. A mixed batch of a
// text message and a sticker (both from another user) counts 2; before stickers
// were message-like the sticker would have been skipped.
func TestCountUnreadMessages_CountsStickers(t *testing.T) {
	other := id.UserID("@element:test.local")
	me := expectedUserID(testActorID)
	intent := &mockIntentAPI{
		messagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{Type: event.EventMessage, ID: "$t1", Sender: other, Content: event.Content{
					Raw: map[string]any{"msgtype": "m.text", "body": "hi"},
				}},
				{Type: event.EventSticker, ID: "$s1", Sender: other, Content: event.Content{
					Raw: map[string]any{"body": "party parrot", "url": "mxc://test.local/stickerid"},
				}},
			},
			End: "", // no more events → scan stops after this batch
		},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	count, ok := a.countUnreadMessages(context.Background(), intent, "!room:test.local", nil, me)
	require.True(t, ok, "scan completes (all events seen)")
	assert.Equal(t, 2, count, "both the text message and the sticker count as unread")
}

// A sticker from the querying user themself is NOT counted (self-authored), same
// as a self-authored m.room.message.
func TestCountUnreadMessages_SkipsOwnSticker(t *testing.T) {
	me := expectedUserID(testActorID)
	intent := &mockIntentAPI{
		messagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{Type: event.EventSticker, ID: "$s1", Sender: me, Content: event.Content{
					Raw: map[string]any{"body": "wave", "url": "mxc://test.local/x"},
				}},
			},
			End: "",
		},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	count, ok := a.countUnreadMessages(context.Background(), intent, "!room:test.local", nil, me)
	require.True(t, ok)
	assert.Equal(t, 0, count, "a self-authored sticker is not unread")
}

// Streaming upload: a body within the cap streams straight through to Synapse
// (no whole-file buffering) and the media event's info.size is the streamed byte
// count; a body larger than the cap fails with the oversize error and sends no
// media event, so an oversized document never gets buffered whole in memory.
func TestSendAttachment_StreamsWithinCapAndRejectsOversize(t *testing.T) {
	t.Run("in-cap body streams through with streamed size", func(t *testing.T) {
		body := []byte("STREAMED-BODY") // 13 bytes
		fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
			return fileServiceResponse(http.StatusOK, "image/png", body), nil
		})

		intent := &mockIntentAPI{
			uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/s")},
			sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$s"},
		}
		a := newMediaTestAdapter(t, fileServiceURL, intent)
		a.cfg.FileService.MaxAttachmentBytes = 1024 // well above the body

		_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
			[]domain.Attachment{{DocumentID: docID1, DisplayName: "s.png", MimeType: "image/png", Size: 999}})
		require.NoError(t, err)
		require.Equal(t, 1, intent.uploadBytesCalled)
		assert.Equal(t, body, intent.lastUploadBytesData, "the whole body streamed through to the upload")
		require.Equal(t, 1, intent.sendMessageEventCalled)
		content, ok := intent.lastSendMsgEventContent.(map[string]any)
		require.True(t, ok)
		info, ok := content["info"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, int64(len(body)), info["size"], "info.size is the streamed byte count")
	})

	t.Run("misreported (too-large) Content-Length still uploads (streamed length)", func(t *testing.T) {
		body := []byte("SHORT") // 5 bytes actually served
		fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
			resp := fileServiceResponse(http.StatusOK, "image/png", body)
			resp.ContentLength = 999 // declares far more than it serves (in-cap)
			return resp, nil
		})

		intent := &mockIntentAPI{
			uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/s")},
			sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$s"},
		}
		a := newMediaTestAdapter(t, fileServiceURL, intent)
		a.cfg.FileService.MaxAttachmentBytes = 1024

		_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
			[]domain.Attachment{{DocumentID: docID1, DisplayName: "s.png", MimeType: "image/png"}})
		require.NoError(t, err, "a short body under a larger declared length must still upload")
		require.Equal(t, 1, intent.uploadBytesCalled)
		assert.Equal(t, int64(-1), intent.lastUploadContentLength,
			"upload streams with unknown length, never the declared Content-Length")
		assert.Equal(t, body, intent.lastUploadBytesData)
	})

	t.Run("oversize body fails and sends no event", func(t *testing.T) {
		fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
			resp := fileServiceResponse(http.StatusOK, "application/octet-stream", make([]byte, 100))
			resp.ContentLength = -1 // exercise the streaming cap backstop
			return resp, nil
		})

		intent := &mockIntentAPI{
			sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$never"},
		}
		a := newMediaTestAdapter(t, fileServiceURL, intent)
		a.cfg.FileService.MaxAttachmentBytes = 10 // below the 100-byte body

		_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "",
			[]domain.Attachment{{DocumentID: docID1, DisplayName: "big.bin", MimeType: "application/octet-stream"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds max attachment size")
		assert.Equal(t, 0, intent.sendMessageEventCalled, "no media event when the cap is exceeded")
	})
}
