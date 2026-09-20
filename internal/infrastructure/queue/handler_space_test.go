package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// ============================================================================
// HandleSetChildren — DTO-boundary and wiring tests.
//
// The full 12-row hierarchy-set-children contract semantics table is exercised
// at the service layer (internal/core/service/space_service_setchildren_test.go),
// which has the fidelity to assert exact write call counts and ordering. These
// tests cover what is specific to this boundary: payload validation and that
// the handler correctly decodes the wire request into service parameters and
// re-encodes the service result onto the wire response.
// ============================================================================

func TestHandleSetChildren_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleSetChildren(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetChildren_MissingParentContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload := mustMarshal(t, dto.SetChildrenRequest{
		DesiredChildContextIDs: []string{uuid.New().String()},
	})

	resp, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetChildren_TooManyDesiredChildrenIsRejected(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	oversized := make([]string, maxDesiredChildContextIDs+1)
	for i := range oversized {
		oversized[i] = uuid.New().String()
	}
	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID:        dto.AlkemioContextID(uuid.New()),
		DesiredChildContextIDs: oversized,
	})

	resp, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetChildren_ParentNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID: dto.AlkemioContextID(uuid.New()),
	})

	// Default: alias not found.
	resp, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeSpaceNotFound)

	// The error path must uphold the same wire invariant as the success path:
	// every array field present as [] on the wire, never null. This is the
	// scenario a reconcile pass hits on every permanent-tombstone category
	// (FR-003) — a consumer that accumulates array-derived counts before
	// branching on `success` must not see a null here.
	wire := mustUnmarshalSetChildrenResponse(t, resp)
	if wire.Added == nil || wire.Removed == nil || wire.PrunedUnknown == nil || wire.UnknownKept == nil ||
		wire.Unresolved == nil || wire.ParentPointersRepaired == nil || wire.ParentPointersDeferred == nil {
		t.Errorf("expected every array field present as [] rather than null on the SPACE_NOT_FOUND error path, got %+v", wire)
	}
}

func TestHandleSetChildren_ChildrenReadFailure_ArraysAreNeverNullOnTheWire(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	parentContextID := uuid.New()
	parentRoomID := id.RoomID("!parent:matrix.example.com")
	matrix.resolveAliasResults[idMapper.SpaceAlias(parentContextID)] = parentRoomID
	matrix.getSpaceChildStateKeysErr = errors.New("synapse state read failed")

	handler := newTestSpaceHandler(matrix)

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID: dto.AlkemioContextID(parentContextID),
	})

	resp, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wire := mustUnmarshalSetChildrenResponse(t, resp)
	if wire.Success {
		t.Error("expected success=false when the children read fails")
	}
	if wire.Added == nil || wire.Removed == nil || wire.PrunedUnknown == nil || wire.UnknownKept == nil ||
		wire.Unresolved == nil || wire.ParentPointersRepaired == nil || wire.ParentPointersDeferred == nil {
		t.Errorf("expected every array field present as [] rather than null on the children-read-failure path, got %+v", wire)
	}
}

func TestHandleSetChildren_Success_AddsMissingChild(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	parentContextID := uuid.New()
	childContextID := uuid.New()
	parentRoomID := id.RoomID("!parent:matrix.example.com")
	childRoomID := id.RoomID("!child:matrix.example.com")

	matrix.resolveAliasResults[idMapper.SpaceAlias(parentContextID)] = parentRoomID
	matrix.resolveAliasResults[idMapper.RoomAlias(childContextID)] = childRoomID

	handler := newTestSpaceHandler(matrix)

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID:        dto.AlkemioContextID(parentContextID),
		DesiredChildContextIDs: []string{childContextID.String()},
		ApplyRemovals:          true,
	})

	respRaw, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := respRaw.(dto.SetChildrenResponse)
	if !ok {
		t.Fatalf("expected dto.SetChildrenResponse, got %T", respRaw)
	}
	if !resp.Success {
		t.Fatalf("expected success=true, got error=%+v", resp.Error)
	}
	if len(resp.Added) != 1 || resp.Added[0] != string(childRoomID) {
		t.Errorf("expected the missing child added on the wire, got %+v", resp.Added)
	}
	if !resp.Changed {
		t.Error("expected changed=true")
	}
	if resp.DryRun {
		t.Error("expected dry_run=false to echo the request")
	}
	if matrix.capturedAddSpaceChildSpaceID != parentRoomID {
		t.Errorf("expected the add to target the resolved parent room, got %q", matrix.capturedAddSpaceChildSpaceID)
	}
}

