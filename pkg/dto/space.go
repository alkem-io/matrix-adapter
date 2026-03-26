package dto

// ============================================================================
// Space DTOs - V3 Protocol
// ============================================================================

// SpaceChildDto represents a child room or subspace within a space.
type SpaceChildDto struct {
	// ChildID is the AlkemioRoomID of a child room or AlkemioContextID of a subspace.
	ChildID   string `json:"child_id"`
	IsSpace   bool   `json:"is_space"`
	Order     string `json:"order,omitempty"`
	Suggested bool   `json:"suggested,omitempty"`
}

// ============================================================================
// CreateSpace (communication.space.create)
// ============================================================================

// CreateSpaceRequest creates a new Matrix Space for an Alkemio context.
// Topic: communication.space.create
type CreateSpaceRequest struct {
	AlkemioContextID AlkemioContextID  `json:"alkemio_context_id"`
	Name             string            `json:"name"`
	Topic            string            `json:"topic,omitempty"`
	AvatarURL        string            `json:"avatar_url,omitempty"`
	ParentContextID  *AlkemioContextID `json:"parent_context_id,omitempty"`
	JoinRule         JoinRule          `json:"join_rule,omitempty"`
	InitialMembers   []AlkemioActorID  `json:"initial_members,omitempty"`
	IsPublic         *bool             `json:"is_public,omitempty"` // Room directory visibility
}

// ============================================================================
// GetSpace (communication.space.get)
// ============================================================================

// GetSpaceRequest retrieves current state of a space.
// Topic: communication.space.get
type GetSpaceRequest struct {
	AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
}

// GetSpaceResponse returns space details.
type GetSpaceResponse struct {
	BaseResponse     `tstype:",extends"`
	AlkemioContextID AlkemioContextID  `json:"alkemio_context_id"`
	DisplayName      string            `json:"display_name"`
	Topic            string            `json:"topic,omitempty"`
	AvatarURL        string            `json:"avatar_url,omitempty"`
	JoinRule         JoinRule          `json:"join_rule"`
	MemberActorIDs   []AlkemioActorID  `json:"member_actor_ids"`
	Children         []SpaceChildDto   `json:"children"`
	ParentContextID  *AlkemioContextID `json:"parent_context_id,omitempty"`
}

// ============================================================================
// UpdateSpace (communication.space.update)
// ============================================================================

// UpdateSpaceRequest updates space metadata.
// Topic: communication.space.update
type UpdateSpaceRequest struct {
	AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
	Name             *string          `json:"name,omitempty"`
	Topic            *string          `json:"topic,omitempty"`
	AvatarURL        *string          `json:"avatar_url,omitempty"`
	JoinRule         *JoinRule        `json:"join_rule,omitempty"`
	IsPublic         *bool            `json:"is_public,omitempty"` // Room directory visibility
}

// ============================================================================
// DeleteSpace (communication.space.delete)
// ============================================================================

// DeleteSpaceRequest archives/deletes a space.
// Topic: communication.space.delete
type DeleteSpaceRequest struct {
	AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
	Reason           string           `json:"reason,omitempty"`
}

// ============================================================================
// ListSpaces (communication.space.list)
// ============================================================================

// ListSpacesRequest retrieves all spaces (admin operation).
// Topic: communication.space.list
type ListSpacesRequest struct {
	Cursor string `json:"cursor,omitempty"`
}

// ListSpacesResponse returns paginated space list.
type ListSpacesResponse struct {
	BaseResponse      `tstype:",extends"`
	AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
	NextCursor        string             `json:"next_cursor,omitempty"`
}

// ============================================================================
// Batch Space Membership (communication.space.member.batch.*)
// ============================================================================

// BatchAddSpaceMemberRequest adds an actor to multiple spaces.
// Topic: communication.space.member.batch.add
type BatchAddSpaceMemberRequest struct {
	ActorID           AlkemioActorID     `json:"actor_id"`
	AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
}

// BatchAddSpaceMemberResponse returns per-space results.
type BatchAddSpaceMemberResponse struct {
	BaseResponse `tstype:",extends"`
	// Results maps AlkemioContextID (string) to operation result.
	Results map[string]BaseResponse `json:"results,omitempty"`
}

// BatchRemoveSpaceMemberRequest removes an actor from multiple spaces.
// Topic: communication.space.member.batch.remove
type BatchRemoveSpaceMemberRequest struct {
	ActorID           AlkemioActorID     `json:"actor_id"`
	AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
	Reason            string             `json:"reason,omitempty"`
}

// BatchRemoveSpaceMemberResponse returns per-space results.
type BatchRemoveSpaceMemberResponse struct {
	BaseResponse `tstype:",extends"`
	Results      map[string]BaseResponse `json:"results,omitempty"`
}
