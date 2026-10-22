package httpinfra

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/imagemeta"
)

type uploadFunc func(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error)

func (f uploadFunc) UploadMedia(ctx context.Context, actor domain.Actor, body io.Reader, size int64, mime, name string) (id.ContentURI, error) {
	return f(ctx, actor, body, size, mime, name)
}

func mediaUploadRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/rest/messaging/media/upload?filename="+url.QueryEscape("résumé + photo.png"), strings.NewReader(body))
	r.Header.Set("Content-Type", "image/png")
	r.Header.Set("X-Alkemio-Actor-Id", "11111111-1111-4111-8111-111111111111")
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
		{"no actor", func(r *http.Request) { r.Header.Del("X-Alkemio-Actor-Id") }, 401},
		{"nil actor", func(r *http.Request) { r.Header.Set("X-Alkemio-Actor-Id", uuid.Nil.String()) }, 401},
		{"reserved guest actor", func(r *http.Request) { r.Header.Set("X-Alkemio-Actor-Id", "00000000-0000-0000-0000-000000000001") }, 401},
		{"guest actor", func(r *http.Request) {
			r.Header.Set("X-Alkemio-Actor-Id", "guest-11111111-1111-4111-8111-111111111111")
		}, 401},
		{"query actor is not identity", func(r *http.Request) {
			r.Header.Del("X-Alkemio-Actor-Id")
			r.URL.RawQuery += "&actorID=11111111-1111-4111-8111-111111111111"
		}, 401},
		{"unknown length", func(r *http.Request) { r.ContentLength = -1 }, 411},
		{"too large", func(r *http.Request) { r.ContentLength = 101 }, 413},
		{"invalid escape", func(r *http.Request) { r.URL.RawQuery = "filename=%ZZ" }, 400},
		{"empty name", func(r *http.Request) { r.URL.RawQuery = "" }, 400},
		{"name control", func(r *http.Request) { r.URL.RawQuery = "filename=file%0Aname" }, 400},
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

func TestMediaUpload_ForwardsBeforeRequestEOF(t *testing.T) {
	firstBytes := make(chan struct{})
	upload := uploadFunc(func(_ context.Context, _ domain.Actor, reader io.Reader, size int64, _, _ string) (id.ContentURI, error) {
		if size != 24 {
			return id.ContentURI{}, io.ErrUnexpectedEOF
		}
		var first [12]byte
		if _, err := io.ReadFull(reader, first[:]); err != nil {
			return id.ContentURI{}, err
		}
		close(firstBytes)
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return id.ContentURI{}, err
		}
		return id.MustParseContentURI("mxc://local.test/streamed"), nil
	})
	mux := http.NewServeMux()
	NewMediaUploadHandler(upload, 100).RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/rest/messaging/media/upload?filename=stream.bin", reader)
	require.NoError(t, err)
	request.ContentLength = 24
	request.Header.Set("X-Alkemio-Actor-Id", "11111111-1111-4111-8111-111111111111")
	request.Header.Set("Content-Type", "application/octet-stream")
	finished := make(chan error, 1)
	go func() {
		response, err := server.Client().Do(request)
		if err == nil {
			if response.StatusCode != http.StatusCreated {
				err = io.ErrUnexpectedEOF
			}
			_ = response.Body.Close()
		}
		finished <- err
	}()
	_, err = writer.Write([]byte("first twelve"))
	require.NoError(t, err)
	select {
	case <-firstBytes:
	case <-time.After(2 * time.Second):
		t.Fatal("upload reader was not forwarded before request EOF")
	}
	_, err = writer.Write([]byte("the rest...."))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, <-finished)
}

func TestMediaUpload_OldInternalRouteIsAbsent(t *testing.T) {
	mux := http.NewServeMux()
	NewMediaUploadHandler(nil, 100).RegisterRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/internal/media/upload", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
}

