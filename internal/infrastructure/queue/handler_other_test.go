package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// ============================================================================
// Test Helpers
// ============================================================================

func otherNewIDMapper() *domain.IDMapper {
	return domain.NewIDMapper(testOtherDomain)
}

func otherNewMatrixPort() *testMockMatrixPort {
	return &testMockMatrixPort{
		homeserverDomain:    testOtherDomain,
		resolveAliasResults: make(map[string]id.RoomID),
		resolveAliasErrs:    make(map[string]error),
	}
}

// ============================================================================
// SpaceHandler Tests
// ============================================================================

func newTestSpaceHandler(matrix *testMockMatrixPort) *SpaceHandler {
	idMapper := otherNewIDMapper()
	logger := &testMockLogger{}
	svc := service.NewSpaceService(matrix, logger, idMapper)
	return NewSpaceHandler(svc, matrix, idMapper)
}

// --- HandleCreateSpace ---

func TestHandleCreateSpace_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	contextID := uuid.New()
	idMapper := otherNewIDMapper()
	alias := idMapper.SpaceAlias(contextID)

	// Alias not found on first check, then space created
	matrix.resolveAliasErrs[alias] = domain.ErrSpaceNotFound
	matrix.createSpaceResult = "!newspace:matrix.example.com"

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.CreateSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		Name:             "Test Space",
	})

	resp, err := handler.HandleCreateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)

	// Verify the handler correctly parsed and forwarded DTO fields
	if matrix.capturedCreateSpaceName != "Test Space" {
		t.Errorf("expected space name 'Test Space', got %q", matrix.capturedCreateSpaceName)
	}
	if matrix.capturedCreateSpaceAlkemioID != contextID {
		t.Errorf("expected alkemio context ID %s, got %s", contextID, matrix.capturedCreateSpaceAlkemioID)
	}
}

