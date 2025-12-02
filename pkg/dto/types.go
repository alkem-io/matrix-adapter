// Package dto defines public Data Transfer Objects for the RabbitMQ protocol.
package dto

import (
	"github.com/google/uuid"
)

// AlkemioRoomID is a UUID v4 or v7 identifying a room in Alkemio.
// The adapter maps this to a Matrix room ID via room alias.
type AlkemioRoomID uuid.UUID

// MarshalJSON implements json.Marshaler for AlkemioRoomID.
func (r AlkemioRoomID) MarshalJSON() ([]byte, error) {
	return uuid.UUID(r).MarshalText()
}

// UnmarshalJSON implements json.Unmarshaler for AlkemioRoomID.
func (r *AlkemioRoomID) UnmarshalJSON(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data[1 : len(data)-1]); err != nil { // Strip quotes
		return err
	}
	*r = AlkemioRoomID(u)
	return nil
}

// String returns the string representation of the AlkemioRoomID.
func (r AlkemioRoomID) String() string {
	return uuid.UUID(r).String()
}

// UUID returns the underlying uuid.UUID value.
func (r AlkemioRoomID) UUID() uuid.UUID {
	return uuid.UUID(r)
}

// AlkemioActorID is a UUID v4 or v7 identifying an actor (user or bot) in Alkemio.
// The adapter maps this to a Matrix user ID.
type AlkemioActorID uuid.UUID

// MarshalJSON implements json.Marshaler for AlkemioActorID.
func (a AlkemioActorID) MarshalJSON() ([]byte, error) {
	return uuid.UUID(a).MarshalText()
}

// UnmarshalJSON implements json.Unmarshaler for AlkemioActorID.
func (a *AlkemioActorID) UnmarshalJSON(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data[1 : len(data)-1]); err != nil { // Strip quotes
		return err
	}
	*a = AlkemioActorID(u)
	return nil
}

// String returns the string representation of the AlkemioActorID.
func (a AlkemioActorID) String() string {
	return uuid.UUID(a).String()
}

// UUID returns the underlying uuid.UUID value.
func (a AlkemioActorID) UUID() uuid.UUID {
	return uuid.UUID(a)
}

// MessageID is an opaque string identifier for a message.
// Internally maps to Matrix Event ID.
type MessageID string

// String returns the string representation of the MessageID.
func (m MessageID) String() string {
	return string(m)
}

// ReactionID is an opaque string identifier for a reaction.
// Internally maps to Matrix Event ID.
type ReactionID string

// String returns the string representation of the ReactionID.
func (r ReactionID) String() string {
	return string(r)
}

// RoomType defines the type of room to create.
type RoomType string

const (
	// RoomTypeCommunity is a general community room.
	RoomTypeCommunity RoomType = "community"
	// RoomTypeDirect is a direct message room between exactly 2 actors.
	RoomTypeDirect RoomType = "direct"
)
