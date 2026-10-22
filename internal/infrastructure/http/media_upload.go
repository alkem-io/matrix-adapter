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
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
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
	// imagePiece is how much of an image body is charged to the budget, and
	// then read, at a time, so a sender holds budget only for what it has
	// sent rather than for what it declared.
	imagePiece = 1 << 20
	// imagePieceTimeout is how long one piece of an image body may take to
	// arrive. A sender slower than that (32 KiB/s) is cut off with 408
	// upload_timeout, so it cannot keep its share of the budget by stalling.
	imagePieceTimeout = 32 * time.Second
	// imagesPerActor is how many images one actor may have being read,
	// rewritten and stored at once, across both budgets.
	imagesPerActor = 2
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
	uploader     mediaUploader
	log          ports.Logger
	maxBytes     int64
	images       waitingBudget
	imageBudget  int64
	rotations    waitingBudget
	actors       actorSlots
	pieceTimeout time.Duration
}

// NewMediaUploadHandler creates the byte-upload bridge without message publication.
func NewMediaUploadHandler(uploader mediaUploader, maxBytes int64, log ports.Logger) *MediaUploadHandler {
	return newMediaUploadHandler(uploader, maxBytes, imageMemoryBudget, imageBudgetWait, log)
}

func newMediaUploadHandler(
	uploader mediaUploader, maxBytes, imageBudget int64, imageWait time.Duration, log ports.Logger,
) *MediaUploadHandler {
	return &MediaUploadHandler{
		uploader:     uploader,
		log:          log,
		maxBytes:     maxBytes,
		images:       waitingBudget{"image", semaphore.NewWeighted(imageBudget), imageWait},
		imageBudget:  imageBudget,
		rotations:    waitingBudget{"rotation", semaphore.NewWeighted(imagemeta.MaxRotationBytes), imageWait},
		actors:       actorSlots{held: map[uuid.UUID]int{}},
		pieceTimeout: imagePieceTimeout,
	}
}

// waitingBudget is a memory budget that an upload waits on only briefly: a
// share it cannot get within wait is refused as errImageBusy.
type waitingBudget struct {
	name string
	sem  *semaphore.Weighted
	wait time.Duration
}

// Acquire takes n bytes of the budget, or returns errImageBusy once the wait
// runs out or ctx is done.
func (b waitingBudget) Acquire(ctx context.Context, n int64) error {
	wait, cancel := context.WithTimeout(ctx, b.wait)
	defer cancel()
	if err := b.sem.Acquire(wait, n); err != nil {
		return fmt.Errorf("%s budget: %w", b.name, errImageBusy)
	}
	return nil
}

// Release returns n bytes taken by Acquire.
func (b waitingBudget) Release(n int64) { b.sem.Release(n) }

// actorSlots counts the images each actor has in flight, so one actor cannot
// take every share of the budgets.
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
// A JPEG, PNG or WebP body (recognised from its bytes) is read whole and stored
// without its location, device and time metadata, with the content type and
// length of the rewritten bytes; a corrupt one is refused as invalid_media.
// Images being rewritten share a fixed memory budget, charged as their bytes
// arrive, and JPEG rotations a second one; an image that cannot get its share
// of either within a short wait, or whose actor already has imagesPerActor
// images in flight, is refused with 503 upload_busy and Retry-After, and
// nothing is uploaded. An image body that stops arriving is cut off with 408
// upload_timeout. Every other body streams through unchanged.
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
		h.logRefusal(actorID, r.ContentLength, err)
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

// logRefusal records who was refused as busy or cut off as too slow, so a
// budget held by one sender is visible and attributable.
func (h *MediaUploadHandler) logRefusal(actorID uuid.UUID, size int64, err error) {
	if errors.Is(err, errImageBusy) || errors.Is(err, os.ErrDeadlineExceeded) {
		h.log.Warn("Image upload refused", "actor_id", actorID.String(), "declared_bytes", size, "reason", err.Error())
	}
}

// errImageBusy reports that an image could not get its share of the memory
// budget in time, or that its actor already has as many images in flight as
// it may.
var errImageBusy = errors.New("image memory budget exhausted")

// errActorBusy refuses an image whose actor already has imagesPerActor in flight.
var errActorBusy = fmt.Errorf("actor has %d images in flight: %w", imagesPerActor, errImageBusy)

// uploadBody returns what to store for a body of size bytes, and a release to
// call once it has been stored. Only the leading bytes needed to recognise an
// image container are read up front, so any other upload still streams to
// Synapse as it arrives and holds no budget. deadline bounds each read of an
// image body.
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
	media, size, contentType, err := h.stripImage(ctx, format, whole)
	if err != nil {
		release()
		return nil, 0, "", nil, err
	}
	return media, size, contentType, release, nil
}

// readImage reads a recognised image whole, after its leading bytes head,
// because its metadata must be removed before any of it is stored. Each piece
// is charged to the image budget before it is read and must arrive within
// pieceTimeout, so a sender that stalls holds a share only for what it has
// sent, and not for long. The charge is imageCopies times the bytes, the most
// the rewrite holds at once, capped at the whole budget so an image larger
// than the budget is still accepted, alone. The returned release gives the
// charge back; on error nothing is left charged. The deadline is not cleared
// here: the server clears it itself once the body ends, and after a failed
// read it stays, so discarding the rest of the body cannot wait on a stalled
// sender either.
func (h *MediaUploadHandler) readImage(
	ctx context.Context, head []byte, body io.Reader, size int64, deadline func(time.Time),
) ([]byte, func(), error) {
	var charged int64
	release := func() { h.images.Release(charged) }
	charge := func(n int64) error {
		share := min(imageCopies*n, h.imageBudget-charged)
		if share <= 0 {
			return nil
		}
		if err := h.images.Acquire(ctx, share); err != nil {
			return err
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
	whole := make([]byte, 0, size)
	for _, piece := range pieces {
		whole = append(whole, piece...)
	}
	return whole, release, nil
}

// stripImage removes the metadata of a recognised image read whole. The
// declared type gives way to the type of the rewritten bytes. Only a broken
// image is invalid media; a rotation refused its budget or cut short by the
// request ending keeps its own error.
func (h *MediaUploadHandler) stripImage(
	ctx context.Context, format imagemeta.Format, whole []byte,
) (io.Reader, int64, string, error) {
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
