package dto

// CheckRoomHTTPRequest is the JSON body sent by the Synapse module to the adapter's check endpoint.
type CheckRoomHTTPRequest struct {
	Creator  string   `json:"creator"`
	Members  []string `json:"members"`
	IsDirect bool     `json:"is_direct"`
}

// CheckRoomHTTPResponse is the JSON body returned by the adapter's check endpoint to the Synapse module.
type CheckRoomHTTPResponse struct {
	Allow         bool   `json:"allow"`
	AlkemioRoomID string `json:"alkemio_room_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// CheckRoomRequest is the RabbitMQ payload sent from adapter to server for room creation consent/dedup check.
type CheckRoomRequest struct {
	CreatorActorID string   `json:"creator_actor_id"`
	MemberActorIDs []string `json:"member_actor_ids"`
	IsDirect       bool     `json:"is_direct"`
}

// CheckRoomResponse is the RabbitMQ payload returned from server to adapter after the room check.
type CheckRoomResponse struct {
	Allow         bool   `json:"allow"`
	AlkemioRoomID string `json:"alkemio_room_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// GetRoomInfoRequest is the RabbitMQ payload sent from adapter to server to retrieve room info during reconciliation.
type GetRoomInfoRequest struct {
	AlkemioRoomID string `json:"alkemio_room_id"`
}

// GetRoomInfoResponse is the RabbitMQ payload returned from server with room details.
// The governance fields (entity_type, parent_context_id, join_rule, visibility)
// are optional: an old server omits them and the adapter falls back to
// thread / platform-driven / shared defaults.
type GetRoomInfoResponse struct {
	AlkemioRoomID string           `json:"alkemio_room_id"`
	Type          string           `json:"type"`
	IsDirect      bool             `json:"is_direct"`
	Members       []RoomInfoMember `json:"members"`
	// EntityType is "thread" for every conversation-backed room ("space" is never served here).
	EntityType string `json:"entity_type,omitempty"`
	// ParentContextID is the owning space id when the room is space-anchored.
	ParentContextID string `json:"parent_context_id,omitempty"`
	// JoinRule is the membership mode Alkemio declares ("restricted" or "invite").
	JoinRule string `json:"join_rule,omitempty"`
	// Visibility is "world_readable" when the owning space is public, else "shared".
	Visibility string `json:"visibility,omitempty"`
}

// RoomInfoMember represents a member in the server-side room info response.
type RoomInfoMember struct {
	ActorID     string `json:"actor_id"`
	DisplayName string `json:"display_name"`
}
