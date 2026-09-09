//nolint:revive // test file — unused test params are acceptable
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

// ============================================================================
// Mock adminAPI
// ============================================================================

type mockAdminAPI struct {
	getUserResult              *UserInfo
	getUserErr                 error
	listRoomsResult            []AdminRoom
	listRoomsErr               error
	getRoomMembersResult       []string
	getRoomMembersErr          error
	getRoomMemberIDsResult     []id.UserID
	getRoomMemberIDsErr        error
	getRoomStateResult         []json.RawMessage
	getRoomStateErr            error
	getStateEventContentResult map[string]interface{}
	getStateEventContentErr    error
	getStateEventContentCalled int
	getCustomStateResult       map[string]map[string]interface{}
	getCustomStateErr          error
	getRoomMessagesResult      *mautrix.RespMessages
	getRoomMessagesErr         error
	getEventResult             *event.Event
	getEventErr                error
	getRelationsResult         []*event.Event
	getRelationsErr            error
	joinRoomErr                error

	// Call tracking
	joinRoomCalls []joinRoomCall
}

type joinRoomCall struct {
	RoomID id.RoomID
	UserID id.UserID
}

func (m *mockAdminAPI) GetUser(_ context.Context, _ id.UserID) (*UserInfo, error) {
	return m.getUserResult, m.getUserErr
}

func (m *mockAdminAPI) ListRooms(_ context.Context, _ int) ([]AdminRoom, error) {
	return m.listRoomsResult, m.listRoomsErr
}

func (m *mockAdminAPI) GetRoomMembers(_ context.Context, _ id.RoomID) ([]string, error) {
	return m.getRoomMembersResult, m.getRoomMembersErr
}

func (m *mockAdminAPI) GetRoomMemberIDs(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return m.getRoomMemberIDsResult, m.getRoomMemberIDsErr
}

func (m *mockAdminAPI) GetRoomState(_ context.Context, _ id.RoomID, _ string) ([]json.RawMessage, error) {
	return m.getRoomStateResult, m.getRoomStateErr
}

func (m *mockAdminAPI) GetStateEventContent(_ context.Context, _ id.RoomID, _ string) (map[string]interface{}, error) {
	m.getStateEventContentCalled++
	return m.getStateEventContentResult, m.getStateEventContentErr
}

func (m *mockAdminAPI) GetCustomState(_ context.Context, _ id.RoomID, _ []string) (map[string]map[string]interface{}, error) {
	return m.getCustomStateResult, m.getCustomStateErr
}

func (m *mockAdminAPI) GetRoomMessages(_ context.Context, _ id.RoomID, _, _ string, _ int) (*mautrix.RespMessages, error) {
	return m.getRoomMessagesResult, m.getRoomMessagesErr
}

func (m *mockAdminAPI) GetEvent(_ context.Context, _ id.RoomID, _ id.EventID) (*event.Event, error) {
	return m.getEventResult, m.getEventErr
}

func (m *mockAdminAPI) GetRelations(_ context.Context, _ id.RoomID, _ id.EventID, _ event.RelationType, _ event.Type) ([]*event.Event, error) {
	return m.getRelationsResult, m.getRelationsErr
}

func (m *mockAdminAPI) JoinRoom(_ context.Context, roomID id.RoomID, userID id.UserID) error {
	m.joinRoomCalls = append(m.joinRoomCalls, joinRoomCall{RoomID: roomID, UserID: userID})
	return m.joinRoomErr
}

// ============================================================================
// Mock Logger
// ============================================================================

// adapterMockLogger implements ports.Logger for admin API tests.
type adapterMockLogger struct{}

func (l *adapterMockLogger) Debug(_ string, _ ...interface{}) {}
func (l *adapterMockLogger) Info(_ string, _ ...interface{})  {}
func (l *adapterMockLogger) Warn(_ string, _ ...interface{})  {}
func (l *adapterMockLogger) Error(_ string, _ ...interface{}) {}
func (l *adapterMockLogger) With(_ ...interface{}) ports.Logger {
	return l
}

