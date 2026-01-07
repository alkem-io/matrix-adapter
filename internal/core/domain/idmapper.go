package domain

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// Reserved UUIDs that should bypass ActorResolver lookup (used directly as-is).
// These are special system IDs that don't exist in the user/virtual_contributor tables.
var (
	// ReservedNilUUID is the nil UUID used for system/anonymous operations.
	ReservedNilUUID = uuid.MustParse("00000000-0000-0000-0000-000000000000")
	// ReservedBotUUID is the special bot/system UUID.
	ReservedBotUUID = uuid.MustParse("ffffffff-ffff-ffff-ffff-fffffffffff0")
)

// isReservedUUID returns true if the UUID is a reserved system ID that should bypass resolution.
func isReservedUUID(id uuid.UUID) bool {
	return id == ReservedNilUUID || id == ReservedBotUUID
}

// ActorResolver provides bidirectional mapping between Alkemio actor IDs and entity IDs.
// This interface is defined here to avoid import cycles with the ports package.
type ActorResolver interface {
	// ResolveActorToEntityID converts an actor ID (agent.id) to an entity ID (user.id or virtual_contributor.id).
	ResolveActorToEntityID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error)
	// ResolveEntityToActorID converts an entity ID back to an actor ID (agent.id).
	ResolveEntityToActorID(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error)
}

// IDMapper centralizes all Alkemio ↔ Matrix ID conversions.
// It provides a single source of truth for ID format strings and conversion logic.
type IDMapper struct {
	homeserverDomain string
	actorResolver    ActorResolver
}

// NewIDMapper creates a new IDMapper with the given homeserver domain.
func NewIDMapper(homeserverDomain string) *IDMapper {
	return &IDMapper{homeserverDomain: homeserverDomain}
}

// SetActorResolver sets the ActorResolver for DB-based actor ID mapping.
// When set, UserID and AlkemioActorID will use the resolver for ID conversion.
// When nil, direct mapping is used (actor ID = Matrix localpart).
func (m *IDMapper) SetActorResolver(resolver ActorResolver) {
	m.actorResolver = resolver
}

// HasActorResolver returns true if an ActorResolver is configured.
func (m *IDMapper) HasActorResolver() bool {
	return m.actorResolver != nil
}

// ============================================================================
// Forward Mapping (Alkemio → Matrix)
// ============================================================================

// RoomAlias constructs the Matrix room alias from an Alkemio room ID.
// Format: #<uuid>:<domain>
func (m *IDMapper) RoomAlias(alkemioRoomID uuid.UUID) string {
	return fmt.Sprintf("#%s:%s", alkemioRoomID.String(), m.homeserverDomain)
}

// SpaceAlias constructs the Matrix space alias from an Alkemio context ID.
// Format: #<uuid>:<domain> (same pattern as rooms - UUIDs are disjoint in Alkemio)
func (m *IDMapper) SpaceAlias(alkemioContextID uuid.UUID) string {
	return fmt.Sprintf("#%s:%s", alkemioContextID.String(), m.homeserverDomain)
}

// UserID constructs the Matrix user ID from an Alkemio actor ID.
// Format: @<uuid>:<domain>
// When ActorResolver is set, the actor ID is first resolved to an entity ID (user.id or virtual_contributor.id).
// When ActorResolver is not set, direct mapping is used (actor ID = Matrix localpart).
// Reserved UUIDs (nil UUID, bot UUID) bypass resolution and are used directly.
// The context parameter is required for database access when using ActorResolver.
func (m *IDMapper) UserID(ctx context.Context, actorID uuid.UUID) (id.UserID, error) {
	localpart := actorID.String()

	// Skip resolution for reserved system UUIDs
	if m.actorResolver != nil && !isReservedUUID(actorID) {
		entityID, err := m.actorResolver.ResolveActorToEntityID(ctx, actorID)
		if err != nil {
			return "", err
		}
		localpart = entityID.String()
	}

	return id.NewUserID(localpart, m.homeserverDomain), nil
}

