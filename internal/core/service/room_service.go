package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// RoomService handles operations related to Matrix rooms.
type RoomService struct {
	matrix   ports.MatrixPort
	logger   ports.Logger
	idMapper *domain.IDMapper
}

// NewRoomService creates a new instance of RoomService.
func NewRoomService(matrix ports.MatrixPort, logger ports.Logger, idMapper *domain.IDMapper) *RoomService {
	return &RoomService{
		matrix:   matrix,
		logger:   logger,
		idMapper: idMapper,
	}
}

// ============================================================================
// New Protocol Methods (communication.room.*)
// ============================================================================

// CreateRoomWithAlkemioID creates a new Matrix room with idempotent alias lookup.
// If the room already exists (alias resolves), it returns success.
// For direct rooms: If a DM already exists between the 2 users, sets the alias on that room.
func (s *RoomService) CreateRoomWithAlkemioID(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	roomType string,
	name, topic, avatarURL, joinRule string,
	isPublic *bool,
	customState map[string]map[string]interface{},
	initialMembers []domain.Actor,
) error {
	s.logger.Info(
		"Creating room with Alkemio ID",
		"alkemio_room_id", alkemioRoomID,
		"type", roomType,
		"name", name,
	)

	// Build alias for idempotency check
	alias := s.idMapper.RoomAlias(alkemioRoomID)

	// Check if room already exists (idempotency)
	existingRoomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err == nil {
		// Room already exists - idempotent success
		s.logger.Info(
			"Room already exists (idempotent)",
			"alkemio_room_id", alkemioRoomID,
			"existing_room_id", existingRoomID,
		)
		return nil
	}

	// If error is not "not found", return it
	if !domain.IsNotFoundError(err) {
		return fmt.Errorf("failed to check room alias: %w", err)
	}

	// For direct rooms with 2 members, check if a DM room already exists between them
	if roomType == "direct" && len(initialMembers) == 2 {
		existingDMRoom, err := s.matrix.FindExistingDirectRoom(ctx, initialMembers[0], initialMembers[1])
		if err != nil {
			s.logger.Warn(
				"Failed to check for existing direct room, proceeding with creation",
				"error", err,
			)
		} else if existingDMRoom != "" {
			// Direct room already exists - set alias on it
			s.logger.Info(
				"Found existing direct room, setting alias",
				"alkemio_room_id", alkemioRoomID,
				"existing_room_id", existingDMRoom,
			)

			if err := s.matrix.SetRoomAlias(ctx, existingDMRoom, alias); err != nil {
				s.logger.Warn(
					"Failed to set alias on existing direct room",
					"error", err,
					"existing_room_id", existingDMRoom,
				)
				// Continue - alias setting failure is non-fatal
			}
			return nil
		}
	}

	// For direct-message rooms, ignore joinRule — they always remain private
	effectiveJoinRule := joinRule
	if roomType == "direct" {
		effectiveJoinRule = ""
	}

	// Create the room with alias
	roomID, err := s.matrix.CreateRoomWithAlias(ctx, alkemioRoomID, roomType, name, topic, avatarURL, effectiveJoinRule, initialMembers)
	if err != nil {
		return fmt.Errorf("failed to create room: %w", err)
	}

	// Set directory visibility if specified
	if isPublic != nil {
		if err := s.matrix.SetRoomDirectoryVisibility(ctx, roomID, *isPublic); err != nil {
			s.logger.Warn("Failed to set room directory visibility",
				"alkemio_room_id", alkemioRoomID, "is_public", *isPublic, "error", err)
		}
	}

	// Set custom io.alkemio.* state events if specified
	if len(customState) > 0 {
		if err := s.matrix.SetCustomState(ctx, roomID, customState); err != nil {
			s.logger.Warn("Failed to set custom state on room",
				"alkemio_room_id", alkemioRoomID, "error", err)
		}
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
	alias := s.idMapper.RoomAlias(alkemioRoomID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return nil, domain.NewRoomNotFoundError(alkemioRoomID.String())
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
		if actorUUID := s.idMapper.AlkemioActorID(memberID); actorUUID != uuid.Nil {
			room.MemberIDs = append(room.MemberIDs, actorUUID)
		}
	}

	// Get room messages
	messages, err := s.matrix.GetRoomMessages(ctx, roomID)
	if err != nil {
		s.logger.Warn("Failed to get room messages", "room_id", roomID, "error", err)
		messages = []domain.Message{}
	}

	// Convert SenderMatrixID to SenderID (Alkemio actor UUID) for messages and reactions
	for i := range messages {
		if messages[i].SenderMatrixID != "" {
			messages[i].SenderID = s.idMapper.AlkemioActorID(id.UserID(messages[i].SenderMatrixID))
		}
		// Convert reaction sender IDs
		for j := range messages[i].Reactions {
			if messages[i].Reactions[j].SenderMatrixID != "" {
				messages[i].Reactions[j].SenderID = s.idMapper.AlkemioActorID(id.UserID(messages[i].Reactions[j].SenderMatrixID))
			}
		}
	}

	room.Messages = messages

	return room, nil
}

// GetRoomAsUser retrieves room details from a specific user's perspective,
// including read state information for messages.
func (s *RoomService) GetRoomAsUser(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	actorID uuid.UUID,
) (*domain.RoomWithReadState, error) {
	s.logger.Info("Getting room as user", "alkemio_room_id", alkemioRoomID, "actor_id", actorID)

	// Get base room data with messages
	room, err := s.GetRoomWithMessages(ctx, alkemioRoomID)
	if err != nil {
		return nil, err
	}

	// Create actor for Matrix operations
	actor := domain.NewActor(actorID)

	// Get unread counts for this user
	unreadSummary, err := s.matrix.GetUnreadCounts(ctx, actor, room.ID, nil)
	if err != nil {
		s.logger.Warn("Failed to get unread counts, assuming all read",
			"room_id", room.ID, "actor_id", actorID, "error", err)
		// Default to all messages read if we can't get unread counts
		return &domain.RoomWithReadState{
			Room:        room,
			UnreadCount: 0,
		}, nil
	}

	// Determine which messages are read/unread based on unread count
	// Messages are sorted oldest first from GetRoomMessages
	// The last `unreadCount` messages are unread
	unreadCount := unreadSummary.RoomUnreadCount
	totalMessages := len(room.Messages)

	// Find the last read event ID (the message just before unread messages start)
	var lastReadEventID string
	var lastReadTS int64
	if totalMessages > 0 && unreadCount < totalMessages {
		// The last read message is at index (totalMessages - unreadCount - 1)
		lastReadIdx := totalMessages - unreadCount - 1
		if lastReadIdx >= 0 {
			lastReadEventID = room.Messages[lastReadIdx].ID
			lastReadTS = room.Messages[lastReadIdx].Timestamp.UnixMilli()
		}
	}

	return &domain.RoomWithReadState{
		Room:            room,
		LastReadEventID: lastReadEventID,
		LastReadTS:      lastReadTS,
		UnreadCount:     unreadCount,
	}, nil
}

// UpdateRoomMetadata updates room name, topic, avatar, join rule, and directory visibility.
func (s *RoomService) UpdateRoomMetadata(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	name, topic, avatarURL *string,
	joinRule *string,
	isPublic *bool,
	customState map[string]map[string]interface{},
) error {
	s.logger.Info("Updating room metadata", "alkemio_room_id", alkemioRoomID)

	// Resolve alias to get Matrix room ID
	alias := s.idMapper.RoomAlias(alkemioRoomID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return domain.NewRoomNotFoundError(alkemioRoomID.String())
	}

	// We need a dummy actor for the update - use the bot
	botActor := domain.Actor{}

	err = s.matrix.UpdateRoomState(ctx, roomID, botActor, name, topic, avatarURL, joinRule)
	if err != nil {
		return fmt.Errorf("failed to update room: %w", err)
	}

	// Set directory visibility if specified
	if isPublic != nil {
		if err := s.matrix.SetRoomDirectoryVisibility(ctx, roomID, *isPublic); err != nil {
			s.logger.Warn("Failed to set room directory visibility",
				"alkemio_room_id", alkemioRoomID, "is_public", *isPublic, "error", err)
		}
	}

	// Set custom io.alkemio.* state events if specified
	if len(customState) > 0 {
		if err := s.matrix.SetCustomState(ctx, roomID, customState); err != nil {
			s.logger.Warn("Failed to set custom state on room",
				"alkemio_room_id", alkemioRoomID, "error", err)
		}
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
	alias := s.idMapper.RoomAlias(alkemioRoomID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		// Room doesn't exist - idempotent success
		if domain.IsNotFoundError(err) {
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
	cursor string,
) ([]uuid.UUID, string, error) {
	s.logger.Info("Listing rooms", "cursor", cursor)

	// Get all joined rooms
	rooms, err := s.matrix.GetAllJoinedRooms(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list rooms: %w", err)
	}

	// Extract Alkemio room IDs from room aliases
	alkemioRoomIDs := s.extractAlkemioRoomIDs(ctx, rooms)

	return alkemioRoomIDs, "", nil
}

// extractAlkemioRoomIDs extracts Alkemio room UUIDs from Matrix rooms.
func (s *RoomService) extractAlkemioRoomIDs(ctx context.Context, rooms []id.RoomID) []uuid.UUID {
	alkemioRoomIDs := make([]uuid.UUID, 0, len(rooms))

	for _, roomID := range rooms {
		room, err := s.matrix.GetRoomDetails(ctx, roomID)
		if err != nil {
			continue
		}

		alkemioID := s.idMapper.AlkemioRoomID(room.Alias)
		if alkemioID != uuid.Nil {
			alkemioRoomIDs = append(alkemioRoomIDs, alkemioID)
		}
	}

	return alkemioRoomIDs
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
	msg, err := s.matrix.GetMessage(ctx, roomID, eventID)
	if err != nil {
		return nil, err
	}

	// Convert SenderMatrixID to SenderID (Alkemio actor UUID)
	if msg != nil && msg.SenderMatrixID != "" {
		msg.SenderID = s.idMapper.AlkemioActorID(id.UserID(msg.SenderMatrixID))
	}

	return msg, nil
}
