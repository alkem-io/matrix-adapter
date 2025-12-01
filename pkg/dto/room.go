package dto

// RoomCreatePayload represents the payload to create a room.
type RoomCreatePayload struct {
	BaseMatrixAdapterEventPayload
	RoomName string            `json:"roomName"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RoomCreateResponsePayload represents the response for room creation.
type RoomCreateResponsePayload struct {
	BaseResponse
	RoomID string `json:"roomId"`
}

// RoomInvitePayload is deprecated in favor of ActorAddToRoomsPayload.
// Keeping it for now if needed for internal logic, but should be phased out.
type RoomInvitePayload struct {
	BaseMatrixAdapterEventPayload
	InviteeID string `json:"inviteeId"`
	RoomID    string `json:"roomId"`
}

// RoomInviteResponsePayload represents the response for room invitation.
type RoomInviteResponsePayload struct {
	BaseResponse
}
