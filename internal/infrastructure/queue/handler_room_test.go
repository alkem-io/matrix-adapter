package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// ============================================================================
// Mock Logger
// ============================================================================

// queueMockLogger implements ports.Logger for tests.
type queueMockLogger struct{}

func (l *queueMockLogger) Debug(_ string, _ ...interface{}) {}
func (l *queueMockLogger) Info(_ string, _ ...interface{})  {}
func (l *queueMockLogger) Warn(_ string, _ ...interface{})  {}
func (l *queueMockLogger) Error(_ string, _ ...interface{}) {}
func (l *queueMockLogger) With(_ ...interface{}) ports.Logger {
	return l
}

// ============================================================================
// Mock MatrixPort
// ============================================================================

// queueMockMatrixPort implements ports.MatrixPort with configurable return values.
type queueMockMatrixPort struct {
	homeserverDomain string

	// ResolveAlias
	resolveAliasResult id.RoomID
	resolveAliasErr    error

	// CreateRoomWithAlias
	createRoomResult id.RoomID
	createRoomErr    error

	// GetRoomDetails
	getRoomDetailsResult *domain.Room
	getRoomDetailsErr    error

	// GetRoomMembers
	getRoomMembersResult []id.UserID
	getRoomMembersErr    error

	// UpdateRoomState
	updateRoomStateErr error

	// SetRoomDirectoryVisibility
	setRoomDirectoryVisibilityErr error

	// SetCustomState
	setCustomStateErr error

	// GetCustomState
	getCustomStateResult map[string]map[string]interface{}
	getCustomStateErr    error

	// DeleteAlias
	deleteAliasErr error

	// KickUser
	kickUserErr error

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

	// GetRoomMessages
	getRoomMessagesResult []domain.Message
	getRoomMessagesErr    error

	// GetLastMessage
	getLastMessageResult *domain.Message
	getLastMessageErr    error

	// GetBatchLastMessages
	getBatchLastMessagesResults map[id.RoomID]*domain.Message
	getBatchLastMessagesErrors  map[id.RoomID]error

	// GetReaction
	getReactionResult *domain.Reaction
	getReactionErr    error

	// GetThreadMessages
	getThreadMessagesResult []domain.Message
	getThreadMessagesErr    error

	// InviteUser
	inviteUserErr error

	// GetAllJoinedRooms
	getAllJoinedRoomsResult []id.RoomID
	getAllJoinedRoomsErr    error

	// GetUnreadCounts
	getUnreadCountsResult *domain.UnreadCountSummary
	getUnreadCountsErr    error

	// FindExistingDirectRoom
	findExistingDirectRoomResult id.RoomID
	findExistingDirectRoomErr    error
}

func (m *queueMockMatrixPort) HomeserverDomain() string {
	return m.homeserverDomain
}

func (m *queueMockMatrixPort) Connect(_ context.Context) error { return nil }
func (m *queueMockMatrixPort) Disconnect() error               { return nil }

func (m *queueMockMatrixPort) EnsureUser(_ context.Context, _ domain.Actor) (id.UserID, error) {
	return "", nil
}

func (m *queueMockMatrixPort) SetUserProfile(_ context.Context, _ domain.Actor) error {
	return nil
}

func (m *queueMockMatrixPort) CreateRoomWithAlias(_ context.Context, _ uuid.UUID, _, _, _, _, _ string, _ map[string]map[string]interface{}, _ []domain.Actor) (id.RoomID, error) {
	return m.createRoomResult, m.createRoomErr
}

func (m *queueMockMatrixPort) InviteUser(_ context.Context, _ id.RoomID, _ domain.Actor, _ domain.Actor) error {
	return m.inviteUserErr
}

func (m *queueMockMatrixPort) GetRoomDetails(_ context.Context, _ id.RoomID) (*domain.Room, error) {
	return m.getRoomDetailsResult, m.getRoomDetailsErr
}

func (m *queueMockMatrixPort) GetRoomMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return m.getRoomMembersResult, m.getRoomMembersErr
}

func (m *queueMockMatrixPort) UpdateRoomState(_ context.Context, _ id.RoomID, _ domain.Actor, _, _, _, _ *string) error {
	return m.updateRoomStateErr
}

func (m *queueMockMatrixPort) SetRoomDirectoryVisibility(_ context.Context, _ id.RoomID, _ bool) error {
	return m.setRoomDirectoryVisibilityErr
}

func (m *queueMockMatrixPort) SetCustomState(_ context.Context, _ id.RoomID, _ map[string]map[string]interface{}) error {
	return m.setCustomStateErr
}

func (m *queueMockMatrixPort) GetCustomState(_ context.Context, _ id.RoomID, _ []string) (map[string]map[string]interface{}, error) {
	return m.getCustomStateResult, m.getCustomStateErr
}

func (m *queueMockMatrixPort) ResolveAlias(_ context.Context, _ string) (id.RoomID, error) {
	return m.resolveAliasResult, m.resolveAliasErr
}

func (m *queueMockMatrixPort) DeleteAlias(_ context.Context, _ string) error {
	return m.deleteAliasErr
}

func (m *queueMockMatrixPort) KickUser(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return m.kickUserErr
}

func (m *queueMockMatrixPort) SendMessage(_ context.Context, _ id.RoomID, _ domain.Actor, _ string) (id.EventID, error) {
	return m.sendMessageResult, m.sendMessageErr
}

func (m *queueMockMatrixPort) SendReply(_ context.Context, _ id.RoomID, _ domain.Actor, _ string, _ id.EventID) (id.EventID, error) {
	return m.sendReplyResult, m.sendReplyErr
}

