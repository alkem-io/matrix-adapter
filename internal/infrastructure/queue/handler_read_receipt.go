package queue

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// ReadReceiptHandler handles queue messages related to read receipt operations.
type ReadReceiptHandler struct {
	service  *service.ReadReceiptService
	matrix   ports.MatrixPort
	idMapper *domain.IDMapper
	logger   ports.Logger
}

// NewReadReceiptHandler creates a new instance of ReadReceiptHandler.
func NewReadReceiptHandler(
	svc *service.ReadReceiptService,
	matrix ports.MatrixPort,
	logger ports.Logger,
) *ReadReceiptHandler {
	return &ReadReceiptHandler{
		service:  svc,
		matrix:   matrix,
		idMapper: domain.NewIDMapper(matrix.HomeserverDomain()),
		logger:   logger,
	}
}

// HandleMarkMessageRead handles the communication.message.read topic.
func (h *ReadReceiptHandler) HandleMarkMessageRead(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.MarkMessageReadRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if req.MessageID == "" {
		return NewInvalidParamError("message_id is required"), nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolveRoomAlias(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	// Convert thread root ID if provided
	var threadRootID *id.EventID
	if req.ThreadRootID != nil && *req.ThreadRootID != "" {
		eventID := id.EventID(*req.ThreadRootID)
		threadRootID = &eventID
	}

	// Mark message as read
	err := h.service.MarkMessageRead(
		ctx,
		req.ActorID.UUID(),
		roomID,
		id.EventID(req.MessageID),
		threadRootID,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleGetUnreadCounts handles the communication.room.unread_counts.get topic.
func (h *ReadReceiptHandler) HandleGetUnreadCounts(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetUnreadCountsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolveRoomAlias(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	// Convert thread root IDs
	var threadRootIDs []id.EventID
	for _, threadID := range req.ThreadRootIDs {
		if threadID != "" {
			threadRootIDs = append(threadRootIDs, id.EventID(threadID))
		}
	}

	// Get unread counts
	summary, err := h.service.GetUnreadCounts(
		ctx,
		req.ActorID.UUID(),
		roomID,
		threadRootIDs,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	// Convert to response
	threadCounts := make(map[string]int)
	for threadID, count := range summary.ThreadUnreadCounts {
		threadCounts[threadID.String()] = count
	}

	return dto.GetUnreadCountsResponse{
		BaseResponse:       dto.NewSuccessResponse(),
		RoomUnreadCount:    summary.RoomUnreadCount,
		ThreadUnreadCounts: threadCounts,
	}, nil
}

// resolveRoomAlias resolves an Alkemio room ID to a Matrix room ID.
func (h *ReadReceiptHandler) resolveRoomAlias(ctx context.Context, alkemioRoomID dto.AlkemioRoomID) (id.RoomID, *dto.BaseResponse) {
	alias := h.idMapper.RoomAlias(alkemioRoomID.UUID())
	roomID, err := h.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		resp := NewRoomNotFoundError(alkemioRoomID.String())
		return "", &resp
	}
	return roomID, nil
}
