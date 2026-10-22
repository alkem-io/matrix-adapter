package httpinfra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/imagemeta"
)

// gatewayGuestActorID is the reserved guest identity emitted by the gateway resolver.
const gatewayGuestActorID = "00000000-0000-0000-0000-000000000001"

const (
	// imageMemoryBudget bounds the bytes held at once by images read whole to
	// be rewritten, as imagemeta.MaxRotationBytes bounds those held by JPEG
	// rotations. Together they are 320 MiB of the pod's 512 Mi limit. They
	// bound live memory only: what the rewrites leave behind as garbage is
	// bounded by the runtime's soft memory limit (package memlimit, 80% of the
	// pod's limit), which makes the collector free it before the heap reaches
	// the pod's limit. A 50 MiB image, the default attachment cap, fits.
	imageMemoryBudget int64 = 160 << 20
	// imageCopies is how many copies of an image can be live while it is
	// rewritten: the bytes received, the rewritten bytes, and the copy made
	// when an orientation-only segment is inserted. A rotation's decode and
	// re-encode are charged to the rotation budget instead.
	imageCopies = 3
	// imageBudgetWait is how long an image waits for either budget before the
	// upload is refused as busy; imageRetryAfter is the retry hint sent with
	// that.
	imageBudgetWait = 2 * time.Second
	imageRetryAfter = "5"
)

// ImageMemoryBytes is the most live memory that images being rewritten may
// hold at once. The process's memory limit must leave room above it.
const ImageMemoryBytes = imageMemoryBudget + imagemeta.MaxRotationBytes

type mediaUploader interface {
	UploadMedia(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error)
}

// MediaUploadHandler accepts original streams through the authenticated gateway.
// The gateway strips client actor headers and resolves the authenticated identity.
type MediaUploadHandler struct {
	uploader    mediaUploader
	maxBytes    int64
	images      waitingBudget
	imageBudget int64
	rotations   waitingBudget
}

// NewMediaUploadHandler creates the byte-upload bridge without message publication.
func NewMediaUploadHandler(uploader mediaUploader, maxBytes int64) *MediaUploadHandler {
	return newMediaUploadHandler(uploader, maxBytes, imageMemoryBudget, imageBudgetWait)
}

func newMediaUploadHandler(uploader mediaUploader, maxBytes, imageBudget int64, imageWait time.Duration) *MediaUploadHandler {
	return &MediaUploadHandler{
		uploader:    uploader,
		maxBytes:    maxBytes,
		images:      waitingBudget{semaphore.NewWeighted(imageBudget), imageWait},
		imageBudget: imageBudget,
		rotations:   waitingBudget{semaphore.NewWeighted(imagemeta.MaxRotationBytes), imageWait},
	}
}

// waitingBudget is a memory budget that an upload waits on only briefly: a
// share it cannot get within wait is refused as errImageBusy.
type waitingBudget struct {
	sem  *semaphore.Weighted
	wait time.Duration
}

// Acquire takes n bytes of the budget, or returns errImageBusy once the wait
// runs out or ctx is done.
func (b waitingBudget) Acquire(ctx context.Context, n int64) error {
	wait, cancel := context.WithTimeout(ctx, b.wait)
	defer cancel()
	if err := b.sem.Acquire(wait, n); err != nil {
		return errImageBusy
	}
	return nil
}

// Release returns n bytes taken by Acquire.
func (b waitingBudget) Release(n int64) { b.sem.Release(n) }

// RegisterRoutes adds the gateway-routed upload to the appservice listener.
func (h *MediaUploadHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /rest/messaging/media/upload", h.handleUpload)
}

// MediaUploadResult identifies a completed synchronous Synapse upload.
type MediaUploadResult struct {
	ContentURI string `json:"contentUri"`
	MediaID    string `json:"mediaId"`
}

// handleUpload streams an original file to Synapse as the gateway-resolved actor.
// The body is raw binary, not JSON. Content-Length is required (including zero),
// Content-Type is the original MIME type, X-Alkemio-Actor-Id is a trusted
// non-guest actor UUID, and filename is an ordinary UTF-8 query parameter.
// A JPEG, PNG or WebP body (recognised from its bytes) is read whole and stored
// without its location, device and time metadata, with the content type and
// length of the rewritten bytes; a corrupt one is refused as invalid_media.
// Images being rewritten share a fixed memory budget, and JPEG rotations a
// second one; an image that cannot get its share of either within a short
// wait is refused with 503 upload_busy and Retry-After, and nothing is
// uploaded. Every other body streams through unchanged.
// No event is published. Success is 201 only after synchronous provider storage;
// failures use stable error categories and do not replay the stream.
func (h *MediaUploadHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	actorID, err := uuid.Parse(r.Header.Get("X-Alkemio-Actor-Id"))
	if err != nil || actorID == uuid.Nil || actorID.String() == gatewayGuestActorID {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_actor"})
		return
	}
	if r.ContentLength < 0 {
		WriteJSON(w, http.StatusLengthRequired, map[string]string{"error": "length_required"})
		return
	}
	if r.ContentLength > h.maxBytes {
		WriteJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "media_too_large"})
		return
	}
	name := r.URL.Query().Get("filename")
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_display_name"})
		return
	}
	contentType := r.Header.Get("Content-Type")
	if _, _, err := mime.ParseMediaType(contentType); err != nil || contentType == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_content_type"})
		return
	}
	// The HTTP transport already frames Content-Length; the cap also bounds any
	// alternate reader supplied by a local handler caller. Only a recognised
	// image is buffered, and never beyond its declared length.
	body := http.MaxBytesReader(w, r.Body, h.maxBytes)
	defer func() { _ = body.Close() }()
	media, size, contentType, release, err := h.uploadBody(r.Context(), body, r.ContentLength, contentType)
	if err != nil {
		writeMediaUploadError(w, err)
		return
	}
	// An image keeps its budget until the upload returns, since Synapse reads
	// the rewritten bytes from memory.
	defer release()
	uri, err := h.uploader.UploadMedia(r.Context(), domain.NewActor(actorID), media, size, contentType, name)
	if err != nil {
		writeMediaUploadError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, MediaUploadResult{ContentURI: uri.String(), MediaID: uri.FileID})
}

