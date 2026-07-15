//nolint:revive // test file
package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// ============================================================================
// Test Helper
// ============================================================================

// testRoomHandler builds a RoomHandler backed by the given mock.
func testRoomHandler(mock *testMockMatrixPort) *RoomHandler {
	mock.homeserverDomain = testDomain
	idMapper := domain.NewIDMapper(testDomain)
	logger := &testMockLogger{}
	svc := service.NewRoomService(mock, logger, idMapper)
	return NewRoomHandler(svc, mock, idMapper)
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
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedCreateRoomName != "Test Room" {
		t.Errorf("expected room name 'Test Room', got %q", mock.capturedCreateRoomName)
	}
	if mock.capturedCreateRoomType != string(dto.RoomTypeCommunity) {
		t.Errorf("expected room type %q, got %q", dto.RoomTypeCommunity, mock.capturedCreateRoomType)
	}
	if mock.capturedCreateRoomAlkemioID != roomID {
		t.Errorf("expected alkemio room ID %s, got %s", roomID, mock.capturedCreateRoomAlkemioID)
	}
	if len(mock.capturedCreateRoomMembers) != 1 {
		t.Fatalf("expected 1 initial member, got %d", len(mock.capturedCreateRoomMembers))
	}
	if mock.capturedCreateRoomMembers[0].ID != memberID {
		t.Errorf("expected member ID %s, got %s", memberID, mock.capturedCreateRoomMembers[0].ID)
	}
}

func TestHandleCreateRoom_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleCreateRoom(context.Background(), []byte(`{invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleCreateRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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

	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetRoom(context.Background(), []byte(`not-json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	payload := mustMarshal(t, dto.GetRoomRequest{})

	result, err := h.HandleGetRoom(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoom_RoomNotFound(t *testing.T) {
	mock := &testMockMatrixPort{
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

	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetRoomAsUser(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoomAsUser_MissingActorID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedUpdateRoomStateName == nil || *mock.capturedUpdateRoomStateName != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %v", mock.capturedUpdateRoomStateName)
	}
	if mock.capturedUpdateRoomStateRoomID != "!room1:test" {
		t.Errorf("expected room ID '!room1:test', got %q", mock.capturedUpdateRoomStateRoomID)
	}
}

func TestHandleUpdateRoom_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleUpdateRoom(context.Background(), []byte(`!!!`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleUpdateRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly converted and forwarded the join rule
	if mock.capturedUpdateRoomStateJoinRule == nil || *mock.capturedUpdateRoomStateJoinRule != string(dto.JoinRulePublic) {
		t.Errorf("expected join rule %q, got %v", dto.JoinRulePublic, mock.capturedUpdateRoomStateJoinRule)
	}
	// Name should not be set
	if mock.capturedUpdateRoomStateName != nil {
		t.Errorf("expected name to be nil, got %q", *mock.capturedUpdateRoomStateName)
	}
}

// ============================================================================
// HandleDeleteRoom Tests
// ============================================================================

func TestHandleDeleteRoom_Success(t *testing.T) {
	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleDeleteRoom(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleDeleteRoom_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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

	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleListRooms(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleListRooms_Empty(t *testing.T) {
	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendMessageResult:  "$evt1:test",
	}
	h := testRoomHandler(mock)

	senderID := uuid.New()
	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(senderID),
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedSendMessageContent != "Hello!" {
		t.Errorf("expected content 'Hello!', got %q", mock.capturedSendMessageContent)
	}
	if mock.capturedSendMessageSender.ID != senderID {
		t.Errorf("expected sender actor ID %s, got %s", senderID, mock.capturedSendMessageSender.ID)
	}
	if mock.capturedSendMessageRoomID != "!room1:test" {
		t.Errorf("expected room ID '!room1:test', got %q", mock.capturedSendMessageRoomID)
	}
}

// F5: a partial send (some events landed, a later one failed) surfaces the
// delivered message id on the response alongside a partial-failure error, so the
// server can record what landed rather than re-sending everything.
func TestHandleSendMessage_PartialFailure_CarriesDeliveredMessageID(t *testing.T) {
	mock := &testMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendMessageErr: &domain.PartialSendError{
			PrimaryEventID: "$text1:test",
			Err:            errors.New("attachment 2 of 2 failed: synapse upload failed"),
		},
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Content:       "hello",
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	require.NoError(t, err)

	resp, ok := result.(dto.SendMessageResponse)
	require.True(t, ok, "expected SendMessageResponse, got %T", result)
	assert.Equal(t, "$text1:test", string(resp.MessageID), "delivered event id must be carried")
	require.NotNil(t, resp.Error, "partial send is not a success")
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error.Message, "partial send")
}

func TestHandleSendMessage_WithThread(t *testing.T) {
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedSendReplyContent != "Reply!" {
		t.Errorf("expected reply content 'Reply!', got %q", mock.capturedSendReplyContent)
	}
	if mock.capturedSendReplyThread != "$parent:test" {
		t.Errorf("expected thread ID '$parent:test', got %q", mock.capturedSendReplyThread)
	}
}

func TestHandleSendMessage_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleSendMessage(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleSendMessage_MissingSenderID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	h := testRoomHandler(&testMockMatrixPort{})

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

func TestHandleSendMessage_WithAttachments(t *testing.T) {
	mock := &testMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendMessageResult:  "$evt1:test",
	}
	h := testRoomHandler(mock)

	w, hgt := 800, 600
	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Content:       "", // attachment-only: empty content must be accepted
		Attachments: []dto.AttachmentRef{{
			DocumentID:  "doc-1",
			DisplayName: "pic.png",
			MimeType:    "image/png",
			Size:        123,
			Width:       &w,
			Height:      &hgt,
		}},
	})

	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)

	if len(mock.capturedSendMessageAttachments) != 1 {
		t.Fatalf("expected 1 attachment forwarded, got %d", len(mock.capturedSendMessageAttachments))
	}
	att := mock.capturedSendMessageAttachments[0]
	if att.DocumentID != "doc-1" || att.DisplayName != "pic.png" || att.MimeType != "image/png" || att.Size != 123 {
		t.Errorf("attachment not forwarded correctly: %+v", att)
	}
	if att.Width == nil || *att.Width != 800 || att.Height == nil || *att.Height != 600 {
		t.Errorf("attachment dims not forwarded: w=%v h=%v", att.Width, att.Height)
	}
}

// The idempotency key on the request is forwarded verbatim to the matrix port
// (for both plain sends and threaded replies) so the adapter can de-duplicate
// retries at the homeserver.
func TestHandleSendMessage_ForwardsIdempotencyKey(t *testing.T) {
	mock := &testMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendMessageResult:  "$evt1:test",
		sendReplyResult:    "$reply1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID:  dto.AlkemioRoomID(uuid.New()),
		SenderActorID:  dto.AlkemioActorID(uuid.New()),
		Content:        "Hello!",
		IdempotencyKey: "key-123",
	})
	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSuccess(t, result)
	if mock.capturedSendMessageIdempotencyKey != "key-123" {
		t.Errorf("expected idempotency key forwarded to SendMessage, got %q", mock.capturedSendMessageIdempotencyKey)
	}

	parentID := dto.MessageID("$parent:test")
	replyPayload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID:   dto.AlkemioRoomID(uuid.New()),
		SenderActorID:   dto.AlkemioActorID(uuid.New()),
		Content:         "Reply!",
		ParentMessageID: &parentID,
		IdempotencyKey:  "key-456",
	})
	if _, err := h.HandleSendMessage(context.Background(), replyPayload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.capturedSendReplyIdempotencyKey != "key-456" {
		t.Errorf("expected idempotency key forwarded to SendReply, got %q", mock.capturedSendReplyIdempotencyKey)
	}
}