func (m *queueMockMatrixPort) RedactEvent(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) error {
	return m.redactEventErr
}

func (m *queueMockMatrixPort) SendReaction(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) (id.EventID, error) {
	return m.sendReactionResult, m.sendReactionErr
}

func (m *queueMockMatrixPort) GetMessage(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Message, error) {
	return m.getMessageResult, m.getMessageErr
}

func (m *queueMockMatrixPort) GetRoomMessages(_ context.Context, _ id.RoomID) ([]domain.Message, error) {
	return m.getRoomMessagesResult, m.getRoomMessagesErr
}

func (m *queueMockMatrixPort) GetLastMessage(_ context.Context, _ id.RoomID) (*domain.Message, error) {
	return m.getLastMessageResult, m.getLastMessageErr
}

func (m *queueMockMatrixPort) GetBatchLastMessages(_ context.Context, _ []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	return m.getBatchLastMessagesResults, m.getBatchLastMessagesErrors
}

func (m *queueMockMatrixPort) GetReactionEventID(_ context.Context, _ id.RoomID, _ id.EventID, _ string, _ domain.Actor) (id.EventID, error) {
	return "", nil
}

func (m *queueMockMatrixPort) GetReaction(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Reaction, error) {
	return m.getReactionResult, m.getReactionErr
}

func (m *queueMockMatrixPort) GetThreadMessages(_ context.Context, _ id.RoomID, _ id.EventID) ([]domain.Message, error) {
	return m.getThreadMessagesResult, m.getThreadMessagesErr
}

func (m *queueMockMatrixPort) FindExistingDirectRoom(_ context.Context, _ domain.Actor, _ domain.Actor) (id.RoomID, error) {
	return m.findExistingDirectRoomResult, m.findExistingDirectRoomErr
}

func (m *queueMockMatrixPort) SetRoomAlias(_ context.Context, _ id.RoomID, _ string) error {
	return nil
}

func (m *queueMockMatrixPort) GetAllJoinedRooms(_ context.Context) ([]id.RoomID, error) {
	return m.getAllJoinedRoomsResult, m.getAllJoinedRoomsErr
}

func (m *queueMockMatrixPort) CreateSpace(_ context.Context, _ uuid.UUID, _, _, _, _ string, _ []domain.Actor) (id.RoomID, error) {
	return "", nil
}

func (m *queueMockMatrixPort) GetSpaceDetails(_ context.Context, _ id.RoomID) (*domain.Space, error) {
	return nil, nil
}

func (m *queueMockMatrixPort) GetSpaceMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return nil, nil
}

func (m *queueMockMatrixPort) UpdateSpaceState(_ context.Context, _ id.RoomID, _, _, _, _ *string) error {
	return nil
}

func (m *queueMockMatrixPort) GetSpaceChildren(_ context.Context, _ id.RoomID) ([]domain.SpaceChild, error) {
	return nil, nil
}

func (m *queueMockMatrixPort) AddSpaceChild(_ context.Context, _ id.RoomID, _ id.RoomID, _ string, _ bool) error {
	return nil
}

func (m *queueMockMatrixPort) SetSpaceParent(_ context.Context, _ id.RoomID, _ id.RoomID) error {
	return nil
}

func (m *queueMockMatrixPort) InviteToSpace(_ context.Context, _ id.RoomID, _ domain.Actor) error {
	return nil
}

func (m *queueMockMatrixPort) KickFromSpace(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return nil
}

func (m *queueMockMatrixPort) SendReadReceipt(_ context.Context, _ domain.Actor, _ id.RoomID, _ id.EventID, _ *id.EventID) error {
	return nil
}

func (m *queueMockMatrixPort) GetUnreadCounts(_ context.Context, _ domain.Actor, _ id.RoomID, _ []id.EventID) (*domain.UnreadCountSummary, error) {
	return m.getUnreadCountsResult, m.getUnreadCountsErr
}

func (m *queueMockMatrixPort) GetBatchUnreadCounts(_ context.Context, _ domain.Actor, _ []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error) {
	return nil, nil
}

// Compile-time check that queueMockMatrixPort implements ports.MatrixPort.
var _ ports.MatrixPort = (*queueMockMatrixPort)(nil)

// ============================================================================
// Test Helper
// ============================================================================

const testDomain = "matrix.test.local"

// testRoomHandler builds a RoomHandler backed by the given mock.
func testRoomHandler(mock *queueMockMatrixPort) *RoomHandler {
	mock.homeserverDomain = testDomain
	idMapper := domain.NewIDMapper(testDomain)
	logger := &queueMockLogger{}
	svc := service.NewRoomService(mock, logger, idMapper)
	return NewRoomHandler(svc, mock, idMapper)
}

// mustMarshal marshals v to JSON or fails the test.
func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return data
}

// assertSuccess checks that result is a type with Success=true.
func assertSuccess(t *testing.T, result interface{}) {
	t.Helper()
	switch v := result.(type) {
	case dto.BaseResponse:
		if !v.Success {
			t.Fatalf("expected success=true, got false; error=%+v", v.Error)
		}
	default:
		// Try to extract BaseResponse from known response types via JSON round-trip
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("failed to marshal result: %v", err)
		}
		var base dto.BaseResponse
		if err := json.Unmarshal(data, &base); err != nil {
			t.Fatalf("failed to unmarshal base response: %v", err)
		}
		if !base.Success {
			t.Fatalf("expected success=true, got false; error=%+v", base.Error)
		}
	}
}

