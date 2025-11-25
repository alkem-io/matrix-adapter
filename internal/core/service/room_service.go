package service

import (
	"context"
	"fmt"

	"github.com/alkemio/matrix-adapter-go/internal/core/domain"
	"github.com/alkemio/matrix-adapter-go/internal/core/ports"
	"maunium.net/go/mautrix/id"
)

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

// CreateRoom creates a new Matrix room on behalf of an actor.
func (s *RoomService) CreateRoom(ctx context.Context, actorID domain.Actor, name string, metadata map[string]string) (id.RoomID, error) {
	s.logger.Info("Creating room", "actor_id", actorID.ID, "name", name)

	roomID, err := s.matrix.CreateRoom(ctx, actorID, name, metadata)
	if err != nil {
		return "", fmt.Errorf("failed to create room: %w", err)
	}

	s.logger.Info("Room created successfully", "room_id", roomID)
	return roomID, nil
}

// InviteUser invites a user to a room.
func (s *RoomService) InviteUser(ctx context.Context, roomID id.RoomID, inviterID, inviteeID domain.Actor) error {
	s.logger.Info("Inviting user to room", "room_id", roomID, "inviter_id", inviterID.ID, "invitee_id", inviteeID.ID)

	err := s.matrix.InviteUser(ctx, roomID, inviterID, inviteeID)
	if err != nil {
		return fmt.Errorf("failed to invite user: %w", err)
	}

	return nil
}

// GetRoomDetails retrieves details about a room.
func (s *RoomService) GetRoomDetails(ctx context.Context, roomID id.RoomID) (*domain.Room, error) {
	return s.matrix.GetRoomDetails(ctx, roomID)
}

// GetRoomMembers retrieves the list of members in a room.
func (s *RoomService) GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error) {
	return s.matrix.GetRoomMembers(ctx, roomID)
}

// UpdateRoomState updates the state of a room (name, topic, alias).
func (s *RoomService) UpdateRoomState(ctx context.Context, roomID id.RoomID, actorID domain.Actor, name, topic, alias string) error {
	return s.matrix.UpdateRoomState(ctx, roomID, actorID, name, topic, alias)
}

// SendMessage sends a text message to a room.
func (s *RoomService) SendMessage(ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string) (id.EventID, error) {
	return s.matrix.SendMessage(ctx, roomID, senderID, content)
}

// SendReply sends a reply to a specific event in a room.
func (s *RoomService) SendReply(ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, threadID id.EventID) (id.EventID, error) {
	return s.matrix.SendReply(ctx, roomID, senderID, content, threadID)
}

// RedactEvent redacts (deletes) an event from a room.
func (s *RoomService) RedactEvent(ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, reason string) error {
	return s.matrix.RedactEvent(ctx, roomID, actorID, eventID, reason)
}

// SendReaction sends a reaction (emoji) to an event.
func (s *RoomService) SendReaction(ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, emoji string) (id.EventID, error) {
	return s.matrix.SendReaction(ctx, roomID, actorID, eventID, emoji)
}

// ForgetRoom forgets a room for an actor.
func (s *RoomService) ForgetRoom(ctx context.Context, roomID id.RoomID, actorID domain.Actor) error {
	s.logger.Info("Forgetting room", "room_id", roomID, "actor_id", actorID.ID)
	return s.matrix.ForgetRoom(ctx, roomID, actorID)
}

// GetMessage retrieves a specific message.
func (s *RoomService) GetMessage(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*domain.Message, error) {
	return s.matrix.GetMessage(ctx, roomID, eventID)
}

// GetReactionEventID finds the event ID of a reaction.
func (s *RoomService) GetReactionEventID(ctx context.Context, roomID id.RoomID, eventID id.EventID, emoji string, senderID domain.Actor) (id.EventID, error) {
	return s.matrix.GetReactionEventID(ctx, roomID, eventID, emoji, senderID)
}
