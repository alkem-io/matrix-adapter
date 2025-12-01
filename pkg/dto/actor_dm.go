package dto

// ActorStartDirectMessagingPayload represents the payload to start a DM.
type ActorStartDirectMessagingPayload struct {
	BaseMatrixAdapterEventPayload
	ReceiverActorID   string `json:"receiverActorID"`
	InitiatingActorID string `json:"initiatingActorID"`
}

// ActorStartDirectMessagingResponsePayload represents the response for starting a DM.
type ActorStartDirectMessagingResponsePayload struct {
	BaseResponse
	// RoomID is the ID of the DM room.
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

// ActorStopDirectMessagingResponsePayload represents the response for stopping a DM.
type ActorStopDirectMessagingResponsePayload struct {
	BaseResponse
}

// ActorRoomsDirectPayload represents the request to list DM rooms for an actor.
type ActorRoomsDirectPayload struct {
	BaseMatrixAdapterEventPayload
	ActorID string `json:"actorID"`
}

// ActorRoomsDirectResponse represents the list of DM rooms.
type ActorRoomsDirectResponse struct {
	BaseResponse
	// Map of OtherActorID -> RoomID
	DirectRooms map[string]string `json:"directRooms"`
}