// assertErrorCode checks that the result is an error response with the given code.
func assertErrorCode(t *testing.T, result interface{}, expectedCode dto.ErrorCode) {
	t.Helper()
	base, ok := result.(dto.BaseResponse)
	if !ok {
		// Try JSON round-trip
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("failed to marshal result: %v", err)
		}
		if err := json.Unmarshal(data, &base); err != nil {
			t.Fatalf("failed to unmarshal base response: %v", err)
		}
	}
	if base.Success {
		t.Fatalf("expected success=false, got true")
	}
	if base.Error == nil {
		t.Fatalf("expected error to be non-nil")
	}
	if base.Error.Code != expectedCode {
		t.Errorf("expected error code=%s, got %s (message=%s)", expectedCode, base.Error.Code, base.Error.Message)
	}
}

// ============================================================================
// DTO Conversion Helper Tests
// ============================================================================

func TestConvertReactionToDTO(t *testing.T) {
	senderID := uuid.New()
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r := domain.Reaction{
		ID:        "$reaction1:test",
		Emoji:     "👍",
		SenderID:  senderID,
		Timestamp: ts,
	}

	result := convertReactionToDTO(r)

	if result.ID != dto.ReactionID("$reaction1:test") {
		t.Errorf("expected reaction ID=$reaction1:test, got %s", result.ID)
	}
	if result.Emoji != "👍" {
		t.Errorf("expected emoji=👍, got %s", result.Emoji)
	}
	if result.SenderActorID != dto.AlkemioActorID(senderID) {
		t.Errorf("expected sender=%s, got %s", senderID, result.SenderActorID)
	}
	if result.Timestamp != ts.UnixMilli() {
		t.Errorf("expected timestamp=%d, got %d", ts.UnixMilli(), result.Timestamp)
	}
}

func TestConvertMessageToDTO(t *testing.T) {
	senderID := uuid.New()
	ts := time.Date(2025, 2, 20, 14, 0, 0, 0, time.UTC)

	t.Run("message without thread or reactions", func(t *testing.T) {
		msg := domain.Message{
			ID:        "$msg1:test",
			Content:   "Hello, world!",
			SenderID:  senderID,
			Timestamp: ts,
		}

		result := convertMessageToDTO(msg)

		if string(result.ID) != "$msg1:test" {
			t.Errorf("expected ID=$msg1:test, got %s", result.ID)
		}
		if result.Content != "Hello, world!" {
			t.Errorf("expected content='Hello, world!', got %s", result.Content)
		}
		if result.ThreadID != nil {
			t.Errorf("expected ThreadID=nil, got %v", result.ThreadID)
		}
		if len(result.Reactions) != 0 {
			t.Errorf("expected 0 reactions, got %d", len(result.Reactions))
		}
	})

	t.Run("message with thread ID", func(t *testing.T) {
		msg := domain.Message{
			ID:        "$msg2:test",
			Content:   "Reply in thread",
			SenderID:  senderID,
			Timestamp: ts,
			ThreadID:  "$parent:test",
		}

		result := convertMessageToDTO(msg)

		if result.ThreadID == nil {
			t.Fatalf("expected ThreadID to be set")
		}
		if string(*result.ThreadID) != "$parent:test" {
			t.Errorf("expected ThreadID=$parent:test, got %s", *result.ThreadID)
		}
	})

	t.Run("message with reactions", func(t *testing.T) {
		msg := domain.Message{
			ID:        "$msg3:test",
			Content:   "React to me",
			SenderID:  senderID,
			Timestamp: ts,
			Reactions: []domain.Reaction{
				{ID: "$r1:test", Emoji: "👍", SenderID: senderID, Timestamp: ts},
				{ID: "$r2:test", Emoji: "❤️", SenderID: senderID, Timestamp: ts},
			},
		}

		result := convertMessageToDTO(msg)

		if len(result.Reactions) != 2 {
			t.Fatalf("expected 2 reactions, got %d", len(result.Reactions))
		}
		if result.Reactions[0].Emoji != "👍" {
			t.Errorf("expected first reaction emoji=👍, got %s", result.Reactions[0].Emoji)
		}
		if result.Reactions[1].Emoji != "❤️" {
			t.Errorf("expected second reaction emoji=❤️, got %s", result.Reactions[1].Emoji)
		}
	})
}

func TestConvertMessagesToDTO(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := convertMessagesToDTO([]domain.Message{})
		if len(result) != 0 {
			t.Errorf("expected 0 messages, got %d", len(result))
		}
	})

	t.Run("multiple messages", func(t *testing.T) {
		ts := time.Now()
		msgs := []domain.Message{
			{ID: "$a:test", Content: "first", SenderID: uuid.New(), Timestamp: ts},
			{ID: "$b:test", Content: "second", SenderID: uuid.New(), Timestamp: ts},
			{ID: "$c:test", Content: "third", SenderID: uuid.New(), Timestamp: ts},
		}

		result := convertMessagesToDTO(msgs)

		if len(result) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(result))
		}
		if result[0].Content != "first" {
			t.Errorf("expected first message content='first', got %s", result[0].Content)
		}
		if result[2].Content != "third" {
			t.Errorf("expected third message content='third', got %s", result[2].Content)
		}
	})
}

func TestConvertMemberIDsToDTO(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := convertMemberIDsToDTO([]uuid.UUID{})
		if len(result) != 0 {
			t.Errorf("expected 0 members, got %d", len(result))
		}
	})

	t.Run("multiple members", func(t *testing.T) {
		id1 := uuid.New()
		id2 := uuid.New()
		result := convertMemberIDsToDTO([]uuid.UUID{id1, id2})

		if len(result) != 2 {
			t.Fatalf("expected 2 members, got %d", len(result))
		}
		if result[0] != dto.AlkemioActorID(id1) {
			t.Errorf("expected first member=%s, got %s", id1, result[0])
		}
		if result[1] != dto.AlkemioActorID(id2) {
			t.Errorf("expected second member=%s, got %s", id2, result[1])
		}
	})
}

