package queue

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// SpaceHandler handles queue messages related to space operations.
type SpaceHandler struct {
	service  *service.SpaceService
	matrix   ports.MatrixPort
	resolver *AliasResolver
}

// NewSpaceHandler creates a new instance of SpaceHandler.
func NewSpaceHandler(service *service.SpaceService, matrix ports.MatrixPort, idMapper *domain.IDMapper) *SpaceHandler {
	return &SpaceHandler{
		service:  service,
		matrix:   matrix,
		resolver: NewAliasResolver(matrix, idMapper),
	}
}

// ============================================================================
// Space Handlers (communication.space.*)
// ============================================================================

// HandleCreateSpace handles communication.space.create topic.
func (h *SpaceHandler) HandleCreateSpace(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.CreateSpaceRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if errResp := RequireUUID(req.AlkemioContextID, "alkemio_context_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(req.Name, "name"); errResp != nil {
		return *errResp, nil
	}

	// Convert initial members to domain actors
	var initialMembers []domain.Actor
	for _, memberID := range req.InitialMembers {
		if memberID.UUID() != uuid.Nil {
			initialMembers = append(initialMembers, domain.NewActor(memberID.UUID()))
		}
	}

	// Convert parent context ID
	var parentContextID *uuid.UUID
	if req.ParentContextID != nil && req.ParentContextID.UUID() != uuid.Nil {
		parentID := req.ParentContextID.UUID()
		parentContextID = &parentID
	}

	err := h.service.CreateSpace(
		ctx,
		req.AlkemioContextID.UUID(),
		req.Name,
		req.Topic,
		req.AvatarURL,
		string(req.JoinRule),
		req.IsPublic,
		req.CustomState,
		parentContextID,
		initialMembers,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleGetSpace handles communication.space.get topic.
func (h *SpaceHandler) HandleGetSpace(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetSpaceRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioContextID, "alkemio_context_id"); errResp != nil {
		return *errResp, nil
	}

	space, err := h.service.GetSpace(ctx, req.AlkemioContextID.UUID())
	if err != nil {
		return MapServiceError(err), nil
	}

	// Convert domain members to DTO
	memberActorIDs := convertMemberIDsToDTO(space.MemberIDs)

	// Convert domain children to DTO
	children := make([]dto.SpaceChildDto, 0, len(space.Children))
	for _, child := range space.Children {
		children = append(children, dto.SpaceChildDto{
			ChildID:   child.ChildID,
			IsSpace:   child.IsSpace,
			Order:     child.Order,
			Suggested: child.Suggested,
		})
	}

	// Convert parent context ID
	var parentContextID *dto.AlkemioContextID
	if space.ParentContextID != nil {
		pid := dto.AlkemioContextID(*space.ParentContextID)
		parentContextID = &pid
	}

	return dto.GetSpaceResponse{
		BaseResponse:     dto.NewSuccessResponse(),
		AlkemioContextID: dto.AlkemioContextID(space.AlkemioContextID),
		DisplayName:      space.Name,
		Topic:            space.Topic,
		AvatarURL:        space.AvatarURL,
		JoinRule:         dto.JoinRule(space.JoinRule),
		CustomState:      space.CustomState,
		MemberActorIDs:   memberActorIDs,
		Children:         children,
		ParentContextID:  parentContextID,
	}, nil
}

// HandleUpdateSpace handles communication.space.update topic.
func (h *SpaceHandler) HandleUpdateSpace(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.UpdateSpaceRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioContextID, "alkemio_context_id"); errResp != nil {
		return *errResp, nil
	}

	// Convert JoinRule pointer
	var joinRule *string
	if req.JoinRule != nil {
		jr := string(*req.JoinRule)
		joinRule = &jr
	}

	err := h.service.UpdateSpace(
		ctx,
		req.AlkemioContextID.UUID(),
		req.Name,
		req.Topic,
		req.AvatarURL,
		joinRule,
		req.IsPublic,
		req.CustomState,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleDeleteSpace handles communication.space.delete topic.
func (h *SpaceHandler) HandleDeleteSpace(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.DeleteSpaceRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioContextID, "alkemio_context_id"); errResp != nil {
		return *errResp, nil
	}

	err := h.service.DeleteSpace(ctx, req.AlkemioContextID.UUID(), req.Reason)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleListSpaces handles communication.space.list topic.
func (h *SpaceHandler) HandleListSpaces(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.ListSpacesRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	alkemioContextIDs, nextCursor, err := h.service.ListSpaces(ctx, req.Cursor)
	if err != nil {
		return MapServiceError(err), nil
	}

	// Convert to DTO type
	contextIDs := make([]dto.AlkemioContextID, 0, len(alkemioContextIDs))
	for _, contextID := range alkemioContextIDs {
		contextIDs = append(contextIDs, dto.AlkemioContextID(contextID))
	}

	return dto.ListSpacesResponse{
		BaseResponse:      dto.NewSuccessResponse(),
		AlkemioContextIDs: contextIDs,
		NextCursor:        nextCursor,
	}, nil
}

// ============================================================================
// Hierarchy Handlers (communication.hierarchy.*)
// ============================================================================

// HandleSetParent handles communication.hierarchy.set_parent topic.
func (h *SpaceHandler) HandleSetParent(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.SetParentRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireNonEmpty(req.ChildID, "child_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.ParentContextID, "parent_context_id"); errResp != nil {
		return *errResp, nil
	}

	err := h.service.SetParent(
		ctx,
		req.ChildID,
		req.IsSpace,
		req.ParentContextID.UUID(),
		req.Order,
		req.Suggested,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// ============================================================================
// Batch Space Membership Handlers (communication.space.member.batch.*)
// ============================================================================

// HandleBatchAddSpaceMember handles communication.space.member.batch.add topic.
func (h *SpaceHandler) HandleBatchAddSpaceMember(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.BatchAddSpaceMemberRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if len(req.AlkemioContextIDs) == 0 {
		return NewInvalidParamError("alkemio_context_ids is required"), nil
	}

	// Convert to UUID slice
	contextIDs := make([]uuid.UUID, 0, len(req.AlkemioContextIDs))
	for _, contextID := range req.AlkemioContextIDs {
		contextIDs = append(contextIDs, contextID.UUID())
	}

	resultErrors := h.service.BatchAddMember(
		ctx,
		req.ActorID.UUID(),
		contextIDs,
	)

	// Convert to response format
	results := make(map[string]dto.BaseResponse)
	for contextID, err := range resultErrors {
		results[contextID] = MapToBatchResult(err)
	}

	return dto.BatchAddSpaceMemberResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Results:      results,
	}, nil
}

// HandleBatchRemoveSpaceMember handles communication.space.member.batch.remove topic.
func (h *SpaceHandler) HandleBatchRemoveSpaceMember(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.BatchRemoveSpaceMemberRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if len(req.AlkemioContextIDs) == 0 {
		return NewInvalidParamError("alkemio_context_ids is required"), nil
	}

	// Convert to UUID slice
	contextIDs := make([]uuid.UUID, 0, len(req.AlkemioContextIDs))
	for _, contextID := range req.AlkemioContextIDs {
		contextIDs = append(contextIDs, contextID.UUID())
	}

	resultErrors := h.service.BatchRemoveMember(
		ctx,
		req.ActorID.UUID(),
		contextIDs,
		req.Reason,
	)

	// Convert to response format
	results := make(map[string]dto.BaseResponse)
	for contextID, err := range resultErrors {
		results[contextID] = MapToBatchResult(err)
	}

	return dto.BatchRemoveSpaceMemberResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Results:      results,
	}, nil
}

// ============================================================================
// Custom State Handlers (communication.space.state.*)
// ============================================================================

// HandleSetSpaceState handles communication.space.state.set topic.
func (h *SpaceHandler) HandleSetSpaceState(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.SetSpaceStateRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioContextID, "alkemio_context_id"); errResp != nil {
		return *errResp, nil
	}

	roomID, errResp := h.resolver.ResolveSpace(ctx, req.AlkemioContextID)
	if errResp != nil {
		return *errResp, nil
	}

	if err := h.matrix.SetCustomState(ctx, roomID, req.State); err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleGetSpaceState handles communication.space.state.get topic.
func (h *SpaceHandler) HandleGetSpaceState(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetSpaceStateRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioContextID, "alkemio_context_id"); errResp != nil {
		return *errResp, nil
	}

	roomID, errResp := h.resolver.ResolveSpace(ctx, req.AlkemioContextID)
	if errResp != nil {
		return *errResp, nil
	}

	state, err := h.matrix.GetCustomState(ctx, roomID, req.EventTypes)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.GetSpaceStateResponse{
		BaseResponse:     dto.NewSuccessResponse(),
		AlkemioContextID: req.AlkemioContextID,
		State:            state,
	}, nil
}