// photoWithLocation is a real JPEG carrying an Exif segment with a device make
// and a GPS latitude, as a phone camera writes it.
func photoWithLocation(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	require.NoError(t, jpeg.Encode(&encoded, image.NewGray(image.Rect(0, 0, 8, 8)), nil))
	plain := encoded.Bytes()
	const tiff = "MM\x00\x2A\x00\x00\x00\x08\x00\x01" +
		"\x01\x0F\x00\x02\x00\x00\x00\x0C\x00\x00\x00\x1A\x00\x00\x00\x00" +
		"SecretPhone\x00"
	const payload = "Exif\x00\x00" + tiff + "GPS 52.3701N SecretGPS"
	exif := append([]byte{0xFF, 0xE1, 0x00, byte(2 + len(payload))}, payload...)
	return append(append(append([]byte{}, plain[:2]...), exif...), plain[2:]...)
}

func capturingUpload(t *testing.T, got *[]byte, size *int64, mimeType *string) uploadFunc {
	t.Helper()
	return func(_ context.Context, _ domain.Actor, reader io.Reader, n int64, mime, _ string) (id.ContentURI, error) {
		bytes, err := io.ReadAll(reader)
		require.NoError(t, err)
		*got, *size, *mimeType = bytes, n, mime
		return id.MustParseContentURI("mxc://local.test/clean"), nil
	}
}

func TestMediaUpload_HandsSynapseTheStrippedImage(t *testing.T) {
	photo := photoWithLocation(t)
	want, err := imagemeta.Strip(imagemeta.FormatJPEG, photo)
	require.NoError(t, err)
	var got []byte
	var size int64
	var mimeType string
	mux := http.NewServeMux()
	NewMediaUploadHandler(capturingUpload(t, &got, &size, &mimeType), 1<<20).RegisterRoutes(mux)
	r := mediaUploadRequest(string(photo))
	// The declared type is not trusted: the bytes decide.
	r.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, r)

	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, want, got)
	require.NotContains(t, string(got), "SecretPhone")
	require.NotContains(t, string(got), "SecretGPS")
	require.Equal(t, int64(len(got)), size, "length sent onward is the stripped length")
	require.Equal(t, "image/jpeg", mimeType, "type sent onward is the stripped image's")
	_, err = jpeg.Decode(bytes.NewReader(got))
	require.NoError(t, err)
}

func TestMediaUpload_RefusesCorruptImageWithoutUploading(t *testing.T) {
	photo := photoWithLocation(t)
	called := false
	upload := uploadFunc(func(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error) {
		called = true
		return id.ContentURI{}, nil
	})
	mux := http.NewServeMux()
	NewMediaUploadHandler(upload, 1<<20).RegisterRoutes(mux)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, mediaUploadRequest(string(photo[:len(photo)/2])))

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.JSONEq(t, `{"error":"invalid_media"}`, w.Body.String())
	require.False(t, called, "unstripped bytes must never reach Synapse")
}

func TestMediaUpload_PassesGIFThroughUnchanged(t *testing.T) {
	var animation bytes.Buffer
	frame := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White})
	require.NoError(t, gif.EncodeAll(&animation, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{10, 10}}))
	var got []byte
	var size int64
	var mimeType string
	mux := http.NewServeMux()
	NewMediaUploadHandler(capturingUpload(t, &got, &size, &mimeType), 1<<20).RegisterRoutes(mux)
	r := mediaUploadRequest(animation.String())
	r.Header.Set("Content-Type", "image/gif")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, r)

	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, animation.Bytes(), got)
	require.Equal(t, int64(animation.Len()), size)
	require.Equal(t, "image/gif", mimeType)
}

// blockingUpload counts its calls and signals entered for each, then holds
// the upload open until release is closed.
func blockingUpload(calls *atomic.Int32, entered chan<- struct{}, release <-chan struct{}) uploadFunc {
	return func(_ context.Context, _ domain.Actor, reader io.Reader, _ int64, _, _ string) (id.ContentURI, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		_, err := io.Copy(io.Discard, reader)
		return id.MustParseContentURI("mxc://local.test/held"), err
	}
}

