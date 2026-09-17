package queue

import (
	"context"
	"encoding/json"

	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// ActorHandler handles queue messages related to actor operations.
type ActorHandler struct {
	service *service.ActorService
	devices *service.DeviceService
}

// NewActorHandler creates a new instance of ActorHandler.
func NewActorHandler(service *service.ActorService, devices *service.DeviceService) *ActorHandler {
	return &ActorHandler{
		service: service,
		devices: devices,
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

	return dto.NewSuccessResponse(), nil
}

// HandleRevokeActorDevices handles communication.actor.devices.revoke:
// delete ALL of the actor's Matrix devices (idempotent — zero devices is a
// success with deleted_count 0). An admin failure is an error response,
// never a false success.
func (h *ActorHandler) HandleRevokeActorDevices(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.RevokeActorDevicesRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}
	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}

	deviceIDs, err := h.devices.RevokeActorDevices(ctx, req.ActorID.UUID(), req.Reason)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.RevokeActorDevicesResponse{
		BaseResponse: dto.NewSuccessResponse(),
		DeletedCount: len(deviceIDs),
		DeviceIDs:    deviceIDs,
	}, nil
}
