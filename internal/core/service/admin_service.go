package service

import (
	"context"
	"fmt"

	"github.com/alkemio/matrix-adapter-go/internal/core/domain"
	"github.com/alkemio/matrix-adapter-go/internal/core/ports"
	"maunium.net/go/mautrix/id"
)

// AdminService handles administrative operations.
type AdminService struct {
	matrix ports.MatrixPort
	logger ports.Logger
}

// NewAdminService creates a new instance of AdminService.
func NewAdminService(matrix ports.MatrixPort, logger ports.Logger) *AdminService {
	return &AdminService{
		matrix: matrix,
		logger: logger,
	}
}

// GetAllRooms returns a list of all rooms the bot has joined.
func (s *AdminService) GetAllRooms(ctx context.Context) ([]domain.Room, error) {
	roomIDs, err := s.matrix.GetAllJoinedRooms(ctx)
	if err != nil {
		return nil, err
	}

	rooms := make([]domain.Room, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		details, err := s.matrix.GetRoomDetails(ctx, roomID)
		if err != nil {
			s.logger.Warn("Failed to get room details", "room_id", roomID, "error", err)
			rooms = append(rooms, domain.Room{ID: roomID})
			continue
		}
		rooms = append(rooms, *details)
	}
	return rooms, nil
}

// ReplicateRoomMembership copies membership from source room to target room.
func (s *AdminService) ReplicateRoomMembership(ctx context.Context, sourceRoomID, targetRoomID id.RoomID, prioritizer domain.Actor) ([]string, []string, error) {
	members, err := s.matrix.GetRoomMembers(ctx, sourceRoomID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get source room members: %w", err)
	}

	var added []string
	var failed []string

	for _, memberID := range members {
		err := s.matrix.InviteUserByID(ctx, targetRoomID, prioritizer, memberID)
		if err != nil {
			s.logger.Warn("Failed to invite user during replication", "user_id", memberID, "error", err)
			failed = append(failed, memberID.String())
		} else {
			added = append(added, memberID.String())
		}
	}

	return added, failed, nil
}