func TestHandleSetChildren_PartialFailureReportsErrorButKeepsArrays(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	parentContextID := uuid.New()
	childContextID := uuid.New()
	parentRoomID := id.RoomID("!parent:matrix.example.com")
	childRoomID := id.RoomID("!child:matrix.example.com")

	matrix.resolveAliasResults[idMapper.SpaceAlias(parentContextID)] = parentRoomID
	matrix.resolveAliasResults[idMapper.RoomAlias(childContextID)] = childRoomID
	matrix.addSpaceChildErr = errors.New("synapse rejected the write")

	handler := newTestSpaceHandler(matrix)

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID:        dto.AlkemioContextID(parentContextID),
		DesiredChildContextIDs: []string{childContextID.String()},
	})

	respRaw, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := respRaw.(dto.SetChildrenResponse)
	if !ok {
		t.Fatalf("expected dto.SetChildrenResponse, got %T", respRaw)
	}
	if resp.Success {
		t.Error("expected success=false when the write failed")
	}
	if resp.Error == nil {
		t.Fatal("expected an error to be populated on partial failure")
	}
	if len(resp.Added) != 0 {
		t.Errorf("expected the failed add absent from added, got %+v", resp.Added)
	}
}

// TestHandleSetChildren_DeadlineExceededIsReportedDistinctlyFromWriteFailure
// asserts the wire-level distinction finding 2 requires: a call aborted by
// its own execution deadline (config.Hierarchy.SetChildrenTimeoutSeconds)
// reports ErrCodeDeadlineExceeded, never ErrCodeMatrixError — the code an
// actual Matrix write rejection reports (see
// TestHandleSetChildren_PartialFailureReportsErrorButKeepsArrays above). A
// negative timeout makes context.WithTimeout produce an already-expired
// deadline deterministically, with no reliance on real elapsed time.
func TestHandleSetChildren_DeadlineExceededIsReportedDistinctlyFromWriteFailure(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	parentContextID := uuid.New()
	childContextID := uuid.New()
	parentRoomID := id.RoomID("!parent:matrix.example.com")
	childRoomID := id.RoomID("!child:matrix.example.com")

	matrix.resolveAliasResults[idMapper.SpaceAlias(parentContextID)] = parentRoomID
	matrix.resolveAliasResults[idMapper.RoomAlias(childContextID)] = childRoomID

	cfg := testHierarchyConfig()
	cfg.Hierarchy.SetChildrenTimeoutSeconds = -1 // already-expired deadline, deterministically
	svc := service.NewSpaceService(matrix, &testMockLogger{}, idMapper, cfg)
	handler := NewSpaceHandler(svc, matrix, idMapper, cfg)

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID:        dto.AlkemioContextID(parentContextID),
		DesiredChildContextIDs: []string{childContextID.String()},
	})

	respRaw, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := respRaw.(dto.SetChildrenResponse)
	if !ok {
		t.Fatalf("expected dto.SetChildrenResponse, got %T", respRaw)
	}
	if resp.Success {
		t.Fatal("expected success=false once the handler's own deadline has already passed")
	}
	if resp.Error == nil || resp.Error.Code != dto.ErrCodeDeadlineExceeded {
		t.Errorf("expected error code %q, got %+v", dto.ErrCodeDeadlineExceeded, resp.Error)
	}
	if len(resp.Added) != 0 {
		t.Errorf("expected zero adds once the deadline has passed, got %+v", resp.Added)
	}
	if matrix.capturedAddSpaceChildSpaceID != "" {
		t.Error("expected AddSpaceChild never called once ctx.Err() is observed")
	}
}

// mustUnmarshalSetChildrenResponse round-trips result through JSON, mirroring
// how the wire actually carries the response, to catch any field the struct
// tags might silently drop.
func mustUnmarshalSetChildrenResponse(t *testing.T, v interface{}) dto.SetChildrenResponse {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var resp dto.SetChildrenResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	return resp
}

func TestHandleSetChildren_ResponseArraysAreNeverNullOnTheWire(t *testing.T) {
	matrix := otherNewMatrixPort()
	idMapper := otherNewIDMapper()

	parentContextID := uuid.New()
	parentRoomID := id.RoomID("!parent:matrix.example.com")
	matrix.resolveAliasResults[idMapper.SpaceAlias(parentContextID)] = parentRoomID

	handler := newTestSpaceHandler(matrix)

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID: dto.AlkemioContextID(parentContextID),
		// No desired children, nothing actual: a genuine zero-drift report call.
	})

	respRaw, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := mustUnmarshalSetChildrenResponse(t, respRaw)
	if resp.Added == nil || resp.Removed == nil || resp.PrunedUnknown == nil || resp.UnknownKept == nil ||
		resp.Unresolved == nil || resp.ParentPointersRepaired == nil || resp.ParentPointersDeferred == nil {
		t.Errorf("expected every array field present as [] rather than null on the wire, got %+v", resp)
	}
}
