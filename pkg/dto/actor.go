package dto

// ============================================================================
// Actor Commands (communication.actor.*)
// ============================================================================

// SyncActorRequest ensures an actor exists in Matrix and updates their profile.
// This is an idempotent operation - calling with the same data has no effect.
// Topic: communication.actor.sync
type SyncActorRequest struct {
	ActorID     AlkemioActorID `json:"actor_id"`
	DisplayName string         `json:"display_name"`
	AvatarURL   string         `json:"avatar_url,omitempty"`
}

// SyncActorResponse confirms the actor sync operation.
type SyncActorResponse struct {
	BaseResponse
}
