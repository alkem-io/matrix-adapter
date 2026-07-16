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

// NewActor creates a new Actor with the given Alkemio actor ID.
func NewActor(id uuid.UUID) Actor {
	return Actor{ID: id}
}

// Room represents a Matrix room.
type Room struct {
	ID          id.RoomID
	AlkemioID   uuid.UUID // Alkemio room UUID
	Alias       string
	Name        string
	Topic       string
	AvatarURL   string
	CustomState map[string]map[string]interface{} // io.alkemio.* state events
	Type        string                            // "community" or "direct"
	MemberIDs   []uuid.UUID                       // Alkemio actor IDs
	Messages    []Message                         // For room.get operations
}

// RoomWithReadState extends Room with user-specific read receipt information.
// Used by GetRoomAsUser to return room details from a specific user's perspective.
type RoomWithReadState struct {
	Room            *Room
	LastReadEventID string // The last message ID the user has read
	LastReadTS      int64  // Timestamp of last read (Unix millis) for comparison
	UnreadCount     int    // Number of unread messages
}

// Space represents a Matrix Space (MSC1772) mapped to an Alkemio context.
type Space struct {
	ID               id.RoomID
	AlkemioContextID uuid.UUID
	Name             string
	Topic            string
	AvatarURL        string
	Alias            string
	JoinRule         string
	CustomState      map[string]map[string]interface{} // io.alkemio.* state events
	MemberIDs        []uuid.UUID
	Children         []SpaceChild
	ParentContextID  *uuid.UUID
}

// SpaceChild represents a child room or subspace within a space.
type SpaceChild struct {
	ChildID   string // Matrix room ID of child
	IsSpace   bool
	Order     string
	Suggested bool
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
	ThreadID       string       // Parent message ID for threads
	Reactions      []Reaction   // Reactions to this message
	Attachments    []Attachment // Media attachments on this message
}

// Attachment is a media reference carried on a message, in either direction.
//
//   - Outbound (web→Matrix): DocumentID is the file-service document whose bytes
//     are uploaded to Synapse; MediaID is unset.
//   - Inbound (Matrix→web): MediaID is the Synapse media id parsed from the
//     event's mxc:// URL; DocumentID is set only when the event carries an
//     io.alkemio.document_id field (an echo of our own outbound media).
//
// The adapter is stateless: it never resolves these refs to URLs or buckets.
type Attachment struct {
	DocumentID  string
	MediaID     string
	DisplayName string
	MimeType    string
	// Size is the caller-declared byte size. It is informational only and is
	// deliberately NOT used to build an outbound event's info.size — that uses
	// the actual streamed byte count (see sendAttachment), so a mis-declared size
	// can never produce an event that lies about its blob length.
	Size   int64
	Width  *int
	Height *int
}

// Reaction represents a reaction event.
type Reaction struct {
	ID             id.EventID
	RoomID         id.RoomID
	MessageID      id.EventID
	Emoji          string
	SenderID       uuid.UUID
	SenderMatrixID string // Matrix User ID for conversion
	Timestamp      time.Time
}

// ReactionEvent represents an incoming reaction event from Matrix.
type ReactionEvent struct {
	AlkemioRoomID uuid.UUID
	MessageID     id.EventID
	ReactionID    id.EventID
	Emoji         string
	SenderActorID uuid.UUID
	Timestamp     time.Time
}

// ReactionRemovedEvent represents a reaction removal event from Matrix.
type ReactionRemovedEvent struct {
	AlkemioRoomID uuid.UUID
	MessageID     id.EventID
	ReactionID    id.EventID
	Emoji         string
	SenderActorID uuid.UUID
	Timestamp     time.Time
}

// RoomUpdatedEvent represents a room property change event from Matrix.
// Only populated fields represent changes; nil means "property unchanged".
type RoomUpdatedEvent struct {
	AlkemioRoomID uuid.UUID
	DisplayName   *string
	AvatarURL     *string
	Topic         *string
	Timestamp     time.Time
}

// SpaceUpdatedEvent represents a space property change event from Matrix.
// Only populated fields represent changes; nil means "property unchanged".
type SpaceUpdatedEvent struct {
	AlkemioContextID uuid.UUID
	DisplayName      *string
	AvatarURL        *string
	Topic            *string
	Timestamp        time.Time
}

// MembershipEvent represents a membership change event from Matrix.
type MembershipEvent struct {
	AlkemioRoomID uuid.UUID
	ActorID       uuid.UUID
	Reason        string
	Timestamp     time.Time
}
