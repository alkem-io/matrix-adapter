package queue

import (
	"context"
	"encoding/json"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
	"github.com/google/uuid"
)

// ActorHandler handles queue messages related to actor operations.
type ActorHandler struct {
	service *service.ActorService
}

// NewActorHandler creates a new instance of ActorHandler.
func NewActorHandler(service *service.ActorService) *ActorHandler {
	return &ActorHandler{
		service: service,
	}
}

// HandleRegister handles the actor registration message.
func (h *ActorHandler) HandleRegister(payload []byte) (interface{}, error) {
	var req dto.ActorRegisterPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.ActorID)
	if err != nil {
		return nil, err
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = actorID.String()
	}

	actor := domain.Actor{
		ID:          actorID,
		DisplayName: displayName,
	}

	matrixID, err := h.service.RegisterActor(context.Background(), actor)
	if err != nil {
		return nil, err
	}

	return dto.ActorRegisterResponse{
		Success:  true,
		MatrixID: string(matrixID),
	}, nil
}

// HandleAddToRooms handles the message to add an actor to multiple rooms.
func (h *ActorHandler) HandleAddToRooms(payload []byte) (interface{}, error) {
	var req dto.ActorAddToRoomsPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.ActorID)
	if err != nil {
		return nil, err
	}

	failed, created := h.service.AddToRooms(context.Background(), domain.Actor{ID: actorID}, req.RoomIDs)

	return dto.ActorAddToRoomsResponse{
		Success:      len(failed) == 0,
		FailedRooms:  failed,
		CreatedRooms: created,
	}, nil
}

// HandleRemoveFromRooms handles the message to remove an actor from multiple rooms.
func (h *ActorHandler) HandleRemoveFromRooms(payload []byte) (interface{}, error) {
	var req dto.ActorRemoveFromRoomsPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.ActorID)
	if err != nil {
		return nil, err
	}

	failed := h.service.RemoveFromRooms(context.Background(), domain.Actor{ID: actorID}, req.RoomIDs)

	return dto.ActorRemoveFromRoomsResponse{
		Success:     len(failed) == 0,
		FailedRooms: failed,
	}, nil
}

// HandleGetRooms handles the message to get all rooms an actor is in.
func (h *ActorHandler) HandleGetRooms(payload []byte) (interface{}, error) {
	var req dto.ActorRoomsPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.ActorID)
	if err != nil {
		return nil, err
	}

	rooms, err := h.service.GetRooms(context.Background(), domain.Actor{ID: actorID})
	if err != nil {
		return nil, err
	}

	return dto.ActorRoomsResponse{
		RoomIDs: rooms,
	}, nil
}

// HandleStartDirectMessaging handles the message to start a DM.
func (h *ActorHandler) HandleStartDirectMessaging(payload []byte) (interface{}, error) {
	var req dto.ActorStartDirectMessagingPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	initiatorID, err := uuid.Parse(req.InitiatingActorID)
	if err != nil {
		return nil, err
	}
	receiverID, err := uuid.Parse(req.ReceiverActorID)
	if err != nil {
		return nil, err
	}

	roomID, err := h.service.CreateDirectRoom(
		context.Background(), domain.Actor{ID: initiatorID}, domain.Actor{ID: receiverID},
	)
	if err != nil {
		return nil, err
	}

	return dto.ActorStartDirectMessagingResponse{
		RoomID: roomID.String(),
		IsNew:  true,
	}, nil
}

// HandleStopDirectMessaging handles the message to stop a DM.
func (h *ActorHandler) HandleStopDirectMessaging(payload []byte) (interface{}, error) {
	var req dto.ActorStopDirectMessagingPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	initiatorID, err := uuid.Parse(req.InitiatingActorID)
	if err != nil {
		return nil, err
	}
	receiverID, err := uuid.Parse(req.ReceiverActorID)
	if err != nil {
		return nil, err
	}

	err = h.service.StopDirectMessaging(
		context.Background(), domain.Actor{ID: initiatorID}, domain.Actor{ID: receiverID},
	)
	if err != nil {
		return nil, err
	}

	return dto.ActorStopDirectMessagingResponse{Success: true}, nil
}

// HandleGetDirectRooms handles the message to get DM rooms.
func (h *ActorHandler) HandleGetDirectRooms(payload []byte) (interface{}, error) {
	var req dto.ActorRoomsDirectPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.ActorID)
	if err != nil {
		return nil, err
	}

	dms, err := h.service.GetDirectRooms(context.Background(), domain.Actor{ID: actorID})
	if err != nil {
		return nil, err
	}

	directRooms := make(map[string]string)
	for userID, roomID := range dms {
		directRooms[userID.String()] = roomID.String()
	}

	return dto.ActorRoomsDirectResponse{
		DirectRooms: directRooms,
	}, nil
}
