package queue

import (
	"context"
	"encoding/json"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// AdminHandler handles queue messages related to admin operations.
type AdminHandler struct {
	service *service.AdminService
}

// NewAdminHandler creates a new instance of AdminHandler.
func NewAdminHandler(service *service.AdminService) *AdminHandler {
	return &AdminHandler{
		service: service,
	}
}

// HandleGetAllRooms handles the message to get all rooms the bot is in.
func (h *AdminHandler) HandleGetAllRooms(_ []byte) (interface{}, error) {
	rooms, err := h.service.GetAllRooms(context.Background())
	if err != nil {
		return nil, err
	}

	roomResponses := make([]dto.RoomDetailsResponse, 0, len(rooms))
	for _, r := range rooms {
		roomResponses = append(
			roomResponses, dto.RoomDetailsResponse{
				RoomID: r.ID.String(),
				Name:   r.Name,
				Topic:  r.Topic,
				Alias:  r.Alias,
			},
		)
	}

	return dto.AdminAllRoomsResponse{
		Rooms: roomResponses,
	}, nil
}

// HandleReplicateRoomMembership handles the message to replicate room membership.
func (h *AdminHandler) HandleReplicateRoomMembership(payload []byte) (interface{}, error) {
	var req dto.AdminReplicateRoomMembershipPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	prioritizerID, err := uuid.Parse(req.ActorToPrioritize)
	if err != nil {
		return nil, err
	}

	added, failed, err := h.service.ReplicateRoomMembership(
		context.Background(), id.RoomID(req.SourceRoomID), id.RoomID(req.TargetRoomID), domain.Actor{ID: prioritizerID},
	)
	if err != nil {
		return nil, err
	}

	return dto.AdminReplicateRoomMembershipResponse{
		Success:     true,
		AddedUsers:  added,
		FailedUsers: failed,
	}, nil
}
