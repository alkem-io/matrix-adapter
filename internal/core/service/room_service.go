package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// roomAliasFormat is the format string for Alkemio room aliases.
const roomAliasFormat = "#%s:%s"

// RoomService handles operations related to Matrix rooms.
type RoomService struct {
	matrix ports.MatrixPort
	logger ports.Logger
}

// NewRoomService creates a new instance of RoomService.
func NewRoomService(matrix ports.MatrixPort, logger ports.Logger) *RoomService {
	return &RoomService{
		matrix: matrix,
		logger: logger,
	}
}

// buildRoomAlias constructs the room alias from an Alkemio room ID.
func (s *RoomService) buildRoomAlias(alkemioRoomID uuid.UUID) string {
	return fmt.Sprintf(roomAliasFormat, alkemioRoomID.String(), s.matrix.HomeserverDomain())
}

// ============================================================================
// New Protocol Methods (communication.room.*)
// ============================================================================

// CreateRoomWithAlkemioID creates a new Matrix room with idempotent alias lookup.
// If the room already exists (alias resolves), it returns success.
func (s *RoomService) CreateRoomWithAlkemioID(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	roomType string,
	name, topic string,
	initialMembers []domain.Actor,
) error {
	s.logger.Info("Creating room with Alkemio ID",
		"alkemio_room_id", alkemioRoomID,
		"type", roomType,
		"name", name)

	// Build alias for idempotency check
	alias := s.buildRoomAlias(alkemioRoomID)

	// Check if room already exists (idempotency)
	existingRoomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err == nil {
		// Room already exists - idempotent success
		s.logger.Info("Room already exists (idempotent)",
			"alkemio_room_id", alkemioRoomID,
			"existing_room_id", existingRoomID)
		return nil
	}

	// If error is not "not found", return it
	if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
		!strings.Contains(strings.ToLower(err.Error()), "m_not_found") {
		return fmt.Errorf("failed to check room alias: %w", err)
	}

	// Create the room with alias
	_, err = s.matrix.CreateRoomWithAlias(ctx, alkemioRoomID, roomType, name, topic, initialMembers)
	if err != nil {
		return fmt.Errorf("failed to create room: %w", err)
	}

	s.logger.Info("Room created successfully", "alkemio_room_id", alkemioRoomID)
	return nil
}

// GetRoomWithMessages retrieves room details including members and messages.
func (s *RoomService) GetRoomWithMessages(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
) (*domain.Room, error) {
	s.logger.Info("Getting room with messages", "alkemio_room_id", alkemioRoomID)

	// Build alias and resolve to Matrix room ID
	alias := s.buildRoomAlias(alkemioRoomID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return nil, fmt.Errorf("room not found: %w", err)
	}

	// Get room details
	room, err := s.matrix.GetRoomDetails(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room details: %w", err)
	}
	room.AlkemioID = alkemioRoomID

	// Get room members and map to Alkemio actor IDs
	members, err := s.matrix.GetRoomMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room members: %w", err)
	}

	room.MemberIDs = make([]uuid.UUID, 0, len(members))
	for _, memberID := range members {
		// Extract UUID from Matrix user ID (@uuid:domain)
		localpart := strings.TrimPrefix(memberID.Localpart(), "@")
		if actorUUID, parseErr := uuid.Parse(localpart); parseErr == nil {
			room.MemberIDs = append(room.MemberIDs, actorUUID)
		}
	}

	// Get room messages
	messages, err := s.matrix.GetRoomMessages(ctx, roomID)
	if err != nil {
		s.logger.Warn("Failed to get room messages", "room_id", roomID, "error", err)
		messages = []domain.Message{}
	}
	room.Messages = messages

	return room, nil
}

// UpdateRoomMetadata updates room name, topic, and visibility.
// Note: isPublic is accepted but not yet implemented (reserved for future use).
func (s *RoomService) UpdateRoomMetadata(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	name, topic *string,
	_ *bool, // isPublic - reserved for future visibility control
) error {
	s.logger.Info("Updating room metadata", "alkemio_room_id", alkemioRoomID)

	// Resolve alias to get Matrix room ID
	alias := s.buildRoomAlias(alkemioRoomID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return fmt.Errorf("room not found: %w", err)
	}

	// Use bot to update room state
	var nameVal, topicVal string
	if name != nil {
		nameVal = *name
	}
	if topic != nil {
		topicVal = *topic
	}

	// We need a dummy actor for the update - use the bot
	botActor := domain.Actor{}

	err = s.matrix.UpdateRoomState(ctx, roomID, botActor, nameVal, topicVal, "")
	if err != nil {
		return fmt.Errorf("failed to update room: %w", err)
	}

	return nil
}

