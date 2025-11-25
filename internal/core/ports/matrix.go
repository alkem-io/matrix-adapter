package ports

import (
	"context"

	"github.com/alkemio/matrix-adapter-go/internal/core/domain"
	"maunium.net/go/mautrix/id"
)

// MatrixPort defines the interface for Matrix operations.
type MatrixPort interface {
	// Connection
	Connect(ctx context.Context) error
	Disconnect() error

	// Actor Management
	EnsureUser(ctx context.Context, actorID domain.Actor) (id.UserID, error)

	// Actor Room Operations
	JoinRoom(ctx context.Context, roomID id.RoomID, actorID domain.Actor) error
	LeaveRoom(ctx context.Context, roomID id.RoomID, actorID domain.Actor) error
	GetUserJoinedRooms(ctx context.Context, actorID domain.Actor) ([]id.RoomID, error)

	// Room Management
	CreateRoom(ctx context.Context, actorID domain.Actor, name string, metadata map[string]string) (id.RoomID, error)
	InviteUser(ctx context.Context, roomID id.RoomID, inviterID domain.Actor, inviteeID domain.Actor) error
	InviteUserByID(ctx context.Context, roomID id.RoomID, inviterID domain.Actor, inviteeID id.UserID) error
	ForgetRoom(ctx context.Context, roomID id.RoomID, actorID domain.Actor) error
	GetRoomDetails(ctx context.Context, roomID id.RoomID) (*domain.Room, error)
	GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error)
	UpdateRoomState(ctx context.Context, roomID id.RoomID, actorID domain.Actor, name, topic, alias string) error

	// Messaging
	SendMessage(ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string) (id.EventID, error)
	SendReply(ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, threadID id.EventID) (id.EventID, error)
	RedactEvent(ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, reason string) error
	SendReaction(ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, emoji string) (id.EventID, error)
	GetMessage(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*domain.Message, error)
	GetReactionEventID(ctx context.Context, roomID id.RoomID, eventID id.EventID, emoji string, senderID domain.Actor) (id.EventID, error)

	// Direct Messaging
	CreateDirectRoom(ctx context.Context, initiator domain.Actor, receiver domain.Actor) (id.RoomID, error)
	GetDirectRooms(ctx context.Context, actorID domain.Actor) (map[id.UserID]id.RoomID, error)

	// Admin Operations
	GetAllJoinedRooms(ctx context.Context) ([]id.RoomID, error)

	// Event Listening
	OnMessage(handler func(msg domain.Message) error)
}
