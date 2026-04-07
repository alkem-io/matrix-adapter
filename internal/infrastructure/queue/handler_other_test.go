package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// ============================================================================
// Mock Types (prefixed with "other" to avoid conflicts with handler_room_test.go)
// ============================================================================

// otherMockLogger implements ports.Logger for testing.
type otherMockLogger struct{}

func (l *otherMockLogger) Debug(_ string, _ ...interface{}) {}
func (l *otherMockLogger) Info(_ string, _ ...interface{})  {}
func (l *otherMockLogger) Warn(_ string, _ ...interface{})  {}
func (l *otherMockLogger) Error(_ string, _ ...interface{}) {}
func (l *otherMockLogger) With(_ ...interface{}) ports.Logger {
	return l
}

// otherMockMatrixPort implements ports.MatrixPort for testing.
type otherMockMatrixPort struct {
	// Connection
	connectErr    error
	disconnectErr error
	hsDomain      string

	// User operations
	ensureUserResult id.UserID
	ensureUserErr    error
	setProfileErr    error

	// Room operations
	createRoomResult      id.RoomID
	createRoomErr         error
	inviteUserErr         error
	getRoomDetailsResult  *domain.Room
	getRoomDetailsErr     error
	getRoomMembersResult  []id.UserID
	getRoomMembersErr     error
	updateRoomStateErr    error
	setDirVisibilityErr   error
	setCustomStateErr     error
	getCustomStateResult  map[string]map[string]interface{}
	getCustomStateErr     error
	resolveAliasResults   map[string]id.RoomID
	resolveAliasErrs      map[string]error
	deleteAliasErr        error
	kickUserErr           error
	sendMessageResult     id.EventID
	sendMessageErr        error
	sendReplyResult       id.EventID
	sendReplyErr          error
	redactEventErr        error
	sendReactionResult    id.EventID
	sendReactionErr       error
	getMessageResult      *domain.Message
	getMessageErr         error
	getRoomMessagesResult []domain.Message
	getRoomMessagesErr    error
	getLastMessageResult  *domain.Message
	getLastMessageErr     error
	getBatchLastMsgsRes   map[id.RoomID]*domain.Message
	getBatchLastMsgsErrs  map[id.RoomID]error
	getReactionEventIDRes id.EventID
	getReactionEventIDErr error
	getReactionResult     *domain.Reaction
	getReactionErr        error
	getThreadMsgsResult   []domain.Message
	getThreadMsgsErr      error
	findDirectRoomResult  id.RoomID
	findDirectRoomErr     error
	setRoomAliasErr       error
	getAllJoinedRoomsRes   []id.RoomID
	getAllJoinedRoomsErr   error

	// Space operations
	createSpaceResult      id.RoomID
	createSpaceErr         error
	getSpaceDetailsResult  *domain.Space
	getSpaceDetailsErr     error
	getSpaceMembersResult  []id.UserID
	getSpaceMembersErr     error
	updateSpaceStateErr    error
	getSpaceChildrenResult []domain.SpaceChild
	getSpaceChildrenErr    error
	addSpaceChildErr       error
	setSpaceParentErr      error
	inviteToSpaceErr       error
	kickFromSpaceErr       error

	// Read receipt operations
	sendReadReceiptErr       error
	getUnreadCountsResult    *domain.UnreadCountSummary
	getUnreadCountsErr       error
	getBatchUnreadCountsRes  map[id.RoomID]int
	getBatchUnreadCountsErrs map[id.RoomID]error
}

func (m *otherMockMatrixPort) Connect(_ context.Context) error { return m.connectErr }
func (m *otherMockMatrixPort) Disconnect() error               { return m.disconnectErr }
func (m *otherMockMatrixPort) HomeserverDomain() string        { return m.hsDomain }

func (m *otherMockMatrixPort) EnsureUser(_ context.Context, _ domain.Actor) (id.UserID, error) {
	return m.ensureUserResult, m.ensureUserErr
}

func (m *otherMockMatrixPort) SetUserProfile(_ context.Context, _ domain.Actor) error {
	return m.setProfileErr
}

func (m *otherMockMatrixPort) CreateRoomWithAlias(_ context.Context, _ uuid.UUID, _, _, _, _, _ string, _ map[string]map[string]interface{}, _ []domain.Actor) (id.RoomID, error) {
	return m.createRoomResult, m.createRoomErr
}

func (m *otherMockMatrixPort) InviteUser(_ context.Context, _ id.RoomID, _, _ domain.Actor) error {
	return m.inviteUserErr
}

func (m *otherMockMatrixPort) GetRoomDetails(_ context.Context, _ id.RoomID) (*domain.Room, error) {
	return m.getRoomDetailsResult, m.getRoomDetailsErr
}

func (m *otherMockMatrixPort) GetRoomMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return m.getRoomMembersResult, m.getRoomMembersErr
}