// ============================================================================
// HandleCreateRoom Tests
// ============================================================================

func TestHandleCreateRoom_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasErr:  domain.ErrRoomNotFound, // room does not exist yet
		createRoomResult: "!newroom:test",
	}
	h := testRoomHandler(mock)

	roomID := uuid.New()
	memberID := uuid.New()
	payload := mustMarshal(t, dto.CreateRoomRequest{
		AlkemioRoomID:  dto.AlkemioRoomID(roomID),
		Type:           dto.RoomTypeCommunity,
		Name:           "Test Room",
		InitialMembers: []dto.AlkemioActorID{dto.AlkemioActorID(memberID)},
	})

	result, err := h.HandleCreateRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

func TestHandleCreateRoom_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleCreateRoom(context.Background(), []byte(`{invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleCreateRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.CreateRoomRequest{
		Type: dto.RoomTypeCommunity,
		Name: "Test Room",
	})

	result, err := h.HandleCreateRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleGetRoom Tests
// ============================================================================

func TestHandleGetRoom_Success(t *testing.T) {
	roomUUID := uuid.New()
	memberUUID := uuid.New()
	memberMatrixID := id.NewUserID(memberUUID.String(), testDomain)
	ts := time.Now()

	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getRoomDetailsResult: &domain.Room{
			ID:   "!room1:test",
			Name: "My Room",
		},
		getRoomMembersResult: []id.UserID{memberMatrixID},
		getRoomMessagesResult: []domain.Message{
			{ID: "$msg1:test", Content: "Hello", SenderMatrixID: string(memberMatrixID), Timestamp: ts},
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetRoomRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
	})

	result, err := h.HandleGetRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetRoomResponse)
	if !ok {
		t.Fatalf("expected GetRoomResponse, got %T", result)
	}
	if resp.DisplayName != "My Room" {
		t.Errorf("expected display_name='My Room', got %s", resp.DisplayName)
	}
	if len(resp.MemberActorIDs) != 1 {
		t.Errorf("expected 1 member, got %d", len(resp.MemberActorIDs))
	}
	if len(resp.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(resp.Messages))
	}
}

func TestHandleGetRoom_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetRoom(context.Background(), []byte(`not-json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetRoomRequest{})

	result, err := h.HandleGetRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoom_RoomNotFound(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasErr: domain.ErrRoomNotFound,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetRoomRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
	})

	result, err := h.HandleGetRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeRoomNotFound)
}

// ============================================================================
// HandleGetRoomAsUser Tests
// ============================================================================

func TestHandleGetRoomAsUser_Success(t *testing.T) {
	roomUUID := uuid.New()
	actorUUID := uuid.New()
	actorMatrixID := id.NewUserID(actorUUID.String(), testDomain)
	ts := time.Now()

	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getRoomDetailsResult: &domain.Room{
			ID:   "!room1:test",
			Name: "User Room",
		},
		getRoomMembersResult: []id.UserID{actorMatrixID},
		getRoomMessagesResult: []domain.Message{
			{ID: "$m1:test", Content: "msg1", SenderMatrixID: string(actorMatrixID), Timestamp: ts},
			{ID: "$m2:test", Content: "msg2", SenderMatrixID: string(actorMatrixID), Timestamp: ts.Add(time.Second)},
		},
		getUnreadCountsResult: &domain.UnreadCountSummary{
			RoomUnreadCount: 1,
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetRoomAsUserRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomUUID),
		ActorID:       dto.AlkemioActorID(actorUUID),
	})

	result, err := h.HandleGetRoomAsUser(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetRoomAsUserResponse)
	if !ok {
		t.Fatalf("expected GetRoomAsUserResponse, got %T", result)
	}
	if resp.UnreadCount != 1 {
		t.Errorf("expected unread_count=1, got %d", resp.UnreadCount)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(resp.Messages))
	}
	// First message should be read, second unread
	if !resp.Messages[0].IsRead {
		t.Errorf("expected first message to be read")
	}
	if resp.Messages[1].IsRead {
		t.Errorf("expected second message to be unread")
	}
}

func TestHandleGetRoomAsUser_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetRoomAsUser(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoomAsUser_MissingActorID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetRoomAsUserRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		// ActorID missing (zero value)
	})

	result, err := h.HandleGetRoomAsUser(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleUpdateRoom Tests
// ============================================================================

func TestHandleUpdateRoom_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	roomID := uuid.New()
	newName := "Updated Name"
	payload := mustMarshal(t, dto.UpdateRoomRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomID),
		Name:          &newName,
	})

	result, err := h.HandleUpdateRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

func TestHandleUpdateRoom_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleUpdateRoom(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleUpdateRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	newName := "Test"
	payload := mustMarshal(t, dto.UpdateRoomRequest{
		Name: &newName,
	})

	result, err := h.HandleUpdateRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleUpdateRoom_WithJoinRule(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	roomID := uuid.New()
	jr := dto.JoinRulePublic
	payload := mustMarshal(t, dto.UpdateRoomRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomID),
		JoinRule:      &jr,
	})

	result, err := h.HandleUpdateRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

// ============================================================================
// HandleDeleteRoom Tests
// ============================================================================

func TestHandleDeleteRoom_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult:   "!room1:test",
		getRoomMembersResult: []id.UserID{},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.DeleteRoomRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		Reason:        "No longer needed",
	})

	result, err := h.HandleDeleteRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

func TestHandleDeleteRoom_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleDeleteRoom(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleDeleteRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.DeleteRoomRequest{Reason: "test"})

	result, err := h.HandleDeleteRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleListRooms Tests