// ============================================================================
// Test helper
// ============================================================================

// newAdminTestAdapter creates a MautrixAdapter wired with a mock adminAPI.
func newAdminTestAdapter(admin adminAPI) *MautrixAdapter {
	return &MautrixAdapter{
		admin:    admin,
		idMapper: domain.NewIDMapper("test.local"),
		logger:   &adapterMockLogger{},
	}
}

// ============================================================================
// isBotAdmin (admin.GetUser)
// ============================================================================

func TestAdminAPI_IsBotAdmin_True(t *testing.T) {
	mock := &mockAdminAPI{
		getUserResult: &UserInfo{Admin: true},
	}
	as := &mockAppserviceAPI{
		botMXID:          "@bot:test.local",
		homeserverDomain: "test.local",
	}
	a := &MautrixAdapter{
		admin:    mock,
		as:       as,
		idMapper: domain.NewIDMapper("test.local"),
		logger:   &adapterMockLogger{},
	}
	if !a.isBotAdmin(context.Background()) {
		t.Error("expected isBotAdmin=true")
	}
}

func TestAdminAPI_IsBotAdmin_False(t *testing.T) {
	mock := &mockAdminAPI{
		getUserResult: &UserInfo{Admin: false},
	}
	as := &mockAppserviceAPI{
		botMXID:          "@bot:test.local",
		homeserverDomain: "test.local",
	}
	a := &MautrixAdapter{
		admin:    mock,
		as:       as,
		idMapper: domain.NewIDMapper("test.local"),
		logger:   &adapterMockLogger{},
	}
	if a.isBotAdmin(context.Background()) {
		t.Error("expected isBotAdmin=false")
	}
}

func TestAdminAPI_IsBotAdmin_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getUserErr: errors.New("connection refused"),
	}
	as := &mockAppserviceAPI{
		botMXID:          "@bot:test.local",
		homeserverDomain: "test.local",
	}
	a := &MautrixAdapter{
		admin:    mock,
		as:       as,
		idMapper: domain.NewIDMapper("test.local"),
		logger:   &adapterMockLogger{},
	}
	if a.isBotAdmin(context.Background()) {
		t.Error("expected isBotAdmin=false on error")
	}
}

// ============================================================================
// isSpaceRoom (admin.GetStateEventContent)
// ============================================================================

