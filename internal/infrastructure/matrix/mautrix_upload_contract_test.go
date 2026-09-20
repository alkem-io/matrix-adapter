package matrix

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
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
	opaque := func(s string) *countingCapReader {
		return &countingCapReader{r: strings.NewReader(s), max: 1 << 20}
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
