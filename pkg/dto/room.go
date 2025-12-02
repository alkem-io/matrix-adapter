package dto

// ============================================================================
// New Protocol DTOs (communication.room.*)
// ============================================================================

// CreateRoomRequest creates a new communication channel.
// Topic: communication.room.create
type CreateRoomRequest struct {
	AlkemioRoomID   AlkemioRoomID     `json:"alkemio_room_id"`
	Type            RoomType          `json:"type"`
	Name            string            `json:"name,omitempty"` // Ignored for 'direct'
	InitialMembers  []AlkemioActorID  `json:"initial_members,omitempty"`
	Topic           string            `json:"topic,omitempty"`
	AvatarURL       string            `json:"avatar_url,omitempty"`
	ParentContextID *AlkemioContextID `json:"parent_context_id,omitempty"`
	JoinRule        JoinRule          `json:"join_rule,omitempty"`
}

// CreateRoomResponse confirms room creation.
type CreateRoomResponse struct {
	BaseResponse
	// No additional data - server already knows the ID
}

// GetRoomRequest retrieves current state of a room.
// Topic: communication.room.get
type GetRoomRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}

// GetRoomResponse returns room details with members and messages.
type GetRoomResponse struct {
	BaseResponse
	AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
	DisplayName    string           `json:"display_name"`
	MemberActorIDs []AlkemioActorID `json:"member_actor_ids"`
	Messages       []MessageDto     `json:"messages"`
}

// UpdateRoomRequest updates room metadata.
// Topic: communication.room.update
type UpdateRoomRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	Name          *string       `json:"name,omitempty"`
	Topic         *string       `json:"topic,omitempty"`
	IsPublic      *bool         `json:"is_public,omitempty"`
	AvatarURL     *string       `json:"avatar_url,omitempty"`
	JoinRule      *JoinRule     `json:"join_rule,omitempty"`
}

// UpdateRoomResponse confirms the update.
type UpdateRoomResponse struct {
	BaseResponse
}

// DeleteRoomRequest archives or deletes a room.
// Topic: communication.room.delete
type DeleteRoomRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	Reason        string        `json:"reason,omitempty"`
}

// DeleteRoomResponse confirms deletion.
type DeleteRoomResponse struct {
	BaseResponse
}

// ListRoomsRequest retrieves all rooms (admin operation).
// Topic: communication.room.list
type ListRoomsRequest struct {
	Cursor string `json:"cursor,omitempty"`
}

// ListRoomsResponse returns paginated room list.
type ListRoomsResponse struct {
	BaseResponse
	AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
	NextCursor     string          `json:"next_cursor,omitempty"`
}
