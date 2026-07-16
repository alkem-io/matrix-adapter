package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

func TestSendMessage_AttachmentFailureRollsBackPriorFanOutEvents(t *testing.T) {
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

	_, err := a.SendMessage(
		context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "files",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "one.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "two.png", MimeType: "image/png"},
			{DocumentID: docID3, DisplayName: "three.png", MimeType: "image/png"},
		},
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "attachment 2 of 3 failed")
	assert.Equal(t, 2, intent.sendMessageEventCalled, "the third attachment must not be attempted")
	assert.Equal(t, []id.EventID{"$text", "$attachment-1"}, intent.redactEventIDs)
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
	assert.Equal(t, "$text1", content[attachmentParentEventIDField])
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
	assert.NotContains(t, c0, attachmentParentEventIDField, "the first attachment is the primary event")
	assert.Equal(t, "second.png", c1["body"])
	assert.Equal(t, docID2, c1["io.alkemio.document_id"])
	assert.Equal(t, "$x", c1[attachmentParentEventIDField])
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
