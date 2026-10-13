package matrix

import (
	"context"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// UploadMedia streams one original upload as its sender, without publishing an event.
func (m *MautrixAdapter) UploadMedia(
	ctx context.Context, actor domain.Actor, body io.Reader, size int64, contentType, displayName string,
) (id.ContentURI, error) {
	if actor.ID == uuid.Nil || size < 0 || size > m.cfg.MaxAttachmentBytes() {
		return id.ContentURI{}, fmt.Errorf("%w: invalid actor or media length", domain.ErrInvalidParam)
	}
	userID, err := m.EnsureUser(ctx, actor)
	if err != nil {
		return id.ContentURI{}, err
	}
	request := uploadRequest(body, size, contentType)
	request.FileName = displayName
	// A consumed stream cannot be replayed, even if a transport failure leaves
	// the caller unsure whether Synapse already stored the media.
	uploaded, err := m.as.Intent(userID).UploadMedia(mautrix.WithMaxRetries(ctx, 0), request)
	if err != nil {
		return id.ContentURI{}, fmt.Errorf("media upload failed: %w", err)
	}
	if uploaded == nil || m.idMapper.LocalMediaID(uploaded.ContentURI.String()) == "" {
		return id.ContentURI{}, fmt.Errorf("upload returned no local media reference")
	}
	return uploaded.ContentURI, nil
}

func (m *MautrixAdapter) validateExistingMedia(att domain.Attachment) error {
	_, _, mimeError := mime.ParseMediaType(att.MimeType)
	if att.MediaID == "" || strings.ContainsAny(att.MediaID, "/?# \t\r\n") ||
		strings.TrimSpace(att.DisplayName) == "" || mimeError != nil ||
		att.Size < 0 || att.Size > m.cfg.MaxAttachmentBytes() ||
		(att.Width == nil) != (att.Height == nil) ||
		(att.Width != nil && (*att.Width <= 0 || *att.Height <= 0)) {
		return fmt.Errorf("%w: invalid existing media descriptor", domain.ErrInvalidParam)
	}
	return nil
}
