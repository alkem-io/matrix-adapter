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

// RevokeActorDevicesRequest deletes ALL of an actor's Matrix devices,
// invalidating their access and refresh tokens. Idempotent: an actor with
// zero devices is a success with deleted_count 0.
// Topic: communication.actor.devices.revoke
type RevokeActorDevicesRequest struct {
	ActorID AlkemioActorID `json:"actor_id"`
	Reason  string         `json:"reason,omitempty"`
}

// RevokeActorDevicesResponse reports the devices deleted for the actor.
type RevokeActorDevicesResponse struct {
	BaseResponse `tstype:",extends"`
	DeletedCount int      `json:"deleted_count"`
	DeviceIDs    []string `json:"device_ids,omitempty"`
}
