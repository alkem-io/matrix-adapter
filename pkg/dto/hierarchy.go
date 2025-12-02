package dto

// ============================================================================
// Hierarchy DTOs - V3 Protocol
// ============================================================================

// SetParentRequest establishes parent-child relationship for a room or subspace.
// Topic: communication.hierarchy.set_parent
type SetParentRequest struct {
	// ChildID is the AlkemioRoomID (for rooms) or AlkemioContextID (for subspaces) to add as child.
	ChildID         string           `json:"child_id"`
	IsSpace         bool             `json:"is_space"`
	ParentContextID AlkemioContextID `json:"parent_context_id"`
	Order           string           `json:"order,omitempty"`
	Suggested       bool             `json:"suggested,omitempty"`
}

// SetParentResponse confirms the hierarchy update.
type SetParentResponse struct {
	BaseResponse
}
