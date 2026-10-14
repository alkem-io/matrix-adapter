package matrix

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// TestUploadRequestWireLength_MautrixContract drives a REAL mautrix.Client
// against a real net/http server and records what actually lands on the wire.
//
// It exists because Synapse rejects an upload with no Content-Length
// ("M_UNKNOWN (HTTP 400): Request must specify a Content-Length"), and whether
// one is sent is decided entirely inside mautrix's FullRequest.compileRequest —
// which treats ReqUploadMedia's two body branches DIFFERENTLY. This test:
//
//  1. proves uploadRequest picks a branch that genuinely emits
//     "Content-Length: 0" for a zero-byte document, and that the naive
//     Content+ContentLength:0 form does NOT (it goes out chunked);
//  2. pins mautrixWireContentLength — the model the intent mock uses — against
//     the library, so a mautrix upgrade that changes the mapping breaks HERE
//     rather than silently making the media mock lie.
func TestUploadRequestWireLength_MautrixContract(t *testing.T) {
	type wire struct {
		contentLength    string
		hasContentLength bool
		transferEncoding []string
		body             []byte
	}

	probe := func(t *testing.T, req mautrix.ReqUploadMedia) wire {
		t.Helper()
		var got wire
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got.contentLength = r.Header.Get("Content-Length")
			_, got.hasContentLength = r.Header["Content-Length"]
			got.transferEncoding = r.TransferEncoding
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			got.body = buf.Bytes()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content_uri":"mxc://test.local/uploaded"}`))
		}))
		defer srv.Close()

		cli, err := mautrix.NewClient(srv.URL, id.UserID("@u:test.local"), "token")
		require.NoError(t, err)
		_, err = cli.UploadMedia(context.Background(), req)
		require.NoError(t, err)
		return got
	}

	// The body reader must be the opaque type production actually passes. net/http
	// special-cases *bytes.Reader / *strings.Reader / *bytes.Buffer and derives a
	// length from them, which would mask the very mapping this test exists to pin.
	opaque := func(s string) io.Reader {
		return io.NopCloser(strings.NewReader(s))
	}

	t.Run("uploadRequest(0) sends a real Content-Length: 0", func(t *testing.T) {
		req := uploadRequest(opaque(""), 0, "application/octet-stream")
		got := probe(t, req)

		assert.True(t, got.hasContentLength,
			"a 0-byte upload MUST carry a Content-Length header; Synapse 400s without one")
		assert.Equal(t, "0", got.contentLength)
		assert.Empty(t, got.transferEncoding, "and must NOT be chunked")
		assert.Empty(t, got.body)
		assert.Equal(t, int64(0), mautrixWireContentLength(req),
			"the mock's model must agree with the wire")
	})

	t.Run("the naive Content+ContentLength:0 form is chunked with NO Content-Length", func(t *testing.T) {
		// This is the shape uploadRequest deliberately avoids. Kept as an
		// executable statement of WHY: if a future mautrix honours
		// RequestLength == 0 this subtest fails and the special case can go.
		req := mautrix.ReqUploadMedia{
			Content:       opaque(""),
			ContentLength: 0,
			ContentType:   "application/octet-stream",
		}
		got := probe(t, req)

		assert.False(t, got.hasContentLength,
			"mautrix's RequestBody branch only honours a length > 0")
		assert.Equal(t, []string{"chunked"}, got.transferEncoding)
		assert.Equal(t, int64(-1), mautrixWireContentLength(req),
			"the mock's model must agree with the wire")
	})

	t.Run("a non-empty body streams with its declared Content-Length", func(t *testing.T) {
		body := []byte("STREAMED")
		req := uploadRequest(opaque(string(body)), int64(len(body)), "image/png")
		require.NotNil(t, req.Content, "a real payload must STREAM, never be buffered into ContentBytes")
		require.Nil(t, req.ContentBytes)

		got := probe(t, req)

		assert.True(t, got.hasContentLength)
		assert.Equal(t, "8", got.contentLength)
		assert.Empty(t, got.transferEncoding)
		assert.Equal(t, body, got.body)
		assert.Equal(t, int64(len(body)), mautrixWireContentLength(req),
			"the mock's model must agree with the wire")
	})
}

type realUploadIntent struct {
	*mockIntentAPI
	client *mautrix.Client
}

func (i *realUploadIntent) UploadMedia(ctx context.Context, request mautrix.ReqUploadMedia) (*mautrix.RespMediaUpload, error) {
	return i.client.UploadMedia(ctx, request)
}

