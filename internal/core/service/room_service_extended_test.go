package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/testutil"
)

// ============================================================================
// mockExtendedMatrixPort — full ports.MatrixPort implementation with
// configurable behaviour for every method tested by the extended suite.
// ============================================================================

type mockExtendedMatrixPort struct {
	// ResolveAlias — per-test func for flexible behaviour
	resolveAliasFunc func(ctx context.Context, alias string) (id.RoomID, error)

	// GetRoomDetails
	getRoomDetailsResult *domain.Room
	getRoomDetailsErr    error

	// GetRoomMembers
	getRoomMembersResult []id.UserID
	getRoomMembersErr    error

	// GetRoomMessages
	getRoomMessagesResult []domain.Message
	getRoomMessagesErr    error

	// GetUnreadCounts
	getUnreadCountsResult *domain.UnreadCountSummary
	getUnreadCountsErr    error

	// GetAllJoinedRooms
	getAllJoinedRoomsResult []id.RoomID
	getAllJoinedRoomsErr    error

	// DeleteAlias
	deleteAliasCalled bool

	// KickUser
	kickUserCalled int
	kickUserErr    error

	// SendMessage
	sendMessageResult id.EventID
	sendMessageErr    error

	// SendReply
	sendReplyResult id.EventID
	sendReplyErr    error

	// RedactEvent
	redactEventErr error

	// SendReaction
	sendReactionResult id.EventID
	sendReactionErr    error

	// GetMessage
	getMessageResult *domain.Message
	getMessageErr    error

	// FindExistingDirectRoom
	findExistingDirectRoomResult id.RoomID
	findExistingDirectRoomErr    error

	// SetRoomAlias
	setRoomAliasCalled bool
	setRoomAliasErr    error

	// SetRoomDirectoryVisibility
	setRoomDirectoryVisibilityCalled bool

	// SetCustomState
	setCustomStateCalled bool

	// CreateRoomWithAlias
	createRoomResult      id.RoomID
	createRoomErr         error
	createRoomCustomState map[string]map[string]interface{}

	// UpdateRoomState
	updateRoomStateErr error
}

// --- Implemented methods (used in tests) ---

func (m *mockExtendedMatrixPort) ResolveAlias(ctx context.Context, alias string) (id.RoomID, error) {
	if m.resolveAliasFunc != nil {
		return m.resolveAliasFunc(ctx, alias)
	}
	return "", domain.NewRoomNotFoundError("no resolveAliasFunc set")
}

func (m *mockExtendedMatrixPort) GetRoomDetails(_ context.Context, _ id.RoomID) (*domain.Room, error) {
	return m.getRoomDetailsResult, m.getRoomDetailsErr
}

func (m *mockExtendedMatrixPort) GetRoomMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return m.getRoomMembersResult, m.getRoomMembersErr
}

func (m *mockExtendedMatrixPort) GetRoomMessages(_ context.Context, _ id.RoomID) ([]domain.Message, error) {
	return m.getRoomMessagesResult, m.getRoomMessagesErr
}

func (m *mockExtendedMatrixPort) GetUnreadCounts(_ context.Context, _ domain.Actor, _ id.RoomID, _ []id.EventID) (*domain.UnreadCountSummary, error) {
	return m.getUnreadCountsResult, m.getUnreadCountsErr
}

func (m *mockExtendedMatrixPort) GetAllJoinedRooms(_ context.Context) ([]id.RoomID, error) {
	return m.getAllJoinedRoomsResult, m.getAllJoinedRoomsErr
}

func (m *mockExtendedMatrixPort) DeleteAlias(_ context.Context, _ string) error {
	m.deleteAliasCalled = true
	return nil
}

func (m *mockExtendedMatrixPort) KickUser(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	m.kickUserCalled++
	return m.kickUserErr
}

func (m *mockExtendedMatrixPort) SendMessage(_ context.Context, _ id.RoomID, _ domain.Actor, _ string) (id.EventID, error) {
	return m.sendMessageResult, m.sendMessageErr
}

func (m *mockExtendedMatrixPort) SendReply(_ context.Context, _ id.RoomID, _ domain.Actor, _ string, _ id.EventID) (id.EventID, error) {
	return m.sendReplyResult, m.sendReplyErr
}