func (m *otherMockMatrixPort) UpdateRoomState(_ context.Context, _ id.RoomID, _ domain.Actor, _, _, _, _ *string) error {
	return m.updateRoomStateErr
}

func (m *otherMockMatrixPort) SetRoomDirectoryVisibility(_ context.Context, _ id.RoomID, _ bool) error {
	return m.setDirVisibilityErr
}

func (m *otherMockMatrixPort) SetCustomState(_ context.Context, _ id.RoomID, _ map[string]map[string]interface{}) error {
	return m.setCustomStateErr
}

func (m *otherMockMatrixPort) GetCustomState(_ context.Context, _ id.RoomID, _ []string) (map[string]map[string]interface{}, error) {
	return m.getCustomStateResult, m.getCustomStateErr
}

func (m *otherMockMatrixPort) ResolveAlias(_ context.Context, alias string) (id.RoomID, error) {
	if m.resolveAliasResults != nil {
		if roomID, ok := m.resolveAliasResults[alias]; ok {
			return roomID, nil
		}
	}
	if m.resolveAliasErrs != nil {
		if err, ok := m.resolveAliasErrs[alias]; ok {
			return "", err
		}
	}
	return "", domain.ErrRoomNotFound
}

func (m *otherMockMatrixPort) DeleteAlias(_ context.Context, _ string) error {
	return m.deleteAliasErr
}

func (m *otherMockMatrixPort) KickUser(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return m.kickUserErr
}

func (m *otherMockMatrixPort) SendMessage(_ context.Context, _ id.RoomID, _ domain.Actor, _ string) (id.EventID, error) {
	return m.sendMessageResult, m.sendMessageErr
}

func (m *otherMockMatrixPort) SendReply(_ context.Context, _ id.RoomID, _ domain.Actor, _ string, _ id.EventID) (id.EventID, error) {
	return m.sendReplyResult, m.sendReplyErr
}

func (m *otherMockMatrixPort) RedactEvent(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) error {
	return m.redactEventErr
}

func (m *otherMockMatrixPort) SendReaction(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) (id.EventID, error) {
	return m.sendReactionResult, m.sendReactionErr
}

func (m *otherMockMatrixPort) GetMessage(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Message, error) {
	return m.getMessageResult, m.getMessageErr
}

func (m *otherMockMatrixPort) GetRoomMessages(_ context.Context, _ id.RoomID) ([]domain.Message, error) {
	return m.getRoomMessagesResult, m.getRoomMessagesErr
}

func (m *otherMockMatrixPort) GetLastMessage(_ context.Context, _ id.RoomID) (*domain.Message, error) {
	return m.getLastMessageResult, m.getLastMessageErr
}

func (m *otherMockMatrixPort) GetBatchLastMessages(_ context.Context, _ []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	return m.getBatchLastMsgsRes, m.getBatchLastMsgsErrs
}

func (m *otherMockMatrixPort) GetReactionEventID(_ context.Context, _ id.RoomID, _ id.EventID, _ string, _ domain.Actor) (id.EventID, error) {
	return m.getReactionEventIDRes, m.getReactionEventIDErr
}

func (m *otherMockMatrixPort) GetReaction(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Reaction, error) {
	return m.getReactionResult, m.getReactionErr
}

func (m *otherMockMatrixPort) GetThreadMessages(_ context.Context, _ id.RoomID, _ id.EventID) ([]domain.Message, error) {
	return m.getThreadMsgsResult, m.getThreadMsgsErr
}

func (m *otherMockMatrixPort) FindExistingDirectRoom(_ context.Context, _, _ domain.Actor) (id.RoomID, error) {
	return m.findDirectRoomResult, m.findDirectRoomErr
}

func (m *otherMockMatrixPort) SetRoomAlias(_ context.Context, _ id.RoomID, _ string) error {
	return m.setRoomAliasErr
}

func (m *otherMockMatrixPort) GetAllJoinedRooms(_ context.Context) ([]id.RoomID, error) {
	return m.getAllJoinedRoomsRes, m.getAllJoinedRoomsErr
}

func (m *otherMockMatrixPort) CreateSpace(_ context.Context, _ uuid.UUID, _, _, _, _ string, _ []domain.Actor) (id.RoomID, error) {
	return m.createSpaceResult, m.createSpaceErr
}

func (m *otherMockMatrixPort) GetSpaceDetails(_ context.Context, _ id.RoomID) (*domain.Space, error) {
	return m.getSpaceDetailsResult, m.getSpaceDetailsErr
}

func (m *otherMockMatrixPort) GetSpaceMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return m.getSpaceMembersResult, m.getSpaceMembersErr
}

func (m *otherMockMatrixPort) UpdateSpaceState(_ context.Context, _ id.RoomID, _, _, _, _ *string) error {
	return m.updateSpaceStateErr
}

