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

// ExistingMediaRef publishes an already-uploaded local Matrix file without
// fetching a document or uploading bytes a second time.
type ExistingMediaRef struct {
	MediaID     string `json:"media_id"`
	DisplayName string `json:"display_name"`
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	Width       *int   `json:"width,omitempty"`
	Height      *int   `json:"height,omitempty"`
}

// ReceivedAttachment is the media reference from a sent or received Matrix event.
type ReceivedAttachment struct {
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
	AlkemioRoomID   AlkemioRoomID     `json:"alkemio_room_id"`
	SenderActorID   AlkemioActorID    `json:"sender_actor_id"`
	Content         string            `json:"content"`                     // Markdown supported (may be empty if only attachments)
	ParentMessageID *MessageID        `json:"parent_message_id,omitempty"` // For threads
	TimeoutMS       int64             `json:"timeout_ms,omitempty"`        // Operation budget, shorter than the caller RPC wait.
	ExistingMedia   *ExistingMediaRef `json:"existing_media,omitempty"`    // May accompany content, which becomes its caption.
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