// ============================================================================

func TestHandleListRooms_Success(t *testing.T) {
	roomUUID := uuid.New()
	alias := "#" + roomUUID.String() + ":" + testDomain

	mock := &queueMockMatrixPort{
		getAllJoinedRoomsResult: []id.RoomID{"!room1:test"},
		getRoomDetailsResult: &domain.Room{
			ID:    "!room1:test",
			Alias: alias,
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.ListRoomsRequest{})

	result, err := h.HandleListRooms(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.ListRoomsResponse)
	if !ok {
		t.Fatalf("expected ListRoomsResponse, got %T", result)
	}
	if len(resp.AlkemioRoomIDs) != 1 {
		t.Errorf("expected 1 room ID, got %d", len(resp.AlkemioRoomIDs))
	}
}

func TestHandleListRooms_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleListRooms(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleListRooms_Empty(t *testing.T) {
	mock := &queueMockMatrixPort{
		getAllJoinedRoomsResult: []id.RoomID{},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.ListRoomsRequest{})

	result, err := h.HandleListRooms(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp := result.(dto.ListRoomsResponse)
	if len(resp.AlkemioRoomIDs) != 0 {
		t.Errorf("expected 0 room IDs, got %d", len(resp.AlkemioRoomIDs))
	}
}

// ============================================================================
// HandleSendMessage Tests
// ============================================================================

func TestHandleSendMessage_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendMessageResult:  "$evt1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Content:       "Hello!",
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.SendMessageResponse)
	if !ok {
		t.Fatalf("expected SendMessageResponse, got %T", result)
	}
	if string(resp.MessageID) != "$evt1:test" {
		t.Errorf("expected message_id=$evt1:test, got %s", resp.MessageID)
	}
	if resp.Timestamp == 0 {
		t.Errorf("expected non-zero timestamp")
	}
}

func TestHandleSendMessage_WithThread(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendReplyResult:    "$reply1:test",
	}
	h := testRoomHandler(mock)

	parentID := dto.MessageID("$parent:test")
	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID:   dto.AlkemioRoomID(uuid.New()),
		SenderActorID:   dto.AlkemioActorID(uuid.New()),
		Content:         "Reply!",
		ParentMessageID: &parentID,
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp := result.(dto.SendMessageResponse)
	if string(resp.MessageID) != "$reply1:test" {
		t.Errorf("expected message_id=$reply1:test, got %s", resp.MessageID)
	}
}

func TestHandleSendMessage_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleSendMessage(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleSendMessage_MissingSenderID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		Content:       "Hello",
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleSendMessage_EmptyContent(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Content:       "",
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleSendMessage_RoomNotFound(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasErr: domain.ErrRoomNotFound,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Content:       "Hello!",
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeRoomNotFound)
}

// ============================================================================
// HandleGetMessage Tests
// ============================================================================

func TestHandleGetMessage_Success(t *testing.T) {
	senderUUID := uuid.New()
	ts := time.Now()
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getMessageResult: &domain.Message{
			ID:             "$msg1:test",
			Content:        "Found it",
			SenderMatrixID: "@" + senderUUID.String() + ":" + testDomain,
			Timestamp:      ts,
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$msg1:test",
	})

	result, err := h.HandleGetMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetMessageResponse)
	if !ok {
		t.Fatalf("expected GetMessageResponse, got %T", result)
	}
	if string(resp.Message.ID) != "$msg1:test" {
		t.Errorf("expected message ID=$msg1:test, got %s", resp.Message.ID)
	}
	if resp.Message.Content != "Found it" {
		t.Errorf("expected content='Found it', got %s", resp.Message.Content)
	}
}

func TestHandleGetMessage_NotFound(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getMessageResult:   nil, // message not found
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$missing:test",
	})

	result, err := h.HandleGetMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeMessageNotFound)
}

func TestHandleGetMessage_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetMessage(context.Background(), []byte(`xxx`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetMessage_MissingMessageID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		// MessageID missing
	})

	result, err := h.HandleGetMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleDeleteMessage Tests
// ============================================================================

func TestHandleDeleteMessage_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.DeleteMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$msg1:test",
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Reason:        "spam",
	})

	result, err := h.HandleDeleteMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

func TestHandleDeleteMessage_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleDeleteMessage(context.Background(), []byte(`[`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleDeleteMessage_MissingFields(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	t.Run("missing room ID", func(t *testing.T) {
		payload := mustMarshal(t, dto.DeleteMessageRequest{
			MessageID:     "$msg:test",
			SenderActorID: dto.AlkemioActorID(uuid.New()),
		})
		result, err := h.HandleDeleteMessage(context.Background(), payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertErrorCode(t, result, dto.ErrCodeInvalidParam)
	})

	t.Run("missing message ID", func(t *testing.T) {
		payload := mustMarshal(t, dto.DeleteMessageRequest{
			AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
			SenderActorID: dto.AlkemioActorID(uuid.New()),
		})
		result, err := h.HandleDeleteMessage(context.Background(), payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertErrorCode(t, result, dto.ErrCodeInvalidParam)
	})

	t.Run("missing sender ID", func(t *testing.T) {
		payload := mustMarshal(t, dto.DeleteMessageRequest{
			AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
			MessageID:     "$msg:test",
		})
		result, err := h.HandleDeleteMessage(context.Background(), payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertErrorCode(t, result, dto.ErrCodeInvalidParam)
	})
}

// ============================================================================
// HandleAddReaction Tests
// ============================================================================

func TestHandleAddReaction_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendReactionResult: "$reaction1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.AddReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$msg1:test",
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Emoji:         "👍",
	})

	result, err := h.HandleAddReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.AddReactionResponse)
	if !ok {
		t.Fatalf("expected AddReactionResponse, got %T", result)
	}
	if string(resp.ReactionID) != "$reaction1:test" {
		t.Errorf("expected reaction_id=$reaction1:test, got %s", resp.ReactionID)
	}
	if resp.Timestamp == 0 {
		t.Errorf("expected non-zero timestamp")
	}
}

func TestHandleAddReaction_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleAddReaction(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleAddReaction_MissingEmoji(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.AddReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$msg1:test",
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Emoji:         "",
	})

	result, err := h.HandleAddReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleAddReaction_MissingMessageID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.AddReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Emoji:         "👍",
	})

	result, err := h.HandleAddReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleRemoveReaction Tests