func (m *mockExtendedMatrixPort) RedactEvent(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) error {
	return m.redactEventErr
}

func (m *mockExtendedMatrixPort) SendReaction(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) (id.EventID, error) {
	return m.sendReactionResult, m.sendReactionErr
}

func (m *mockExtendedMatrixPort) GetMessage(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Message, error) {
	return m.getMessageResult, m.getMessageErr
}

func (m *mockExtendedMatrixPort) FindExistingDirectRoom(_ context.Context, _ domain.Actor, _ domain.Actor) (id.RoomID, error) {
	return m.findExistingDirectRoomResult, m.findExistingDirectRoomErr
}

func (m *mockExtendedMatrixPort) SetRoomAlias(_ context.Context, _ id.RoomID, _ string) error {
	m.setRoomAliasCalled = true
	return m.setRoomAliasErr
}

func (m *mockExtendedMatrixPort) SetRoomDirectoryVisibility(_ context.Context, _ id.RoomID, _ bool) error {
	m.setRoomDirectoryVisibilityCalled = true
	return nil
}

func (m *mockExtendedMatrixPort) SetCustomState(_ context.Context, _ id.RoomID, _ map[string]map[string]interface{}) error {
	m.setCustomStateCalled = true
	return nil
}

func (m *mockExtendedMatrixPort) CreateRoomWithAlias(_ context.Context, _ uuid.UUID, _ string, _, _, _, _ string, customState map[string]map[string]interface{}, _ []domain.Actor) (id.RoomID, error) {
	m.createRoomCustomState = customState
	if m.createRoomErr != nil {
		return "", m.createRoomErr
	}
	if m.createRoomResult != "" {
		return m.createRoomResult, nil
	}
	return "!newroom:test.local", nil
}

func (m *mockExtendedMatrixPort) UpdateRoomState(_ context.Context, _ id.RoomID, _ domain.Actor, _, _, _, _ *string) error {
	return m.updateRoomStateErr
}

// --- Stub implementations for remaining MatrixPort interface methods ---

func (m *mockExtendedMatrixPort) Connect(_ context.Context) error { return nil }
func (m *mockExtendedMatrixPort) Disconnect() error               { return nil }
func (m *mockExtendedMatrixPort) HomeserverDomain() string        { return "test.local" }
func (m *mockExtendedMatrixPort) EnsureUser(_ context.Context, _ domain.Actor) (id.UserID, error) {
	return "@bot:test.local", nil
}
func (m *mockExtendedMatrixPort) SetUserProfile(_ context.Context, _ domain.Actor) error { return nil }
func (m *mockExtendedMatrixPort) InviteUser(_ context.Context, _ id.RoomID, _ domain.Actor, _ domain.Actor) error {
	return nil
}
func (m *mockExtendedMatrixPort) GetCustomState(_ context.Context, _ id.RoomID, _ []string) (map[string]map[string]interface{}, error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) GetLastMessage(_ context.Context, _ id.RoomID) (*domain.Message, error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) GetBatchLastMessages(_ context.Context, _ []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) GetReactionEventID(_ context.Context, _ id.RoomID, _ id.EventID, _ string, _ domain.Actor) (id.EventID, error) {
	return "", nil
}
func (m *mockExtendedMatrixPort) GetReaction(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Reaction, error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) GetThreadMessages(_ context.Context, _ id.RoomID, _ id.EventID) ([]domain.Message, error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) CreateSpace(_ context.Context, _ uuid.UUID, _, _, _ string, _ string, _ []domain.Actor) (id.RoomID, error) {
	return "", nil
}
func (m *mockExtendedMatrixPort) GetSpaceDetails(_ context.Context, _ id.RoomID) (*domain.Space, error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) GetSpaceMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) UpdateSpaceState(_ context.Context, _ id.RoomID, _, _, _, _ *string) error {
	return nil
}
func (m *mockExtendedMatrixPort) GetSpaceChildren(_ context.Context, _ id.RoomID) ([]domain.SpaceChild, error) {
	return nil, nil
}
func (m *mockExtendedMatrixPort) AddSpaceChild(_ context.Context, _ id.RoomID, _ id.RoomID, _ string, _ bool) error {
	return nil
}
func (m *mockExtendedMatrixPort) SetSpaceParent(_ context.Context, _ id.RoomID, _ id.RoomID) error {
	return nil
}
func (m *mockExtendedMatrixPort) InviteToSpace(_ context.Context, _ id.RoomID, _ domain.Actor) error {
	return nil
}
func (m *mockExtendedMatrixPort) KickFromSpace(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return nil
}
func (m *mockExtendedMatrixPort) SendReadReceipt(_ context.Context, _ domain.Actor, _ id.RoomID, _ id.EventID, _ *id.EventID) error {
	return nil
}
func (m *mockExtendedMatrixPort) GetBatchUnreadCounts(_ context.Context, _ domain.Actor, _ []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error) {
	return nil, nil
}