// serveUpload sends body as a new actor each time, so only the shared budgets
// limit how many uploads run at once.
func serveUpload(mux *http.ServeMux, body []byte, contentType string) *httptest.ResponseRecorder {
	r := mediaUploadRequest(string(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("X-Alkemio-Actor-Id", uuid.NewString())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func awaitEntered(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("upload did not reach the uploader")
	}
}

func TestMediaUpload_RefusesImagesBeyondTheMemoryBudget(t *testing.T) {
	photo := photoWithLocation(t)
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	// Room for exactly two photos being rewritten at once.
	newMediaUploadHandler(blockingUpload(&calls, entered, release), 1<<20, 2*imageCopies*int64(len(photo)), 20*time.Millisecond).RegisterRoutes(mux)

	held := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { held <- serveUpload(mux, photo, "image/jpeg") }()
		awaitEntered(t, entered)
	}

	excess := make(chan *httptest.ResponseRecorder, 1)
	go func() { excess <- serveUpload(mux, photo, "image/jpeg") }()
	var busy *httptest.ResponseRecorder
	select {
	case busy = <-excess:
	case <-entered:
		close(release)
		t.Fatal("an image beyond the budget reached the uploader")
	}

	require.Equal(t, http.StatusServiceUnavailable, busy.Code)
	require.JSONEq(t, `{"error":"upload_busy"}`, busy.Body.String())
	require.Equal(t, imageRetryAfter, busy.Header().Get("Retry-After"))
	require.Equal(t, int32(2), calls.Load(), "nothing is uploaded for the refused image")

	close(release)
	for range 2 {
		require.Equal(t, http.StatusCreated, (<-held).Code)
	}
	go func() { held <- serveUpload(mux, photo, "image/jpeg") }()
	awaitEntered(t, entered)
	require.Equal(t, http.StatusCreated, (<-held).Code, "budget is returned once uploads succeed")
}

func TestMediaUpload_ReturnsImageBudgetAfterFailure(t *testing.T) {
	photo := photoWithLocation(t)
	fail := true
	upload := uploadFunc(func(_ context.Context, _ domain.Actor, reader io.Reader, _ int64, _, _ string) (id.ContentURI, error) {
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return id.ContentURI{}, err
		}
		if fail {
			return id.ContentURI{}, io.ErrUnexpectedEOF
		}
		return id.MustParseContentURI("mxc://local.test/after"), nil
	})
	mux := http.NewServeMux()
	// Room for one photo only, so any budget left behind refuses the next.
	newMediaUploadHandler(upload, 1<<20, imageCopies*int64(len(photo)), 20*time.Millisecond).RegisterRoutes(mux)

	require.Equal(t, http.StatusBadGateway, serveUpload(mux, photo, "image/jpeg").Code)
	require.Equal(t, http.StatusBadRequest, serveUpload(mux, photo[:len(photo)/2], "image/jpeg").Code)

	fail = false
	require.Equal(t, http.StatusCreated, serveUpload(mux, photo, "image/jpeg").Code)
}

func TestMediaUpload_AcceptsAnImageLargerThanTheWholeBudget(t *testing.T) {
	photo := photoWithLocation(t)
	var got []byte
	var size int64
	var mimeType string
	mux := http.NewServeMux()
	newMediaUploadHandler(capturingUpload(t, &got, &size, &mimeType), 1<<20, int64(len(photo)), 20*time.Millisecond).RegisterRoutes(mux)

	require.Equal(t, http.StatusCreated, serveUpload(mux, photo, "image/jpeg").Code)
	require.Equal(t, "image/jpeg", mimeType)
}

func TestMediaUpload_DoesNotGateFilesThatAreNotImages(t *testing.T) {
	photo := photoWithLocation(t)
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	newMediaUploadHandler(blockingUpload(&calls, entered, release), 1<<20, imageCopies*int64(len(photo)), 20*time.Millisecond).RegisterRoutes(mux)
	held := make(chan *httptest.ResponseRecorder, 1)
	go func() { held <- serveUpload(mux, photo, "image/jpeg") }()
	awaitEntered(t, entered)

	document := make(chan *httptest.ResponseRecorder, 1)
	go func() { document <- serveUpload(mux, []byte("plain document bytes"), "application/pdf") }()
	awaitEntered(t, entered)
	require.Equal(t, int32(2), calls.Load(), "a document streams while the image budget is fully held")

	close(release)
	require.Equal(t, http.StatusCreated, (<-held).Code)
	require.Equal(t, http.StatusCreated, (<-document).Code)
}

