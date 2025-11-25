package dto

// AdminAllRoomsPayload represents the request to list all rooms (admin only).
type AdminAllRoomsPayload struct {
	BaseMatrixAdapterEventPayload
}

// AdminAllRoomsResponse represents the list of all rooms.
type AdminAllRoomsResponse struct {
	Rooms []RoomDetailsResponse `json:"rooms"`
}

// AdminReplicateRoomMembershipPayload represents the request to replicate membership from one room to another.
type AdminReplicateRoomMembershipPayload struct {
	BaseMatrixAdapterEventPayload
	TargetRoomID      string `json:"targetRoomID"`
	SourceRoomID      string `json:"sourceRoomID"`
	ActorToPrioritize string `json:"actorToPrioritize"`
}

// AdminReplicateRoomMembershipResponse represents the response for replication.
type AdminReplicateRoomMembershipResponse struct {
	Success     bool     `json:"success"`
	AddedUsers  []string `json:"addedUsers"`
	FailedUsers []string `json:"failedUsers"`
}
