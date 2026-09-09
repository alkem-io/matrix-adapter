package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/config"
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

	// setChildrenTimeout bounds one HandleSetChildren call's own execution —
	// see config.Hierarchy.SetChildrenTimeoutSeconds. Set once at
	// construction from configuration rather than read as a compile-time
	// constant at call time, so it is overridable the same way the adapter's
	// other hierarchy budgets are.
	setChildrenTimeout time.Duration
}

// NewSpaceHandler creates a new instance of SpaceHandler.
func NewSpaceHandler(service *service.SpaceService, matrix ports.MatrixPort, idMapper *domain.IDMapper, cfg *config.Config) *SpaceHandler {
	return &SpaceHandler{
		service:            service,
		matrix:             matrix,
		resolver:           NewAliasResolver(matrix, idMapper),
		setChildrenTimeout: time.Duration(cfg.Hierarchy.SetChildrenTimeoutSeconds * float64(time.Second)),
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

// maxDesiredChildContextIDs caps how many children one set_children call may
// name. The platform's forum holds a category count in the tens to low
// hundreds; this is a generous multiple of that expected scale, not a tuned
// operational limit — its purpose is to reject a pathologically oversized
// request outright rather than let it run the write loops for however long
// that many entries take before the handler's own deadline below cuts it off.
const maxDesiredChildContextIDs = 2000

// HandleSetChildren handles communication.hierarchy.set_children topic.
func (h *SpaceHandler) HandleSetChildren(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.SetChildrenRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.ParentContextID, "parent_context_id"); errResp != nil {
		return *errResp, nil
	}
	if len(req.DesiredChildContextIDs) > maxDesiredChildContextIDs {
		return NewInvalidParamError(fmt.Sprintf(
			"desired_child_context_ids has %d entries, exceeding the %d maximum per call",
			len(req.DesiredChildContextIDs), maxDesiredChildContextIDs)), nil
	}

	ctx, cancel := context.WithTimeout(ctx, h.setChildrenTimeout)
	defer cancel()

	result, err := h.service.SetChildren(ctx, service.SetChildrenParams{
		ParentContextID:        req.ParentContextID.UUID(),
		DesiredChildContextIDs: req.DesiredChildContextIDs,
		ChildrenAreSpaces:      req.ChildrenAreSpaces,
		ApplyRemovals:          req.ApplyRemovals,
		PruneUnknown:           req.PruneUnknown,
		SyncChildParent:        req.SyncChildParent,
		DryRun:                 req.DryRun,
	})
	if err != nil {
		// result is nil here (e.g. SPACE_NOT_FOUND, or a children-read failure) —
		// newSetChildrenResponse still normalizes every array to [] rather than
		// letting them marshal as null, exactly as it does on the success path.
		return newSetChildrenResponse(MapServiceError(err), nil, req.DryRun), nil
	}

	base := dto.NewSuccessResponse()
	if !result.Success {
		// Honest partial converge: some or all attempted writes did not
		// complete after others may have succeeded. No compensating rollback
		// — the caller repeats the operation until a pass reports success.
		//
		// The abort cause is reported distinctly so a caller does not read a
		// self-inflicted timeout as a Matrix-side failure: when the call's own
		// deadline (SetChildrenTimeout) was reached before every write it
		// still needed to attempt, and no attempted write actually failed,
		// that is reported as ErrCodeDeadlineExceeded ("run me again, I ran
		// out of time"), never as a write failure. Any actual write rejection
		// by Matrix — even one that happens to coincide with the deadline —
		// takes priority and is reported as ErrCodeMatrixError, since that is
		// the more actionable signal.
		switch {
		case result.DeadlineExceeded && !result.WriteFailed:
			base = dto.NewErrorResponse(dto.ErrCodeDeadlineExceeded,
				"set_children reached its own execution deadline before every hierarchy write could be attempted; no attempted write failed — convergence is partial, repeat the call to continue")
		case result.WriteFailed:
			base = dto.NewErrorResponse(dto.ErrCodeMatrixError, "one or more hierarchy writes failed; convergence is partial")
		default:
			// Failed with neither a rejected write nor an expired deadline:
			// a Matrix *read* this pass's decisions depend on (alias
			// resolution, extra-edge classification, room-side parent
			// pointers) did not answer, so the pass deliberately withheld
			// the actions that read would have justified. Saying "writes
			// failed" here would send an operator hunting a write rejection
			// that never happened.
			base = dto.NewErrorResponse(dto.ErrCodeMatrixError,
				"one or more Matrix reads this pass depends on did not answer, so part of the convergence was withheld rather than performed on an unverified read; convergence is partial, repeat the call to continue")
		}
	}

	return newSetChildrenResponse(base, result, req.DryRun), nil
}

// newSetChildrenResponse builds the wire response from a service result that
// may be nil (the error branches, where SetChildren returned before producing
// one). Every array field is normalized to [] rather than left as a nil Go
// slice on either branch — the generated TS contract declares them
// non-nullable string[], and a caller that accumulates counts from these
// arrays before checking `success` (the documented usage pattern) would throw
// on a null. This is the single place that response shape is assembled, so
// the invariant cannot drift between the success and error paths again.
func newSetChildrenResponse(base dto.BaseResponse, result *service.SetChildrenResult, dryRun bool) dto.SetChildrenResponse {
	resp := dto.SetChildrenResponse{
		BaseResponse:           base,
		Added:                  []string{},
		Removed:                []string{},
		PrunedUnknown:          []string{},
		UnknownKept:            []string{},
		Unresolved:             []string{},
		ParentPointersRepaired: []string{},
		ParentPointersDeferred: []string{},
		DryRun:                 dryRun,
	}
	if result != nil {
		resp.Added = emptyIfNil(result.Added)
		resp.Removed = emptyIfNil(result.Removed)
		resp.PrunedUnknown = emptyIfNil(result.PrunedUnknown)
		resp.UnknownKept = emptyIfNil(result.UnknownKept)
		resp.Unresolved = emptyIfNil(result.Unresolved)
		resp.ParentPointersRepaired = emptyIfNil(result.ParentPointersRepaired)
		resp.ParentPointersDeferred = emptyIfNil(result.ParentPointersDeferred)
		resp.Changed = result.Changed
	}
	return resp
}

// emptyIfNil normalizes a nil slice to an empty one so response arrays are
// always present on the wire rather than sometimes null.
func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
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
