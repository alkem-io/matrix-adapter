//nolint:revive // test file
package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// ============================================================================
// Shared Test Constants
// ============================================================================

const testDomain = "matrix.test.local"
const testOtherDomain = "matrix.example.com"

// ============================================================================
// Mock Logger
// ============================================================================

// testMockLogger implements ports.Logger for tests.
type testMockLogger struct{}

func (l *testMockLogger) Debug(_ string, _ ...interface{}) {}
func (l *testMockLogger) Info(_ string, _ ...interface{})  {}
func (l *testMockLogger) Warn(_ string, _ ...interface{})  {}
func (l *testMockLogger) Error(_ string, _ ...interface{}) {}
func (l *testMockLogger) With(_ ...interface{}) ports.Logger {
	return l
}

// ============================================================================
// Mock MatrixPort (union of all fields from both test files)
// ============================================================================

// testMockMatrixPort implements ports.MatrixPort with configurable return values.
type testMockMatrixPort struct {
	// Connection
	homeserverDomain string
	connectErr       error
	disconnectErr    error

	// User operations
	ensureUserResult id.UserID
	ensureUserErr    error
	setProfileErr    error

	// ResolveAlias — simple (single value) mode
	resolveAliasResult id.RoomID
	resolveAliasErr    error

	// ResolveAlias — map mode (takes precedence when non-nil)
	resolveAliasResults map[string]id.RoomID
	resolveAliasErrs    map[string]error

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

	// GetReactionEventID
	getReactionEventIDResult id.EventID
	getReactionEventIDErr    error

	// GetReaction
	getReactionResult *domain.Reaction
	getReactionErr    error

	// GetThreadMessages
	getThreadMessagesResult []domain.Message
	getThreadMessagesErr    error

	// InviteUser
	inviteUserErr error

	// FindExistingDirectRoom
	findExistingDirectRoomResult id.RoomID
	findExistingDirectRoomErr    error

	// SetRoomAlias
	setRoomAliasErr error

	// GetAllJoinedRooms
	getAllJoinedRoomsResult []id.RoomID
	getAllJoinedRoomsErr    error

	// GetUnreadCounts
	getUnreadCountsResult *domain.UnreadCountSummary
	getUnreadCountsErr    error

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
	getBatchUnreadCountsRes  map[id.RoomID]int
	getBatchUnreadCountsErrs map[id.RoomID]error

	// ====================================================================
	// Captured inputs — used by success-path tests to verify DTO parsing
	// ====================================================================

	// CreateRoomWithAlias
	capturedCreateRoomAlkemioID   uuid.UUID
	capturedCreateRoomType        string
	capturedCreateRoomName        string
	capturedCreateRoomJoinRule    string
	capturedCreateRoomCustomState map[string]map[string]interface{}
	capturedCreateRoomMembers     []domain.Actor

	// SendMessage
	capturedSendMessageRoomID  id.RoomID
	capturedSendMessageSender  domain.Actor
	capturedSendMessageContent string

	// SendReply
	capturedSendReplyRoomID  id.RoomID
	capturedSendReplyContent string
	capturedSendReplyThread  id.EventID

	// UpdateRoomState
	capturedUpdateRoomStateRoomID   id.RoomID
	capturedUpdateRoomStateName     *string
	capturedUpdateRoomStateTopic    *string
	capturedUpdateRoomStateJoinRule *string

	// SetCustomState
	capturedSetCustomStateRoomID id.RoomID
	capturedSetCustomState       map[string]map[string]interface{}

	// GetCustomState
	capturedGetCustomStateRoomID     id.RoomID
	capturedGetCustomStateEventTypes []string

	// KickUser
	capturedKickUserRoomID id.RoomID
	capturedKickUserUserID id.UserID

	// RedactEvent
	capturedRedactEventRoomID id.RoomID
	capturedRedactEventID     id.EventID

	// SendReaction
	capturedSendReactionRoomID  id.RoomID
	capturedSendReactionEventID id.EventID
	capturedSendReactionEmoji   string

	// InviteUser
	capturedInviteUserRoomID  id.RoomID
	capturedInviteUserInvitee domain.Actor

	// EnsureUser
	capturedEnsureUserActor domain.Actor

	// SetUserProfile
	capturedSetProfileActor domain.Actor

	// Space operations
	capturedCreateSpaceAlkemioID uuid.UUID
	capturedCreateSpaceName      string
	capturedCreateSpaceJoinRule  string
	capturedCreateSpaceMembers   []domain.Actor

	capturedUpdateSpaceStateRoomID   id.RoomID
	capturedUpdateSpaceStateName     *string
	capturedUpdateSpaceStateTopic    *string
	capturedUpdateSpaceStateJoinRule *string

	capturedAddSpaceChildSpaceID id.RoomID
	capturedAddSpaceChildChildID id.RoomID

	capturedSetSpaceParentChildID  id.RoomID
	capturedSetSpaceParentParentID id.RoomID

	capturedInviteToSpaceRoomID id.RoomID
	capturedInviteToSpaceActor  domain.Actor

	capturedKickFromSpaceRoomID id.RoomID
	capturedKickFromSpaceUserID id.UserID

	// Read receipt operations
	capturedSendReadReceiptActor    domain.Actor
	capturedSendReadReceiptRoomID   id.RoomID
	capturedSendReadReceiptEventID  id.EventID
	capturedSendReadReceiptThreadID *id.EventID

	capturedGetUnreadCountsActor     domain.Actor
	capturedGetUnreadCountsRoomID    id.RoomID
	capturedGetUnreadCountsThreadIDs []id.EventID

	capturedBatchUnreadCountsActor   domain.Actor
	capturedBatchUnreadCountsRoomIDs []id.RoomID
}

