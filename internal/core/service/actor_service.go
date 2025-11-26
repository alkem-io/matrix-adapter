// Package service implements the core business logic of the application.
package service

import (
	"context"
	"fmt"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"maunium.net/go/mautrix/id"
)

// ActorService handles operations related to actors (users/agents).
type ActorService struct {
	matrix ports.MatrixPort
	logger ports.Logger
}

// NewActorService creates a new instance of ActorService.
func NewActorService(matrix ports.MatrixPort, logger ports.Logger) *ActorService {
	return &ActorService{
		matrix: matrix,
		logger: logger,
	}
}

// RegisterActor ensures that an actor exists on the Matrix homeserver.
func (s *ActorService) RegisterActor(ctx context.Context, actor domain.Actor) (id.UserID, error) {
	s.logger.Info("Registering actor", "actor_id", actor.ID)

	userID, err := s.matrix.EnsureUser(ctx, actor)
	if err != nil {
		return "", fmt.Errorf("failed to register actor: %w", err)
	}

	s.logger.Info("Actor registered successfully", "actor_id", actor.ID, "matrix_id", userID)
	return userID, nil
}

// AddToRooms adds an actor to a list of rooms.
func (s *ActorService) AddToRooms(ctx context.Context, actorID domain.Actor, roomIDs []string) ([]string, []string) {
	var failed []string
	for _, roomIDStr := range roomIDs {
		roomID := id.RoomID(roomIDStr)
		if err := s.matrix.JoinRoom(ctx, roomID, actorID); err != nil {
			s.logger.Error("Failed to join room", "actor_id", actorID.ID, "room_id", roomID, "error", err)
			failed = append(failed, roomIDStr)
		}
	}
	return failed, []string{}
}

// RemoveFromRooms removes an actor from a list of rooms.
func (s *ActorService) RemoveFromRooms(ctx context.Context, actorID domain.Actor, roomIDs []string) []string {
	var failed []string
	for _, roomIDStr := range roomIDs {
		roomID := id.RoomID(roomIDStr)
		if err := s.matrix.LeaveRoom(ctx, roomID, actorID); err != nil {
			s.logger.Error("Failed to leave room", "actor_id", actorID.ID, "room_id", roomID, "error", err)
			failed = append(failed, roomIDStr)
		}
	}
	return failed
}

// GetRooms returns the list of rooms an actor has joined.
func (s *ActorService) GetRooms(ctx context.Context, actorID domain.Actor) ([]string, error) {
	rooms, err := s.matrix.GetUserJoinedRooms(ctx, actorID)
	if err != nil {
		return nil, err
	}
	roomIDs := make([]string, 0, len(rooms))
	for _, r := range rooms {
		roomIDs = append(roomIDs, string(r))
	}
	return roomIDs, nil
}

// CreateDirectRoom creates a direct message room between two actors.
func (s *ActorService) CreateDirectRoom(ctx context.Context, initiator domain.Actor, receiver domain.Actor) (
	id.RoomID, error,
) {
	s.logger.Info("Creating DM room", "initiator", initiator.ID, "receiver", receiver.ID)
	return s.matrix.CreateDirectRoom(ctx, initiator, receiver)
}

// GetDirectRooms returns the list of DM rooms for an actor.
func (s *ActorService) GetDirectRooms(ctx context.Context, actorID domain.Actor) (map[id.UserID]id.RoomID, error) {
	return s.matrix.GetDirectRooms(ctx, actorID)
}

// StopDirectMessaging stops a DM between two actors (leaves/forgets the room).
func (s *ActorService) StopDirectMessaging(ctx context.Context, initiator domain.Actor, receiver domain.Actor) error {
	// Get DM rooms for initiator
	dms, err := s.matrix.GetDirectRooms(ctx, initiator)
	if err != nil {
		return err
	}

	// Resolve receiver Matrix ID
	receiverUserID, err := s.matrix.EnsureUser(ctx, receiver)
	if err != nil {
		return err
	}

	roomID, ok := dms[receiverUserID]
	if !ok {
		return fmt.Errorf("no DM found with receiver")
	}

	// Forget the room
	return s.matrix.ForgetRoom(ctx, roomID, initiator)
}