func TestAdminAPI_IsSpaceRoom_IsSpace(t *testing.T) {
	mock := &mockAdminAPI{
		getStateEventContentResult: map[string]interface{}{"type": "m.space"},
	}
	a := newAdminTestAdapter(mock)
	isSpace, err := a.isSpaceRoom(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isSpace {
		t.Error("expected isSpace=true")
	}
}

func TestAdminAPI_IsSpaceRoom_IsRoom(t *testing.T) {
	mock := &mockAdminAPI{
		getStateEventContentResult: map[string]interface{}{},
	}
	a := newAdminTestAdapter(mock)
	isSpace, err := a.isSpaceRoom(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isSpace {
		t.Error("expected isSpace=false for room without type")
	}
}

func TestAdminAPI_IsSpaceRoom_NilContent(t *testing.T) {
	mock := &mockAdminAPI{
		getStateEventContentResult: nil,
	}
	a := newAdminTestAdapter(mock)
	isSpace, err := a.isSpaceRoom(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isSpace {
		t.Error("expected isSpace=false for nil content")
	}
}

func TestAdminAPI_IsSpaceRoom_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getStateEventContentErr: errors.New("not found"),
	}
	a := newAdminTestAdapter(mock)
	isSpace, err := a.isSpaceRoom(context.Background(), "!room:test.local")
	if err == nil {
		t.Fatal("expected error")
	}
	if isSpace {
		t.Error("expected isSpace=false on error")
	}
}

func TestAdminAPI_IsSpaceRoom_NonSpaceType(t *testing.T) {
	mock := &mockAdminAPI{
		getStateEventContentResult: map[string]interface{}{"type": "m.custom_room"},
	}
	a := newAdminTestAdapter(mock)
	isSpace, err := a.isSpaceRoom(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isSpace {
		t.Error("expected isSpace=false for non m.space type")
	}
}

// ============================================================================
// GetRoomMembers (admin.GetRoomMemberIDs)
// ============================================================================

func TestAdminAPI_GetRoomMembers_Success(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMemberIDsResult: []id.UserID{
			"@user1:test.local",
			"@user2:test.local",
		},
	}
	a := newAdminTestAdapter(mock)
	members, err := a.GetRoomMembers(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
	if members[0] != "@user1:test.local" {
		t.Errorf("expected @user1:test.local, got %s", members[0])
	}
}

func TestAdminAPI_GetRoomMembers_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMemberIDsErr: errors.New("room not found"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetRoomMembers(context.Background(), "!room:test.local")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminAPI_GetRoomMembers_Empty(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMemberIDsResult: []id.UserID{},
	}
	a := newAdminTestAdapter(mock)
	members, err := a.GetRoomMembers(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

// ============================================================================
// GetRoomMessages (admin.GetRoomMessages)
// ============================================================================

func TestAdminAPI_GetRoomMessages_Success(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					Type:      event.EventMessage,
					ID:        "$msg1",
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    "Hello world",
						},
					},
				},
				{
					Type:      event.EventMessage,
					ID:        "$msg2",
					Sender:    "@user2:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    "Hi there",
						},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetRoomMessages(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", msgs[0].Content)
	}
	if msgs[1].Content != "Hi there" {
		t.Errorf("expected 'Hi there', got %q", msgs[1].Content)
	}
}

func TestAdminAPI_GetRoomMessages_WithReactions(t *testing.T) {
	msgID := id.EventID("$msg1")
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					Type:      event.EventMessage,
					ID:        msgID,
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    "React to me",
						},
					},
				},
				{
					Type:      event.EventReaction,
					ID:        "$reaction1",
					Sender:    "@user2:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.ReactionEventContent{
							RelatesTo: event.RelatesTo{
								EventID: msgID,
								Key:     "\U0001F44D",
								Type:    event.RelAnnotation,
							},
						},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetRoomMessages(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if len(msgs[0].Reactions) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(msgs[0].Reactions))
	}
	if msgs[0].Reactions[0].Emoji != "\U0001F44D" {
		t.Errorf("expected thumbs up emoji, got %q", msgs[0].Reactions[0].Emoji)
	}
}

func TestAdminAPI_GetRoomMessages_EmptyRoom(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{},
		},
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetRoomMessages(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestAdminAPI_GetRoomMessages_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesErr: errors.New("forbidden"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetRoomMessages(context.Background(), "!room:test.local")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminAPI_GetRoomMessages_SkipsNonMessageEvents(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					Type:      event.EventMessage,
					ID:        "$msg1",
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    "Real message",
						},
					},
				},
				{
					Type:      event.StateRoomName,
					ID:        "$state1",
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.RoomNameEventContent{Name: "Room Name"},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetRoomMessages(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (state events skipped), got %d", len(msgs))
	}
}

// ============================================================================
// GetMessage (admin.GetEvent)
// ============================================================================

func TestAdminAPI_GetMessage_Success(t *testing.T) {
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        "$msg1",
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Hello",
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$msg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "Hello" {
		t.Errorf("expected 'Hello', got %q", msg.Content)
	}
	if msg.ID != "$msg1" {
		t.Errorf("expected ID '$msg1', got %q", msg.ID)
	}
}

