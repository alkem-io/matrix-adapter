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

// CreateActorSessionRequest mints a Matrix access token for ONE actor — the
// authenticated caller on the server side. There is deliberately no other
// field: no subject can be named (workspace#091 M1/D4).
// Topic: communication.actor.session.create
type CreateActorSessionRequest struct {
	ActorID AlkemioActorID `json:"actor_id"`
}

// CreateActorSessionResponse carries the credential triple a Matrix client
// needs. AccessToken is a live credential: it must never be logged.
type CreateActorSessionResponse struct {
	BaseResponse `tstype:",extends"`
	UserID       string `json:"user_id"`
	DeviceID     string `json:"device_id"`
	AccessToken  string `json:"access_token"`
}

// RevokeActorSessionsRequest deletes every Matrix device of ONE actor, and with
// them their access and refresh tokens. Sent when the Alkemio user is deleted.
// Topic: communication.actor.sessions.revoke
type RevokeActorSessionsRequest struct {
	ActorID AlkemioActorID `json:"actor_id"`
}

// RevokeActorSessionsResponse reports how many devices were deleted.
type RevokeActorSessionsResponse struct {
	BaseResponse   `tstype:",extends"`
	DevicesRevoked int `json:"devices_revoked"`
}
