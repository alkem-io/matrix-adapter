package service

import (
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// AttachmentToReceivedDTO maps a single inbound domain attachment to the wire
// DTO, translating empty DocumentID/MediaID to nil pointers. This is the single
// source of truth for the domain.Attachment -> dto.ReceivedAttachment mapping;
// both the event publisher and the queue message handlers call it so a future
// field addition only has to change one place.
func AttachmentToReceivedDTO(a domain.Attachment) dto.ReceivedAttachment {
	ra := dto.ReceivedAttachment{
		DisplayName: a.DisplayName,
		MimeType:    a.MimeType,
		Size:        a.Size,
		Width:       a.Width,
		Height:      a.Height,
	}
	if a.DocumentID != "" {
		docID := a.DocumentID
		ra.DocumentID = &docID
	}
	if a.MediaID != "" {
		mediaID := a.MediaID
		ra.MediaID = &mediaID
	}
	return ra
}

// AttachmentsToReceivedDTO maps a slice of inbound domain attachments to the
// wire DTO via AttachmentToReceivedDTO. Returns nil for an empty input so the
// omitempty JSON field is elided rather than serialized as [].
func AttachmentsToReceivedDTO(attachments []domain.Attachment) []dto.ReceivedAttachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]dto.ReceivedAttachment, 0, len(attachments))
	for _, a := range attachments {
		result = append(result, AttachmentToReceivedDTO(a))
	}
	return result
}