func TestAdminAPI_GetMessage_WithThreadID(t *testing.T) {
	threadRoot := id.EventID("$thread_root")
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        "$reply1",
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Thread reply",
					RelatesTo: &event.RelatesTo{
						Type:    event.RelThread,
						EventID: threadRoot,
						InReplyTo: &event.InReplyTo{
							EventID: threadRoot,
						},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$reply1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ThreadID != threadRoot.String() {
		t.Errorf("expected thread ID %q, got %q", threadRoot, msg.ThreadID)
	}
}

func TestAdminAPI_GetMessage_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getEventErr: errors.New("event not found"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetMessage(context.Background(), "!room:test.local", "$nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminAPI_GetMessage_RawFallback(t *testing.T) {
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        "$msg_raw",
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Raw: map[string]interface{}{
					"body":    "Raw body message",
					"msgtype": "m.text",
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$msg_raw")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "Raw body message" {
		t.Errorf("expected 'Raw body message', got %q", msg.Content)
	}
}

// ============================================================================
// GetThreadMessages (admin.GetEvent + admin.GetRelations)
// ============================================================================

func TestAdminAPI_GetThreadMessages_RootAndReplies(t *testing.T) {
	threadRootID := id.EventID("$root1")
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        threadRootID,
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Root message",
				},
			},
		},
		getRelationsResult: []*event.Event{
			{
				Type:      event.EventMessage,
				ID:        "$reply1",
				Sender:    "@user2:test.local",
				Timestamp: time.Now().UnixMilli(),
				Content: event.Content{
					Parsed: &event.MessageEventContent{
						MsgType: event.MsgText,
						Body:    "Reply 1",
						RelatesTo: &event.RelatesTo{
							Type:    event.RelThread,
							EventID: threadRootID,
						},
					},
				},
			},
			{
				Type:      event.EventMessage,
				ID:        "$reply2",
				Sender:    "@user3:test.local",
				Timestamp: time.Now().UnixMilli(),
				Content: event.Content{
					Parsed: &event.MessageEventContent{
						MsgType: event.MsgText,
						Body:    "Reply 2",
						RelatesTo: &event.RelatesTo{
							Type:    event.RelThread,
							EventID: threadRootID,
						},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetThreadMessages(context.Background(), "!room:test.local", threadRootID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect: reply1, reply2, root (root appended last)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (root + 2 replies), got %d", len(msgs))
	}
	if msgs[0].Content != "Reply 1" {
		t.Errorf("expected first message 'Reply 1', got %q", msgs[0].Content)
	}
	if msgs[1].Content != "Reply 2" {
		t.Errorf("expected second message 'Reply 2', got %q", msgs[1].Content)
	}
	if msgs[2].Content != "Root message" {
		t.Errorf("expected last message 'Root message', got %q", msgs[2].Content)
	}
}

func TestAdminAPI_GetThreadMessages_RootOnly(t *testing.T) {
	threadRootID := id.EventID("$root_only")
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        threadRootID,
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Lone message",
				},
			},
		},
		getRelationsErr: errors.New("no relations"),
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetThreadMessages(context.Background(), "!room:test.local", threadRootID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (root only), got %d", len(msgs))
	}
	if msgs[0].Content != "Lone message" {
		t.Errorf("expected 'Lone message', got %q", msgs[0].Content)
	}
}

func TestAdminAPI_GetThreadMessages_RootFetchError(t *testing.T) {
	mock := &mockAdminAPI{
		getEventErr: errors.New("event not found"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetThreadMessages(context.Background(), "!room:test.local", "$nonexistent")
	if err == nil {
		t.Fatal("expected error when root event not found")
	}
}

func TestAdminAPI_GetThreadMessages_RepliesSetThreadID(t *testing.T) {
	threadRootID := id.EventID("$root_thread")
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        threadRootID,
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Root",
				},
			},
		},
		getRelationsResult: []*event.Event{
			{
				Type:      event.EventMessage,
				ID:        "$reply_in_thread",
				Sender:    "@user2:test.local",
				Timestamp: time.Now().UnixMilli(),
				Content: event.Content{
					Parsed: &event.MessageEventContent{
						MsgType: event.MsgText,
						Body:    "Reply",
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetThreadMessages(context.Background(), "!room:test.local", threadRootID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) < 1 {
		t.Fatal("expected at least 1 message")
	}
	// First message is the reply — should have ThreadID set to root
	if msgs[0].ThreadID != threadRootID.String() {
		t.Errorf("expected reply thread ID %q, got %q", threadRootID, msgs[0].ThreadID)
	}
}

func TestAdminAPI_GetThreadMessages_EmptyRelations(t *testing.T) {
	threadRootID := id.EventID("$root_empty_rels")
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        threadRootID,
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Root with empty relations",
				},
			},
		},
		getRelationsResult: []*event.Event{},
	}
	a := newAdminTestAdapter(mock)
	msgs, err := a.GetThreadMessages(context.Background(), "!room:test.local", threadRootID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only root message since relations list is empty
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (root only), got %d", len(msgs))
	}
	if msgs[0].Content != "Root with empty relations" {
		t.Errorf("expected 'Root with empty relations', got %q", msgs[0].Content)
	}
}