// ============================================================================
// Helper
// ============================================================================

// testIDMapper is a shared IDMapper for constructing Matrix IDs in tests.
var testIDMapper = domain.NewIDMapper("test.local")

func newTestService(matrix *mockExtendedMatrixPort) *RoomService {
	return NewRoomService(matrix, &testutil.MockLogger{}, domain.NewIDMapper("test.local"))
}

// ============================================================================
// 1. CreateRoomWithAlkemioID — extended scenarios
// ============================================================================

func TestCreateRoom_Idempotent_AliasExists(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!existing:test.local", nil // alias already resolves
		},
	}
	svc := newTestService(matrix)

	err := svc.CreateRoomWithAlkemioID(context.Background(), uuid.New(),
		"community", "Name", "", "", "", nil, nil, nil)

	if err != nil {
		t.Fatalf("expected idempotent success, got: %v", err)
	}
}

func TestCreateRoom_AliasCheckNonNotFoundError(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", errors.New("network error")
		},
	}
	svc := newTestService(matrix)

	err := svc.CreateRoomWithAlkemioID(context.Background(), uuid.New(),
		"community", "Name", "", "", "", nil, nil, nil)

	if err == nil {
		t.Fatal("expected error for non-not-found resolve alias failure")
	}
}

func TestCreateRoom_DM_ExistingRoomFound(t *testing.T) {
	actor1 := domain.NewActor(uuid.New())
	actor2 := domain.NewActor(uuid.New())

	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
		findExistingDirectRoomResult: "!dm:test.local",
		findExistingDirectRoomErr:    nil,
	}
	svc := newTestService(matrix)

	err := svc.CreateRoomWithAlkemioID(context.Background(), uuid.New(),
		"direct", "", "", "", "", nil, nil, []domain.Actor{actor1, actor2})

	if err != nil {
		t.Fatalf("expected success when existing DM found, got: %v", err)
	}
	if !matrix.setRoomAliasCalled {
		t.Error("expected SetRoomAlias to be called on existing DM room")
	}
}

func TestCreateRoom_DM_AliasSetFailure_NonFatal(t *testing.T) {
	actor1 := domain.NewActor(uuid.New())
	actor2 := domain.NewActor(uuid.New())

	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
		findExistingDirectRoomResult: "!dm:test.local",
		findExistingDirectRoomErr:    nil,
		setRoomAliasErr:              errors.New("alias conflict"),
	}
	svc := newTestService(matrix)

	err := svc.CreateRoomWithAlkemioID(context.Background(), uuid.New(),
		"direct", "", "", "", "", nil, nil, []domain.Actor{actor1, actor2})

	// Alias set failure is non-fatal; method should return nil
	if err != nil {
		t.Fatalf("expected success despite alias set failure, got: %v", err)
	}
}

func TestCreateRoom_CreateRoomError(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
		createRoomErr: errors.New("homeserver unavailable"),
	}
	svc := newTestService(matrix)

	err := svc.CreateRoomWithAlkemioID(context.Background(), uuid.New(),
		"community", "Name", "", "", "", nil, nil, nil)

	if err == nil {
		t.Fatal("expected error when CreateRoomWithAlias fails")
	}
}

func TestCreateRoom_IsPublic_SetsDirectoryVisibility(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
	}
	svc := newTestService(matrix)

	isPublic := true
	err := svc.CreateRoomWithAlkemioID(context.Background(), uuid.New(),
		"community", "Name", "", "", "", &isPublic, nil, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matrix.setRoomDirectoryVisibilityCalled {
		t.Error("expected SetRoomDirectoryVisibility to be called when isPublic is set")
	}
}

