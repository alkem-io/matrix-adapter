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
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/imagemeta"
)

// gatewayGuestActorID is the reserved guest identity emitted by the gateway resolver.
const gatewayGuestActorID = "00000000-0000-0000-0000-000000000001"

type mediaUploader interface {
	UploadMedia(context.Context, domain.Actor, io.Reader, int64, string, string) (id.ContentURI, error)
}

// MediaUploadHandler accepts original streams through the authenticated gateway.
// The gateway strips client actor headers and resolves the authenticated identity.
type MediaUploadHandler struct {
	uploader mediaUploader
	maxBytes int64
}

// NewMediaUploadHandler creates the byte-upload bridge without message publication.
func NewMediaUploadHandler(uploader mediaUploader, maxBytes int64) *MediaUploadHandler {
	return &MediaUploadHandler{uploader: uploader, maxBytes: maxBytes}
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
// Every other body streams through unchanged.
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
	media, size, contentType, err := uploadBody(body, r.ContentLength, contentType)
	if err != nil {
		writeMediaUploadError(w, err)
		return
	}
	uri, err := h.uploader.UploadMedia(r.Context(), domain.NewActor(actorID), media, size, contentType, name)
	if err != nil {
		writeMediaUploadError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, MediaUploadResult{ContentURI: uri.String(), MediaID: uri.FileID})
}

// uploadBody returns what to store for a body of size bytes. Only the leading
// bytes needed to recognise an image container are read up front, so any
// other upload still streams to Synapse as it arrives. A recognised image is
// read whole (bounded by the upload cap) because its metadata must be removed
// before any of it is stored; the declared type then gives way to the type of
// the rewritten bytes.
func uploadBody(body io.Reader, size int64, contentType string) (io.Reader, int64, string, error) {
	head, format, err := sniffImage(body, size)
	if err != nil {
		return nil, 0, "", err
	}
	if format == imagemeta.FormatNone {
		return io.MultiReader(bytes.NewReader(head), body), size, contentType, nil
	}
	var whole bytes.Buffer
	whole.Write(head)
	if _, err := whole.ReadFrom(io.LimitReader(body, size-int64(len(head)))); err != nil {
		return nil, 0, "", err
	}
	if int64(whole.Len()) != size {
		return nil, 0, "", io.ErrUnexpectedEOF
	}
	stripped, err := imagemeta.Strip(format, whole.Bytes())
	if err != nil {
		return nil, 0, "", fmt.Errorf("%w: %w", domain.ErrInvalidParam, err)
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
