package dto

// ActorAddToRoomsPayload represents the payload for adding an actor to multiple rooms.
type ActorAddToRoomsPayload struct {
	BaseMatrixAdapterEventPayload
	RoomIDs []string `json:"roomIDs"`
	ActorID string   `json:"actorID"`
}

// ActorAddToRoomsResponse represents the response for adding an actor to rooms.
type ActorAddToRoomsResponse struct {
	Success      bool     `json:"success"`
	FailedRooms  []string `json:"failedRooms,omitempty"`
	CreatedRooms []string `json:"createdRooms,omitempty"` // If rooms were created on the fly? Unlikely, but keeping generic.
}

// ActorRemoveFromRoomsPayload represents the payload for removing an actor from multiple rooms.
type ActorRemoveFromRoomsPayload struct {
	BaseMatrixAdapterEventPayload
	RoomIDs []string `json:"roomIDs"`
	ActorID string   `json:"actorID"`
}

// ActorRemoveFromRoomsResponse represents the response for removing an actor from rooms.
type ActorRemoveFromRoomsResponse struct {
	Success     bool     `json:"success"`
	FailedRooms []string `json:"failedRooms,omitempty"`
}

// ActorRoomsPayload represents the request to list rooms for an actor.
type ActorRoomsPayload struct {
	BaseMatrixAdapterEventPayload
	ActorID string `json:"actorID"`
}

// ActorRoomsResponse represents the list of rooms an actor is in.
type ActorRoomsResponse struct {
	RoomIDs []string `json:"roomIDs"`
}