func TestCreateRoom_CustomState_Included(t *testing.T) {
	// customState is passed to CreateRoomWithAlias (as initial state), not SetCustomState.
	// Verify it is forwarded correctly to the mock.
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
	}
	svc := newTestService(matrix)

	customState := map[string]map[string]interface{}{
		"io.alkemio.visibility": {"hidden": true},
	}
	err := svc.CreateRoomWithAlkemioID(context.Background(), uuid.New(),
		"community", "Name", "", "", "", nil, customState, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Assert the customState argument was forwarded to CreateRoomWithAlias.
	if matrix.createRoomCustomState == nil {
		t.Fatal("expected customState to be captured by CreateRoomWithAlias mock")
	}
	vis, ok := matrix.createRoomCustomState["io.alkemio.visibility"]
	if !ok {
		t.Fatal("expected 'io.alkemio.visibility' key in captured customState")
	}
	if v, ok := vis["hidden"].(bool); !ok || !v {
		t.Errorf("expected hidden=true, got %v", vis["hidden"])
	}
}

// ============================================================================
// 2. GetRoomWithMessages
// ============================================================================

func TestGetRoomWithMessages_Success(t *testing.T) {
	senderUUID := uuid.New()
	senderMatrixID := string(testIDMapper.UserID(senderUUID))

	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsResult: &domain.Room{
			ID:    "!room1:test.local",
			Alias: testIDMapper.RoomAlias(uuid.Nil),
			Name:  "Test Room",
		},
		getRoomMembersResult: []id.UserID{id.UserID(senderMatrixID)},
		getRoomMessagesResult: []domain.Message{
			{
				ID:             "$evt1",
				SenderMatrixID: senderMatrixID,
				Content:        "hello",
				Timestamp:      time.Now(),
			},
		},
	}
	svc := newTestService(matrix)

	room, err := svc.GetRoomWithMessages(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if room == nil {
		t.Fatal("expected non-nil room")
	}
	if len(room.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(room.Messages))
	}
	if room.Messages[0].SenderID != senderUUID {
		t.Errorf("expected SenderID %s, got %s", senderUUID, room.Messages[0].SenderID)
	}
}

func TestGetRoomWithMessages_NotFound(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
	}
	svc := newTestService(matrix)

	_, err := svc.GetRoomWithMessages(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error when room not found")
	}
	if !domain.IsRoomNotFoundError(err) {
		t.Errorf("expected room not found error, got: %v", err)
	}
}

func TestGetRoomWithMessages_DetailsError(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsErr: errors.New("details unavailable"),
	}
	svc := newTestService(matrix)

	_, err := svc.GetRoomWithMessages(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error when GetRoomDetails fails")
	}
}

func TestGetRoomWithMessages_MembersError(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsResult: &domain.Room{ID: "!room1:test.local", Name: "R"},
		getRoomMembersErr:    errors.New("members unavailable"),
	}
	svc := newTestService(matrix)

	_, err := svc.GetRoomWithMessages(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error when GetRoomMembers fails")
	}
}

func TestGetRoomWithMessages_MessagesError_Graceful(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsResult: &domain.Room{ID: "!room1:test.local", Name: "R"},
		getRoomMembersResult: []id.UserID{},
		getRoomMessagesErr:   errors.New("messages unavailable"),
	}
	svc := newTestService(matrix)

	room, err := svc.GetRoomWithMessages(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("expected graceful fallback, got error: %v", err)
	}
	if len(room.Messages) != 0 {
		t.Errorf("expected empty messages on error, got %d", len(room.Messages))
	}
}

