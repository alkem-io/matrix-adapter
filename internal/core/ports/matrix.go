package ports

import (
	"context"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// MatrixPort defines the interface for Matrix operations.
type MatrixPort interface {
	// Connection
	Connect(ctx context.Context) error
	Disconnect() error

	// HomeserverDomain returns the homeserver domain for room alias construction.
	HomeserverDomain() string

	// Actor Management
	EnsureUser(ctx context.Context, actorID domain.Actor) (id.UserID, error)
	SetUserProfile(ctx context.Context, actorID domain.Actor) error

	// Room Management
	CreateRoomWithAlias(ctx context.Context, alkemioRoomID uuid.UUID, roomType string, name, topic string, initialMembers []domain.Actor) (id.RoomID, error)
	InviteUser(ctx context.Context, roomID id.RoomID, inviterID domain.Actor, inviteeID domain.Actor) error
	GetRoomDetails(ctx context.Context, roomID id.RoomID) (*domain.Room, error)
	GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error)
	UpdateRoomState(ctx context.Context, roomID id.RoomID, actorID domain.Actor, name, topic, alias string) error

	// Room Alias Operations
	ResolveAlias(ctx context.Context, alias string) (id.RoomID, error)
	DeleteAlias(ctx context.Context, alias string) error
	KickUser(ctx context.Context, roomID id.RoomID, userID id.UserID, reason string) error

	// Messaging
	SendMessage(ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string) (id.EventID, error)
	SendReply(
		ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, threadID id.EventID,
	) (id.EventID, error)
	RedactEvent(ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, reason string) error
	SendReaction(
		ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, emoji string,
	) (id.EventID, error)
	GetMessage(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*domain.Message, error)
	GetRoomMessages(ctx context.Context, roomID id.RoomID) ([]domain.Message, error)
	GetReactionEventID(
		ctx context.Context, roomID id.RoomID, eventID id.EventID, emoji string, senderID domain.Actor,
	) (id.EventID, error)
	GetReaction(ctx context.Context, roomID id.RoomID, reactionID id.EventID) (*domain.Reaction, error)

	// Admin Operations
	GetAllJoinedRooms(ctx context.Context) ([]id.RoomID, error)

	// ============================================================================
	// Space Operations (MSC1772)
	// ============================================================================

	// CreateSpace creates a Matrix Space room with the given parameters.
	CreateSpace(ctx context.Context, alkemioContextID uuid.UUID, name, topic, avatarURL string, joinRule string, initialMembers []domain.Actor) (id.RoomID, error)

	// GetSpaceDetails retrieves space metadata and state.
	GetSpaceDetails(ctx context.Context, roomID id.RoomID) (*domain.Space, error)

	// GetSpaceMembers returns the list of members in a space.
	GetSpaceMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error)

	// UpdateSpaceState updates space name, topic, avatar, or join rule.
	UpdateSpaceState(ctx context.Context, roomID id.RoomID, name, topic, avatarURL, joinRule string) error

	// GetSpaceChildren returns child rooms and subspaces of a space.
	GetSpaceChildren(ctx context.Context, roomID id.RoomID) ([]domain.SpaceChild, error)

	// AddSpaceChild adds a room or subspace as a child of a space.
	AddSpaceChild(ctx context.Context, spaceID id.RoomID, childID id.RoomID, order string, suggested bool) error

	// SetSpaceParent sets the parent space for a room or subspace (m.space.parent state event).
	SetSpaceParent(ctx context.Context, childID id.RoomID, parentID id.RoomID) error

	// InviteToSpace invites a user to a space.
	InviteToSpace(ctx context.Context, spaceID id.RoomID, inviteeID domain.Actor) error

	// KickFromSpace kicks a user from a space.
	KickFromSpace(ctx context.Context, spaceID id.RoomID, userID id.UserID, reason string) error

	// Event Listening
	OnMessage(handler func(msg domain.Message) error)
}
