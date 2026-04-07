package matrix

import (
	"reflect"

	"maunium.net/go/mautrix/event"
)

// Custom Alkemio state event types registered with mautrix-go's TypeMap
// so the SDK's StateStore can parse them without "unsupported event type" warnings.
var (
	StateAlkemioVisibility = event.Type{Type: "io.alkemio.visibility", Class: event.StateEventType}
)

// AlkemioVisibilityContent is the content of the io.alkemio.visibility state event.
type AlkemioVisibilityContent struct {
	Visible bool `json:"visible"`
}

func init() {
	event.TypeMap[StateAlkemioVisibility] = reflect.TypeOf(AlkemioVisibilityContent{})
}