// ============================================================================

func TestHandleRemoveReaction_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.RemoveReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		ReactionID:    "$reaction1:test",
		SenderActorID: dto.AlkemioActorID(uuid.New()),
	})

	result, err := h.HandleRemoveReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

func TestHandleRemoveReaction_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleRemoveReaction(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleRemoveReaction_MissingReactionID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.RemoveReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
	})

	result, err := h.HandleRemoveReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleRemoveReaction_MissingSenderID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.RemoveReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		ReactionID:    "$reaction1:test",
	})

	result, err := h.HandleRemoveReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleGetReaction Tests
// ============================================================================

func TestHandleGetReaction_Success(t *testing.T) {
	senderUUID := uuid.New()
	ts := time.Now()
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getReactionResult: &domain.Reaction{
			ID:        "$reaction1:test",
			Emoji:     "❤️",
			SenderID:  senderUUID,
			Timestamp: ts,
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		ReactionID:    "$reaction1:test",
	})

	result, err := h.HandleGetReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetReactionResponse)
	if !ok {
		t.Fatalf("expected GetReactionResponse, got %T", result)
	}
	if resp.Reaction.Emoji != "❤️" {
		t.Errorf("expected emoji=❤️, got %s", resp.Reaction.Emoji)
	}
}

func TestHandleGetReaction_NotFound(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getReactionResult:  nil,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		ReactionID:    "$missing:test",
	})

	result, err := h.HandleGetReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeReactionNotFound)
}

