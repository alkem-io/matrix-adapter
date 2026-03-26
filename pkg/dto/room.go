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
	IsPublic        *bool             `json:"is_public,omitempty"` // Room directory visibility
}

// GetRoomRequest retrieves current state of a room.
// Topic: communication.room.get
type GetRoomRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}

// GetRoomResponse returns room details with members and messages.
type GetRoomResponse struct {
	BaseResponse   `tstype:",extends"`
	AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
	DisplayName    string           `json:"display_name"`
	AvatarURL      string           `json:"avatar_url,omitempty"`
	MemberActorIDs []AlkemioActorID `json:"member_actor_ids"`
	Messages       []MessageDto     `json:"messages"`
}

// UpdateRoomRequest updates room metadata.
// Topic: communication.room.update
type UpdateRoomRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	Name          *string       `json:"name,omitempty"`
	Topic         *string       `json:"topic,omitempty"`
	AvatarURL     *string       `json:"avatar_url,omitempty"`
	JoinRule      *JoinRule     `json:"join_rule,omitempty"`
	IsPublic      *bool         `json:"is_public,omitempty"` // Room directory visibility
}

// DeleteRoomRequest archives or deletes a room.
// Topic: communication.room.delete
type DeleteRoomRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	Reason        string        `json:"reason,omitempty"`
}

// ListRoomsRequest retrieves all rooms (admin operation).
// Topic: communication.room.list
type ListRoomsRequest struct {
	Cursor string `json:"cursor,omitempty"`
}

// ListRoomsResponse returns paginated room list.
type ListRoomsResponse struct {
	BaseResponse   `tstype:",extends"`
	AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
	NextCursor     string          `json:"next_cursor,omitempty"`
}

// ============================================================================
// Room Members Query (communication.room.members.*)
// ============================================================================

// GetRoomMembersRequest retrieves the list of members in a room.
// Topic: communication.room.members.get
type GetRoomMembersRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}

// GetRoomMembersResponse returns the list of joined member actor IDs.
type GetRoomMembersResponse struct {
	BaseResponse   `tstype:",extends"`
	AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
	MemberActorIDs []AlkemioActorID `json:"member_actor_ids"`
}

// ============================================================================
// User-Scoped Room Query (communication.room.get.as_user)
// ============================================================================

// GetRoomAsUserRequest retrieves room state from a specific user's perspective.
// Topic: communication.room.get.as_user
type GetRoomAsUserRequest struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	ActorID       AlkemioActorID `json:"actor_id"`
}

// GetRoomAsUserResponse returns room details with user-specific read state.
type GetRoomAsUserResponse struct {
	BaseResponse    `tstype:",extends"`
	AlkemioRoomID   AlkemioRoomID             `json:"alkemio_room_id"`
	DisplayName     string                    `json:"display_name"`
	AvatarURL       string                    `json:"avatar_url,omitempty"`
	MemberActorIDs  []AlkemioActorID          `json:"member_actor_ids"`
	Messages        []MessageWithReadStateDto `json:"messages"`
	LastReadEventID *MessageID                `json:"last_read_event_id,omitempty"`
	UnreadCount     int                       `json:"unread_count"`
}

// MessageWithReadStateDto extends MessageDto with read receipt info for a specific user.
type MessageWithReadStateDto struct {
	MessageDto `tstype:",extends"`
	IsRead     bool `json:"is_read"` // Has the requesting user read this message?
}