func TestGetRoomWithMessages_SenderIDMapping(t *testing.T) {
	senderUUID := uuid.New()
	reactionSenderUUID := uuid.New()

	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsResult: &domain.Room{ID: "!room1:test.local", Name: "R"},
		getRoomMembersResult: []id.UserID{},
		getRoomMessagesResult: []domain.Message{
			{
				ID:             "$evt1",
				SenderMatrixID: string(testIDMapper.UserID(senderUUID)),
				Content:        "hi",
				Timestamp:      time.Now(),
				Reactions: []domain.Reaction{
					{
						ID:             "$react1",
						Emoji:          "thumbsup",
						SenderMatrixID: string(testIDMapper.UserID(reactionSenderUUID)),
					},
				},
			},
		},
	}
	svc := newTestService(matrix)

	room, err := svc.GetRoomWithMessages(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if room.Messages[0].SenderID != senderUUID {
		t.Errorf("message SenderID: expected %s, got %s", senderUUID, room.Messages[0].SenderID)
	}
	if room.Messages[0].Reactions[0].SenderID != reactionSenderUUID {
		t.Errorf("reaction SenderID: expected %s, got %s", reactionSenderUUID, room.Messages[0].Reactions[0].SenderID)
	}
}

// ============================================================================
// 3. GetRoomAsUser
// ============================================================================

func TestGetRoomAsUser_Success_WithUnread(t *testing.T) {
	senderUUID := uuid.New()
	actorUUID := uuid.New()

	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsResult: &domain.Room{ID: "!room1:test.local", Name: "R"},
		getRoomMembersResult: []id.UserID{},
		getRoomMessagesResult: []domain.Message{
			{ID: "$evt1", SenderMatrixID: string(testIDMapper.UserID(senderUUID)), Timestamp: time.Unix(1000, 0)},
			{ID: "$evt2", SenderMatrixID: string(testIDMapper.UserID(senderUUID)), Timestamp: time.Unix(2000, 0)},
			{ID: "$evt3", SenderMatrixID: string(testIDMapper.UserID(senderUUID)), Timestamp: time.Unix(3000, 0)},
		},
		getUnreadCountsResult: &domain.UnreadCountSummary{RoomUnreadCount: 1},
	}
	svc := newTestService(matrix)

	result, err := svc.GetRoomAsUser(context.Background(), uuid.New(), actorUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UnreadCount != 1 {
		t.Errorf("expected UnreadCount 1, got %d", result.UnreadCount)
	}
	// Last read should be $evt2 (index 1, which is totalMessages - unreadCount - 1 = 3-1-1 = 1)
	if result.LastReadEventID != "$evt2" {
		t.Errorf("expected LastReadEventID '$evt2', got '%s'", result.LastReadEventID)
	}
}

func TestGetRoomAsUser_AllRead(t *testing.T) {
	senderUUID := uuid.New()

	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsResult: &domain.Room{ID: "!room1:test.local", Name: "R"},
		getRoomMembersResult: []id.UserID{},
		getRoomMessagesResult: []domain.Message{
			{ID: "$evt1", SenderMatrixID: string(testIDMapper.UserID(senderUUID)), Timestamp: time.Unix(1000, 0)},
			{ID: "$evt2", SenderMatrixID: string(testIDMapper.UserID(senderUUID)), Timestamp: time.Unix(2000, 0)},
		},
		getUnreadCountsResult: &domain.UnreadCountSummary{RoomUnreadCount: 0},
	}
	svc := newTestService(matrix)

	result, err := svc.GetRoomAsUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UnreadCount != 0 {
		t.Errorf("expected UnreadCount 0, got %d", result.UnreadCount)
	}
	// Last read should be $evt2 (the last message)
	if result.LastReadEventID != "$evt2" {
		t.Errorf("expected LastReadEventID '$evt2', got '%s'", result.LastReadEventID)
	}
}

func TestGetRoomAsUser_UnreadCountError_GracefulFallback(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomDetailsResult:  &domain.Room{ID: "!room1:test.local", Name: "R"},
		getRoomMembersResult:  []id.UserID{},
		getRoomMessagesResult: []domain.Message{},
		getUnreadCountsErr:    errors.New("sync unavailable"),
	}
	svc := newTestService(matrix)

	result, err := svc.GetRoomAsUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("expected graceful fallback, got error: %v", err)
	}
	if result.UnreadCount != 0 {
		t.Errorf("expected UnreadCount 0 on fallback, got %d", result.UnreadCount)
	}
}

// ============================================================================
// 4. UpdateRoomMetadata — extended scenarios
// ============================================================================

