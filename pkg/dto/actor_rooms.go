package dto

// ActorAddToRoomsPayload represents the payload for adding an actor to multiple rooms.
type ActorAddToRoomsPayload struct {
	BaseMatrixAdapterEventPayload
	RoomIDs []string `json:"roomIDs"`
	ActorID string   `json:"actorID"`
}

// ActorAddToRoomsResponsePayload represents the response for adding an actor to rooms.
type ActorAddToRoomsResponsePayload struct {
	BaseResponse
	FailedRooms  []string `json:"failedRooms,omitempty"`
	CreatedRooms []string `json:"createdRooms,omitempty"` // If rooms were created on the fly? Unlikely, but keeping generic.
}

// ActorRemoveFromRoomsPayload represents the payload for removing an actor from multiple rooms.
type ActorRemoveFromRoomsPayload struct {
	BaseMatrixAdapterEventPayload
	RoomIDs []string `json:"roomIDs"`
	ActorID string   `json:"actorID"`
}

// ActorRemoveFromRoomsResponsePayload represents the response for removing an actor from rooms.
type ActorRemoveFromRoomsResponsePayload struct {
	BaseResponse
	FailedRooms []string `json:"failedRooms,omitempty"`
}

// ActorRoomsPayload represents the request to list rooms for an actor.
type ActorRoomsPayload struct {
	BaseMatrixAdapterEventPayload
	ActorID string `json:"actorID"`
}

// ActorRoomsResponsePayload represents the list of rooms an actor is in.
type ActorRoomsResponsePayload struct {
	BaseResponse
	RoomIDs []string `json:"roomIDs"`
}
