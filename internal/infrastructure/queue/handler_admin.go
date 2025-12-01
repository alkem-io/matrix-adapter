package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
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
		return MapServiceError(err), nil
	}

	roomResponses := make([]dto.RoomDetailsResponse, 0, len(rooms))
	for _, r := range rooms {
		roomResponses = append(
			roomResponses, dto.RoomDetailsResponse{
				BaseResponse: dto.NewSuccessResponse(),
				RoomID:       r.ID.String(),
				Name:         r.Name,
				Topic:        r.Topic,
				Alias:        r.Alias,
			},
		)
	}

	return dto.AdminAllRoomsResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Rooms:        roomResponses,
	}, nil
}

// HandleReplicateRoomMembership handles the message to replicate room membership.
func (h *AdminHandler) HandleReplicateRoomMembership(payload []byte) (interface{}, error) {
	var req dto.AdminReplicateRoomMembershipPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	prioritizerID, err := uuid.Parse(req.ActorToPrioritize)
	if err != nil {
		return NewValidationError(fmt.Sprintf("invalid actorToPrioritize ID: %s", err.Error())), nil
	}

	added, failed, err := h.service.ReplicateRoomMembership(
		context.Background(), id.RoomID(req.SourceRoomID), id.RoomID(req.TargetRoomID), domain.Actor{ID: prioritizerID},
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.AdminReplicateRoomMembershipResponsePayload{
		BaseResponse: dto.NewSuccessResponse(),
		AddedUsers:   added,
		FailedUsers:  failed,
	}, nil
}