// Compile-time check that testMockMatrixPort implements ports.MatrixPort.
var _ ports.MatrixPort = (*testMockMatrixPort)(nil)

func (m *testMockMatrixPort) HomeserverDomain() string {
	return m.homeserverDomain
}

func (m *testMockMatrixPort) Connect(_ context.Context) error { return m.connectErr }
func (m *testMockMatrixPort) Disconnect() error               { return m.disconnectErr }

func (m *testMockMatrixPort) EnsureUser(_ context.Context, actor domain.Actor) (id.UserID, error) {
	m.capturedEnsureUserActor = actor
	return m.ensureUserResult, m.ensureUserErr
}

func (m *testMockMatrixPort) SetUserProfile(_ context.Context, actor domain.Actor) error {
	m.capturedSetProfileActor = actor
	return m.setProfileErr
}

func (m *testMockMatrixPort) CreateRoomWithAlias(_ context.Context, alkemioRoomID uuid.UUID, roomType, name, _, _, joinRule string, customState map[string]map[string]interface{}, members []domain.Actor) (id.RoomID, error) {
	m.capturedCreateRoomAlkemioID = alkemioRoomID
	m.capturedCreateRoomType = roomType
	m.capturedCreateRoomName = name
	m.capturedCreateRoomJoinRule = joinRule
	m.capturedCreateRoomCustomState = customState
	m.capturedCreateRoomMembers = members
	return m.createRoomResult, m.createRoomErr
}

func (m *testMockMatrixPort) InviteUser(_ context.Context, roomID id.RoomID, _ domain.Actor, invitee domain.Actor) error {
	m.capturedInviteUserRoomID = roomID
	m.capturedInviteUserInvitee = invitee
	return m.inviteUserErr
}

func (m *testMockMatrixPort) GetRoomDetails(_ context.Context, _ id.RoomID) (*domain.Room, error) {
	return m.getRoomDetailsResult, m.getRoomDetailsErr
}

func (m *testMockMatrixPort) GetRoomMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return m.getRoomMembersResult, m.getRoomMembersErr
}

func (m *testMockMatrixPort) UpdateRoomState(_ context.Context, roomID id.RoomID, _ domain.Actor, name, topic, _ *string, joinRule *string) error {
	m.capturedUpdateRoomStateRoomID = roomID
	m.capturedUpdateRoomStateName = name
	m.capturedUpdateRoomStateTopic = topic
	m.capturedUpdateRoomStateJoinRule = joinRule
	return m.updateRoomStateErr
}

func (m *testMockMatrixPort) SetRoomDirectoryVisibility(_ context.Context, _ id.RoomID, _ bool) error {
	return m.setRoomDirectoryVisibilityErr
}

func (m *testMockMatrixPort) SetCustomState(_ context.Context, roomID id.RoomID, state map[string]map[string]interface{}) error {
	m.capturedSetCustomStateRoomID = roomID
	m.capturedSetCustomState = state
	return m.setCustomStateErr
}

func (m *testMockMatrixPort) GetCustomState(_ context.Context, roomID id.RoomID, eventTypes []string) (map[string]map[string]interface{}, error) {
	m.capturedGetCustomStateRoomID = roomID
	m.capturedGetCustomStateEventTypes = eventTypes
	return m.getCustomStateResult, m.getCustomStateErr
}

func (m *testMockMatrixPort) ResolveAlias(_ context.Context, alias string) (id.RoomID, error) {
	// Map mode takes precedence when non-nil
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
	// Fall back to simple mode if maps are nil
	if m.resolveAliasResults == nil && m.resolveAliasErrs == nil {
		return m.resolveAliasResult, m.resolveAliasErr
	}
	// Maps are set but alias not found in either
	return "", domain.ErrRoomNotFound
}

