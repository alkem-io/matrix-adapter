package dto

// AdminAllRoomsPayload represents the request to list all rooms (admin only).
type AdminAllRoomsPayload struct {
	BaseMatrixAdapterEventPayload
}

// AdminAllRoomsResponse represents the list of all rooms.
type AdminAllRoomsResponse struct {
	BaseResponse
	Rooms []RoomDetailsResponse `json:"rooms"`
}

// AdminReplicateRoomMembershipPayload represents the request to replicate membership from one room to another.
type AdminReplicateRoomMembershipPayload struct {
	BaseMatrixAdapterEventPayload
	TargetRoomID      string `json:"targetRoomID"`
	SourceRoomID      string `json:"sourceRoomID"`
	ActorToPrioritize string `json:"actorToPrioritize"`
}

// AdminReplicateRoomMembershipResponsePayload represents the response for replication.
type AdminReplicateRoomMembershipResponsePayload struct {
	BaseResponse
	AddedUsers  []string `json:"addedUsers"`
	FailedUsers []string `json:"failedUsers"`
}
