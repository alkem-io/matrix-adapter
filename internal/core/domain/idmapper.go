package domain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// IDMapper centralizes all Alkemio ↔ Matrix ID conversions.
// It provides a single source of truth for ID format strings and conversion logic.
type IDMapper struct {
	homeserverDomain string
}

// NewIDMapper creates a new IDMapper with the given homeserver domain.
func NewIDMapper(homeserverDomain string) *IDMapper {
	return &IDMapper{homeserverDomain: homeserverDomain}
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
func (m *IDMapper) UserID(actorID uuid.UUID) id.UserID {
	return id.NewUserID(actorID.String(), m.homeserverDomain)
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

// HomeserverDomain returns the homeserver domain used for ID mapping.
func (m *IDMapper) HomeserverDomain() string {
	return m.homeserverDomain
}
