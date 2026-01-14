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
	BaseResponse `tstype:",extends"`
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
	BaseResponse `tstype:",extends"`
	Results      map[string]BaseResponse `json:"results,omitempty"`
}

// ============================================================================
// Batch Unread Counts DTOs (communication.room.batch.unread_counts.*)
// ============================================================================

// BatchGetUnreadCountsRequest gets unread counts for multiple rooms.
// Topic: communication.room.batch.unread_counts.get
type BatchGetUnreadCountsRequest struct {
	ActorID        AlkemioActorID  `json:"actor_id"`
	AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
}

// BatchGetUnreadCountsResponse returns per-room unread counts.
type BatchGetUnreadCountsResponse struct {
	BaseResponse `tstype:",extends"`
	// UnreadCounts maps AlkemioRoomID (string) to unread count.
	// Rooms with errors will not be included in the map.
	UnreadCounts map[string]int `json:"unread_counts"`
	// Errors maps AlkemioRoomID (string) to error response for failed rooms.
	Errors map[string]BaseResponse `json:"errors,omitempty"`
}

// ============================================================================
// Last Message DTOs (communication.room.last_message.*, communication.room.batch.last_messages.*)
// ============================================================================

// GetLastMessageRequest gets the most recent message in a room.
// Topic: communication.room.last_message.get
type GetLastMessageRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}

// GetLastMessageResponse returns the last message or null if room is empty.
type GetLastMessageResponse struct {
	BaseResponse `tstype:",extends"`
	Message      *MessageDto `json:"message"` // nil if no messages
}

// BatchGetLastMessagesRequest gets the most recent message for multiple rooms.
// Topic: communication.room.batch.last_messages.get
type BatchGetLastMessagesRequest struct {
	AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
}

// BatchGetLastMessagesResponse returns per-room last messages.
type BatchGetLastMessagesResponse struct {
	BaseResponse `tstype:",extends"`
	// Messages maps AlkemioRoomID (string) to last message (null if empty room).
	Messages map[string]*MessageDto `json:"messages"`
	// Errors maps AlkemioRoomID (string) to error response for failed rooms.
	Errors map[string]BaseResponse `json:"errors,omitempty"`
}
