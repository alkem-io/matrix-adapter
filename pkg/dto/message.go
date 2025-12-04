package dto

import "time"

// ============================================================================
// New Protocol DTOs (communication.message.*)
// ============================================================================

// MessageDto represents a message in a room.
type MessageDto struct {
	ID            MessageID      `json:"id"`
	Content       string         `json:"content"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Timestamp     time.Time      `json:"timestamp"`
	Reactions     []ReactionDto  `json:"reactions"`
	ThreadID      *MessageID     `json:"thread_id,omitempty"`
}

// SendMessageRequest sends a text message to a room.
// Topic: communication.message.send
type SendMessageRequest struct {
	AlkemioRoomID   AlkemioRoomID  `json:"alkemio_room_id"`
	SenderActorID   AlkemioActorID `json:"sender_actor_id"`
	Content         string         `json:"content"`                     // Markdown supported
	ParentMessageID *MessageID     `json:"parent_message_id,omitempty"` // For threads
}

// SendMessageResponse returns the message ID and timestamp.
type SendMessageResponse struct {
	BaseResponse `tstype:",extends"`
	MessageID    MessageID `json:"message_id"`
	Timestamp    time.Time `json:"timestamp"` // UTC time from Matrix
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
