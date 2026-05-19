//nolint:revive // methods on unexported wrapper must match interface names
package matrix

import (
	"context"
	"net/http"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// intentAPI abstracts the mautrix-go IntentAPI operations used by MautrixAdapter.
// It includes methods defined on IntentAPI itself and frequently used methods
// inherited from the embedded *mautrix.Client. This enables unit testing by
// allowing injection of mock implementations.
type intentAPI interface {
	// --- IntentAPI own methods ---
	EnsureRegistered(ctx context.Context) error
	EnsureJoined(ctx context.Context, roomID id.RoomID, extra ...appservice.EnsureJoinedParams) error
	SendMessageEvent(ctx context.Context, roomID id.RoomID, eventType event.Type, contentJSON any, extra ...mautrix.ReqSendEvent) (*mautrix.RespSendEvent, error)
	SendStateEvent(ctx context.Context, roomID id.RoomID, eventType event.Type, stateKey string, contentJSON any, extra ...mautrix.ReqSendEvent) (*mautrix.RespSendEvent, error)
	State(ctx context.Context, roomID id.RoomID) (mautrix.RoomStateMap, error)

	// --- IntentAPI overrides of *mautrix.Client methods ---
	// These match IntentAPI signatures, which add extra variadic params.
	SetDisplayName(ctx context.Context, displayName string) error
	SetAvatarURL(ctx context.Context, avatarURL id.ContentURI) error
	SendText(ctx context.Context, roomID id.RoomID, text string) (*mautrix.RespSendEvent, error)
	RedactEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID, extra ...mautrix.ReqRedact) (*mautrix.RespSendEvent, error)
	CreateRoom(ctx context.Context, req *mautrix.ReqCreateRoom) (*mautrix.RespCreateRoom, error)
	JoinedRooms(ctx context.Context) (*mautrix.RespJoinedRooms, error)
	LeaveRoom(ctx context.Context, roomID id.RoomID, extra ...interface{}) (*mautrix.RespLeaveRoom, error)
	InviteUser(ctx context.Context, roomID id.RoomID, req *mautrix.ReqInviteUser, extraContent ...map[string]interface{}) (*mautrix.RespInviteUser, error)
	KickUser(ctx context.Context, roomID id.RoomID, req *mautrix.ReqKickUser, extraContent ...map[string]interface{}) (*mautrix.RespKickUser, error)
	ResolveAlias(ctx context.Context, alias id.RoomAlias) (*mautrix.RespAliasResolve, error)
	GetAliases(ctx context.Context, roomID id.RoomID) (*mautrix.RespAliasList, error)
	CreateAlias(ctx context.Context, alias id.RoomAlias, roomID id.RoomID) (*mautrix.RespAliasCreate, error)
	DeleteAlias(ctx context.Context, alias id.RoomAlias) (*mautrix.RespAliasDelete, error)

	// --- Inherited from *mautrix.Client (not overridden by IntentAPI) ---
	SendReceipt(ctx context.Context, roomID id.RoomID, eventID id.EventID, receiptType event.ReceiptType, content interface{}) error
	SetReadMarkers(ctx context.Context, roomID id.RoomID, content interface{}) error
	GetAccountData(ctx context.Context, name string, output interface{}) error
	SetAccountData(ctx context.Context, name string, data interface{}) error
	GetRoomAccountData(ctx context.Context, roomID id.RoomID, name string, output interface{}) error
	Messages(ctx context.Context, roomID id.RoomID, from, to string, dir mautrix.Direction, filter *mautrix.FilterPart, limit int) (*mautrix.RespMessages, error)
	CreateFilter(ctx context.Context, filter *mautrix.Filter) (*mautrix.RespCreateFilter, error)
	SyncRequest(ctx context.Context, timeout int, since, filterID string, fullState bool, setPresence event.Presence) (*mautrix.RespSync, error)
	GetEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*event.Event, error)
	Whoami(ctx context.Context) (*mautrix.RespWhoami, error)
	BuildClientURL(urlPath ...any) string
	MakeRequest(ctx context.Context, method string, httpURL string, reqBody any, resBody any) ([]byte, error)
}

// appserviceAPI abstracts the mautrix-go AppService used by MautrixAdapter.
// Field accesses (HomeserverDomain, Host, Router, Events) are exposed as
// getter methods on the wrapper.
type appserviceAPI interface {
	BotIntent() intentAPI
	BotMXID() id.UserID
	Intent(userID id.UserID) intentAPI
	Start()
	Stop()
	HomeserverDomain() string
	Host() *appservice.HostConfig
	Router() *http.ServeMux
	Events() <-chan *event.Event
	// SetMembership updates the StateStore cache for a user's membership in a room.
	// Must be called after admin.JoinRoom to prevent EnsureJoined from creating duplicate joins.
	SetMembership(ctx context.Context, roomID id.RoomID, userID id.UserID, membership event.Membership) error
}

// appserviceWrapper wraps *appservice.AppService to satisfy appserviceAPI.
// It bridges the gap between concrete return types (*appservice.IntentAPI)
// and the intentAPI interface.
type appserviceWrapper struct {
	as *appservice.AppService
}

var _ appserviceAPI = (*appserviceWrapper)(nil)
var _ intentAPI = (*appservice.IntentAPI)(nil)

func (w *appserviceWrapper) BotIntent() intentAPI {
	return w.as.BotIntent()
}

func (w *appserviceWrapper) BotMXID() id.UserID {
	return w.as.BotMXID()
}

func (w *appserviceWrapper) Intent(userID id.UserID) intentAPI {
	return w.as.Intent(userID)
}

func (w *appserviceWrapper) Start() {
	w.as.Start()
}

func (w *appserviceWrapper) Stop() {
	w.as.Stop()
}

func (w *appserviceWrapper) HomeserverDomain() string {
	return w.as.HomeserverDomain
}

func (w *appserviceWrapper) Host() *appservice.HostConfig {
	return &w.as.Host
}

func (w *appserviceWrapper) Router() *http.ServeMux {
	return w.as.Router
}

func (w *appserviceWrapper) Events() <-chan *event.Event {
	return w.as.Events
}

func (w *appserviceWrapper) SetMembership(ctx context.Context, roomID id.RoomID, userID id.UserID, membership event.Membership) error {
	return w.as.StateStore.SetMembership(ctx, roomID, userID, membership)
}
