package dto

// ============================================================================
// Batch Room Membership DTOs (communication.room.member.batch.*)
// ============================================================================

// BatchAddMemberRequest adds a single actor to multiple rooms.
// Topic: communication.room.member.batch.add
type BatchAddMemberRequest struct {
	ActorID        AlkemioActorID  `json:"actor_id"`
	AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
}

// BatchAddMemberResponse returns per-room results.
type BatchAddMemberResponse struct {
	BaseResponse
	// Results maps AlkemioRoomID (string) to operation result.
	// Only populated if BaseResponse.Success is true (batch was processed).
	Results map[string]BaseResponse `json:"results,omitempty"`
}

// BatchRemoveMemberRequest removes a single actor from multiple rooms.
// Topic: communication.room.member.batch.remove
type BatchRemoveMemberRequest struct {
	ActorID        AlkemioActorID  `json:"actor_id"`
	AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
	Reason         string          `json:"reason,omitempty"`
}

// BatchRemoveMemberResponse returns per-room results.
type BatchRemoveMemberResponse struct {
	BaseResponse
	Results map[string]BaseResponse `json:"results,omitempty"`
}
