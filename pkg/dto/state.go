package dto

// ============================================================================
// Custom State API (io.alkemio.* state events)
// ============================================================================

// SetRoomStateRequest sets custom io.alkemio.* state events on a room.
// Topic: communication.room.state.set
type SetRoomStateRequest struct {
	AlkemioRoomID AlkemioRoomID                     `json:"alkemio_room_id"`
	State         map[string]map[string]interface{} `json:"state"` // e.g. {"io.alkemio.visibility": {"visible": true}}
}

// GetRoomStateRequest retrieves custom io.alkemio.* state events from a room.
// Topic: communication.room.state.get
type GetRoomStateRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	EventTypes    []string      `json:"event_types,omitempty"` // Filter to specific types; empty = all io.alkemio.*
}

// GetRoomStateResponse returns custom state events.
type GetRoomStateResponse struct {
	BaseResponse  `tstype:",extends"`
	AlkemioRoomID AlkemioRoomID                     `json:"alkemio_room_id"`
	State         map[string]map[string]interface{} `json:"state"`
}

// SetSpaceStateRequest sets custom io.alkemio.* state events on a space.
// Topic: communication.space.state.set
type SetSpaceStateRequest struct {
	AlkemioContextID AlkemioContextID                  `json:"alkemio_context_id"`
	State            map[string]map[string]interface{} `json:"state"`
}

// GetSpaceStateRequest retrieves custom io.alkemio.* state events from a space.
// Topic: communication.space.state.get
type GetSpaceStateRequest struct {
	AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
	EventTypes       []string         `json:"event_types,omitempty"`
}

// GetSpaceStateResponse returns custom state events.
type GetSpaceStateResponse struct {
	BaseResponse     `tstype:",extends"`
	AlkemioContextID AlkemioContextID                  `json:"alkemio_context_id"`
	State            map[string]map[string]interface{} `json:"state"`
}