func (m *otherMockMatrixPort) GetSpaceChildren(_ context.Context, _ id.RoomID) ([]domain.SpaceChild, error) {
	return m.getSpaceChildrenResult, m.getSpaceChildrenErr
}

func (m *otherMockMatrixPort) AddSpaceChild(_ context.Context, _, _ id.RoomID, _ string, _ bool) error {
	return m.addSpaceChildErr
}

func (m *otherMockMatrixPort) SetSpaceParent(_ context.Context, _, _ id.RoomID) error {
	return m.setSpaceParentErr
}

func (m *otherMockMatrixPort) InviteToSpace(_ context.Context, _ id.RoomID, _ domain.Actor) error {
	return m.inviteToSpaceErr
}

func (m *otherMockMatrixPort) KickFromSpace(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return m.kickFromSpaceErr
}

func (m *otherMockMatrixPort) SendReadReceipt(_ context.Context, _ domain.Actor, _ id.RoomID, _ id.EventID, _ *id.EventID) error {
	return m.sendReadReceiptErr
}

func (m *otherMockMatrixPort) GetUnreadCounts(_ context.Context, _ domain.Actor, _ id.RoomID, _ []id.EventID) (*domain.UnreadCountSummary, error) {
	return m.getUnreadCountsResult, m.getUnreadCountsErr
}

func (m *otherMockMatrixPort) GetBatchUnreadCounts(_ context.Context, _ domain.Actor, _ []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error) {
	return m.getBatchUnreadCountsRes, m.getBatchUnreadCountsErrs
}

// otherMockReadReceiptService implements ports.ReadReceiptServicePort for testing.
type otherMockReadReceiptService struct {
	markMessageReadErr    error
	getUnreadCountsRes    *domain.UnreadCountSummary
	getUnreadCountsErr    error
}

func (m *otherMockReadReceiptService) MarkMessageRead(_ context.Context, _ uuid.UUID, _ id.RoomID, _ id.EventID, _ *id.EventID) error {
	return m.markMessageReadErr
}

func (m *otherMockReadReceiptService) GetUnreadCounts(_ context.Context, _ uuid.UUID, _ id.RoomID, _ []id.EventID) (*domain.UnreadCountSummary, error) {
	return m.getUnreadCountsRes, m.getUnreadCountsErr
}

// ============================================================================
// Test Helpers
// ============================================================================

const otherTestHSDomain = "matrix.example.com"

func otherNewIDMapper() *domain.IDMapper {
	return domain.NewIDMapper(otherTestHSDomain)
}

func otherNewMatrixPort() *otherMockMatrixPort {
	return &otherMockMatrixPort{
		hsDomain:          otherTestHSDomain,
		resolveAliasResults: make(map[string]id.RoomID),
		resolveAliasErrs:    make(map[string]error),
	}
}

// otherAssertSuccess asserts the response is a success BaseResponse.
func otherAssertSuccess(t *testing.T, resp interface{}) {
	t.Helper()
	switch r := resp.(type) {
	case dto.BaseResponse:
		if !r.Success {
			t.Fatalf("expected success, got error: %+v", r.Error)
		}
	default:
		// Responses embedding BaseResponse - check via JSON roundtrip
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		var base dto.BaseResponse
		if err := json.Unmarshal(data, &base); err != nil {
			t.Fatalf("failed to unmarshal base response: %v", err)
		}
		if !base.Success {
			t.Fatalf("expected success, got error: %+v", base.Error)
		}
	}
}

// otherAssertError asserts the response is an error with the given code.
func otherAssertError(t *testing.T, resp interface{}, expectedCode dto.ErrorCode) {
	t.Helper()
	br, ok := resp.(dto.BaseResponse)
	if !ok {
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		if err := json.Unmarshal(data, &br); err != nil {
			t.Fatalf("failed to unmarshal base response: %v", err)
		}
	}
	if br.Success {
		t.Fatalf("expected error with code %s, got success", expectedCode)
	}
	if br.Error == nil {
		t.Fatalf("expected error, got nil")
	}
	if br.Error.Code != expectedCode {
		t.Errorf("expected error code %s, got %s (message: %s)", expectedCode, br.Error.Code, br.Error.Message)
	}
}

// ============================================================================
// SpaceHandler Tests
// ============================================================================

func newTestSpaceHandler(matrix *otherMockMatrixPort) *SpaceHandler {
	idMapper := otherNewIDMapper()
	logger := &otherMockLogger{}
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
	otherAssertSuccess(t, resp)
}

