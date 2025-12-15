// Package domain defines the core business entities and value objects.
package domain

import (
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// ReadReceipt represents a user's read status in a room or thread.
type ReadReceipt struct {
	RoomID    id.RoomID   // Matrix Room ID
	UserID    id.UserID   // Matrix User ID (reader)
	EventID   id.EventID  // ID of the last read message
	ThreadID  *id.EventID // Thread root ID (nil for main timeline)
	Timestamp time.Time   // Timestamp of the receipt
}

// UnreadCount represents the number of unread messages for a user.
type UnreadCount struct {
	RoomID      id.RoomID   // Matrix Room ID
	ThreadID    *id.EventID // Thread root ID (nil for main timeline)
	Count       int         // Number of unread messages
	Highlighted bool        // Whether there are mentions/highlights
}

// ReadReceiptEvent represents an incoming read receipt event from Matrix.
type ReadReceiptEvent struct {
	AlkemioRoomID uuid.UUID   // Alkemio room UUID
	UserID        uuid.UUID   // Alkemio actor UUID
	EventID       id.EventID  // Event that was marked as read
	ThreadID      *id.EventID // Thread root ID (nil for main timeline)
	Timestamp     time.Time   // Timestamp of the receipt
}

// MessageEditedEvent represents a message edit event from Matrix.
type MessageEditedEvent struct {
	AlkemioRoomID   uuid.UUID   // Alkemio room UUID
	OriginalEventID id.EventID  // ID of the original message
	NewEventID      id.EventID  // ID of the edit event
	SenderID        uuid.UUID   // Alkemio actor UUID of editor
	NewContent      string      // New message content
	ThreadID        *id.EventID // Thread root ID (nil for main timeline)
	Timestamp       time.Time   // Timestamp of the edit
}

// MessageRedactedEvent represents a message redaction event from Matrix.
type MessageRedactedEvent struct {
	AlkemioRoomID    uuid.UUID   // Alkemio room UUID
	RedactedEventID  id.EventID  // ID of the redacted message
	RedactionEventID id.EventID  // ID of the redaction event
	RedactorID       uuid.UUID   // Alkemio actor UUID of redactor
	Reason           string      // Optional reason for redaction
	ThreadID         *id.EventID // Thread root ID (nil for main timeline)
	Timestamp        time.Time   // Timestamp of the redaction
}

// RoomCreatedEvent represents a room creation event from Matrix.
type RoomCreatedEvent struct {
	AlkemioRoomID uuid.UUID // Alkemio room UUID
	MatrixRoomID  id.RoomID // Matrix room ID
	CreatorID     uuid.UUID // Alkemio actor UUID of creator
	RoomType      string    // Room type: "standard", "direct", "space"
	Name          string    // Room name
	Topic         string    // Room topic
	Timestamp     time.Time // Timestamp of creation
}

// RoomMemberUpdatedEvent represents a membership change event from Matrix.
type RoomMemberUpdatedEvent struct {
	AlkemioRoomID uuid.UUID // Alkemio room UUID
	MemberID      uuid.UUID // Alkemio actor UUID of the member
	SenderID      uuid.UUID // Alkemio actor UUID who performed the action
	Membership    string    // Membership state: "invite", "join", "leave", "ban", "knock"
	Timestamp     time.Time // Timestamp of the change
}

// UnreadCountSummary aggregates unread counts for a room and its threads.
type UnreadCountSummary struct {
	RoomUnreadCount    int                // Count of unread messages in main timeline
	ThreadUnreadCounts map[id.EventID]int // Map of thread root ID to unread count
}