func (m *testMockMatrixPort) DeleteAlias(_ context.Context, _ string) error {
	return m.deleteAliasErr
}

func (m *testMockMatrixPort) KickUser(_ context.Context, roomID id.RoomID, userID id.UserID, _ string) error {
	m.capturedKickUserRoomID = roomID
	m.capturedKickUserUserID = userID
	return m.kickUserErr
}

func (m *testMockMatrixPort) SendMessage(_ context.Context, roomID id.RoomID, sender domain.Actor, content string) (id.EventID, error) {
	m.capturedSendMessageRoomID = roomID
	m.capturedSendMessageSender = sender
	m.capturedSendMessageContent = content
	return m.sendMessageResult, m.sendMessageErr
}

func (m *testMockMatrixPort) SendReply(_ context.Context, roomID id.RoomID, _ domain.Actor, content string, threadID id.EventID) (id.EventID, error) {
	m.capturedSendReplyRoomID = roomID
	m.capturedSendReplyContent = content
	m.capturedSendReplyThread = threadID
	return m.sendReplyResult, m.sendReplyErr
}

func (m *testMockMatrixPort) RedactEvent(_ context.Context, roomID id.RoomID, _ domain.Actor, eventID id.EventID, _ string) error {
	m.capturedRedactEventRoomID = roomID
	m.capturedRedactEventID = eventID
	return m.redactEventErr
}

func (m *testMockMatrixPort) SendReaction(_ context.Context, roomID id.RoomID, _ domain.Actor, eventID id.EventID, emoji string) (id.EventID, error) {
	m.capturedSendReactionRoomID = roomID
	m.capturedSendReactionEventID = eventID
	m.capturedSendReactionEmoji = emoji
	return m.sendReactionResult, m.sendReactionErr
}

func (m *testMockMatrixPort) GetMessage(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Message, error) {
	return m.getMessageResult, m.getMessageErr
}

func (m *testMockMatrixPort) GetRoomMessages(_ context.Context, _ id.RoomID) ([]domain.Message, error) {
	return m.getRoomMessagesResult, m.getRoomMessagesErr
}

func (m *testMockMatrixPort) GetLastMessage(_ context.Context, _ id.RoomID) (*domain.Message, error) {
	return m.getLastMessageResult, m.getLastMessageErr
}

func (m *testMockMatrixPort) GetBatchLastMessages(_ context.Context, _ []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	return m.getBatchLastMessagesResults, m.getBatchLastMessagesErrors
}

func (m *testMockMatrixPort) GetReactionEventID(_ context.Context, _ id.RoomID, _ id.EventID, _ string, _ domain.Actor) (id.EventID, error) {
	return m.getReactionEventIDResult, m.getReactionEventIDErr
}

func (m *testMockMatrixPort) GetReaction(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Reaction, error) {
	return m.getReactionResult, m.getReactionErr
}

func (m *testMockMatrixPort) GetThreadMessages(_ context.Context, _ id.RoomID, _ id.EventID) ([]domain.Message, error) {
	return m.getThreadMessagesResult, m.getThreadMessagesErr
}

func (m *testMockMatrixPort) FindExistingDirectRoom(_ context.Context, _, _ domain.Actor) (id.RoomID, error) {
	return m.findExistingDirectRoomResult, m.findExistingDirectRoomErr
}

func (m *testMockMatrixPort) SetRoomAlias(_ context.Context, _ id.RoomID, _ string) error {
	return m.setRoomAliasErr
}

func (m *testMockMatrixPort) GetAllJoinedRooms(_ context.Context) ([]id.RoomID, error) {
	return m.getAllJoinedRoomsResult, m.getAllJoinedRoomsErr
}

func (m *testMockMatrixPort) CreateSpace(_ context.Context, alkemioContextID uuid.UUID, name, _, _ string, joinRule string, members []domain.Actor) (id.RoomID, error) {
	m.capturedCreateSpaceAlkemioID = alkemioContextID
	m.capturedCreateSpaceName = name
	m.capturedCreateSpaceJoinRule = joinRule
	m.capturedCreateSpaceMembers = members
	return m.createSpaceResult, m.createSpaceErr
}

func (m *testMockMatrixPort) GetSpaceDetails(_ context.Context, _ id.RoomID) (*domain.Space, error) {
	return m.getSpaceDetailsResult, m.getSpaceDetailsErr
}

func (m *testMockMatrixPort) GetSpaceMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return m.getSpaceMembersResult, m.getSpaceMembersErr
}