func TestHandleCreateSpace_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleCreateSpace(context.Background(), []byte(`{invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleCreateSpace_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.CreateSpaceRequest{
		Name: "Test Space",
		// AlkemioContextID is zero-value
	})

	resp, err := handler.HandleCreateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleCreateSpace_MissingName(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	payload, _ := json.Marshal(dto.CreateSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		// Name is empty
	})

	resp, err := handler.HandleCreateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleCreateSpace_ServiceError(t *testing.T) {
	matrix := otherNewMatrixPort()
	contextID := uuid.New()
	idMapper := otherNewIDMapper()
	alias := idMapper.SpaceAlias(contextID)

	// Alias check returns non-not-found error
	matrix.resolveAliasErrs[alias] = errors.New("connection timeout") //nolint:err113

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.CreateSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		Name:             "Test Space",
	})

	resp, err := handler.HandleCreateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

// --- HandleGetSpace ---

func TestHandleGetSpace_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	contextID := uuid.New()
	idMapper := otherNewIDMapper()
	alias := idMapper.SpaceAlias(contextID)
	roomID := id.RoomID("!space1:matrix.example.com")

	matrix.resolveAliasResults[alias] = roomID
	matrix.getSpaceDetailsResult = &domain.Space{
		ID:    roomID,
		Name:  "Test Space",
		Topic: "A test space",
		Alias: alias,
	}
	matrix.getSpaceMembersResult = []id.UserID{}
	matrix.getSpaceChildrenResult = []domain.SpaceChild{}

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.GetSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
	})

	resp, err := handler.HandleGetSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	getResp, ok := resp.(dto.GetSpaceResponse)
	if !ok {
		t.Fatalf("expected GetSpaceResponse, got %T", resp)
	}
	if !getResp.Success {
		t.Fatalf("expected success, got error: %+v", getResp.Error)
	}
	if getResp.DisplayName != "Test Space" {
		t.Errorf("expected display name 'Test Space', got '%s'", getResp.DisplayName)
	}
}

func TestHandleGetSpace_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleGetSpace(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetSpace_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.GetSpaceRequest{})

	resp, err := handler.HandleGetSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetSpace_NotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	payload, _ := json.Marshal(dto.GetSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
	})

	// Default resolveAlias returns ErrRoomNotFound for unknown aliases
	resp, err := handler.HandleGetSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeSpaceNotFound)
}

// --- HandleUpdateSpace ---

func TestHandleUpdateSpace_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	contextID := uuid.New()
	idMapper := otherNewIDMapper()
	alias := idMapper.SpaceAlias(contextID)
	roomID := id.RoomID("!space1:matrix.example.com")

	matrix.resolveAliasResults[alias] = roomID

	handler := newTestSpaceHandler(matrix)

	newName := "Updated Name"
	payload, _ := json.Marshal(dto.UpdateSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		Name:             &newName,
	})

	resp, err := handler.HandleUpdateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)

	// Verify the handler correctly parsed and forwarded DTO fields
	if matrix.capturedUpdateSpaceStateName == nil || *matrix.capturedUpdateSpaceStateName != "Updated Name" {
		t.Errorf("expected space name 'Updated Name', got %v", matrix.capturedUpdateSpaceStateName)
	}
	if matrix.capturedUpdateSpaceStateRoomID != roomID {
		t.Errorf("expected space room ID %q, got %q", roomID, matrix.capturedUpdateSpaceStateRoomID)
	}
}

func TestHandleUpdateSpace_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleUpdateSpace(context.Background(), []byte(`{broken`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleUpdateSpace_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.UpdateSpaceRequest{})

	resp, err := handler.HandleUpdateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleUpdateSpace_NotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	newName := "Updated"
	payload, _ := json.Marshal(dto.UpdateSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		Name:             &newName,
	})

	resp, err := handler.HandleUpdateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeSpaceNotFound)
}

// --- HandleDeleteSpace ---

func TestHandleDeleteSpace_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	contextID := uuid.New()
	idMapper := otherNewIDMapper()
	alias := idMapper.SpaceAlias(contextID)
	roomID := id.RoomID("!space1:matrix.example.com")

	matrix.resolveAliasResults[alias] = roomID
	matrix.getSpaceMembersResult = []id.UserID{}

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.DeleteSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		Reason:           "testing",
	})

	resp, err := handler.HandleDeleteSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)
}

func TestHandleDeleteSpace_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleDeleteSpace(context.Background(), []byte(`???`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleDeleteSpace_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.DeleteSpaceRequest{})

	resp, err := handler.HandleDeleteSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleDeleteSpace_IdempotentNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	payload, _ := json.Marshal(dto.DeleteSpaceRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
	})

	// Space not found should be idempotent success
	resp, err := handler.HandleDeleteSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)
}

// --- HandleListSpaces ---

func TestHandleListSpaces_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	matrix.getAllJoinedRoomsResult = []id.RoomID{}

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.ListSpacesRequest{})

	resp, err := handler.HandleListSpaces(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	listResp, ok := resp.(dto.ListSpacesResponse)
	if !ok {
		t.Fatalf("expected ListSpacesResponse, got %T", resp)
	}
	if !listResp.Success {
		t.Fatalf("expected success, got error: %+v", listResp.Error)
	}
}

func TestHandleListSpaces_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleListSpaces(context.Background(), []byte(`{{{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleListSpaces_ServiceError(t *testing.T) {
	matrix := otherNewMatrixPort()
	matrix.getAllJoinedRoomsErr = errors.New("connection failed") //nolint:err113

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.ListSpacesRequest{})

	resp, err := handler.HandleListSpaces(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

// --- HandleSetParent ---

func TestHandleSetParent_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	parentContextID := uuid.New()
	childID := uuid.New()
	parentAlias := idMapper.SpaceAlias(parentContextID)
	childAlias := idMapper.RoomAlias(childID)
	parentRoomID := id.RoomID("!parent:matrix.example.com")
	childRoomID := id.RoomID("!child:matrix.example.com")

	matrix.resolveAliasResults[parentAlias] = parentRoomID
	matrix.resolveAliasResults[childAlias] = childRoomID

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.SetParentRequest{
		ChildID:         childID.String(),
		IsSpace:         false,
		ParentContextID: dto.AlkemioContextID(parentContextID),
	})

	resp, err := handler.HandleSetParent(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)

	// Verify the handler correctly parsed and forwarded DTO fields
	if matrix.capturedAddSpaceChildChildID != childRoomID {
		t.Errorf("expected child room ID %q, got %q", childRoomID, matrix.capturedAddSpaceChildChildID)
	}
	if matrix.capturedAddSpaceChildSpaceID != parentRoomID {
		t.Errorf("expected parent space ID %q, got %q", parentRoomID, matrix.capturedAddSpaceChildSpaceID)
	}
	if matrix.capturedSetSpaceParentChildID != childRoomID {
		t.Errorf("expected set-parent child ID %q, got %q", childRoomID, matrix.capturedSetSpaceParentChildID)
	}
	if matrix.capturedSetSpaceParentParentID != parentRoomID {
		t.Errorf("expected set-parent parent ID %q, got %q", parentRoomID, matrix.capturedSetSpaceParentParentID)
	}
}

func TestHandleSetParent_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleSetParent(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetParent_MissingChildID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	parentContextID := uuid.New()
	payload, _ := json.Marshal(dto.SetParentRequest{
		// ChildID is empty
		ParentContextID: dto.AlkemioContextID(parentContextID),
	})

	resp, err := handler.HandleSetParent(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetParent_MissingParentContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.SetParentRequest{
		ChildID: uuid.New().String(),
		// ParentContextID is zero-value
	})

	resp, err := handler.HandleSetParent(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetParent_ParentNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	parentContextID := uuid.New()
	childID := uuid.New()
	payload, _ := json.Marshal(dto.SetParentRequest{
		ChildID:         childID.String(),
		ParentContextID: dto.AlkemioContextID(parentContextID),
	})

	// Default: alias not found
	resp, err := handler.HandleSetParent(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeSpaceNotFound)
}

// --- HandleBatchAddSpaceMember ---

func TestHandleBatchAddSpaceMember_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	contextID1 := uuid.New()
	contextID2 := uuid.New()
	alias1 := idMapper.SpaceAlias(contextID1)
	alias2 := idMapper.SpaceAlias(contextID2)

	matrix.resolveAliasResults[alias1] = "!space1:matrix.example.com"
	matrix.resolveAliasResults[alias2] = "!space2:matrix.example.com"

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.BatchAddSpaceMemberRequest{
		ActorID: dto.AlkemioActorID(actorID),
		AlkemioContextIDs: []dto.AlkemioContextID{
			dto.AlkemioContextID(contextID1),
			dto.AlkemioContextID(contextID2),
		},
	})

	resp, err := handler.HandleBatchAddSpaceMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	batchResp, ok := resp.(dto.BatchAddSpaceMemberResponse)
	if !ok {
		t.Fatalf("expected BatchAddSpaceMemberResponse, got %T", resp)
	}
	if !batchResp.Success {
		t.Fatalf("expected success, got error: %+v", batchResp.Error)
	}
	if len(batchResp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(batchResp.Results))
	}

	// Verify the handler correctly parsed the actor ID (last invitee captured)
	if matrix.capturedInviteToSpaceActor.ID != actorID {
		t.Errorf("expected invite actor ID %s, got %s", actorID, matrix.capturedInviteToSpaceActor.ID)
	}
}

func TestHandleBatchAddSpaceMember_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleBatchAddSpaceMember(context.Background(), []byte(`bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchAddSpaceMember_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	payload, _ := json.Marshal(dto.BatchAddSpaceMemberRequest{
		AlkemioContextIDs: []dto.AlkemioContextID{dto.AlkemioContextID(contextID)},
	})

	resp, err := handler.HandleBatchAddSpaceMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchAddSpaceMember_EmptyContextIDs(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.BatchAddSpaceMemberRequest{
		ActorID:           dto.AlkemioActorID(actorID),
		AlkemioContextIDs: []dto.AlkemioContextID{},
	})

	resp, err := handler.HandleBatchAddSpaceMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

// --- HandleBatchRemoveSpaceMember ---

func TestHandleBatchRemoveSpaceMember_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	contextID := uuid.New()
	alias := idMapper.SpaceAlias(contextID)

	matrix.resolveAliasResults[alias] = "!space1:matrix.example.com"

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.BatchRemoveSpaceMemberRequest{
		ActorID:           dto.AlkemioActorID(actorID),
		AlkemioContextIDs: []dto.AlkemioContextID{dto.AlkemioContextID(contextID)},
		Reason:            "removed from group",
	})

	resp, err := handler.HandleBatchRemoveSpaceMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	batchResp, ok := resp.(dto.BatchRemoveSpaceMemberResponse)
	if !ok {
		t.Fatalf("expected BatchRemoveSpaceMemberResponse, got %T", resp)
	}
	if !batchResp.Success {
		t.Fatalf("expected success, got error: %+v", batchResp.Error)
	}

	// Verify the handler correctly parsed and forwarded DTO fields
	expectedUserID := id.NewUserID(actorID.String(), testOtherDomain)
	if matrix.capturedKickFromSpaceUserID != expectedUserID {
		t.Errorf("expected kick user ID %q, got %q", expectedUserID, matrix.capturedKickFromSpaceUserID)
	}
	if matrix.capturedKickFromSpaceRoomID != "!space1:matrix.example.com" {
		t.Errorf("expected kick space room ID '!space1:matrix.example.com', got %q", matrix.capturedKickFromSpaceRoomID)
	}
}

func TestHandleBatchRemoveSpaceMember_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleBatchRemoveSpaceMember(context.Background(), []byte(`nope`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchRemoveSpaceMember_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	payload, _ := json.Marshal(dto.BatchRemoveSpaceMemberRequest{
		AlkemioContextIDs: []dto.AlkemioContextID{dto.AlkemioContextID(contextID)},
	})

	resp, err := handler.HandleBatchRemoveSpaceMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchRemoveSpaceMember_EmptyContextIDs(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.BatchRemoveSpaceMemberRequest{
		ActorID:           dto.AlkemioActorID(actorID),
		AlkemioContextIDs: []dto.AlkemioContextID{},
	})

	resp, err := handler.HandleBatchRemoveSpaceMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

// --- HandleSetSpaceState ---

func TestHandleSetSpaceState_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()
	contextID := uuid.New()
	alias := idMapper.SpaceAlias(contextID)
	roomID := id.RoomID("!space1:matrix.example.com")

	matrix.resolveAliasResults[alias] = roomID

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.SetSpaceStateRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		State: map[string]map[string]interface{}{
			"io.alkemio.visibility": {"visible": true},
		},
	})

	resp, err := handler.HandleSetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)

	// Verify the handler correctly parsed and forwarded DTO fields
	if matrix.capturedSetCustomStateRoomID != roomID {
		t.Errorf("expected room ID %q, got %q", roomID, matrix.capturedSetCustomStateRoomID)
	}
	if matrix.capturedSetCustomState == nil {
		t.Fatal("expected custom state to be non-nil")
	}
	vis, ok := matrix.capturedSetCustomState["io.alkemio.visibility"]
	if !ok {
		t.Fatal("expected io.alkemio.visibility in captured state")
	}
	if v, ok := vis["visible"].(bool); !ok || !v {
		t.Errorf("expected visible=true in captured state, got %v", vis["visible"])
	}
}

func TestHandleSetSpaceState_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleSetSpaceState(context.Background(), []byte(`{bad}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetSpaceState_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.SetSpaceStateRequest{
		State: map[string]map[string]interface{}{
			"io.alkemio.test": {"key": "value"},
		},
	})

	resp, err := handler.HandleSetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetSpaceState_SpaceNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	payload, _ := json.Marshal(dto.SetSpaceStateRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		State: map[string]map[string]interface{}{
			"io.alkemio.test": {"key": "value"},
		},
	})

	resp, err := handler.HandleSetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeSpaceNotFound)
}

func TestHandleSetSpaceState_MatrixError(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()
	contextID := uuid.New()
	alias := idMapper.SpaceAlias(contextID)
	roomID := id.RoomID("!space1:matrix.example.com")

	matrix.resolveAliasResults[alias] = roomID
	matrix.setCustomStateErr = errors.New("matrix internal error") //nolint:err113

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.SetSpaceStateRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
		State: map[string]map[string]interface{}{
			"io.alkemio.test": {"key": "value"},
		},
	})

	resp, err := handler.HandleSetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

