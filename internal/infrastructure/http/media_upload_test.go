package httpinfra

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

type uploadFunc func(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error)

func (f uploadFunc) UploadMedia(ctx context.Context, actor domain.Actor, body io.Reader, size int64, mime, name string) (id.ContentURI, error) {
	return f(ctx, actor, body, size, mime, name)
}

func mediaUploadRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/internal/media/upload", strings.NewReader(body))
	r.Header.Set("Content-Type", "image/png")
	r.Header.Set("X-Alkemio-Actor-ID", "11111111-1111-4111-8111-111111111111")
	r.Header.Set("X-Alkemio-Display-Name", url.PathEscape("résumé + photo.png"))
	return r
}

func TestMediaUpload_StreamsTrustedActorAndDecodedName(t *testing.T) {
	for _, body := range []string{"", "PNG bytes"} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			upload := uploadFunc(func(ctx context.Context, actor domain.Actor, reader io.Reader, size int64, mime, name string) (id.ContentURI, error) {
				calls++
				require.Equal(t, uuid.MustParse("11111111-1111-4111-8111-111111111111"), actor.ID)
				require.Equal(t, int64(len(body)), size)
				require.Equal(t, "image/png", mime)
				require.Equal(t, "résumé + photo.png", name)
				bytes, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Equal(t, body, string(bytes))
				require.NoError(t, ctx.Err())
				return id.MustParseContentURI("mxc://local.test/uploaded"), nil
			})
			mux := http.NewServeMux()
			NewMediaUploadHandler(upload, 100).RegisterRoutes(mux)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, mediaUploadRequest(body))
			require.Equal(t, http.StatusCreated, w.Code)
			require.JSONEq(t, `{"contentUri":"mxc://local.test/uploaded","mediaId":"uploaded"}`, w.Body.String())
			require.Equal(t, 1, calls)
		})
	}
}

func TestMediaUpload_RejectsInvalidInputBeforeUpload(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*http.Request)
		status int
	}{
		{"no actor", func(r *http.Request) { r.Header.Del("X-Alkemio-Actor-ID") }, 400},
		{"nil actor", func(r *http.Request) { r.Header.Set("X-Alkemio-Actor-ID", uuid.Nil.String()) }, 400},
		{"unknown length", func(r *http.Request) { r.ContentLength = -1 }, 411},
		{"too large", func(r *http.Request) { r.ContentLength = 101 }, 413},
		{"invalid escape", func(r *http.Request) { r.Header.Set("X-Alkemio-Display-Name", "%ZZ") }, 400},
		{"empty name", func(r *http.Request) { r.Header.Del("X-Alkemio-Display-Name") }, 400},
		{"name control", func(r *http.Request) { r.Header.Set("X-Alkemio-Display-Name", "file%0Aname") }, 400},
		{"missing mime", func(r *http.Request) { r.Header.Del("Content-Type") }, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			upload := uploadFunc(func(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error) {
				called = true
				return id.ContentURI{}, nil
			})
			mux := http.NewServeMux()
			NewMediaUploadHandler(upload, 100).RegisterRoutes(mux)
			r := mediaUploadRequest("data")
			tc.change(r)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.False(t, called)
		})
	}
}

func TestMediaUpload_CancellationAndUpstreamFailureAreNotSuccess(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, io.ErrUnexpectedEOF} {
		calls := 0
		upload := uploadFunc(func(ctx context.Context, _ domain.Actor, _ io.Reader, _ int64, _, _ string) (id.ContentURI, error) {
			calls++
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			return id.ContentURI{}, failure
		})
		mux := http.NewServeMux()
		NewMediaUploadHandler(upload, 100).RegisterRoutes(mux)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, mediaUploadRequest("data").WithContext(ctx))
		require.GreaterOrEqual(t, w.Code, 400)
		require.NotContains(t, w.Body.String(), "contentUri")
		require.Equal(t, 1, calls)
	}
}
