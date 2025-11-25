package dto

// RoomDeletePayload represents the payload to delete (leave/forget) a room.
type RoomDeletePayload struct {
	BaseMatrixAdapterEventPayload
	RoomID string `json:"roomID"`
}

// RoomDeleteResponse represents the response for room deletion.
type RoomDeleteResponse struct {
	Success bool `json:"success"`
}

// RoomDetailsPayload represents the payload to get room details.
type RoomDetailsPayload struct {
	BaseMatrixAdapterEventPayload
	RoomID    string `json:"roomID"`
	WithState bool   `json:"withState,omitempty"`
}

// RoomDetailsResponse represents the room details.
type RoomDetailsResponse struct {
	RoomID      string            `json:"roomID"`
	Name        string            `json:"name"`
	Topic       string            `json:"topic"`
	Alias       string            `json:"alias,omitempty"`
	IsDirect    bool              `json:"isDirect"`
	JoinedCount int               `json:"joinedCount"`
	State       map[string]string `json:"state,omitempty"` // Key-Value pairs of state events if requested
}

// RoomMembersPayload represents the payload to get room members.
type RoomMembersPayload struct {
	BaseMatrixAdapterEventPayload
	RoomID string `json:"roomID"`
}

// RoomMembersResponse represents the list of room members.
type RoomMembersResponse struct {
	RoomID  string   `json:"roomID"`
	UserIDs []string `json:"userIDs"` // Matrix User IDs
}

// RoomUpdateStatePayload represents the payload to update room state (name, topic, etc).
type RoomUpdateStatePayload struct {
	BaseMatrixAdapterEventPayload
	RoomID string `json:"roomID"`
	Name   string `json:"name,omitempty"`
	Topic  string `json:"topic,omitempty"`
	Alias  string `json:"alias,omitempty"`
}

// RoomUpdateStateResponse represents the response for state update.
type RoomUpdateStateResponse struct {
	Success bool `json:"success"`
}