// --- HandleGetSpaceState ---

func TestHandleGetSpaceState_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()
	contextID := uuid.New()
	alias := idMapper.SpaceAlias(contextID)
	roomID := id.RoomID("!space1:matrix.example.com")

	matrix.resolveAliasResults[alias] = roomID
	matrix.getCustomStateResult = map[string]map[string]interface{}{
		"io.alkemio.visibility": {"visible": true},
	}

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.GetSpaceStateRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
	})

	resp, err := handler.HandleGetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stateResp, ok := resp.(dto.GetSpaceStateResponse)
	if !ok {
		t.Fatalf("expected GetSpaceStateResponse, got %T", resp)
	}
	if !stateResp.Success {
		t.Fatalf("expected success, got error: %+v", stateResp.Error)
	}
	if stateResp.State == nil {
		t.Fatal("expected state to be non-nil")
	}
	if _, ok := stateResp.State["io.alkemio.visibility"]; !ok {
		t.Error("expected io.alkemio.visibility in state")
	}

	// Verify the handler correctly forwarded the room ID to the mock
	if matrix.capturedGetCustomStateRoomID != roomID {
		t.Errorf("expected room ID %q, got %q", roomID, matrix.capturedGetCustomStateRoomID)
	}
}

func TestHandleGetSpaceState_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleGetSpaceState(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetSpaceState_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.GetSpaceStateRequest{})

	resp, err := handler.HandleGetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetSpaceState_SpaceNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	contextID := uuid.New()
	payload, _ := json.Marshal(dto.GetSpaceStateRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
	})

	resp, err := handler.HandleGetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeSpaceNotFound)
}