func TestHandleGetReaction_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetReaction(context.Background(), []byte(`not-json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetReaction_MissingReactionID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
	})

	result, err := h.HandleGetReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetReaction_ResolvesSenderID(t *testing.T) {
	senderUUID := uuid.New()
	senderMatrixID := "@" + senderUUID.String() + ":" + testDomain
	ts := time.Now()

	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getReactionResult: &domain.Reaction{
			ID:             "$reaction1:test",
			Emoji:          "🎉",
			SenderID:       uuid.Nil, // Not pre-resolved
			SenderMatrixID: senderMatrixID,
			Timestamp:      ts,
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		ReactionID:    "$reaction1:test",
	})

	result, err := h.HandleGetReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp := result.(dto.GetReactionResponse)
	if resp.Reaction.SenderActorID != dto.AlkemioActorID(senderUUID) {
		t.Errorf("expected sender to be resolved to %s, got %s", senderUUID, resp.Reaction.SenderActorID)
	}
}

// ============================================================================
// HandleBatchAddMember Tests
// ============================================================================

func TestHandleBatchAddMember_Success(t *testing.T) {
	roomID1 := uuid.New()
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.BatchAddMemberRequest{
		ActorID:        dto.AlkemioActorID(uuid.New()),
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(roomID1)},
	})

	result, err := h.HandleBatchAddMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.BatchAddMemberResponse)
	if !ok {
		t.Fatalf("expected BatchAddMemberResponse, got %T", result)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}
	// Check the per-room result
	roomResult, exists := resp.Results[roomID1.String()]
	if !exists {
		t.Fatalf("expected result for room %s", roomID1)
	}
	if !roomResult.Success {
		t.Errorf("expected per-room result success=true")
	}
}

func TestHandleBatchAddMember_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleBatchAddMember(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchAddMember_MissingActorID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.BatchAddMemberRequest{
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(uuid.New())},
	})

	result, err := h.HandleBatchAddMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchAddMember_EmptyRoomIDs(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.BatchAddMemberRequest{
		ActorID:        dto.AlkemioActorID(uuid.New()),
		AlkemioRoomIDs: []dto.AlkemioRoomID{},
	})

	result, err := h.HandleBatchAddMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleBatchRemoveMember Tests
// ============================================================================

func TestHandleBatchRemoveMember_Success(t *testing.T) {
	roomID1 := uuid.New()
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.BatchRemoveMemberRequest{
		ActorID:        dto.AlkemioActorID(uuid.New()),
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(roomID1)},
		Reason:         "no longer relevant",
	})

	result, err := h.HandleBatchRemoveMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.BatchRemoveMemberResponse)
	if !ok {
		t.Fatalf("expected BatchRemoveMemberResponse, got %T", result)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestHandleBatchRemoveMember_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleBatchRemoveMember(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchRemoveMember_MissingActorID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.BatchRemoveMemberRequest{
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(uuid.New())},
	})

	result, err := h.HandleBatchRemoveMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchRemoveMember_EmptyRoomIDs(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.BatchRemoveMemberRequest{
		ActorID:        dto.AlkemioActorID(uuid.New()),
		AlkemioRoomIDs: []dto.AlkemioRoomID{},
	})

	result, err := h.HandleBatchRemoveMember(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleGetRoomMembers Tests
// ============================================================================

func TestHandleGetRoomMembers_Success(t *testing.T) {
	memberUUID := uuid.New()
	memberMatrixID := id.NewUserID(memberUUID.String(), testDomain)

	mock := &queueMockMatrixPort{
		resolveAliasResult:   "!room1:test",
		getRoomMembersResult: []id.UserID{memberMatrixID},
	}
	h := testRoomHandler(mock)

	roomID := uuid.New()
	payload := mustMarshal(t, dto.GetRoomMembersRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomID),
	})

	result, err := h.HandleGetRoomMembers(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetRoomMembersResponse)
	if !ok {
		t.Fatalf("expected GetRoomMembersResponse, got %T", result)
	}
	if len(resp.MemberActorIDs) != 1 {
		t.Fatalf("expected 1 member, got %d", len(resp.MemberActorIDs))
	}
	if resp.MemberActorIDs[0] != dto.AlkemioActorID(memberUUID) {
		t.Errorf("expected member=%s, got %s", memberUUID, resp.MemberActorIDs[0])
	}
}

func TestHandleGetRoomMembers_FiltersNonGhostUsers(t *testing.T) {
	// A non-ghost user (Matrix user ID that doesn't parse to UUID) should be filtered
	memberUUID := uuid.New()
	memberMatrixID := id.NewUserID(memberUUID.String(), testDomain)
	nonGhostUser := id.UserID("@admin:" + testDomain) // not a valid UUID localpart

	mock := &queueMockMatrixPort{
		resolveAliasResult:   "!room1:test",
		getRoomMembersResult: []id.UserID{memberMatrixID, nonGhostUser},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetRoomMembersRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
	})

	result, err := h.HandleGetRoomMembers(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp := result.(dto.GetRoomMembersResponse)
	if len(resp.MemberActorIDs) != 1 {
		t.Errorf("expected 1 member (non-ghost filtered), got %d", len(resp.MemberActorIDs))
	}
}

func TestHandleGetRoomMembers_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetRoomMembers(context.Background(), []byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoomMembers_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetRoomMembersRequest{})

	result, err := h.HandleGetRoomMembers(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleGetThreadMessages Tests
// ============================================================================

func TestHandleGetThreadMessages_Success(t *testing.T) {
	senderUUID := uuid.New()
	ts := time.Now()

	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getThreadMessagesResult: []domain.Message{
			{ID: "$thread_root:test", Content: "Root", SenderMatrixID: "@" + senderUUID.String() + ":" + testDomain, Timestamp: ts},
			{ID: "$reply1:test", Content: "Reply", SenderMatrixID: "@" + senderUUID.String() + ":" + testDomain, Timestamp: ts.Add(time.Second), ThreadID: "$thread_root:test"},
		},
	}
	h := testRoomHandler(mock)

	roomID := uuid.New()
	payload := mustMarshal(t, dto.GetThreadMessagesRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomID),
		ThreadID:      "$thread_root:test",
	})

	result, err := h.HandleGetThreadMessages(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetThreadMessagesResponse)
	if !ok {
		t.Fatalf("expected GetThreadMessagesResponse, got %T", result)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(resp.Messages))
	}
	if string(resp.ThreadID) != "$thread_root:test" {
		t.Errorf("expected thread_id=$thread_root:test, got %s", resp.ThreadID)
	}
}

func TestHandleGetThreadMessages_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetThreadMessages(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetThreadMessages_MissingThreadID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetThreadMessagesRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
	})

	result, err := h.HandleGetThreadMessages(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetThreadMessages_RoomNotFound(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasErr: domain.ErrRoomNotFound,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetThreadMessagesRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		ThreadID:      "$thread:test",
	})

	result, err := h.HandleGetThreadMessages(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeRoomNotFound)
}

// ============================================================================
// HandleGetLastMessage Tests
// ============================================================================

func TestHandleGetLastMessage_Success(t *testing.T) {
	senderUUID := uuid.New()
	ts := time.Now()

	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getLastMessageResult: &domain.Message{
			ID:             "$last:test",
			Content:        "Last message",
			SenderMatrixID: "@" + senderUUID.String() + ":" + testDomain,
			Timestamp:      ts,
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetLastMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
	})

	result, err := h.HandleGetLastMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetLastMessageResponse)
	if !ok {
		t.Fatalf("expected GetLastMessageResponse, got %T", result)
	}
	if resp.Message == nil {
		t.Fatalf("expected message to be non-nil")
	}
	if resp.Message.Content != "Last message" {
		t.Errorf("expected content='Last message', got %s", resp.Message.Content)
	}
}

func TestHandleGetLastMessage_EmptyRoom(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult:   "!room1:test",
		getLastMessageResult: nil, // no messages
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetLastMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
	})

	result, err := h.HandleGetLastMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp := result.(dto.GetLastMessageResponse)
	if resp.Message != nil {
		t.Errorf("expected message to be nil for empty room")
	}
}

func TestHandleGetLastMessage_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetLastMessage(context.Background(), []byte(`bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetLastMessage_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetLastMessageRequest{})

	result, err := h.HandleGetLastMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleBatchGetLastMessages Tests
// ============================================================================

func TestHandleBatchGetLastMessages_Success(t *testing.T) {
	roomID1 := uuid.New()
	matrixRoomID := id.RoomID("!room1:test")
	senderUUID := uuid.New()
	ts := time.Now()

	msg := &domain.Message{
		ID:             "$last:test",
		Content:        "Latest",
		SenderMatrixID: "@" + senderUUID.String() + ":" + testDomain,
		Timestamp:      ts,
	}

	mock := &queueMockMatrixPort{
		resolveAliasResult: matrixRoomID,
		getBatchLastMessagesResults: map[id.RoomID]*domain.Message{
			matrixRoomID: msg,
		},
		getBatchLastMessagesErrors: map[id.RoomID]error{},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.BatchGetLastMessagesRequest{
		AlkemioRoomIDs: []dto.AlkemioRoomID{dto.AlkemioRoomID(roomID1)},
	})

	result, err := h.HandleBatchGetLastMessages(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.BatchGetLastMessagesResponse)
	if !ok {
		t.Fatalf("expected BatchGetLastMessagesResponse, got %T", result)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message entry, got %d", len(resp.Messages))
	}
	msgResult, exists := resp.Messages[roomID1.String()]
	if !exists {
		t.Fatalf("expected result for room %s", roomID1)
	}
	if msgResult == nil {
		t.Fatalf("expected non-nil message")
	}
	if msgResult.Content != "Latest" {
		t.Errorf("expected content='Latest', got %s", msgResult.Content)
	}
}

func TestHandleBatchGetLastMessages_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleBatchGetLastMessages(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchGetLastMessages_EmptyRoomIDs(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.BatchGetLastMessagesRequest{
		AlkemioRoomIDs: []dto.AlkemioRoomID{},
	})

	result, err := h.HandleBatchGetLastMessages(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// HandleSetRoomState Tests
// ============================================================================

func TestHandleSetRoomState_Success(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SetRoomStateRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		State: map[string]map[string]interface{}{
			"io.alkemio.visibility": {"visible": true},
		},
	})

	result, err := h.HandleSetRoomState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

func TestHandleSetRoomState_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleSetRoomState(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleSetRoomState_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.SetRoomStateRequest{
		State: map[string]map[string]interface{}{
			"io.alkemio.test": {"key": "val"},
		},
	})

	result, err := h.HandleSetRoomState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleSetRoomState_RoomNotFound(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasErr: domain.ErrRoomNotFound,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SetRoomStateRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		State: map[string]map[string]interface{}{
			"io.alkemio.test": {"key": "val"},
		},
	})

	result, err := h.HandleSetRoomState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeRoomNotFound)
}