func TestUploadMedia_WireContractAndNoReplay(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		for _, body := range []string{"", "original bytes"} {
			t.Run(http.StatusText(status)+body, func(t *testing.T) {
				var calls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, "résumé + photo.png", r.URL.Query().Get("filename"))
					require.Equal(t, int64(len(body)), r.ContentLength)
					require.Empty(t, r.TransferEncoding)
					require.Equal(t, "image/png", r.Header.Get("Content-Type"))
					actual, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Equal(t, body, string(actual))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"content_uri":"mxc://test.local/uploaded"}`))
				}))
				defer srv.Close()
				client, err := mautrix.NewClient(srv.URL, expectedUserID(testActorID), "fixture-token")
				require.NoError(t, err)
				client.DefaultHTTPRetries = 3
				intent := &realUploadIntent{mockIntentAPI: &mockIntentAPI{}, client: client}
				a := newMediaTestAdapterWithIntent(t, intent)
				_, err = a.UploadMedia(context.Background(), testActor(testActorID, ""), io.NopCloser(strings.NewReader(body)), int64(len(body)), "image/png", "résumé + photo.png")
				if status == http.StatusOK {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				require.Equal(t, int32(1), calls.Load(), "a failed streamed upload must not be replayed")
				require.Zero(t, intent.sendMessageEventCalled)
			})
		}
	}
}

func TestUploadMedia_RejectsUnknownOrOversizedLengthBeforeUpload(t *testing.T) {
	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, intent)
	for _, size := range []int64{-1, a.cfg.MaxAttachmentBytes() + 1} {
		_, err := a.UploadMedia(context.Background(), testActor(testActorID, ""), strings.NewReader(""), size, "image/png", "file.png")
		require.ErrorIs(t, err, domain.ErrInvalidParam)
	}
	require.Zero(t, intent.uploadBytesCalled)
}

func TestSendReply_RejectsInvalidExistingMediaBeforeIntent(t *testing.T) {
	for _, att := range []domain.Attachment{
		{DisplayName: "file", MimeType: "image/png", Size: 1},
		{MediaID: "mxc://foreign.test/id", DisplayName: "file", MimeType: "image/png", Size: 1},
		{MediaID: "id", DisplayName: "file", MimeType: "image/png", Size: -1},
	} {
		intent := &mockIntentAPI{}
		a := newMediaTestAdapter(t, intent)
		_, err := a.SendReply(context.Background(), "!room:test.local", testActor(testActorID, ""), "", "$parent", []domain.Attachment{att})
		require.ErrorIs(t, err, domain.ErrInvalidParam)
		require.Zero(t, intent.sendMessageEventCalled)
		require.Zero(t, intent.uploadBytesCalled)
	}
}

func TestUploadMedia_UsesSenderWithoutPublishing(t *testing.T) {
	for _, body := range []string{"", "processed bytes"} {
		t.Run(body, func(t *testing.T) {
			bot := &mockIntentAPI{}
			sender := &mockIntentAPI{uploadBytesResult: &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/staged")}}
			a := newMediaTestAdapter(t, sender)
			a.as = newMockAS(bot, map[id.UserID]intentAPI{expectedUserID(testActorID): sender})
			uri, err := a.UploadMedia(context.Background(), testActor(testActorID, ""), strings.NewReader(body), int64(len(body)), "image/png", "résumé.png")
			require.NoError(t, err)
			require.Equal(t, "staged", uri.FileID)
			require.Equal(t, 1, sender.uploadBytesCalled)
			require.Equal(t, int64(len(body)), sender.lastUploadWireContentLength)
			require.Equal(t, body, string(sender.lastUploadBytesData))
			require.Zero(t, bot.uploadBytesCalled)
			require.Zero(t, sender.sendMessageEventCalled)
			require.Zero(t, sender.sendTextCalled)
		})
	}
}

func TestSendReply_ExistingMediaNeverTransfersBytes(t *testing.T) {
	sender := &mockIntentAPI{sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$sent"}}
	a := newMediaTestAdapter(t, sender)
	message, err := a.SendReply(context.Background(), "!room:test.local", testActor(testActorID, ""), "", "$parent", []domain.Attachment{{
		MediaID: "existing", DisplayName: "résumé.png", MimeType: "image/png", Size: 42,
	}})
	require.NoError(t, err)
	require.Equal(t, "existing", message.Attachments[0].MediaID)
	require.Zero(t, sender.uploadBytesCalled)
	require.Equal(t, 1, sender.sendMessageEventCalled)
	content := sender.lastSendMsgEventContent.(map[string]any)
	require.NotContains(t, content, "io.alkemio.document_id")
	require.Equal(t, "mxc://test.local/existing", content["url"])
	require.Equal(t, threadRelation("$parent"), content["m.relates_to"])
}

func TestUploadMedia_CancellationReachesUpstream(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer srv.Close()
	client, err := mautrix.NewClient(srv.URL, expectedUserID(testActorID), "fixture-token")
	require.NoError(t, err)
	client.DefaultHTTPRetries = 3
	intent := &realUploadIntent{mockIntentAPI: &mockIntentAPI{}, client: client}
	a := newMediaTestAdapterWithIntent(t, intent)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := a.UploadMedia(ctx, testActor(testActorID, ""), strings.NewReader("data"), 4, "image/png", "file.png")
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upload never reached upstream")
	}
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("upload ignored cancellation")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream request stayed open")
	}
	require.Equal(t, int32(1), calls.Load())
	require.Zero(t, intent.sendMessageEventCalled)
}