func TestHandleCreateSpace_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleCreateSpace(context.Background(), []byte(`{invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetSpace_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.GetSpaceRequest{})

	resp, err := handler.HandleGetSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeSpaceNotFound)
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
	otherAssertSuccess(t, resp)
}

func TestHandleUpdateSpace_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleUpdateSpace(context.Background(), []byte(`{broken`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleUpdateSpace_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.UpdateSpaceRequest{})

	resp, err := handler.HandleUpdateSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeSpaceNotFound)
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
	otherAssertSuccess(t, resp)
}

func TestHandleDeleteSpace_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleDeleteSpace(context.Background(), []byte(`???`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleDeleteSpace_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.DeleteSpaceRequest{})

	resp, err := handler.HandleDeleteSpace(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertSuccess(t, resp)
}

// --- HandleListSpaces ---

func TestHandleListSpaces_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	matrix.getAllJoinedRoomsRes = []id.RoomID{}

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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
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
	otherAssertSuccess(t, resp)
}

func TestHandleSetParent_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleSetParent(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleSetParent_MissingParentContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.SetParentRequest{
		ChildID: "some-child-id",
		// ParentContextID is zero-value
	})

	resp, err := handler.HandleSetParent(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeSpaceNotFound)
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
}

func TestHandleBatchAddSpaceMember_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleBatchAddSpaceMember(context.Background(), []byte(`bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
}

func TestHandleBatchRemoveSpaceMember_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleBatchRemoveSpaceMember(context.Background(), []byte(`nope`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertSuccess(t, resp)
}

func TestHandleSetSpaceState_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleSetSpaceState(context.Background(), []byte(`{bad}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeSpaceNotFound)
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
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
}

func TestHandleGetSpaceState_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	resp, err := handler.HandleGetSpaceState(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetSpaceState_MissingContextID(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	payload, _ := json.Marshal(dto.GetSpaceStateRequest{})

	resp, err := handler.HandleGetSpaceState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeSpaceNotFound)
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
}

// ============================================================================
// ActorHandler Tests
// ============================================================================

func newTestActorHandler(matrix *otherMockMatrixPort) *ActorHandler {
	logger := &otherMockLogger{}
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
	otherAssertSuccess(t, resp)
}

func TestHandleSyncActor_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestActorHandler(matrix)

	resp, err := handler.HandleSyncActor(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
}

// ============================================================================
// ReadReceiptHandler Tests
// ============================================================================

func newTestReadReceiptHandler(
	matrix *otherMockMatrixPort,
	svc *otherMockReadReceiptService,
) *ReadReceiptHandler {
	idMapper := otherNewIDMapper()
	logger := &otherMockLogger{}
	return NewReadReceiptHandler(svc, matrix, idMapper, logger)
}

// --- HandleMarkMessageRead ---

func TestHandleMarkMessageRead_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertSuccess(t, resp)
}

func TestHandleMarkMessageRead_WithThreadID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertSuccess(t, resp)
}

func TestHandleMarkMessageRead_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	resp, err := handler.HandleMarkMessageRead(context.Background(), []byte(`invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_MissingRoomID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_MissingMessageID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleMarkMessageRead_RoomNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertError(t, resp, dto.ErrCodeRoomNotFound)
}

func TestHandleMarkMessageRead_ServiceError(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
}

// --- HandleGetUnreadCounts ---

func TestHandleGetUnreadCounts_Success(t *testing.T) {
	matrix := otherNewMatrixPort()
	threadEventID := id.EventID("$thread1:matrix.example.com")
	svc := &otherMockReadReceiptService{
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
}

func TestHandleGetUnreadCounts_InvalidJSON(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	resp, err := handler.HandleGetUnreadCounts(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetUnreadCounts_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.GetUnreadCountsRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
	})

	resp, err := handler.HandleGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetUnreadCounts_MissingRoomID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	actorID := uuid.New()
	payload, _ := json.Marshal(dto.GetUnreadCountsRequest{
		ActorID: dto.AlkemioActorID(actorID),
	})

	resp, err := handler.HandleGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleGetUnreadCounts_RoomNotFound(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertError(t, resp, dto.ErrCodeRoomNotFound)
}

func TestHandleGetUnreadCounts_ServiceError(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{
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
	otherAssertError(t, resp, dto.ErrCodeMatrixError)
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

	svc := &otherMockReadReceiptService{}
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
	svc := &otherMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchGetUnreadCounts_MissingActorID(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
	handler := newTestReadReceiptHandler(matrix, svc)

	roomUUID := uuid.New()
	payload, _ := json.Marshal(dto.BatchGetUnreadCountsRequest{
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(roomUUID)},
	})

	resp, err := handler.HandleBatchGetUnreadCounts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
}

func TestHandleBatchGetUnreadCounts_EmptyRoomIDs(t *testing.T) {
	matrix := otherNewMatrixPort()
	svc := &otherMockReadReceiptService{}
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
	otherAssertError(t, resp, dto.ErrCodeInvalidParam)
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

	svc := &otherMockReadReceiptService{}
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

	svc := &otherMockReadReceiptService{}
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
