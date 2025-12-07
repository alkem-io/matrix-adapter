// Package dto provides Data Transfer Objects for the Matrix Adapter.
package dto

// ============================================================================
// DM Request DTOs (Feature 006 - Room Creation Control)
// ============================================================================

// DMRequestedEvent represents the event published when Synapse requests approval
// for a DM room creation between two Alkemio users.
// This is published to the communication.room.dm.requested topic.
type DMRequestedEvent struct {
	// InitiatorActorID is the Alkemio UUID of the user who initiated the DM request.
	InitiatorActorID string `json:"initiator_actor_id"`
	// TargetActorID is the Alkemio UUID of the user who is being invited to the DM.
	TargetActorID string `json:"target_actor_id"`
}

// DMWebhookPayload represents the payload received from Synapse's DM request webhook.
// The spam checker module sends this when a user attempts to create a DM room.
type DMWebhookPayload struct {
	// Inviter is the Matrix user ID of the user initiating the DM (format: @{uuid}:{domain}).
	Inviter string `json:"inviter"`
	// Invitee is the Matrix user ID of the user being invited (format: @{uuid}:{domain}).
	Invitee string `json:"invitee"`
}
