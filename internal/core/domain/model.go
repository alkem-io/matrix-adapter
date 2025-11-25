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
}

// Room represents a Matrix room.
type Room struct {
	ID    id.RoomID
	Alias string
	Name  string
	Topic string
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
}
