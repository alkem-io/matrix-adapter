// Package dto defines public Data Transfer Objects for the RabbitMQ protocol.
package dto

import (
	"github.com/google/uuid"
)

// AlkemioRoomID is a UUID v4 or v7 identifying a room in Alkemio.
// The adapter maps this to a Matrix room ID via room alias.
type AlkemioRoomID uuid.UUID

// MarshalText implements encoding.TextMarshaler for AlkemioRoomID.
// This is automatically used by encoding/json for JSON marshaling.
func (r AlkemioRoomID) MarshalText() ([]byte, error) {
	return uuid.UUID(r).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler for AlkemioRoomID.
// This is automatically used by encoding/json for JSON unmarshaling.
func (r *AlkemioRoomID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
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

// MarshalText implements encoding.TextMarshaler for AlkemioActorID.
// This is automatically used by encoding/json for JSON marshaling.
func (a AlkemioActorID) MarshalText() ([]byte, error) {
	return uuid.UUID(a).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler for AlkemioActorID.
// This is automatically used by encoding/json for JSON unmarshaling.
func (a *AlkemioActorID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
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

// AlkemioContextID is a UUID v4 or v7 identifying a context (Space/Subspace) in Alkemio.
// The adapter maps this to a Matrix Space room.
type AlkemioContextID uuid.UUID

// MarshalText implements encoding.TextMarshaler for AlkemioContextID.
// This is automatically used by encoding/json for JSON marshaling.
func (c AlkemioContextID) MarshalText() ([]byte, error) {
	return uuid.UUID(c).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler for AlkemioContextID.
// This is automatically used by encoding/json for JSON unmarshaling.
func (c *AlkemioContextID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}
	*c = AlkemioContextID(u)
	return nil
}

// String returns the string representation of the AlkemioContextID.
func (c AlkemioContextID) String() string {
	return uuid.UUID(c).String()
}

// UUID returns the underlying uuid.UUID value.
func (c AlkemioContextID) UUID() uuid.UUID {
	return uuid.UUID(c)
}

// RoomType defines the type of room to create.
type RoomType string

const (
	// RoomTypeCommunity is a general community room.
	RoomTypeCommunity RoomType = "community"
	// RoomTypeDirect is a direct message room between exactly 2 actors.
	RoomTypeDirect RoomType = "direct"
)

// JoinRule defines access control for rooms and spaces.
type JoinRule string

const (
	// JoinRulePublic allows anyone to join.
	JoinRulePublic JoinRule = "public"
	// JoinRuleInvite requires an invitation to join.
	JoinRuleInvite JoinRule = "invite"
	// JoinRuleRestricted allows members of parent space to join (MSC3083).
	JoinRuleRestricted JoinRule = "restricted"
)
