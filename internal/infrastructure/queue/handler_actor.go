package queue

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
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

// HandleSyncActor handles communication.actor.sync topic.
// This is an idempotent operation that ensures an actor exists and updates their profile.
func (h *ActorHandler) HandleSyncActor(payload []byte) (interface{}, error) {
	var req dto.SyncActorRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if req.ActorID.UUID() == uuid.Nil {
		return NewInvalidParamError("actor_id is required"), nil
	}
	if req.DisplayName == "" {
		return NewInvalidParamError("display_name is required"), nil
	}

	err := h.service.SyncActor(
		context.Background(),
		req.ActorID.UUID(),
		req.DisplayName,
		req.AvatarURL,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.SyncActorResponse{BaseResponse: dto.NewSuccessResponse()}, nil
}
