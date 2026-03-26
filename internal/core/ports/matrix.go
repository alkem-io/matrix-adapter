package ports

import (
	"context"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
)

// MatrixPort defines the interface for Matrix operations.
type MatrixPort interface {
	// Connect establishes a connection to the Matrix homeserver.
	Connect(ctx context.Context) error
	// Disconnect terminates the connection to the Matrix homeserver.
	Disconnect() error

	// HomeserverDomain returns the homeserver domain for room alias construction.
	HomeserverDomain() string

	// EnsureUser ensures that a Matrix user exists for the given actor, creating if necessary.
	EnsureUser(ctx context.Context, actorID domain.Actor) (id.UserID, error)
	// SetUserProfile updates the display name and avatar for a Matrix user.
	SetUserProfile(ctx context.Context, actorID domain.Actor) error

	// CreateRoomWithAlias creates a new Matrix room with the specified alias and initial members.
	CreateRoomWithAlias(ctx context.Context, alkemioRoomID uuid.UUID, roomType string, name, topic, avatarURL, joinRule string, initialMembers []domain.Actor) (id.RoomID, error)
	// InviteUser invites a user to a Matrix room on behalf of another user.
	InviteUser(ctx context.Context, roomID id.RoomID, inviterID domain.Actor, inviteeID domain.Actor) error
	// GetRoomDetails retrieves room metadata including name, topic, and state.
	GetRoomDetails(ctx context.Context, roomID id.RoomID) (*domain.Room, error)
	// GetRoomMembers returns the list of member user IDs in a room.
	GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error)
	// UpdateRoomState updates a room's name, topic, avatar, join rule, and canonical alias.
	// nil pointers mean "no change"; non-nil (including empty string) means "set this value".
	UpdateRoomState(ctx context.Context, roomID id.RoomID, actorID domain.Actor, name, topic, avatarURL, joinRule *string) error

	// SetRoomDirectoryVisibility sets whether a room appears in the public room directory.
	SetRoomDirectoryVisibility(ctx context.Context, roomID id.RoomID, isPublic bool) error
	// SetCustomState sets custom io.alkemio.* state events on a room.
	SetCustomState(ctx context.Context, roomID id.RoomID, state map[string]map[string]interface{}) error
	// GetCustomState retrieves io.alkemio.* state events from a room.
	// If eventTypes is empty, returns all io.alkemio.* state events.
	GetCustomState(ctx context.Context, roomID id.RoomID, eventTypes []string) (map[string]map[string]interface{}, error)

	// ResolveAlias resolves a room alias to a room ID.
	ResolveAlias(ctx context.Context, alias string) (id.RoomID, error)
	// DeleteAlias removes a room alias from the homeserver.
	DeleteAlias(ctx context.Context, alias string) error
	// KickUser removes a user from a room with the specified reason.
	KickUser(ctx context.Context, roomID id.RoomID, userID id.UserID, reason string) error

	// SendMessage sends a text message to a room on behalf of a user.
	SendMessage(ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string) (id.EventID, error)
	// SendReply sends a threaded reply to an existing message.
	SendReply(
		ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, threadID id.EventID,
	) (id.EventID, error)
	// RedactEvent deletes an event from a room.
	RedactEvent(ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, reason string) error
	// SendReaction adds an emoji reaction to an event.
	SendReaction(
		ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, emoji string,
	) (id.EventID, error)
	// GetMessage retrieves a single message event from a room.
	GetMessage(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*domain.Message, error)
	// GetRoomMessages retrieves all messages from a room.
	GetRoomMessages(ctx context.Context, roomID id.RoomID) ([]domain.Message, error)
	// GetLastMessage retrieves the most recent message in a room.
	// Returns nil if the room has no messages.
	GetLastMessage(ctx context.Context, roomID id.RoomID) (*domain.Message, error)
	// GetBatchLastMessages retrieves the most recent message for multiple rooms in parallel.
	// Returns a map of roomID to message (nil if no messages), and a map of roomIDs that had errors.
	GetBatchLastMessages(ctx context.Context, roomIDs []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error)
	// GetReactionEventID finds the event ID of a specific reaction by a user.
	GetReactionEventID(
		ctx context.Context, roomID id.RoomID, eventID id.EventID, emoji string, senderID domain.Actor,
	) (id.EventID, error)
	// GetReaction retrieves a reaction event by its ID.
	GetReaction(ctx context.Context, roomID id.RoomID, reactionID id.EventID) (*domain.Reaction, error)

	// GetThreadMessages retrieves all messages in a thread including the thread root.
	GetThreadMessages(ctx context.Context, roomID id.RoomID, threadRootID id.EventID) ([]domain.Message, error)

	// FindExistingDirectRoom finds an existing direct room between two users.
	// Returns the room ID if found, or empty string if no direct room exists.
	FindExistingDirectRoom(ctx context.Context, user1 domain.Actor, user2 domain.Actor) (id.RoomID, error)

	// SetRoomAlias sets a room alias for an existing room.
	SetRoomAlias(ctx context.Context, roomID id.RoomID, alias string) error

	// GetAllJoinedRooms returns all rooms the appservice bot has joined.
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
	// nil pointers mean "no change"; non-nil (including empty string) means "set this value".
	UpdateSpaceState(ctx context.Context, roomID id.RoomID, name, topic, avatarURL, joinRule *string) error

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

	// ============================================================================
	// Read Receipt Operations (008-read-receipts)
	// ============================================================================

	// SendReadReceipt sends a read receipt for a message in a room.
	// If threadRootID is provided, sends an m.read.thread receipt for thread-level tracking.
	// Otherwise, sends a standard m.read receipt for room-level tracking.
	SendReadReceipt(ctx context.Context, actorID domain.Actor, roomID id.RoomID, eventID id.EventID, threadRootID *id.EventID) error

	// GetUnreadCounts retrieves unread message counts for a room and optionally specific threads.
	// Returns room-level unread count and per-thread unread counts for any specified threadRootIDs.
	GetUnreadCounts(ctx context.Context, actorID domain.Actor, roomID id.RoomID, threadRootIDs []id.EventID) (*domain.UnreadCountSummary, error)

	// GetBatchUnreadCounts retrieves unread counts for multiple rooms in a single sync call.
	// Returns a map of roomID to unread count, and a map of roomIDs that had errors.
	GetBatchUnreadCounts(ctx context.Context, actorID domain.Actor, roomIDs []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error)
}
