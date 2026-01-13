package dto

// ============================================================================
// New Protocol DTOs (communication.reaction.*)
// ============================================================================

// ReactionDto represents a reaction to a message.
type ReactionDto struct {
	ID            ReactionID     `json:"id"`
	Emoji         string         `json:"emoji"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Timestamp     int64          `json:"timestamp"` // Unix milliseconds
}

// AddReactionRequest adds an emoji reaction to a message.
// Topic: communication.reaction.add
type AddReactionRequest struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	MessageID     MessageID      `json:"message_id"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Emoji         string         `json:"emoji"`
}

// AddReactionResponse returns the reaction ID and timestamp.
type AddReactionResponse struct {
	BaseResponse `tstype:",extends"`
	ReactionID   ReactionID `json:"reaction_id"`
	// Timestamp is the approximate creation time (Unix milliseconds).
	// Note: This is set locally when the adapter receives confirmation from Matrix,
	// not the exact server timestamp. The difference should be negligible (<100ms)
	// assuming synchronized clocks (NTP).
	Timestamp int64 `json:"timestamp"`
}

// RemoveReactionRequest removes a previously added reaction.
// Topic: communication.reaction.remove
type RemoveReactionRequest struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	ReactionID    ReactionID     `json:"reaction_id"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
}

// GetReactionRequest retrieves details of a specific reaction.
// Topic: communication.reaction.get
type GetReactionRequest struct {
	AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
	ReactionID    ReactionID    `json:"reaction_id"`
}

// GetReactionResponse returns reaction details.
type GetReactionResponse struct {
	BaseResponse `tstype:",extends"`
	Reaction     ReactionDto `json:"reaction"`
}