func (m *testMockMatrixPort) UpdateSpaceState(_ context.Context, roomID id.RoomID, name, topic, _ *string, joinRule *string) error {
	m.capturedUpdateSpaceStateRoomID = roomID
	m.capturedUpdateSpaceStateName = name
	m.capturedUpdateSpaceStateTopic = topic
	m.capturedUpdateSpaceStateJoinRule = joinRule
	return m.updateSpaceStateErr
}

func (m *testMockMatrixPort) GetSpaceChildren(_ context.Context, _ id.RoomID) ([]domain.SpaceChild, error) {
	return m.getSpaceChildrenResult, m.getSpaceChildrenErr
}

func (m *testMockMatrixPort) AddSpaceChild(_ context.Context, spaceID id.RoomID, childID id.RoomID, _ string, _ bool) error {
	m.capturedAddSpaceChildSpaceID = spaceID
	m.capturedAddSpaceChildChildID = childID
	return m.addSpaceChildErr
}

func (m *testMockMatrixPort) SetSpaceParent(_ context.Context, childID id.RoomID, parentID id.RoomID) error {
	m.capturedSetSpaceParentChildID = childID
	m.capturedSetSpaceParentParentID = parentID
	return m.setSpaceParentErr
}

func (m *testMockMatrixPort) InviteToSpace(_ context.Context, roomID id.RoomID, invitee domain.Actor) error {
	m.capturedInviteToSpaceRoomID = roomID
	m.capturedInviteToSpaceActor = invitee
	return m.inviteToSpaceErr
}

func (m *testMockMatrixPort) KickFromSpace(_ context.Context, roomID id.RoomID, userID id.UserID, _ string) error {
	m.capturedKickFromSpaceRoomID = roomID
	m.capturedKickFromSpaceUserID = userID
	return m.kickFromSpaceErr
}

func (m *testMockMatrixPort) SendReadReceipt(_ context.Context, actor domain.Actor, roomID id.RoomID, eventID id.EventID, threadRootID *id.EventID) error {
	m.capturedSendReadReceiptActor = actor
	m.capturedSendReadReceiptRoomID = roomID
	m.capturedSendReadReceiptEventID = eventID
	m.capturedSendReadReceiptThreadID = threadRootID
	return m.sendReadReceiptErr
}

func (m *testMockMatrixPort) GetUnreadCounts(_ context.Context, actor domain.Actor, roomID id.RoomID, threadRootIDs []id.EventID) (*domain.UnreadCountSummary, error) {
	m.capturedGetUnreadCountsActor = actor
	m.capturedGetUnreadCountsRoomID = roomID
	m.capturedGetUnreadCountsThreadIDs = threadRootIDs
	return m.getUnreadCountsResult, m.getUnreadCountsErr
}

func (m *testMockMatrixPort) GetBatchUnreadCounts(_ context.Context, actor domain.Actor, roomIDs []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error) {
	m.capturedBatchUnreadCountsActor = actor
	m.capturedBatchUnreadCountsRoomIDs = roomIDs
	return m.getBatchUnreadCountsRes, m.getBatchUnreadCountsErrs
}

// ============================================================================
// Mock ReadReceiptService
// ============================================================================

// testMockReadReceiptService implements ports.ReadReceiptServicePort for testing.
type testMockReadReceiptService struct {
	markMessageReadErr error
	getUnreadCountsRes *domain.UnreadCountSummary
	getUnreadCountsErr error

	// Captured inputs
	capturedMarkReadActorID  uuid.UUID
	capturedMarkReadRoomID   id.RoomID
	capturedMarkReadEventID  id.EventID
	capturedMarkReadThreadID *id.EventID

	capturedGetUnreadActorID   uuid.UUID
	capturedGetUnreadRoomID    id.RoomID
	capturedGetUnreadThreadIDs []id.EventID
}

func (m *testMockReadReceiptService) MarkMessageRead(_ context.Context, actorID uuid.UUID, roomID id.RoomID, eventID id.EventID, threadID *id.EventID) error {
	m.capturedMarkReadActorID = actorID
	m.capturedMarkReadRoomID = roomID
	m.capturedMarkReadEventID = eventID
	m.capturedMarkReadThreadID = threadID
	return m.markMessageReadErr
}

func (m *testMockReadReceiptService) GetUnreadCounts(_ context.Context, actorID uuid.UUID, roomID id.RoomID, threadIDs []id.EventID) (*domain.UnreadCountSummary, error) {
	m.capturedGetUnreadActorID = actorID
	m.capturedGetUnreadRoomID = roomID
	m.capturedGetUnreadThreadIDs = threadIDs
	return m.getUnreadCountsRes, m.getUnreadCountsErr
}

// ============================================================================
// Shared Test Helpers
// ============================================================================

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