// errImageBusy reports that an image could not get its share of the memory
// budget in time.
var errImageBusy = errors.New("image memory budget exhausted")

// uploadBody returns what to store for a body of size bytes, and a release to
// call once it has been stored. Only the leading bytes needed to recognise an
// image container are read up front, so any other upload still streams to
// Synapse as it arrives and holds no budget.
func (h *MediaUploadHandler) uploadBody(
	ctx context.Context, body io.Reader, size int64, contentType string,
) (io.Reader, int64, string, func(), error) {
	head, format, err := sniffImage(body, size)
	if err != nil {
		return nil, 0, "", nil, err
	}
	if format == imagemeta.FormatNone {
		return io.MultiReader(bytes.NewReader(head), body), size, contentType, func() {}, nil
	}
	release, err := h.reserveImageMemory(ctx, size)
	if err != nil {
		return nil, 0, "", nil, err
	}
	media, size, contentType, err := h.stripImage(ctx, format, head, body, size)
	if err != nil {
		release()
		return nil, 0, "", nil, err
	}
	return media, size, contentType, release, nil
}

// reserveImageMemory takes this image's share of the memory budget, waiting
// briefly for other images to finish. An image larger than the whole budget
// takes all of it, so it is still accepted, alone.
func (h *MediaUploadHandler) reserveImageMemory(ctx context.Context, size int64) (func(), error) {
	weight := min(imageCopies*size, h.imageBudget)
	if err := h.images.Acquire(ctx, weight); err != nil {
		return nil, err
	}
	return func() { h.images.Release(weight) }, nil
}

// stripImage reads a recognised image whole, after its leading bytes head,
// because its metadata must be removed before any of it is stored. The buffer
// is sized from the declared length, which the caller has already capped, so
// it never grows. The declared type gives way to the type of the rewritten
// bytes. Only a broken image is invalid media; a rotation refused its budget
// or cut short by the request ending keeps its own error.
func (h *MediaUploadHandler) stripImage(
	ctx context.Context, format imagemeta.Format, head []byte, body io.Reader, size int64,
) (io.Reader, int64, string, error) {
	whole := make([]byte, size)
	read := copy(whole, head)
	if _, err := io.ReadFull(body, whole[read:]); err != nil {
		return nil, 0, "", err
	}
	stripped, err := imagemeta.Strip(ctx, format, whole, h.rotations)
	if errors.Is(err, imagemeta.ErrMalformed) {
		return nil, 0, "", fmt.Errorf("%w: %w", domain.ErrInvalidParam, err)
	}
	if err != nil {
		return nil, 0, "", err
	}
	return bytes.NewReader(stripped), int64(len(stripped)), format.ContentType(), nil
}

// sniffImage reads leading bytes only until the container is known, so a body
// that cannot be an image is never held back waiting for more of it.
func sniffImage(body io.Reader, size int64) ([]byte, imagemeta.Format, error) {
	head := make([]byte, 0, min(size, imagemeta.SniffLen))
	for len(head) < cap(head) {
		if format, decided := imagemeta.Sniff(head); decided {
			return head, format, nil
		}
		n, err := body.Read(head[len(head):cap(head)])
		head = head[:len(head)+n]
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, imagemeta.FormatNone, err
		}
	}
	format, _ := imagemeta.Sniff(head)
	return head, format, nil
}

func writeMediaUploadError(w http.ResponseWriter, err error) {
	var sizeError *http.MaxBytesError
	var upstream mautrix.HTTPError
	switch {
	case errors.Is(err, errImageBusy):
		w.Header().Set("Retry-After", imageRetryAfter)
		WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "upload_busy"})
	case errors.As(err, &sizeError):
		WriteJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "media_too_large"})
	case errors.Is(err, context.DeadlineExceeded):
		WriteJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "upload_timeout"})
	case errors.Is(err, context.Canceled):
		WriteJSON(w, http.StatusRequestTimeout, map[string]string{"error": "upload_canceled"})
	case errors.As(err, &upstream):
		WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream_rejected"})
	case errors.Is(err, domain.ErrInvalidParam):
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_media"})
	default:
		WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream_upload_failed"})
	}
}