func TestHandleGetSpaceState_MatrixError(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()
	contextID := uuid.New()
	alias := idMapper.SpaceAlias(contextID)
	roomID := id.RoomID("!space1:matrix.example.com")

	matrix.resolveAliasResults[alias] = roomID
	matrix.getCustomStateErr = errors.New("matrix error") //nolint:err113

	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.GetSpaceStateRequest{
		AlkemioContextID: dto.AlkemioContextID(contextID),
	})

	resp, err := handler.HandleGetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

// ============================================================================
// ActorHandler Tests
// ============================================================================

func newTestActorHandler(matrix *testMockMatrixPort) *ActorHandler {
	logger := &testMockLogger{}
	svc := service.NewActorService(matrix, logger)
	return NewActorHandler(svc)
}

func TestHandleSyncActor_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	matrix.ensureUserResult = "@actor1:matrix.example.com"

	handler := newTestActorHandler(matrix)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.SyncActorRequest{
		ActorID:     dto.AlkemioActorID(actorID),
		DisplayName: "Test Actor",
		AvatarURL:   "mxc://example.com/avatar",
	})

	resp, err := handler.HandleSyncActor(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)

	// Verify the handler correctly parsed and forwarded DTO fields
	if matrix.capturedEnsureUserActor.ID != actorID {
		t.Errorf("expected actor ID %s, got %s", actorID, matrix.capturedEnsureUserActor.ID)
	}
	if matrix.capturedSetProfileActor.DisplayName != "Test Actor" {
		t.Errorf("expected display name 'Test Actor', got %q", matrix.capturedSetProfileActor.DisplayName)
	}
	if matrix.capturedSetProfileActor.AvatarURL != "mxc://example.com/avatar" {
		t.Errorf("expected avatar URL 'mxc://example.com/avatar', got %q", matrix.capturedSetProfileActor.AvatarURL)
	}
}