func TestMediaUpload_LimitsTheImagesOneActorHasInFlight(t *testing.T) {
	photo := photoWithLocation(t)
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	newMediaUploadHandler(blockingUpload(&calls, entered, release), 1<<20, imageMemoryBudget, 20*time.Millisecond).RegisterRoutes(mux)
	const actor = "22222222-2222-4222-8222-222222222222"
	upload := func(actorID, contentType string, body []byte) *httptest.ResponseRecorder {
		r := mediaUploadRequest(string(body))
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("X-Alkemio-Actor-Id", actorID)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	held := make(chan *httptest.ResponseRecorder, imagesPerActor+2)
	for range imagesPerActor {
		go func() { held <- upload(actor, "image/jpeg", photo) }()
		awaitEntered(t, entered)
	}

	excess := make(chan *httptest.ResponseRecorder, 1)
	go func() { excess <- upload(actor, "image/jpeg", photo) }()
	var busy *httptest.ResponseRecorder
	select {
	case busy = <-excess:
	case <-entered:
		close(release)
		t.Fatal("an image beyond the actor's limit reached the uploader")
	}

	require.Equal(t, http.StatusServiceUnavailable, busy.Code)
	require.JSONEq(t, `{"error":"upload_busy"}`, busy.Body.String())
	require.Equal(t, imageRetryAfter, busy.Header().Get("Retry-After"))
	require.Equal(t, int32(imagesPerActor), calls.Load(), "nothing is uploaded for the refused image")

	go func() { held <- upload(uuid.NewString(), "image/jpeg", photo) }()
	awaitEntered(t, entered)
	go func() { held <- upload(actor, "application/pdf", []byte("plain document bytes")) }()
	awaitEntered(t, entered)
	require.Equal(t, int32(imagesPerActor+2), calls.Load(), "other actors' images and this actor's documents still go through")

	close(release)
	for range imagesPerActor + 2 {
		require.Equal(t, http.StatusCreated, (<-held).Code)
	}
	go func() { held <- upload(actor, "image/jpeg", photo) }()
	awaitEntered(t, entered)
	require.Equal(t, http.StatusCreated, (<-held).Code, "the actor's slots are returned")
}

// slowImageUpload starts an upload that declares declared bytes of JPEG but
// sends only the bytes that identify it, then waits for writer to send more or
// close. The response status, or zero when there is none, arrives on the
// channel.
func slowImageUpload(t *testing.T, server *httptest.Server, actor string, declared int64) (*io.PipeWriter, <-chan int) {
	t.Helper()
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	request, err := http.NewRequest(http.MethodPost, server.URL+"/rest/messaging/media/upload?filename=slow.jpg", reader)
	require.NoError(t, err)
	request.ContentLength = declared
	request.Header.Set("X-Alkemio-Actor-Id", actor)
	request.Header.Set("Content-Type", "image/jpeg")
	statuses := make(chan int, 1)
	go func() {
		response, err := server.Client().Do(request)
		if err != nil {
			statuses <- 0
			return
		}
		_ = response.Body.Close()
		statuses <- response.StatusCode
	}()
	_, err = writer.Write([]byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\x01"))
	require.NoError(t, err)
	return writer, statuses
}

// charged reports whether at least n bytes of the image budget are taken.
func charged(h *MediaUploadHandler, budget, n int64) bool {
	if h.images.sem.TryAcquire(budget - n + 1) {
		h.images.sem.Release(budget - n + 1)
		return false
	}
	return true
}

func TestMediaUpload_ChargesTheImageBudgetOnlyForBytesReceived(t *testing.T) {
	photo := photoWithLocation(t)
	var got []byte
	var size int64
	var mimeType string
	const budget = 4 * imagePiece
	h := newMediaUploadHandler(capturingUpload(t, &got, &size, &mimeType), 64<<20, budget, imageBudgetWait)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	// Registered first so it runs last, after the stalled request is closed.
	t.Cleanup(server.Close)
	// Declares 50 MiB, three times the whole budget, and sends four bytes.
	writer, statuses := slowImageUpload(t, server, uuid.NewString(), 50<<20)
	require.Eventually(t, func() bool { return charged(h, budget, imageCopies*imagePiece) }, 2*time.Second, 5*time.Millisecond,
		"the stalled sender is charged for the piece it is sending")
	require.False(t, charged(h, budget, imageCopies*imagePiece+imageCopies*imagePiece), "and for nothing it has not sent")

	require.Equal(t, http.StatusCreated, serveUpload(mux, photo, "image/jpeg").Code,
		"another image fits beside a sender that declared more than the budget")
	require.Equal(t, "image/jpeg", mimeType)

	require.NoError(t, writer.CloseWithError(io.ErrClosedPipe))
	<-statuses
	require.Eventually(t, func() bool { return !charged(h, budget, 1) }, 2*time.Second, 5*time.Millisecond,
		"an abandoned image returns its charge")
}

func TestMediaUpload_CutsOffAnImageSenderThatStalls(t *testing.T) {
	called := false
	upload := uploadFunc(func(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error) {
		called = true
		return id.ContentURI{}, nil
	})
	h := newMediaUploadHandler(upload, 64<<20, imageMemoryBudget, imageBudgetWait)
	h.pieceTimeout = 100 * time.Millisecond
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	// Registered first so it runs last, after the stalled request is closed.
	t.Cleanup(server.Close)
	const actor = "33333333-3333-4333-8333-333333333333"

	_, statuses := slowImageUpload(t, server, actor, 10<<20)

	var status int
	select {
	case status = <-statuses:
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled image sender was not cut off")
	}
	require.Equal(t, http.StatusRequestTimeout, status)
	require.False(t, called, "nothing is uploaded for the cut-off image")
	require.True(t, h.images.sem.TryAcquire(imageMemoryBudget), "the cut-off image returns its charge")
	for range imagesPerActor {
		require.True(t, h.actors.take(uuid.MustParse(actor)), "and its actor's slot")
	}
}

// The handler leaves the last piece's read deadline in place; this pins that
// the server clears it once the body ends, so a slow store is not cancelled.
func TestMediaUpload_ImageReadDeadlineDoesNotCutOffItsStorage(t *testing.T) {
	photo := photoWithLocation(t)
	upload := uploadFunc(func(ctx context.Context, _ domain.Actor, reader io.Reader, _ int64, _, _ string) (id.ContentURI, error) {
		// Storage outlasts the read deadline of every piece.
		select {
		case <-ctx.Done():
			return id.ContentURI{}, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		_, err := io.Copy(io.Discard, reader)
		return id.MustParseContentURI("mxc://local.test/stored"), err
	})
	h := newMediaUploadHandler(upload, 1<<20, imageMemoryBudget, imageBudgetWait)
	h.pieceTimeout = 50 * time.Millisecond
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/rest/messaging/media/upload?filename=photo.jpg", bytes.NewReader(photo))
	require.NoError(t, err)
	request.Header.Set("X-Alkemio-Actor-Id", uuid.NewString())
	request.Header.Set("Content-Type", "image/jpeg")

	response, err := server.Client().Do(request)

	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusCreated, response.StatusCode)
}

// truncatedBody ends early as net/http's request body does when a client
// disconnects before sending its declared Content-Length.
type truncatedBody struct{ io.Reader }

func (b truncatedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func TestMediaUpload_BodyShorterThanItsLengthFailsAsBadGateway(t *testing.T) {
	photo := photoWithLocation(t)
	// As the Matrix adapter does, the stream is read to its end and a failed
	// read comes back as a mautrix transport error.
	upload := uploadFunc(func(_ context.Context, _ domain.Actor, reader io.Reader, _ int64, _, _ string) (id.ContentURI, error) {
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return id.ContentURI{}, fmt.Errorf("media upload failed: %w", mautrix.HTTPError{Message: "request error", WrappedError: err})
		}
		return id.MustParseContentURI("mxc://local.test/short"), nil
	})
	for name, tc := range map[string]struct {
		sent     []byte
		declared int
		error    string
	}{
		"image":             {photo[:len(photo)/2], len(photo), "upstream_upload_failed"},
		"file":              {[]byte("plain document bytes"), 64, "upstream_rejected"},
		"file of few bytes": {[]byte("doc"), 64, "upstream_rejected"},
	} {
		t.Run(name, func(t *testing.T) {
			mux := http.NewServeMux()
			NewMediaUploadHandler(upload, 1<<20).RegisterRoutes(mux)
			r := mediaUploadRequest("")
			r.Body = io.NopCloser(truncatedBody{bytes.NewReader(tc.sent)})
			r.ContentLength = int64(tc.declared)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, r)

			require.Equal(t, http.StatusBadGateway, w.Code)
			require.JSONEq(t, `{"error":"`+tc.error+`"}`, w.Body.String())
		})
	}
}
