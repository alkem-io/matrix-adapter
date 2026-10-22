package httpinfra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"sync"
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
	// imageMemoryBudget bounds the live bytes of images held whole to be
	// rewritten, so the heap peaks near 400 MiB, under the pod's 512 Mi limit.
	imageMemoryBudget int64 = 160 << 20
	// imageCopies is how many copies of an image are live at once while it is read and rewritten.
	imageCopies = 2
	// imageBudgetWait is how long an image waits for the budget before 503 upload_busy.
	imageBudgetWait = 2 * time.Second
	// imageRetryAfter is the Retry-After hint sent with upload_busy.
	imageRetryAfter = "5"
	// imagePiece is how much of an image body is charged to the budget, then read, at a time.
	imagePiece = 1 << 20
	// imagePieceTimeout is how long one piece may take to arrive before 408 upload_timeout.
	imagePieceTimeout = 32 * time.Second
	// imagesPerActor is how many images one actor may have in flight at once.
	imagesPerActor = 2
)

type mediaUploader interface {
	UploadMedia(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error)
}

// MediaUploadHandler accepts original streams through the authenticated gateway.
// The gateway strips client actor headers and resolves the authenticated identity.
type MediaUploadHandler struct {
	uploader     mediaUploader
	maxBytes     int64
	sem          *semaphore.Weighted
	imageBudget  int64
	imageWait    time.Duration
	actors       actorSlots
	pieceTimeout time.Duration
}

// NewMediaUploadHandler creates the byte-upload bridge without message publication.
func NewMediaUploadHandler(uploader mediaUploader, maxBytes int64) *MediaUploadHandler {
	return &MediaUploadHandler{
		uploader:     uploader,
		maxBytes:     maxBytes,
		sem:          semaphore.NewWeighted(imageMemoryBudget),
		imageBudget:  imageMemoryBudget,
		imageWait:    imageBudgetWait,
		actors:       actorSlots{held: map[uuid.UUID]int{}},
		pieceTimeout: imagePieceTimeout,
	}
}

// actorSlots counts the images each actor has in flight, so one actor cannot
// take every share of the budget.
type actorSlots struct {
	mu   sync.Mutex
	held map[uuid.UUID]int
}

// take claims a slot for actor, or reports false when it has none left.
func (s *actorSlots) take(actor uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[actor] >= imagesPerActor {
		return false
	}
	s.held[actor]++
	return true
}

// give returns a slot claimed by take.
func (s *actorSlots) give(actor uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[actor]--; s.held[actor] <= 0 {
		delete(s.held, actor)
	}
}

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
// A JPEG, PNG or WebP body is stripped of its metadata first (see uploadBody).
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
	controller := http.NewResponseController(w)
	// A writer without read deadlines (a test recorder) leaves the read unbounded.
	deadline := func(t time.Time) { _ = controller.SetReadDeadline(t) }
	media, size, contentType, release, err := h.uploadBody(r.Context(), actorID, body, r.ContentLength, contentType, deadline)
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
// budget in time, or that its actor already has as many images in flight as
// it may.
var errImageBusy = errors.New("image memory budget exhausted")

// errActorBusy refuses an image whose actor already has imagesPerActor in flight.
var errActorBusy = fmt.Errorf("actor has %d images in flight: %w", imagesPerActor, errImageBusy)

// uploadBody returns what to store and a release to call once it is stored.
// Only a recognised image is read whole and charged to the budget; any other
// body streams through as it arrives.
func (h *MediaUploadHandler) uploadBody(
	ctx context.Context, actorID uuid.UUID, body io.Reader, size int64, contentType string, deadline func(time.Time),
) (io.Reader, int64, string, func(), error) {
	head, format, err := sniffImage(body, size)
	if err != nil {
		return nil, 0, "", nil, err
	}
	if format == imagemeta.FormatNone {
		return io.MultiReader(bytes.NewReader(head), body), size, contentType, func() {}, nil
	}
	if !h.actors.take(actorID) {
		return nil, 0, "", nil, errActorBusy
	}
	whole, releaseMemory, err := h.readImage(ctx, head, body, size, deadline)
	if err != nil {
		h.actors.give(actorID)
		return nil, 0, "", nil, err
	}
	release := func() {
		releaseMemory()
		h.actors.give(actorID)
	}
	media, size, contentType, err := stripImage(format, whole)
	if err != nil {
		release()
		return nil, 0, "", nil, err
	}
	return media, size, contentType, release, nil
}

// readImage does not clear the read deadline: the server clears it itself once
// the body ends, and after a failed read it stays, so discarding the rest of
// the body cannot wait on a stalled sender either.
func (h *MediaUploadHandler) readImage(
	ctx context.Context, head []byte, body io.Reader, size int64, deadline func(time.Time),
) ([]byte, func(), error) {
	var charged int64
	release := func() { h.sem.Release(charged) }
	charge := func(n int64) error {
		share := min(imageCopies*n, h.imageBudget-charged)
		if share <= 0 {
			return nil
		}
		wait, cancel := context.WithTimeout(ctx, h.imageWait)
		defer cancel()
		if err := h.sem.Acquire(wait, share); err != nil {
			return errImageBusy
		}
		charged += share
		return nil
	}
	if err := charge(int64(len(head))); err != nil {
		return nil, nil, err
	}
	pieces := [][]byte{head}
	for read := int64(len(head)); read < size; {
		n := min(size-read, imagePiece)
		deadline(time.Now().Add(h.pieceTimeout))
		if err := charge(n); err != nil {
			release()
			return nil, nil, err
		}
		piece := make([]byte, n)
		if _, err := io.ReadFull(body, piece); err != nil {
			release()
			return nil, nil, err
		}
		pieces = append(pieces, piece)
		read += n
	}
	return bytes.Join(pieces, nil), release, nil
}

// stripImage removes the metadata of a recognised image read whole. The
// declared type gives way to the type of the rewritten bytes. A broken image
// is invalid media.
func stripImage(format imagemeta.Format, whole []byte) (io.Reader, int64, string, error) {
	stripped, err := imagemeta.Strip(format, whole)
	if err != nil {
		return nil, 0, "", fmt.Errorf("%w: %w", domain.ErrInvalidParam, err)
	}
	return bytes.NewReader(stripped), int64(len(stripped)), format.ContentType(), nil
}

// sniffImage reads the leading bytes that identify an image container. A body
// that ends before them is classified on what arrived and fails where any
// short body does.
func sniffImage(body io.Reader, size int64) ([]byte, imagemeta.Format, error) {
	head := make([]byte, min(size, imagemeta.SniffLen))
	n, err := io.ReadFull(body, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, imagemeta.FormatNone, err
	}
	return head[:n], imagemeta.Sniff(head[:n]), nil
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
	case errors.Is(err, os.ErrDeadlineExceeded):
		WriteJSON(w, http.StatusRequestTimeout, map[string]string{"error": "upload_timeout"})
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
