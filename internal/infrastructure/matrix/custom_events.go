package matrix

import (
	"reflect"

	"maunium.net/go/mautrix/event"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// Custom Alkemio state event types registered with mautrix-go's TypeMap
// so the SDK's StateStore can parse them without "unsupported event type" warnings.
var (
	StateAlkemioVisibility = event.Type{Type: "io.alkemio.visibility", Class: event.StateEventType}
	StateAlkemioPending    = event.Type{Type: "io.alkemio.pending", Class: event.StateEventType}
	// StateAlkemioEntity is the platform identity marker of a governed room (data-model E3).
	StateAlkemioEntity = event.Type{Type: "io.alkemio.entity", Class: event.StateEventType}
	// StateAlkemioGovernance is the governance marker of a governed room (data-model E3).
	StateAlkemioGovernance = event.Type{Type: "io.alkemio.governance", Class: event.StateEventType}
)

// AlkemioVisibilityContent is the content of the io.alkemio.visibility state event.
type AlkemioVisibilityContent struct {
	Visible bool `json:"visible"`
}

// AlkemioPendingContent is the content of the io.alkemio.pending state event.
type AlkemioPendingContent struct {
	AlkemioRoomID string `json:"alkemio_room_id"`
}

// AlkemioEntityContent is the content of the io.alkemio.entity state event.
// The canonical struct is domain.EntityMarker; this alias keeps the
// infrastructure naming convention beside the other io.alkemio.* contents.
type AlkemioEntityContent = domain.EntityMarker

// AlkemioGovernanceContent is the content of the io.alkemio.governance state
// event. The canonical struct is domain.GovernanceMarker.
type AlkemioGovernanceContent = domain.GovernanceMarker

func init() {
	event.TypeMap[StateAlkemioVisibility] = reflect.TypeOf(AlkemioVisibilityContent{})
	event.TypeMap[StateAlkemioPending] = reflect.TypeOf(AlkemioPendingContent{})
	event.TypeMap[StateAlkemioEntity] = reflect.TypeOf(AlkemioEntityContent{})
	event.TypeMap[StateAlkemioGovernance] = reflect.TypeOf(AlkemioGovernanceContent{})
}
