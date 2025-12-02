package queue

import (
	"context"
	"encoding/json"

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
func (h *ActorHandler) HandleSyncActor(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.SyncActorRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(req.DisplayName, "display_name"); errResp != nil {
		return *errResp, nil
	}

	err := h.service.SyncActor(
		ctx,
		req.ActorID.UUID(),
		req.DisplayName,
		req.AvatarURL,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.SyncActorResponse{BaseResponse: dto.NewSuccessResponse()}, nil
}