// ============================================================================
// GetCustomState (admin.GetCustomState)
// ============================================================================

func TestAdminAPI_GetCustomState_Success(t *testing.T) {
	expected := map[string]map[string]interface{}{
		"io.alkemio.room.type": {
			"type": "community",
		},
		"io.alkemio.room.visibility": {
			"visible": true,
		},
	}
	mock := &mockAdminAPI{
		getCustomStateResult: expected,
	}
	a := newAdminTestAdapter(mock)
	result, err := a.GetCustomState(context.Background(), "!room:test.local", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 state events, got %d", len(result))
	}
	if result["io.alkemio.room.type"]["type"] != "community" {
		t.Errorf("expected type 'community', got %v", result["io.alkemio.room.type"]["type"])
	}
}

func TestAdminAPI_GetCustomState_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getCustomStateErr: errors.New("forbidden"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetCustomState(context.Background(), "!room:test.local", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminAPI_GetCustomState_Empty(t *testing.T) {
	mock := &mockAdminAPI{
		getCustomStateResult: map[string]map[string]interface{}{},
	}
	a := newAdminTestAdapter(mock)
	result, err := a.GetCustomState(context.Background(), "!room:test.local", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 state events, got %d", len(result))
	}
}

func TestAdminAPI_GetCustomState_WithSpecificTypes(t *testing.T) {
	expected := map[string]map[string]interface{}{
		"io.alkemio.room.type": {
			"type": "direct",
		},
	}
	mock := &mockAdminAPI{
		getCustomStateResult: expected,
	}
	a := newAdminTestAdapter(mock)
	result, err := a.GetCustomState(context.Background(), "!room:test.local", []string{"io.alkemio.room.type"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 state event, got %d", len(result))
	}
}

// ============================================================================
// GetLastMessage (admin.GetRoomMessages progressive fetch)
// ============================================================================

func TestAdminAPI_GetLastMessage_Success(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					Type:      event.EventMessage,
					ID:        "$latest",
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    "Latest message",
						},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msg, err := a.GetLastMessage(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Content != "Latest message" {
		t.Errorf("expected 'Latest message', got %q", msg.Content)
	}
}

func TestAdminAPI_GetLastMessage_NoMessages(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{},
		},
	}
	a := newAdminTestAdapter(mock)
	msg, err := a.GetLastMessage(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Errorf("expected nil message for empty room, got %+v", msg)
	}
}

func TestAdminAPI_GetLastMessage_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesErr: errors.New("server error"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetLastMessage(context.Background(), "!room:test.local")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminAPI_GetLastMessage_SkipsNonMessages(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					Type:      event.StateRoomName,
					ID:        "$state",
					Sender:    "@bot:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.RoomNameEventContent{Name: "Name"},
					},
				},
				{
					Type:      event.EventMessage,
					ID:        "$actual_msg",
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    "Found me",
						},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msg, err := a.GetLastMessage(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Content != "Found me" {
		t.Errorf("expected 'Found me', got %q", msg.Content)
	}
}

func TestAdminAPI_GetLastMessage_WithReaction(t *testing.T) {
	msgID := id.EventID("$msg_with_reaction")
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					Type:      event.EventReaction,
					ID:        "$reaction_on_latest",
					Sender:    "@user2:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.ReactionEventContent{
							RelatesTo: event.RelatesTo{
								EventID: msgID,
								Key:     "\U0001F389",
								Type:    event.RelAnnotation,
							},
						},
					},
				},
				{
					Type:      event.EventMessage,
					ID:        msgID,
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
					Content: event.Content{
						Parsed: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    "Celebrated",
						},
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	msg, err := a.GetLastMessage(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if len(msg.Reactions) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(msg.Reactions))
	}
	if msg.Reactions[0].Emoji != "\U0001F389" {
		t.Errorf("expected party emoji, got %q", msg.Reactions[0].Emoji)
	}
}

