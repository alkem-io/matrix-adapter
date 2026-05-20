package matrix

import (
	"reflect"

	"maunium.net/go/mautrix/event"
)

// Custom Alkemio state event types registered with mautrix-go's TypeMap
// so the SDK's StateStore can parse them without "unsupported event type" warnings.
var (
	StateAlkemioVisibility = event.Type{Type: "io.alkemio.visibility", Class: event.StateEventType}
	StateAlkemioPending    = event.Type{Type: "io.alkemio.pending", Class: event.StateEventType}
)

// AlkemioVisibilityContent is the content of the io.alkemio.visibility state event.
type AlkemioVisibilityContent struct {
	Visible bool `json:"visible"`
}

// AlkemioPendingContent is the content of the io.alkemio.pending state event.
type AlkemioPendingContent struct {
	AlkemioRoomID string `json:"alkemio_room_id"`
}

func init() {
	event.TypeMap[StateAlkemioVisibility] = reflect.TypeOf(AlkemioVisibilityContent{})
	event.TypeMap[StateAlkemioPending] = reflect.TypeOf(AlkemioPendingContent{})
}
