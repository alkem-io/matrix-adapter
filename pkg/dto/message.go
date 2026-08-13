package dto

// ============================================================================
// New Protocol DTOs (communication.message.*)
// ============================================================================

// MessageDto represents a message in a room.
type MessageDto struct {
	ID            MessageID            `json:"id"`
	Content       string               `json:"content"`
	SenderActorID AlkemioActorID       `json:"sender_actor_id"`
	Timestamp     int64                `json:"timestamp"` // Unix milliseconds
	Reactions     []ReactionDto        `json:"reactions"`
	ThreadID      *MessageID           `json:"thread_id,omitempty"`
	Attachments   []ReceivedAttachment `json:"attachments,omitempty"`
}

// AttachmentRef is a reference to a file-service document to be sent as a
// Matrix media event. The adapter fetches the document bytes, uploads them to
// the homeserver, and embeds the DocumentID as io.alkemio.document_id on the
// outbound event.
type AttachmentRef struct {
	DocumentID  string `json:"document_id"`  // Alkemio file-service document id (conversation bucket)
	DisplayName string `json:"display_name"` // Filename / caption shown in clients
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	Width       *int   `json:"width,omitempty"`
	Height      *int   `json:"height,omitempty"`
}

// ReceivedAttachment is a raw media reference surfaced on an inbound message.
// MediaID is the Synapse media id, used by the server as the re-home key.
//
// DocumentID is set when the event carries io.alkemio.document_id, which marks
// an echo of our own outbound media. It is an UNAUTHENTICATED HINT: the field is
// part of user-writable event content, and the adapter cannot tell its own sends
// from a human's (it impersonates actor X as X — the same MXID the human gets
// from OIDC), so it surfaces the field verbatim for every sender. The consumer
// MUST authorize it before acting on it — the Alkemio server does, requiring the
// named document to sit in the message's room bucket with createdBy == the
// message sender.
//
// The adapter never resolves either ref — the server re-homes and resolves URLs.
type ReceivedAttachment struct {
	DocumentID  *string `json:"document_id,omitempty"`
	MediaID     *string `json:"media_id,omitempty"`
	DisplayName string  `json:"display_name"`
	MimeType    string  `json:"mime_type"`
	Size        int64   `json:"size"`
	Width       *int    `json:"width,omitempty"`
	Height      *int    `json:"height,omitempty"`
}

// SendMessageRequest sends a text message to a room.
// Topic: communication.message.send
type SendMessageRequest struct {
	AlkemioRoomID   AlkemioRoomID   `json:"alkemio_room_id"`
	SenderActorID   AlkemioActorID  `json:"sender_actor_id"`
	Content         string          `json:"content"`                     // Markdown supported (may be empty if only attachments)
	ParentMessageID *MessageID      `json:"parent_message_id,omitempty"` // For threads
	Attachments     []AttachmentRef `json:"attachments,omitempty"`       // Media doc refs (<=10)
}

// SendMessageResponse returns the message ID and timestamp.
type SendMessageResponse struct {
	BaseResponse `tstype:",extends"`
	MessageID    MessageID `json:"message_id"`
	// Timestamp is the approximate creation time (Unix milliseconds).
	// Note: This is set locally when the adapter receives confirmation from Matrix,
	// not the exact server timestamp. The difference should be negligible (<100ms)
	// assuming synchronized clocks (NTP).
	Timestamp int64 `json:"timestamp"`
}

// GetMessageRequest retrieves details of a specific message.
// Topic: communication.message.get
type GetMessageRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	MessageID     MessageID     `json:"message_id"`
}

// GetMessageResponse returns message details.
type GetMessageResponse struct {
	BaseResponse `tstype:",extends"`
	Message      MessageDto `json:"message"`
}

// DeleteMessageRequest redacts/deletes a message.
// Topic: communication.message.delete
type DeleteMessageRequest struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	MessageID     MessageID      `json:"message_id"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Reason        string         `json:"reason,omitempty"`
}

// ============================================================================
// Thread Message DTOs (communication.thread.*)
// ============================================================================

// GetThreadMessagesRequest retrieves messages in a thread.
// Topic: communication.thread.messages.get
type GetThreadMessagesRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	ThreadID      MessageID     `json:"thread_id"`
}

// GetThreadMessagesResponse returns thread messages.
type GetThreadMessagesResponse struct {
	BaseResponse  `tstype:",extends"`
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	ThreadID      MessageID     `json:"thread_id"`
	Messages      []MessageDto  `json:"messages"`
}