// ============================================================================
// HandleGetRoomState Tests
// ============================================================================

func TestHandleGetRoomState_Success(t *testing.T) {
	expectedState := map[string]map[string]interface{}{
		"io.alkemio.visibility": {"visible": true},
	}
	mock := &queueMockMatrixPort{
		resolveAliasResult:   "!room1:test",
		getCustomStateResult: expectedState,
	}
	h := testRoomHandler(mock)

	roomID := uuid.New()
	payload := mustMarshal(t, dto.GetRoomStateRequest{
		AlkemioRoomID: dto.AlkemioRoomID(roomID),
	})

	result, err := h.HandleGetRoomState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	resp, ok := result.(dto.GetRoomStateResponse)
	if !ok {
		t.Fatalf("expected GetRoomStateResponse, got %T", result)
	}
	if resp.State == nil {
		t.Fatalf("expected state to be non-nil")
	}
	vis, exists := resp.State["io.alkemio.visibility"]
	if !exists {
		t.Fatalf("expected io.alkemio.visibility state key")
	}
	if vis["visible"] != true {
		t.Errorf("expected visible=true, got %v", vis["visible"])
	}
}

func TestHandleGetRoomState_WithEventTypeFilter(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		getCustomStateResult: map[string]map[string]interface{}{
			"io.alkemio.visibility": {"visible": false},
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.GetRoomStateRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		EventTypes:    []string{"io.alkemio.visibility"},
	})

	result, err := h.HandleGetRoomState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
}

func TestHandleGetRoomState_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	result, err := h.HandleGetRoomState(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoomState_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	payload := mustMarshal(t, dto.GetRoomStateRequest{})

	result, err := h.HandleGetRoomState(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

// ============================================================================
// ResolveSenderID Tests
// ============================================================================

func TestResolveSenderID(t *testing.T) {
	h := testRoomHandler(&queueMockMatrixPort{})

	t.Run("empty matrix ID returns Nil", func(t *testing.T) {
		result := h.resolveSenderID(context.Background(), "")
		if result != uuid.Nil {
			t.Errorf("expected uuid.Nil, got %s", result)
		}
	})

	t.Run("valid ghost user ID is resolved", func(t *testing.T) {
		actorUUID := uuid.New()
		matrixID := "@" + actorUUID.String() + ":" + testDomain
		result := h.resolveSenderID(context.Background(), matrixID)
		if result != actorUUID {
			t.Errorf("expected %s, got %s", actorUUID, result)
		}
	})

	t.Run("non-ghost user ID returns Nil", func(t *testing.T) {
		result := h.resolveSenderID(context.Background(), "@admin:"+testDomain)
		if result != uuid.Nil {
			t.Errorf("expected uuid.Nil for non-ghost user, got %s", result)
		}
	})
}

// ============================================================================
// Service Error Propagation Tests
// ============================================================================

func TestHandleSendMessage_ServiceError(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendMessageErr:     domain.ErrForbidden,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Content:       "test",
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeNotAllowed)
}

func TestHandleAddReaction_ServiceError(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendReactionErr:    domain.ErrRoomNotFound,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.AddReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$msg:test",
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Emoji:         "👍",
	})

	result, err := h.HandleAddReaction(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeRoomNotFound)
}

func TestHandleDeleteMessage_ServiceError(t *testing.T) {
	mock := &queueMockMatrixPort{
		resolveAliasResult: "!room1:test",
		redactEventErr:     domain.ErrForbidden,
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.DeleteMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$msg:test",
		SenderActorID: dto.AlkemioActorID(uuid.New()),
	})

	result, err := h.HandleDeleteMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeNotAllowed)
}