// RoomAliasLocalpart returns the localpart for a room alias (without # and domain).
// Used when creating rooms with mautrix SDK.
func (m *IDMapper) RoomAliasLocalpart(alkemioRoomID uuid.UUID) string {
	return alkemioRoomID.String()
}

// SpaceAliasLocalpart returns the localpart for a space alias (without # and domain).
// Used when creating spaces with mautrix SDK.
func (m *IDMapper) SpaceAliasLocalpart(alkemioContextID uuid.UUID) string {
	return alkemioContextID.String()
}

// ============================================================================
// Backward Mapping (Matrix → Alkemio)
// ============================================================================

// AlkemioRoomID extracts the Alkemio UUID from a Matrix room alias.
// Returns uuid.Nil if the alias is invalid or doesn't match the homeserver domain.
func (m *IDMapper) AlkemioRoomID(alias string) uuid.UUID {
	if alias == "" {
		return uuid.Nil
	}

	// Alias format: #uuid:domain
	alias = strings.TrimPrefix(alias, "#")
	parts := strings.Split(alias, ":")
	if len(parts) < 2 || parts[1] != m.homeserverDomain {
		return uuid.Nil
	}

	alkemioID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil
	}
	return alkemioID
}

// AlkemioContextID extracts the Alkemio UUID from a Matrix space alias.
// Returns uuid.Nil if the alias is invalid or doesn't match the homeserver domain.
// Note: Uses same format as room aliases (#uuid:domain) since UUIDs are disjoint.
func (m *IDMapper) AlkemioContextID(alias string) uuid.UUID {
	if alias == "" {
		return uuid.Nil
	}

	// Alias format: #uuid:domain (same as rooms)
	alias = strings.TrimPrefix(alias, "#")
	parts := strings.Split(alias, ":")
	if len(parts) < 2 || parts[1] != m.homeserverDomain {
		return uuid.Nil
	}

	contextID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil
	}
	return contextID
}

// AlkemioActorID extracts the Alkemio UUID from a Matrix user ID.
// Returns uuid.Nil if the user ID is invalid or doesn't match the homeserver domain.
// Note: This is the legacy method without ActorResolver support. Use AlkemioActorIDWithContext
// when ActorResolver may be configured.
func (m *IDMapper) AlkemioActorID(userID id.UserID) uuid.UUID {
	localpart := userID.Localpart()
	// Remove any leading @ that might be in the localpart
	localpart = strings.TrimPrefix(localpart, "@")

	actorID, err := uuid.Parse(localpart)
	if err != nil {
		return uuid.Nil
	}
	return actorID
}

// AlkemioActorIDWithContext extracts the Alkemio actor UUID from a Matrix user ID.
// When ActorResolver is set, the localpart (entity ID) is resolved to the actor ID.
// When ActorResolver is not set, direct mapping is used (localpart = actor ID).
// Reserved UUIDs (nil UUID, bot UUID) bypass resolution and are returned directly.
// The context parameter is required for database access when using ActorResolver.
func (m *IDMapper) AlkemioActorIDWithContext(ctx context.Context, userIDStr string) (uuid.UUID, error) {
	userID := id.UserID(userIDStr)
	localpart := userID.Localpart()
	// Remove any leading @ that might be in the localpart
	localpart = strings.TrimPrefix(localpart, "@")

	parsedID, err := uuid.Parse(localpart)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID in user ID: %w", err)
	}

	// Skip resolution for reserved system UUIDs
	if m.actorResolver != nil && !isReservedUUID(parsedID) {
		actorID, err := m.actorResolver.ResolveEntityToActorID(ctx, parsedID)
		if err != nil {
			return uuid.Nil, err
		}
		return actorID, nil
	}

	// Direct mapping - localpart is the actor ID (or reserved UUID)
	return parsedID, nil
}

// HomeserverDomain returns the homeserver domain used for ID mapping.
func (m *IDMapper) HomeserverDomain() string {
	return m.homeserverDomain
}
