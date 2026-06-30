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
// DocumentID is set when the event carries io.alkemio.document_id (an echo of
// our own outbound media); MediaID is set for Element-origin media (the Synapse
// media id, used by the server as the re-home key). The adapter never resolves
// either — the server re-homes and resolves URLs.
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
	// IdempotencyKey, when set, makes the send safe to retry: the adapter derives
	// a deterministic Matrix transaction ID per emitted event from this key, so a
	// retry with the same key is de-duplicated by the homeserver instead of
	// producing duplicate text/attachment events. It must be unique per logical
	// send and STABLE across retries of that same send (a request UUID minted by
	// the caller). When empty, sends are at-most-once: a partially-failed
	// multi-event send may duplicate already-delivered events if retried.
	IdempotencyKey string `json:"idempotency_key,omitempty"`
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
