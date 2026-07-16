package matrix

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// adminAPI defines the interface for Synapse Admin API operations used by MautrixAdapter.
// This enables unit testing by allowing injection of mock implementations.
type adminAPI interface {
	GetUser(ctx context.Context, userID id.UserID) (*UserInfo, error)
	ListRooms(ctx context.Context, limit int) ([]AdminRoom, error)
	GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]string, error)
	GetRoomMemberIDs(ctx context.Context, roomID id.RoomID) ([]id.UserID, error)
	GetRoomState(ctx context.Context, roomID id.RoomID, eventType string) ([]json.RawMessage, error)
	GetStateEventContent(ctx context.Context, roomID id.RoomID, eventType string) (map[string]interface{}, error)
	GetCustomState(ctx context.Context, roomID id.RoomID, eventTypes []string) (map[string]map[string]interface{}, error)
	GetRoomMessages(ctx context.Context, roomID id.RoomID, from, dir string, limit int) (*mautrix.RespMessages, error)
	GetEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*event.Event, error)
	GetRelations(ctx context.Context, roomID id.RoomID, eventID id.EventID, relType event.RelationType, eventType event.Type) ([]*event.Event, error)
	JoinRoom(ctx context.Context, roomID id.RoomID, userID id.UserID) error
}