// DeleteRoomFully kicks all members, leaves the room, and removes the alias.
func (s *RoomService) DeleteRoomFully(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	reason string,
) error {
	s.logger.Info("Deleting room fully", "alkemio_room_id", alkemioRoomID, "reason", reason)

	// Resolve alias to get Matrix room ID
	alias := s.buildRoomAlias(alkemioRoomID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		// Room doesn't exist - idempotent success
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil
		}
		return fmt.Errorf("failed to resolve room alias: %w", err)
	}

	// Get room members to kick
	members, err := s.matrix.GetRoomMembers(ctx, roomID)
	if err != nil {
		s.logger.Warn("Failed to get room members for kick", "room_id", roomID, "error", err)
	} else {
		// Kick all members
		for _, memberID := range members {
			if err := s.matrix.KickUser(ctx, roomID, memberID, reason); err != nil {
				s.logger.Warn("Failed to kick user", "user_id", memberID, "room_id", roomID, "error", err)
			}
		}
	}

	// Delete the alias
	if err := s.matrix.DeleteAlias(ctx, alias); err != nil {
		s.logger.Warn("Failed to delete room alias", "alias", alias, "error", err)
	}

	s.logger.Info("Room deleted successfully", "alkemio_room_id", alkemioRoomID)
	return nil
}

// ListRooms returns a paginated list of Alkemio room IDs.
func (s *RoomService) ListRooms(
	ctx context.Context,
	limit int,
	cursor string,
) ([]uuid.UUID, string, error) {
	s.logger.Info("Listing rooms", "limit", limit, "cursor", cursor)

	// Get all joined rooms
	rooms, err := s.matrix.GetAllJoinedRooms(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list rooms: %w", err)
	}

	// Extract Alkemio room IDs from room aliases
	alkemioRoomIDs := s.extractAlkemioRoomIDs(ctx, rooms)

	// Apply pagination limit
	if limit > 0 && len(alkemioRoomIDs) > limit {
		alkemioRoomIDs = alkemioRoomIDs[:limit]
	}

	return alkemioRoomIDs, "", nil
}

// extractAlkemioRoomIDs extracts Alkemio room UUIDs from Matrix rooms.
func (s *RoomService) extractAlkemioRoomIDs(ctx context.Context, rooms []id.RoomID) []uuid.UUID {
	alkemioRoomIDs := make([]uuid.UUID, 0, len(rooms))
	homeserverDomain := s.matrix.HomeserverDomain()

	for _, roomID := range rooms {
		room, err := s.matrix.GetRoomDetails(ctx, roomID)
		if err != nil {
			continue
		}

		alkemioID := s.parseAlkemioIDFromAlias(room.Alias, homeserverDomain)
		if alkemioID != uuid.Nil {
			alkemioRoomIDs = append(alkemioRoomIDs, alkemioID)
		}
	}

	return alkemioRoomIDs
}

// parseAlkemioIDFromAlias extracts the Alkemio UUID from a room alias.
func (s *RoomService) parseAlkemioIDFromAlias(alias, homeserverDomain string) uuid.UUID {
	if alias == "" {
		return uuid.Nil
	}

	// Alias format: #uuid:domain
	alias = strings.TrimPrefix(alias, "#")
	parts := strings.Split(alias, ":")
	if len(parts) < 2 || parts[1] != homeserverDomain {
		return uuid.Nil
	}

	alkemioID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil
	}
	return alkemioID
}

// ============================================================================
// Shared Methods (used by handlers)
// ============================================================================

// SendMessage sends a text message to a room.
func (s *RoomService) SendMessage(
	ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string,
) (id.EventID, error) {
	return s.matrix.SendMessage(ctx, roomID, senderID, content)
}

// SendReply sends a reply to a specific event in a room.
func (s *RoomService) SendReply(
	ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, threadID id.EventID,
) (id.EventID, error) {
	return s.matrix.SendReply(ctx, roomID, senderID, content, threadID)
}

// RedactEvent redacts (deletes) an event from a room.
func (s *RoomService) RedactEvent(
	ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, reason string,
) error {
	return s.matrix.RedactEvent(ctx, roomID, actorID, eventID, reason)
}

// SendReaction sends a reaction (emoji) to an event.
func (s *RoomService) SendReaction(
	ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, emoji string,
) (id.EventID, error) {
	return s.matrix.SendReaction(ctx, roomID, actorID, eventID, emoji)
}

// GetMessage retrieves a specific message.
func (s *RoomService) GetMessage(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*domain.Message, error) {
	return s.matrix.GetMessage(ctx, roomID, eventID)
}