// Defense-in-depth: more than the allowed number of attachments is rejected
// before any Matrix event is emitted.
func TestHandleSendMessage_TooManyAttachments(t *testing.T) {
	mock := &testMockMatrixPort{resolveAliasResult: "!room1:test", sendMessageResult: "$evt1:test"}
	h := testRoomHandler(mock)

	atts := make([]dto.AttachmentRef, maxAttachmentsPerMessage+1)
	for i := range atts {
		atts[i] = dto.AttachmentRef{DocumentID: "doc", DisplayName: "x", MimeType: "image/png"}
	}
	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Attachments:   atts,
	})
	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
	if mock.capturedSendMessageRoomID != "" {
		t.Error("SendMessage should not have been called when attachment count is rejected")
	}
}

// An attachment with an empty document_id is rejected early with a clear error.
func TestHandleSendMessage_EmptyAttachmentDocumentID(t *testing.T) {
	mock := &testMockMatrixPort{resolveAliasResult: "!room1:test", sendMessageResult: "$evt1:test"}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.SendMessageRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Attachments: []dto.AttachmentRef{
			{DocumentID: "", DisplayName: "x", MimeType: "image/png"},
		},
	})
	result, err := h.HandleSendMessage(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
	if mock.capturedSendMessageRoomID != "" {
		t.Error("SendMessage should not have been called when a document_id is empty")
	}
}