func TestHandleSyncActor_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestActorHandler(matrix)

	resp, err := handler.HandleSyncActor(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSyncActor_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestActorHandler(matrix)

	payload, _ := json.Marshal(dto.SyncActorRequest{
		DisplayName: "Test Actor",
	})

	resp, err := handler.HandleSyncActor(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSyncActor_MissingDisplayName(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestActorHandler(matrix)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.SyncActorRequest{
		ActorID: dto.AlkemioActorID(actorID),
		// DisplayName is empty
	})

	resp, err := handler.HandleSyncActor(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSyncActor_EnsureUserError(t *testing.T) {
	matrix := otherNewMatrixPort()
	matrix.ensureUserErr = errors.New("matrix unavailable") //nolint:err113

	handler := newTestActorHandler(matrix)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.SyncActorRequest{
		ActorID:     dto.AlkemioActorID(actorID),
		DisplayName: "Test Actor",
	})

	resp, err := handler.HandleSyncActor(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

func TestHandleSyncActor_SetProfileError(t *testing.T) {
	matrix := otherNewMatrixPort()
	matrix.ensureUserResult = "@actor1:matrix.example.com"
	matrix.setProfileErr = errors.New("profile update failed") //nolint:err113

	handler := newTestActorHandler(matrix)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.SyncActorRequest{
		ActorID:     dto.AlkemioActorID(actorID),
		DisplayName: "Test Actor",
	})

	resp, err := handler.HandleSyncActor(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

// ============================================================================
// ReadReceiptHandler Tests
// ============================================================================

func newTestReadReceiptHandler(
	matrix *testMockMatrixPort,
	svc *testMockReadReceiptService,
) *ReadReceiptHandler {
	idMapper := otherNewIDMapper()
	logger := &testMockLogger{}
	return NewReadReceiptHandler(svc, matrix, idMapper, logger)
}

// --- HandleMarkMessageRead ---

func TestHandleMarkMessageRead_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID := uuid.New()
	alias := idMapper.RoomAlias(roomUUID)
	matrixRoomID := id.RoomID("!room1:matrix.example.com")

	matrix.resolveAliasResults[alias] = matrixRoomID

	handler := newTestReadReceiptHandler(matrix, svc)

	payload, _ := json.Marshal(dto.MarkMessageReadRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		MessageID:     "$event1:matrix.example.com",
	})

	resp, err := handler.HandleMarkMessageRead(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)

	// Verify the handler correctly parsed and forwarded DTO fields
	if svc.capturedMarkReadActorID != actorID {
		t.Errorf("expected actor ID %s, got %s", actorID, svc.capturedMarkReadActorID)
	}
	if svc.capturedMarkReadRoomID != matrixRoomID {
		t.Errorf("expected room ID %q, got %q", matrixRoomID, svc.capturedMarkReadRoomID)
	}
	if svc.capturedMarkReadEventID != "$event1:matrix.example.com" {
		t.Errorf("expected event ID '$event1:matrix.example.com', got %q", svc.capturedMarkReadEventID)
	}
	if svc.capturedMarkReadThreadID != nil {
		t.Errorf("expected nil thread ID, got %v", svc.capturedMarkReadThreadID)
	}
}

func TestHandleMarkMessageRead_WithThreadID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID := uuid.New()
	alias := idMapper.RoomAlias(roomUUID)
	matrixRoomID := id.RoomID("!room1:matrix.example.com")

	matrix.resolveAliasResults[alias] = matrixRoomID

	handler := newTestReadReceiptHandler(matrix, svc)

	threadID := dto.MessageID("$thread1:matrix.example.com")
	payload, _ := json.Marshal(dto.MarkMessageReadRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		MessageID:     "$event1:matrix.example.com",
		ThreadID:      &threadID,
	})

	resp, err := handler.HandleMarkMessageRead(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, resp)

	// Verify the handler correctly parsed and forwarded the thread ID
	if svc.capturedMarkReadActorID != actorID {
		t.Errorf("expected actor ID %s, got %s", actorID, svc.capturedMarkReadActorID)
	}
	if svc.capturedMarkReadThreadID == nil {
		t.Fatal("expected thread ID to be non-nil")
	}
	if *svc.capturedMarkReadThreadID != id.EventID("$thread1:matrix.example.com") {
		t.Errorf("expected thread ID '$thread1:matrix.example.com', got %q", *svc.capturedMarkReadThreadID)
	}
}

func TestHandleMarkMessageRead_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	resp, err := handler.HandleMarkMessageRead(context.Background(), []byte(`invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.MarkMessageReadRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		MessageID:     "$event1:matrix.example.com",
	})

	resp, err := handler.HandleMarkMessageRead(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_MissingRoomID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.MarkMessageReadRequest{
		ActorID:   dto.AlkemioActorID(actorID),
		MessageID: "$event1:matrix.example.com",
	})

	resp, err := handler.HandleMarkMessageRead(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_MissingMessageID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	actorID := uuid.New()
	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.MarkMessageReadRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		// MessageID is empty
	})

	resp, err := handler.HandleMarkMessageRead(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_RoomNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	actorID := uuid.New()
	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.MarkMessageReadRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		MessageID:     "$event1:matrix.example.com",
	})

	// Default: alias not found
	resp, err := handler.HandleMarkMessageRead(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeRoomNotFound)
}

func TestHandleMarkMessageRead_ServiceError(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{
		markMessageReadErr: errors.New("receipt send failed"), //nolint:err113
	}
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID := uuid.New()
	alias := idMapper.RoomAlias(roomUUID)
	matrixRoomID := id.RoomID("!room1:matrix.example.com")

	matrix.resolveAliasResults[alias] = matrixRoomID

	handler := newTestReadReceiptHandler(matrix, svc)

	payload, _ := json.Marshal(dto.MarkMessageReadRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		MessageID:     "$event1:matrix.example.com",
	})

	resp, err := handler.HandleMarkMessageRead(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

// --- HandleGetUnreadCounts ---

func TestHandleGetUnreadCounts_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	threadEventID := id.EventID("$thread1:matrix.example.com")
	svc := &testMockReadReceiptService{
		getUnreadCountsRes: &domain.UnreadCountSummary{
			RoomUnreadCount: 5,
			ThreadUnreadCounts: map[id.EventID]int{
				threadEventID: 2,
			},
		},
	}
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID := uuid.New()
	alias := idMapper.RoomAlias(roomUUID)
	matrixRoomID := id.RoomID("!room1:matrix.example.com")

	matrix.resolveAliasResults[alias] = matrixRoomID

	handler := newTestReadReceiptHandler(matrix, svc)

	payload, _ := json.Marshal(dto.GetUnreadCountsRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		ThreadIDs:     []dto.MessageID{"$thread1:matrix.example.com"},
	})

	resp, err := handler.HandleGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unreadResp, ok := resp.(dto.GetUnreadCountsResponse)
	if !ok {
		t.Fatalf("expected GetUnreadCountsResponse, got %T", resp)
	}
	if !unreadResp.Success {
		t.Fatalf("expected success, got error: %+v", unreadResp.Error)
	}
	if unreadResp.RoomUnreadCount != 5 {
		t.Errorf("expected room unread count 5, got %d", unreadResp.RoomUnreadCount)
	}
	if len(unreadResp.ThreadUnreadCounts) != 1 {
		t.Errorf("expected 1 thread count, got %d", len(unreadResp.ThreadUnreadCounts))
	}

	// Verify the handler correctly parsed and forwarded DTO fields
	if svc.capturedGetUnreadActorID != actorID {
		t.Errorf("expected actor ID %s, got %s", actorID, svc.capturedGetUnreadActorID)
	}
	if svc.capturedGetUnreadRoomID != matrixRoomID {
		t.Errorf("expected room ID %q, got %q", matrixRoomID, svc.capturedGetUnreadRoomID)
	}
	if len(svc.capturedGetUnreadThreadIDs) != 1 {
		t.Fatalf("expected 1 thread ID, got %d", len(svc.capturedGetUnreadThreadIDs))
	}
	if svc.capturedGetUnreadThreadIDs[0] != threadEventID {
		t.Errorf("expected thread ID %q, got %q", threadEventID, svc.capturedGetUnreadThreadIDs[0])
	}
}

func TestHandleGetUnreadCounts_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	resp, err := handler.HandleGetUnreadCounts(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetUnreadCounts_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.GetUnreadCountsRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
	})

	resp, err := handler.HandleGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetUnreadCounts_MissingRoomID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.GetUnreadCountsRequest{
		ActorID: dto.AlkemioActorID(actorID),
	})

	resp, err := handler.HandleGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetUnreadCounts_RoomNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	actorID := uuid.New()
	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.GetUnreadCountsRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
	})

	resp, err := handler.HandleGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeRoomNotFound)
}

func TestHandleGetUnreadCounts_ServiceError(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{
		getUnreadCountsErr: errors.New("sync failed"), //nolint:err113
	}
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID := uuid.New()
	alias := idMapper.RoomAlias(roomUUID)
	matrixRoomID := id.RoomID("!room1:matrix.example.com")

	matrix.resolveAliasResults[alias] = matrixRoomID

	handler := newTestReadReceiptHandler(matrix, svc)

	payload, _ := json.Marshal(dto.GetUnreadCountsRequest{
		ActorID:       dto.AlkemioActorID(actorID),
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
	})

	resp, err := handler.HandleGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeMatrixError)
}

// --- HandleBatchGetUnreadCounts ---

func TestHandleBatchGetUnreadCounts_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID1 := uuid.New()
	roomUUID2 := uuid.New()
	alias1 := idMapper.RoomAlias(roomUUID1)
	alias2 := idMapper.RoomAlias(roomUUID2)
	matrixRoomID1 := id.RoomID("!room1:matrix.example.com")
	matrixRoomID2 := id.RoomID("!room2:matrix.example.com")

	matrix.resolveAliasResults[alias1] = matrixRoomID1
	matrix.resolveAliasResults[alias2] = matrixRoomID2
	matrix.getBatchUnreadCountsRes = map[id.RoomID]int{
		matrixRoomID1: 3,
		matrixRoomID2: 7,
	}
	matrix.getBatchUnreadCountsErrs = map[id.RoomID]error{}

	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	payload, _ := json.Marshal(dto.BatchGetUnreadCountsRequest{
		ActorID: dto.AlkemioActorID(actorID),
		AlkemioRoomIDs: []dto.AlkemioRoomID{
			dto.AlkemioRoomID(roomUUID1),
			dto.AlkemioRoomID(roomUUID2),
		},
	})

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	batchResp, ok := resp.(dto.BatchGetUnreadCountsResponse)
	if !ok {
		t.Fatalf("expected BatchGetUnreadCountsResponse, got %T", resp)
	}
	if !batchResp.Success {
		t.Fatalf("expected success, got error: %+v", batchResp.Error)
	}
	if len(batchResp.UnreadCounts) != 2 {
		t.Errorf("expected 2 unread counts, got %d", len(batchResp.UnreadCounts))
	}
	if batchResp.UnreadCounts[roomUUID1.String()] != 3 {
		t.Errorf("expected room1 unread count 3, got %d", batchResp.UnreadCounts[roomUUID1.String()])
	}
	if batchResp.UnreadCounts[roomUUID2.String()] != 7 {
		t.Errorf("expected room2 unread count 7, got %d", batchResp.UnreadCounts[roomUUID2.String()])
	}
}

func TestHandleBatchGetUnreadCounts_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchGetUnreadCounts_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.BatchGetUnreadCountsRequest{
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(roomUUID)},
	})

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchGetUnreadCounts_EmptyRoomIDs(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.BatchGetUnreadCountsRequest{
		ActorID:        dto.AlkemioActorID(actorID),
		AlkemioRoomIDs: []dto.AlkemioRoomID{},
	})

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchGetUnreadCounts_PartialErrors(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID1 := uuid.New()
	roomUUID2 := uuid.New()
	alias1 := idMapper.RoomAlias(roomUUID1)
	matrixRoomID1 := id.RoomID("!room1:matrix.example.com")

	// Room1 resolves, Room2 does not
	matrix.resolveAliasResults[alias1] = matrixRoomID1
	// Room2 alias not in resolveAliasResults, default returns ErrRoomNotFound

	matrix.getBatchUnreadCountsRes = map[id.RoomID]int{
		matrixRoomID1: 5,
	}
	matrix.getBatchUnreadCountsErrs = map[id.RoomID]error{}

	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	payload, _ := json.Marshal(dto.BatchGetUnreadCountsRequest{
		ActorID: dto.AlkemioActorID(actorID),
		AlkemioRoomIDs: []dto.AlkemioRoomID{
			dto.AlkemioRoomID(roomUUID1),
			dto.AlkemioRoomID(roomUUID2),
		},
	})

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	batchResp, ok := resp.(dto.BatchGetUnreadCountsResponse)
	if !ok {
		t.Fatalf("expected BatchGetUnreadCountsResponse, got %T", resp)
	}
	if !batchResp.Success {
		t.Fatalf("expected overall success, got error: %+v", batchResp.Error)
	}
	// Room1 should have unread count
	if batchResp.UnreadCounts[roomUUID1.String()] != 5 {
		t.Errorf("expected room1 unread count 5, got %d", batchResp.UnreadCounts[roomUUID1.String()])
	}
	// Room2 should be in errors
	if batchResp.Errors == nil {
		t.Fatal("expected errors map to be non-nil")
	}
	room2Err, ok := batchResp.Errors[roomUUID2.String()]
	if !ok {
		t.Fatal("expected room2 to be in errors")
	}
	if room2Err.Success {
		t.Error("expected room2 error to have success=false")
	}
}

func TestHandleBatchGetUnreadCounts_MatrixBatchErrors(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	actorID := uuid.New()
	roomUUID1 := uuid.New()
	alias1 := idMapper.RoomAlias(roomUUID1)
	matrixRoomID1 := id.RoomID("!room1:matrix.example.com")

	matrix.resolveAliasResults[alias1] = matrixRoomID1
	matrix.getBatchUnreadCountsRes = map[id.RoomID]int{}
	matrix.getBatchUnreadCountsErrs = map[id.RoomID]error{
		matrixRoomID1: errors.New("sync error for room"), //nolint:err113
	}

	svc := &testMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	payload, _ := json.Marshal(dto.BatchGetUnreadCountsRequest{
		ActorID:        dto.AlkemioActorID(actorID),
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(roomUUID1)},
	})

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	batchResp, ok := resp.(dto.BatchGetUnreadCountsResponse)
	if !ok {
		t.Fatalf("expected BatchGetUnreadCountsResponse, got %T", resp)
	}
	if !batchResp.Success {
		t.Fatalf("expected overall success, got error: %+v", batchResp.Error)
	}
	if batchResp.Errors == nil {
		t.Fatal("expected errors map to be non-nil")
	}
	if _, ok := batchResp.Errors[roomUUID1.String()]; !ok {
		t.Error("expected room1 to have an error entry")
	}
}
