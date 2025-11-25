package dto

// ActorStartDirectMessagingPayload represents the payload to start a DM.
type ActorStartDirectMessagingPayload struct {
	BaseMatrixAdapterEventPayload
	ReceiverActorID   string `json:"receiverActorID"`
	InitiatingActorID string `json:"initiatingActorID"`
}

// ActorStartDirectMessagingResponse represents the response for starting a DM.
type ActorStartDirectMessagingResponse struct {
	RoomID string `json:"roomID"`
	// IsNew indicates if the room was just created or already existed.
	IsNew bool `json:"isNew"`
}

// ActorStopDirectMessagingPayload represents the payload to stop a DM (leave/forget).
type ActorStopDirectMessagingPayload struct {
	BaseMatrixAdapterEventPayload
	ReceiverActorID   string `json:"receiverActorID"`
	InitiatingActorID string `json:"initiatingActorID"`
}

// ActorStopDirectMessagingResponse represents the response for stopping a DM.
type ActorStopDirectMessagingResponse struct {
	Success bool `json:"success"`
}

// ActorRoomsDirectPayload represents the request to list DM rooms for an actor.
type ActorRoomsDirectPayload struct {
	BaseMatrixAdapterEventPayload
	ActorID string `json:"actorID"`
}

// ActorRoomsDirectResponse represents the list of DM rooms.
type ActorRoomsDirectResponse struct {
	// Map of OtherActorID -> RoomID
	DirectRooms map[string]string `json:"directRooms"`
}
