package dto

import "time"

// ============================================================================
// New Protocol DTOs (communication.reaction.*)
// ============================================================================

// ReactionDto represents a reaction to a message.
type ReactionDto struct {
	ID            ReactionID     `json:"id"`
	Emoji         string         `json:"emoji"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Timestamp     time.Time      `json:"timestamp"`
}

// AddReactionRequest adds an emoji reaction to a message.
// Topic: communication.reaction.add
type AddReactionRequest struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	MessageID     MessageID      `json:"message_id"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Emoji         string         `json:"emoji"`
}

// AddReactionResponse returns the reaction ID.
type AddReactionResponse struct {
	BaseResponse `tstype:",extends"`
	ReactionID   ReactionID `json:"reaction_id"`
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