func TestUpdateRoomMetadata_Success(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
	}
	svc := newTestService(matrix)

	name := "Updated"
	err := svc.UpdateRoomMetadata(context.Background(), uuid.New(),
		&name, nil, nil, nil, nil, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateRoomMetadata_NotFound(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
	}
	svc := newTestService(matrix)

	err := svc.UpdateRoomMetadata(context.Background(), uuid.New(),
		nil, nil, nil, nil, nil, nil)

	if err == nil {
		t.Fatal("expected error when room not found")
	}
	if !domain.IsRoomNotFoundError(err) {
		t.Errorf("expected room not found error, got: %v", err)
	}
}

func TestUpdateRoomMetadata_UpdateError(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		updateRoomStateErr: errors.New("state update failed"),
	}
	svc := newTestService(matrix)

	err := svc.UpdateRoomMetadata(context.Background(), uuid.New(),
		nil, nil, nil, nil, nil, nil)

	if err == nil {
		t.Fatal("expected error when UpdateRoomState fails")
	}
}

func TestUpdateRoomMetadata_IsPublic(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
	}
	svc := newTestService(matrix)

	isPublic := true
	err := svc.UpdateRoomMetadata(context.Background(), uuid.New(),
		nil, nil, nil, nil, &isPublic, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matrix.setRoomDirectoryVisibilityCalled {
		t.Error("expected SetRoomDirectoryVisibility to be called")
	}
}

func TestUpdateRoomMetadata_CustomState(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
	}
	svc := newTestService(matrix)

	customState := map[string]map[string]interface{}{
		"io.alkemio.visibility": {"hidden": true},
	}
	err := svc.UpdateRoomMetadata(context.Background(), uuid.New(),
		nil, nil, nil, nil, nil, customState)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matrix.setCustomStateCalled {
		t.Error("expected SetCustomState to be called")
	}
}

// ============================================================================
// 5. DeleteRoomFully
// ============================================================================

func TestDeleteRoomFully_Success(t *testing.T) {
	user1 := id.UserID("@user1:test.local")
	user2 := id.UserID("@user2:test.local")

	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomMembersResult: []id.UserID{user1, user2},
	}
	svc := newTestService(matrix)

	err := svc.DeleteRoomFully(context.Background(), uuid.New(), "cleanup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matrix.kickUserCalled != 2 {
		t.Errorf("expected 2 kick calls, got %d", matrix.kickUserCalled)
	}
	if !matrix.deleteAliasCalled {
		t.Error("expected DeleteAlias to be called")
	}
}

func TestDeleteRoomFully_NotFound_Idempotent(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", domain.NewRoomNotFoundError("not found")
		},
	}
	svc := newTestService(matrix)

	err := svc.DeleteRoomFully(context.Background(), uuid.New(), "cleanup")
	if err != nil {
		t.Fatalf("expected idempotent success for not-found room, got: %v", err)
	}
}

func TestDeleteRoomFully_AliasResolveError(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "", errors.New("network error")
		},
	}
	svc := newTestService(matrix)

	err := svc.DeleteRoomFully(context.Background(), uuid.New(), "cleanup")
	if err == nil {
		t.Fatal("expected error for non-not-found resolve failure")
	}
}

func TestDeleteRoomFully_KickFailures_NonFatal(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		resolveAliasFunc: func(_ context.Context, _ string) (id.RoomID, error) {
			return "!room1:test.local", nil
		},
		getRoomMembersResult: []id.UserID{"@u1:test.local", "@u2:test.local"},
		kickUserErr:          errors.New("kick failed"),
	}
	svc := newTestService(matrix)

	err := svc.DeleteRoomFully(context.Background(), uuid.New(), "cleanup")
	// Kick failures are logged but not fatal
	if err != nil {
		t.Fatalf("expected success despite kick failures, got: %v", err)
	}
	if matrix.kickUserCalled != 2 {
		t.Errorf("expected 2 kick attempts, got %d", matrix.kickUserCalled)
	}
}

// ============================================================================
// 6. ListRooms
// ============================================================================

func TestListRooms_Success(t *testing.T) {
	roomUUID := uuid.New()
	roomAlias := testIDMapper.RoomAlias(roomUUID)

	matrix := &mockExtendedMatrixPort{
		getAllJoinedRoomsResult: []id.RoomID{"!room1:test.local"},
		getRoomDetailsResult:    &domain.Room{ID: "!room1:test.local", Alias: roomAlias},
	}
	svc := newTestService(matrix)

	ids, cursor, err := svc.ListRooms(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got '%s'", cursor)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 room ID, got %d", len(ids))
	}
	if ids[0] != roomUUID {
		t.Errorf("expected room UUID %s, got %s", roomUUID, ids[0])
	}
}

