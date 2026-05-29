package queue

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// ReadReceiptHandler handles queue messages related to read receipt operations.
type ReadReceiptHandler struct {
	service  ports.ReadReceiptServicePort
	matrix   ports.MatrixPort
	resolver *AliasResolver
	idMapper *domain.IDMapper
	logger   ports.Logger
}

// NewReadReceiptHandler creates a new instance of ReadReceiptHandler.
func NewReadReceiptHandler(
	svc ports.ReadReceiptServicePort,
	matrix ports.MatrixPort,
	idMapper *domain.IDMapper,
	logger ports.Logger,
) *ReadReceiptHandler {
	return &ReadReceiptHandler{
		service:  svc,
		matrix:   matrix,
		resolver: NewAliasResolver(matrix, idMapper),
		idMapper: idMapper,
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
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	// Convert thread ID if provided
	var threadID *id.EventID
	if req.ThreadID != nil && *req.ThreadID != "" {
		eventID := id.EventID(*req.ThreadID)
		threadID = &eventID
	}

	// Mark message as read
	err := h.service.MarkMessageRead(
		ctx,
		req.ActorID.UUID(),
		roomID,
		id.EventID(req.MessageID),
		threadID,
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
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	// Convert thread IDs
	var threadIDs []id.EventID
	for _, tid := range req.ThreadIDs {
		if tid != "" {
			threadIDs = append(threadIDs, id.EventID(tid))
		}
	}

	// Get unread counts
	summary, err := h.service.GetUnreadCounts(
		ctx,
		req.ActorID.UUID(),
		roomID,
		threadIDs,
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

// HandleBatchGetUnreadCounts handles the communication.room.batch.unread_counts.get topic.
// Uses a single Matrix sync call for all rooms for efficiency.
func (h *ReadReceiptHandler) HandleBatchGetUnreadCounts(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.BatchGetUnreadCountsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if len(req.AlkemioRoomIDs) == 0 {
		return NewInvalidParamError("alkemio_room_ids is required"), nil
	}

	// First, resolve all room aliases to Matrix room IDs
	matrixRoomIDs := make([]id.RoomID, 0, len(req.AlkemioRoomIDs))
	alkemioToMatrix := make(map[id.RoomID]dto.AlkemioRoomID) // Reverse mapping
	errors := make(map[string]dto.BaseResponse)

	for _, alkemioRoomID := range req.AlkemioRoomIDs {
		roomID, errResp := h.resolver.ResolveRoom(ctx, alkemioRoomID)
		if errResp != nil {
			errors[alkemioRoomID.String()] = *errResp
			continue
		}
		matrixRoomIDs = append(matrixRoomIDs, roomID)
		alkemioToMatrix[roomID] = alkemioRoomID
	}

	// Use efficient batch method - single sync for all rooms
	actor := domain.Actor{ID: req.ActorID.UUID()}
	batchResults, batchErrors := h.matrix.GetBatchUnreadCounts(ctx, actor, matrixRoomIDs)

	// Convert results back to Alkemio room IDs
	unreadCounts := make(map[string]int)
	for matrixRoomID, count := range batchResults {
		alkemioRoomID := alkemioToMatrix[matrixRoomID]
		unreadCounts[alkemioRoomID.String()] = count
	}
	for matrixRoomID, err := range batchErrors {
		alkemioRoomID := alkemioToMatrix[matrixRoomID]
		errors[alkemioRoomID.String()] = MapToBatchResult(err)
	}

	resp := dto.BatchGetUnreadCountsResponse{
		BaseResponse: dto.NewSuccessResponse(),
		UnreadCounts: unreadCounts,
	}
	if len(errors) > 0 {
		resp.Errors = errors
	}
	return resp, nil
}
