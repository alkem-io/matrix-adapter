// Package domain defines the core business entities and value objects.
package domain

import (
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// Actor represents a user or agent in the system.
type Actor struct {
	ID          uuid.UUID
	MatrixID    id.UserID
	DisplayName string
	AvatarURL   string
}

// Room represents a Matrix room.
type Room struct {
	ID        id.RoomID
	AlkemioID uuid.UUID // Alkemio room UUID
	Alias     string
	Name      string
	Topic     string
	Type      string      // "community" or "direct"
	MemberIDs []uuid.UUID // Alkemio actor IDs
	Messages  []Message   // For room.get operations
}

// Message represents a chat event.
type Message struct {
	ID             string
	RoomID         string // Matrix Room ID
	RoomName       string
	SenderID       uuid.UUID
	SenderMatrixID string // Matrix User ID
	Content        string
	Timestamp      time.Time
	ThreadID       string     // Parent message ID for threads
	Reactions      []Reaction // Reactions to this message
}

// Reaction represents a reaction event.
type Reaction struct {
	ID        id.EventID
	RoomID    id.RoomID
	MessageID id.EventID
	Emoji     string
	SenderID  uuid.UUID
	Timestamp time.Time
}