func TestHandleSendMessage_RoomNotFound(t *testing.T) {
	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetMessage(context.Background(), []byte(`xxx`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetMessage_MissingMessageID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedRedactEventID != "$msg1:test" {
		t.Errorf("expected redacted event ID '$msg1:test', got %q", mock.capturedRedactEventID)
	}
	if mock.capturedRedactEventRoomID != "!room1:test" {
		t.Errorf("expected redact room ID '!room1:test', got %q", mock.capturedRedactEventRoomID)
	}
}

func TestHandleDeleteMessage_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleDeleteMessage(context.Background(), []byte(`[`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleDeleteMessage_MissingFields(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
		resolveAliasResult: "!room1:test",
		sendReactionResult: "$reaction1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.AddReactionRequest{
		AlkemioRoomID: dto.AlkemioRoomID(uuid.New()),
		MessageID:     "$msg1:test",
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Emoji:         "\xf0\x9f\x91\x8d",
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedSendReactionEmoji != "\xf0\x9f\x91\x8d" {
		t.Errorf("expected emoji thumbs up, got %q", mock.capturedSendReactionEmoji)
	}
	if mock.capturedSendReactionEventID != "$msg1:test" {
		t.Errorf("expected event ID '$msg1:test', got %q", mock.capturedSendReactionEventID)
	}
	if mock.capturedSendReactionRoomID != "!room1:test" {
		t.Errorf("expected room ID '!room1:test', got %q", mock.capturedSendReactionRoomID)
	}
}

func TestHandleAddReaction_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleAddReaction(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleAddReaction_MissingEmoji(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly forwarded the reaction ID for redaction
	if mock.capturedRedactEventID != "$reaction1:test" {
		t.Errorf("expected redacted event ID '$reaction1:test', got %q", mock.capturedRedactEventID)
	}
}

func TestHandleRemoveReaction_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleRemoveReaction(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleRemoveReaction_MissingReactionID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetReaction(context.Background(), []byte(`not-json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetReaction_MissingReactionID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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

	mock := &testMockMatrixPort{
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
	actorID := uuid.New()
	mock := &testMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.BatchAddMemberRequest{
		ActorID:        dto.AlkemioActorID(actorID),
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedInviteUserRoomID != "!room1:test" {
		t.Errorf("expected invite room ID '!room1:test', got %q", mock.capturedInviteUserRoomID)
	}
	if mock.capturedInviteUserInvitee.ID != actorID {
		t.Errorf("expected invitee actor ID %s, got %s", actorID, mock.capturedInviteUserInvitee.ID)
	}
}

func TestHandleBatchAddMember_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleBatchAddMember(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchAddMember_MissingActorID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	h := testRoomHandler(&testMockMatrixPort{})

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
	actorID := uuid.New()
	mock := &testMockMatrixPort{
		resolveAliasResult: "!room1:test",
	}
	h := testRoomHandler(mock)

	payload := mustMarshal(t, dto.BatchRemoveMemberRequest{
		ActorID:        dto.AlkemioActorID(actorID),
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedKickUserRoomID != "!room1:test" {
		t.Errorf("expected kick room ID '!room1:test', got %q", mock.capturedKickUserRoomID)
	}
	expectedUserID := id.NewUserID(actorID.String(), testDomain)
	if mock.capturedKickUserUserID != expectedUserID {
		t.Errorf("expected kick user ID %q, got %q", expectedUserID, mock.capturedKickUserUserID)
	}
}

func TestHandleBatchRemoveMember_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleBatchRemoveMember(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchRemoveMember_MissingActorID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	h := testRoomHandler(&testMockMatrixPort{})

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

	mock := &testMockMatrixPort{
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

	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetRoomMembers(context.Background(), []byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoomMembers_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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

	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetThreadMessages(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetThreadMessages_MissingThreadID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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

	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetLastMessage(context.Background(), []byte(`bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetLastMessage_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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

	mock := &testMockMatrixPort{
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
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleBatchGetLastMessages(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleBatchGetLastMessages_EmptyRoomIDs(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly parsed and forwarded DTO fields
	if mock.capturedSetCustomStateRoomID != "!room1:test" {
		t.Errorf("expected room ID '!room1:test', got %q", mock.capturedSetCustomStateRoomID)
	}
	if mock.capturedSetCustomState == nil {
		t.Fatal("expected custom state to be non-nil")
	}
	vis, ok := mock.capturedSetCustomState["io.alkemio.visibility"]
	if !ok {
		t.Fatal("expected io.alkemio.visibility in captured state")
	}
	if v, ok := vis["visible"].(bool); !ok || !v {
		t.Errorf("expected visible=true in captured state, got %v", vis["visible"])
	}
}

func TestHandleSetRoomState_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleSetRoomState(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleSetRoomState_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
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
	if v, ok := vis["visible"].(bool); !ok || !v {
		t.Errorf("expected visible=true, got %v", vis["visible"])
	}
}

func TestHandleGetRoomState_WithEventTypeFilter(t *testing.T) {
	mock := &testMockMatrixPort{
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

	// Verify the handler correctly forwarded the event type filter
	if len(mock.capturedGetCustomStateEventTypes) != 1 {
		t.Fatalf("expected 1 event type filter, got %d", len(mock.capturedGetCustomStateEventTypes))
	}
	if mock.capturedGetCustomStateEventTypes[0] != "io.alkemio.visibility" {
		t.Errorf("expected event type 'io.alkemio.visibility', got %q", mock.capturedGetCustomStateEventTypes[0])
	}
	if mock.capturedGetCustomStateRoomID != "!room1:test" {
		t.Errorf("expected room ID '!room1:test', got %q", mock.capturedGetCustomStateRoomID)
	}
}

func TestHandleGetRoomState_InvalidJSON(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

	result, err := h.HandleGetRoomState(context.Background(), []byte(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, result, dto.ErrCodeInvalidParam)
}

func TestHandleGetRoomState_MissingRoomID(t *testing.T) {
	h := testRoomHandler(&testMockMatrixPort{})

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
	h := testRoomHandler(&testMockMatrixPort{})

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
	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
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
	mock := &testMockMatrixPort{
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
