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

// ReceivedAttachment is the media reference from a sent or received Matrix event.
// DocumentID is a user-writable hint. The server checks the conversation bucket,
// document policy and content hash against the provider row before reusing it.
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
	TimeoutMS       int64           `json:"timeout_ms,omitempty"`        // Operation budget, shorter than the caller RPC wait.
	Attachments     []AttachmentRef `json:"attachments,omitempty"`       // One media doc ref; mutually exclusive with content
}

// SendMessageResponse returns the message ID and timestamp.
type SendMessageResponse struct {
	BaseResponse `tstype:",extends"`
	MessageID    MessageID            `json:"message_id"`
	Content      string               `json:"content"`
	Attachments  []ReceivedAttachment `json:"attachments,omitempty"`
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
