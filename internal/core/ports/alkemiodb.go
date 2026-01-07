// Package ports defines interfaces for driven adapters.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// ActorResolver provides bidirectional mapping between Alkemio actor IDs and entity IDs.
// Actor ID = agent.id (used by Alkemio Server for authorization)
// Entity ID = user.id or virtual_contributor.id (used as Matrix localpart)
//
// This is a temporary adapter for the migration period where Matrix user localparts
// need to use user/VC IDs instead of agent IDs.
type ActorResolver interface {
	// ResolveActorToEntityID converts an Alkemio actor ID (agent.id) to the corresponding
	// entity ID (user.id or virtual_contributor.id). Used when creating Matrix user IDs.
	// Returns ErrActorNotFound if the actor ID is not found in user or virtual_contributor tables.
	ResolveActorToEntityID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error)

	// ResolveEntityToActorID converts an entity ID (user.id or virtual_contributor.id)
	// back to the Alkemio actor ID (agent.id). Used for outbound Matrix events.
	// Returns ErrEntityNotFound if the entity ID is not found in user or virtual_contributor tables.
	ResolveEntityToActorID(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error)

	// Close releases database connection resources.
	Close()
}