// ============================================================================
// GetReaction (admin.GetEvent)
// ============================================================================

func TestAdminAPI_GetReaction_Success(t *testing.T) {
	targetMsgID := id.EventID("$target_msg")
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventReaction,
			ID:        "$reaction1",
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: targetMsgID,
						Key:     "\u2764\uFE0F",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	reaction, err := a.GetReaction(context.Background(), "!room:test.local", "$reaction1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reaction.Emoji != "\u2764\uFE0F" {
		t.Errorf("expected heart emoji, got %q", reaction.Emoji)
	}
	if reaction.MessageID != targetMsgID {
		t.Errorf("expected message ID %q, got %q", targetMsgID, reaction.MessageID)
	}
}

func TestAdminAPI_GetReaction_NotAReaction(t *testing.T) {
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventMessage,
			ID:        "$msg1",
			Sender:    "@user1:test.local",
			Timestamp: time.Now().UnixMilli(),
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Not a reaction",
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetReaction(context.Background(), "!room:test.local", "$msg1")
	if err == nil {
		t.Fatal("expected error for non-reaction event")
	}
}

func TestAdminAPI_GetReaction_FetchError(t *testing.T) {
	mock := &mockAdminAPI{
		getEventErr: errors.New("not found"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.GetReaction(context.Background(), "!room:test.local", "$missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminAPI_GetReaction_SenderAndTimestamp(t *testing.T) {
	ts := int64(1700000000000)
	mock := &mockAdminAPI{
		getEventResult: &event.Event{
			Type:      event.EventReaction,
			ID:        "$reaction2",
			Sender:    "@alice:test.local",
			Timestamp: ts,
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: "$some_msg",
						Key:     "\U0001F525",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	reaction, err := a.GetReaction(context.Background(), "!room:test.local", "$reaction2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reaction.SenderMatrixID != "@alice:test.local" {
		t.Errorf("expected sender '@alice:test.local', got %q", reaction.SenderMatrixID)
	}
	if reaction.Timestamp != time.UnixMilli(ts) {
		t.Errorf("expected timestamp %v, got %v", time.UnixMilli(ts), reaction.Timestamp)
	}
}

// ============================================================================
// getLatestEventID (admin.GetRoomMessages)
// ============================================================================

func TestAdminAPI_GetLatestEventID_Success(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{
					ID:        "$latest_event",
					Type:      event.EventMessage,
					Sender:    "@user1:test.local",
					Timestamp: time.Now().UnixMilli(),
				},
			},
		},
	}
	a := newAdminTestAdapter(mock)
	eventID, err := a.getLatestEventID(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventID != "$latest_event" {
		t.Errorf("expected '$latest_event', got %q", eventID)
	}
}

func TestAdminAPI_GetLatestEventID_EmptyRoom(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{},
		},
	}
	a := newAdminTestAdapter(mock)
	eventID, err := a.getLatestEventID(context.Background(), "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventID != "" {
		t.Errorf("expected empty event ID for empty room, got %q", eventID)
	}
}

func TestAdminAPI_GetLatestEventID_Error(t *testing.T) {
	mock := &mockAdminAPI{
		getRoomMessagesErr: errors.New("admin API error"),
	}
	a := newAdminTestAdapter(mock)
	_, err := a.getLatestEventID(context.Background(), "!room:test.local")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ============================================================================
// findReactionByEmojiAndSender (pure method)
// ============================================================================

func TestAdminAPI_FindReaction_Found(t *testing.T) {
	a := newAdminTestAdapter(nil)
	events := []*event.Event{
		{
			Type:   event.EventReaction,
			ID:     "$reaction_match",
			Sender: "@user1:test.local",
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: "$msg1",
						Key:     "\U0001F44D",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
	}
	reactionID, err := a.findReactionByEmojiAndSender(events, "@user1:test.local", "\U0001F44D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reactionID != "$reaction_match" {
		t.Errorf("expected '$reaction_match', got %q", reactionID)
	}
}

func TestAdminAPI_FindReaction_NotFound(t *testing.T) {
	a := newAdminTestAdapter(nil)
	events := []*event.Event{
		{
			Type:   event.EventReaction,
			ID:     "$other_reaction",
			Sender: "@user2:test.local",
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: "$msg1",
						Key:     "\U0001F44E",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
	}
	_, err := a.findReactionByEmojiAndSender(events, "@user1:test.local", "\U0001F44D")
	if err == nil {
		t.Fatal("expected error when reaction not found")
	}
}

func TestAdminAPI_FindReaction_WrongSender(t *testing.T) {
	a := newAdminTestAdapter(nil)
	events := []*event.Event{
		{
			Type:   event.EventReaction,
			ID:     "$reaction",
			Sender: "@user2:test.local",
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: "$msg1",
						Key:     "\U0001F44D",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
	}
	_, err := a.findReactionByEmojiAndSender(events, "@user1:test.local", "\U0001F44D")
	if err == nil {
		t.Fatal("expected error when sender doesn't match")
	}
}

func TestAdminAPI_FindReaction_EmptyEvents(t *testing.T) {
	a := newAdminTestAdapter(nil)
	_, err := a.findReactionByEmojiAndSender([]*event.Event{}, "@user1:test.local", "\U0001F44D")
	if err == nil {
		t.Fatal("expected error for empty events list")
	}
}

func TestAdminAPI_FindReaction_WrongEmoji(t *testing.T) {
	a := newAdminTestAdapter(nil)
	events := []*event.Event{
		{
			Type:   event.EventReaction,
			ID:     "$reaction",
			Sender: "@user1:test.local",
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: "$msg1",
						Key:     "\U0001F44E",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
	}
	_, err := a.findReactionByEmojiAndSender(events, "@user1:test.local", "\U0001F44D")
	if err == nil {
		t.Fatal("expected error when emoji doesn't match")
	}
}

// ============================================================================
// getIntentForRoom admin-join fallback (exercises admin.JoinRoom)
// ============================================================================

func TestAdminAPI_GetIntentForRoom_AdminJoinFallback(t *testing.T) {
	// Scenario: bot is NOT a member and no ghost users exist.
	// getIntentForRoom should admin-join the bot as a last resort.
	botIntent := &mockIntentAPI{}
	mock := &mockAdminAPI{
		// GetRoomMembers returns members without the bot → bot not in room.
		getRoomMembersResult: []string{"@other:test.local"},
	}
	as := &mockAppserviceAPI{
		botIntent:        botIntent,
		intents:          map[id.UserID]intentAPI{},
		botMXID:          "@bot:test.local",
		homeserverDomain: "test.local",
	}
	a := &MautrixAdapter{
		admin:    mock,
		as:       as,
		idMapper: domain.NewIDMapper("test.local"),
		logger:   &adapterMockLogger{},
	}
	_ = a.getIntentForRoom(context.Background(), "!room:test.local")
	if len(mock.joinRoomCalls) != 1 {
		t.Fatalf("expected 1 admin JoinRoom call, got %d", len(mock.joinRoomCalls))
	}
	if mock.joinRoomCalls[0].RoomID != "!room:test.local" {
		t.Errorf("expected room '!room:test.local', got %q", mock.joinRoomCalls[0].RoomID)
	}
	if mock.joinRoomCalls[0].UserID != "@bot:test.local" {
		t.Errorf("expected user '@bot:test.local', got %q", mock.joinRoomCalls[0].UserID)
	}
}

func TestAdminAPI_GetIntentForRoom_AdminJoinError(t *testing.T) {
	// Scenario: admin-join fails — getIntentForRoom should still return bot intent
	// without panicking.
	botIntent := &mockIntentAPI{}
	mock := &mockAdminAPI{
		getRoomMembersResult: []string{},
		joinRoomErr:          errors.New("forbidden"),
	}
	as := &mockAppserviceAPI{
		botIntent:        botIntent,
		intents:          map[id.UserID]intentAPI{},
		botMXID:          "@bot:test.local",
		homeserverDomain: "test.local",
	}
	a := &MautrixAdapter{
		admin:    mock,
		as:       as,
		idMapper: domain.NewIDMapper("test.local"),
		logger:   &adapterMockLogger{},
	}
	intent := a.getIntentForRoom(context.Background(), "!room:test.local")
	if intent == nil {
		t.Fatal("expected non-nil intent even when admin join fails")
	}
}

// ============================================================================
// buildLastMessageWithReactions (internal helper)
// ============================================================================

func TestAdminAPI_BuildLastMessage_WithReactions(t *testing.T) {
	msgID := id.EventID("$msg_build")
	events := []*event.Event{
		{
			Type:      event.EventReaction,
			ID:        "$rxn_build",
			Sender:    "@reactor:test.local",
			Timestamp: 1700000000000,
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: msgID,
						Key:     "\U0001F525",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
		{
			Type:      event.EventMessage,
			ID:        msgID,
			Sender:    "@author:test.local",
			Timestamp: 1700000000000,
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "Build test",
				},
			},
		},
	}
	a := newAdminTestAdapter(nil)
	msg, err := a.buildLastMessageWithReactions(events, "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Content != "Build test" {
		t.Errorf("expected 'Build test', got %q", msg.Content)
	}
	if len(msg.Reactions) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(msg.Reactions))
	}
}

func TestAdminAPI_BuildLastMessage_NoMessages(t *testing.T) {
	events := []*event.Event{
		{
			Type:      event.StateRoomName,
			ID:        "$state_only",
			Sender:    "@bot:test.local",
			Timestamp: 1700000000000,
			Content: event.Content{
				Parsed: &event.RoomNameEventContent{Name: "Test"},
			},
		},
	}
	a := newAdminTestAdapter(nil)
	msg, err := a.buildLastMessageWithReactions(events, "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Error("expected nil when no message events in list")
	}
}

func TestAdminAPI_BuildLastMessage_ReactionForDifferentMessage(t *testing.T) {
	msgID := id.EventID("$msg_target")
	events := []*event.Event{
		{
			Type:      event.EventReaction,
			ID:        "$rxn_other",
			Sender:    "@reactor:test.local",
			Timestamp: 1700000000000,
			Content: event.Content{
				Parsed: &event.ReactionEventContent{
					RelatesTo: event.RelatesTo{
						EventID: "$other_msg", // Different from the message
						Key:     "\U0001F44D",
						Type:    event.RelAnnotation,
					},
				},
			},
		},
		{
			Type:      event.EventMessage,
			ID:        msgID,
			Sender:    "@author:test.local",
			Timestamp: 1700000000000,
			Content: event.Content{
				Parsed: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    "My message",
				},
			},
		},
	}
	a := newAdminTestAdapter(nil)
	msg, err := a.buildLastMessageWithReactions(events, "!room:test.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	// Reaction targets a different message, so it should not be attached
	if len(msg.Reactions) != 0 {
		t.Errorf("expected 0 reactions (target mismatch), got %d", len(msg.Reactions))
	}
}