func TestListRooms_Error(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		getAllJoinedRoomsErr: errors.New("listing failed"),
	}
	svc := newTestService(matrix)

	_, _, err := svc.ListRooms(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when GetAllJoinedRooms fails")
	}
}

// ============================================================================
// 7. SendMessage — passthrough
// ============================================================================

func TestSendMessage_Success(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		sendMessageResult: "$msg1:test.local",
	}
	svc := newTestService(matrix)

	eventID, err := svc.SendMessage(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventID != "$msg1:test.local" {
		t.Errorf("expected event ID '$msg1:test.local', got '%s'", eventID)
	}
}

func TestSendMessage_Error(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		sendMessageErr: errors.New("send failed"),
	}
	svc := newTestService(matrix)

	_, err := svc.SendMessage(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "hello")
	if err == nil {
		t.Fatal("expected error from SendMessage")
	}
}

// ============================================================================
// 8. SendReply — passthrough
// ============================================================================

func TestSendReply_Success(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		sendReplyResult: "$reply1:test.local",
	}
	svc := newTestService(matrix)

	eventID, err := svc.SendReply(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "reply text", "$thread:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventID != "$reply1:test.local" {
		t.Errorf("expected event ID '$reply1:test.local', got '%s'", eventID)
	}
}

func TestSendReply_Error(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		sendReplyErr: errors.New("reply failed"),
	}
	svc := newTestService(matrix)

	_, err := svc.SendReply(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "reply text", "$thread:test.local")
	if err == nil {
		t.Fatal("expected error from SendReply")
	}
}

// ============================================================================
// 9. RedactEvent — passthrough
// ============================================================================

func TestRedactEvent_Success(t *testing.T) {
	matrix := &mockExtendedMatrixPort{}
	svc := newTestService(matrix)

	err := svc.RedactEvent(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "$evt:test.local", "spam")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRedactEvent_Error(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		redactEventErr: errors.New("redact failed"),
	}
	svc := newTestService(matrix)

	err := svc.RedactEvent(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "$evt:test.local", "spam")
	if err == nil {
		t.Fatal("expected error from RedactEvent")
	}
}

// ============================================================================
// 10. SendReaction — passthrough
// ============================================================================

func TestSendReaction_Success(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		sendReactionResult: "$react1:test.local",
	}
	svc := newTestService(matrix)

	eventID, err := svc.SendReaction(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "$evt:test.local", "thumbsup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventID != "$react1:test.local" {
		t.Errorf("expected '$react1:test.local', got '%s'", eventID)
	}
}

func TestSendReaction_Error(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		sendReactionErr: errors.New("reaction failed"),
	}
	svc := newTestService(matrix)

	_, err := svc.SendReaction(context.Background(), "!room:test.local", domain.NewActor(uuid.New()), "$evt:test.local", "thumbsup")
	if err == nil {
		t.Fatal("expected error from SendReaction")
	}
}

// ============================================================================
// 11. GetMessage — with sender mapping
// ============================================================================

func TestGetMessage_Success_SenderMapping(t *testing.T) {
	senderUUID := uuid.New()

	matrix := &mockExtendedMatrixPort{
		getMessageResult: &domain.Message{
			ID:             "$msg1",
			Content:        "hello",
			SenderMatrixID: string(testIDMapper.UserID(senderUUID)),
			Timestamp:      time.Now(),
		},
	}
	svc := newTestService(matrix)

	msg, err := svc.GetMessage(context.Background(), "!room:test.local", "$msg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.SenderID != senderUUID {
		t.Errorf("expected SenderID %s, got %s", senderUUID, msg.SenderID)
	}
}

func TestGetMessage_Error(t *testing.T) {
	matrix := &mockExtendedMatrixPort{
		getMessageErr: errors.New("message not found"),
	}
	svc := newTestService(matrix)

	_, err := svc.GetMessage(context.Background(), "!room:test.local", "$msg1")
	if err == nil {
		t.Fatal("expected error from GetMessage")
	}
}
