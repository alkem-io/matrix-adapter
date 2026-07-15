package matrix

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// ============================================================================
// Mock intentAPI
// ============================================================================

type mockIntentAPI struct {
	// Return values
	ensureRegisteredErr    error
	sendTextResult         *mautrix.RespSendEvent
	sendTextErr            error
	sendMessageEventResult *mautrix.RespSendEvent
	sendMessageEventErr    error
	sendStateEventResult   *mautrix.RespSendEvent
	sendStateEventErr      error
	createRoomResult       *mautrix.RespCreateRoom
	createRoomErr          error
	createAliasResult      *mautrix.RespAliasCreate
	createAliasErr         error
	deleteAliasErr         error
	resolveAliasResult     *mautrix.RespAliasResolve
	resolveAliasErr        error
	kickUserErr            error
	leaveRoomErr           error
	redactEventResult      *mautrix.RespSendEvent
	redactEventErr         error
	setDisplayNameErr      error
	setAvatarURLErr        error
	makeRequestErr         error
	joinedRoomsResult      *mautrix.RespJoinedRooms
	joinedRoomsErr         error
	ensureJoinedErr        error
	getAliasesResult       *mautrix.RespAliasList
	getAliasesErr          error
	messagesResult         *mautrix.RespMessages
	messagesErr            error
	stateResult            mautrix.RoomStateMap
	stateErr               error
	sendReceiptErr         error
	setReadMarkersErr      error
	getAccountDataErr      error
	getAccountDataByKey    map[string]error
	setAccountDataErr      error
	getRoomAccountDataErr  error
	createFilterResult     *mautrix.RespCreateFilter
	createFilterErr        error
	syncRequestResult      *mautrix.RespSync
	syncRequestErr         error
	getEventResult         *event.Event
	getEventErr            error
	whoamiResult           *mautrix.RespWhoami
	whoamiErr              error
	inviteUserErr          error
	uploadBytesResult      *mautrix.RespMediaUpload
	uploadBytesErr         error
	downloadBytesResult    []byte
	downloadBytesErr       error

	// Call tracking
	ensureRegisteredCalled int
	sendTextCalled         int
	sendMessageEventCalled int
	sendStateEventCalled   int
	createRoomCalled       int
	createAliasCalled      int
	deleteAliasCalled      int
	resolveAliasCalled     int
	kickUserCalled         int
	leaveRoomCalled        int
	redactEventCalled      int
	setDisplayNameCalled   int
	setAvatarURLCalled     int
	makeRequestCalled      int
	ensureJoinedCalled     int
	sendReceiptCalled      int
	setReadMarkersCalled   int
	setAccountDataCalled   int
	inviteUserCalled       int

	// Captured arguments
	lastSendTextRoomID         id.RoomID
	lastSendTextContent        string
	lastCreateRoomReq          *mautrix.ReqCreateRoom
	lastCreateAliasAlias       id.RoomAlias
	lastCreateAliasRoomID      id.RoomID
	lastKickUserRoomID         id.RoomID
	lastKickUserReq            *mautrix.ReqKickUser
	lastLeaveRoomID            id.RoomID
	lastRedactRoomID           id.RoomID
	lastRedactEventID          id.EventID
	lastDisplayName            string
	lastMakeRequestMethod      string
	lastMakeRequestURL         string
	lastSendStateEventRoomID   id.RoomID
	lastSendStateEventType     event.Type
	lastSendStateEventStateKey string
	lastSendStateEventContent  any
	lastSendMsgEventRoomID     id.RoomID
	lastSendMsgEventType       event.Type
	lastSendMsgEventContent    any
	lastSendMsgEventExtra      []mautrix.ReqSendEvent
	// sendMsgEventTxns accumulates the transaction ID seen on each
	// SendMessageEvent call (empty string when none was supplied), in call order.
	sendMsgEventTxns []string
	// sendMsgEventContents accumulates the content passed to each
	// SendMessageEvent call, in call order (parallel to sendMsgEventTxns).
	sendMsgEventContents      []any
	lastEnsureJoinedRoomID    id.RoomID
	lastSetAccountDataName    string
	lastSetAccountDataContent interface{}
	lastBuildClientURLParts   []any
	buildClientURLResult      string

	// Media
	uploadBytesCalled    int
	lastUploadBytesData  []byte
	lastUploadBytesType  string
	downloadBytesCalled  int
	lastDownloadBytesMXC id.ContentURI
}

var _ intentAPI = (*mockIntentAPI)(nil)

func (m *mockIntentAPI) EnsureRegistered(_ context.Context) error {
	m.ensureRegisteredCalled++
	return m.ensureRegisteredErr
}

func (m *mockIntentAPI) EnsureJoined(_ context.Context, roomID id.RoomID, _ ...appservice.EnsureJoinedParams) error {
	m.ensureJoinedCalled++
	m.lastEnsureJoinedRoomID = roomID
	return m.ensureJoinedErr
}

func (m *mockIntentAPI) SendMessageEvent(_ context.Context, roomID id.RoomID, eventType event.Type, contentJSON any, extra ...mautrix.ReqSendEvent) (*mautrix.RespSendEvent, error) {
	m.sendMessageEventCalled++
	m.lastSendMsgEventRoomID = roomID
	m.lastSendMsgEventType = eventType
	m.lastSendMsgEventContent = contentJSON
	m.lastSendMsgEventExtra = extra
	var txn string
	if len(extra) > 0 {
		txn = extra[0].TransactionID
	}
	m.sendMsgEventTxns = append(m.sendMsgEventTxns, txn)
	m.sendMsgEventContents = append(m.sendMsgEventContents, contentJSON)
	return m.sendMessageEventResult, m.sendMessageEventErr
}

func (m *mockIntentAPI) SendStateEvent(_ context.Context, roomID id.RoomID, eventType event.Type, stateKey string, contentJSON any, _ ...mautrix.ReqSendEvent) (*mautrix.RespSendEvent, error) {
	m.sendStateEventCalled++
	m.lastSendStateEventRoomID = roomID
	m.lastSendStateEventType = eventType
	m.lastSendStateEventStateKey = stateKey
	m.lastSendStateEventContent = contentJSON
	return m.sendStateEventResult, m.sendStateEventErr
}

func (m *mockIntentAPI) State(_ context.Context, _ id.RoomID) (mautrix.RoomStateMap, error) {
	return m.stateResult, m.stateErr
}

func (m *mockIntentAPI) SetDisplayName(_ context.Context, displayName string) error {
	m.setDisplayNameCalled++
	m.lastDisplayName = displayName
	return m.setDisplayNameErr
}

func (m *mockIntentAPI) SetAvatarURL(_ context.Context, _ id.ContentURI) error {
	m.setAvatarURLCalled++
	return m.setAvatarURLErr
}

func (m *mockIntentAPI) SendText(_ context.Context, roomID id.RoomID, text string) (*mautrix.RespSendEvent, error) {
	m.sendTextCalled++
	m.lastSendTextRoomID = roomID
	m.lastSendTextContent = text
	return m.sendTextResult, m.sendTextErr
}

func (m *mockIntentAPI) RedactEvent(_ context.Context, roomID id.RoomID, eventID id.EventID, _ ...mautrix.ReqRedact) (*mautrix.RespSendEvent, error) {
	m.redactEventCalled++
	m.lastRedactRoomID = roomID
	m.lastRedactEventID = eventID
	return m.redactEventResult, m.redactEventErr
}

func (m *mockIntentAPI) CreateRoom(_ context.Context, req *mautrix.ReqCreateRoom) (*mautrix.RespCreateRoom, error) {
	m.createRoomCalled++
	m.lastCreateRoomReq = req
	return m.createRoomResult, m.createRoomErr
}

func (m *mockIntentAPI) JoinedRooms(_ context.Context) (*mautrix.RespJoinedRooms, error) {
	return m.joinedRoomsResult, m.joinedRoomsErr
}

func (m *mockIntentAPI) LeaveRoom(_ context.Context, roomID id.RoomID, _ ...interface{}) (*mautrix.RespLeaveRoom, error) {
	m.leaveRoomCalled++
	m.lastLeaveRoomID = roomID
	return &mautrix.RespLeaveRoom{}, m.leaveRoomErr
}

func (m *mockIntentAPI) InviteUser(_ context.Context, _ id.RoomID, _ *mautrix.ReqInviteUser, _ ...map[string]interface{}) (*mautrix.RespInviteUser, error) {
	m.inviteUserCalled++
	return &mautrix.RespInviteUser{}, m.inviteUserErr
}

func (m *mockIntentAPI) KickUser(_ context.Context, roomID id.RoomID, req *mautrix.ReqKickUser, _ ...map[string]interface{}) (*mautrix.RespKickUser, error) {
	m.kickUserCalled++
	m.lastKickUserRoomID = roomID
	m.lastKickUserReq = req
	return &mautrix.RespKickUser{}, m.kickUserErr
}

func (m *mockIntentAPI) ResolveAlias(_ context.Context, _ id.RoomAlias) (*mautrix.RespAliasResolve, error) {
	m.resolveAliasCalled++
	return m.resolveAliasResult, m.resolveAliasErr
}

func (m *mockIntentAPI) GetAliases(_ context.Context, _ id.RoomID) (*mautrix.RespAliasList, error) {
	return m.getAliasesResult, m.getAliasesErr
}

func (m *mockIntentAPI) CreateAlias(_ context.Context, alias id.RoomAlias, roomID id.RoomID) (*mautrix.RespAliasCreate, error) {
	m.createAliasCalled++
	m.lastCreateAliasAlias = alias
	m.lastCreateAliasRoomID = roomID
	return m.createAliasResult, m.createAliasErr
}

func (m *mockIntentAPI) DeleteAlias(_ context.Context, _ id.RoomAlias) (*mautrix.RespAliasDelete, error) {
	m.deleteAliasCalled++
	return &mautrix.RespAliasDelete{}, m.deleteAliasErr
}

func (m *mockIntentAPI) UploadBytes(_ context.Context, data []byte, contentType string) (*mautrix.RespMediaUpload, error) {
	m.uploadBytesCalled++
	m.lastUploadBytesData = data
	m.lastUploadBytesType = contentType
	return m.uploadBytesResult, m.uploadBytesErr
}

// UploadMedia records the streamed upload the same way UploadBytes recorded the
// buffered one: it drains req.Content into lastUploadBytesData and captures the
// content type, so the existing upload assertions carry over to the streamed
// path. A read error from the streaming reader (e.g. the oversize cap tripping)
// is surfaced so sendAttachment can classify it.
func (m *mockIntentAPI) UploadMedia(_ context.Context, req mautrix.ReqUploadMedia) (*mautrix.RespMediaUpload, error) {
	m.uploadBytesCalled++
	m.lastUploadBytesType = req.ContentType
	if req.Content != nil {
		data, err := io.ReadAll(req.Content)
		m.lastUploadBytesData = data
		if err != nil {
			return nil, err
		}
	} else {
		m.lastUploadBytesData = req.ContentBytes
	}
	if m.uploadBytesErr != nil {
		return nil, m.uploadBytesErr
	}
	if m.uploadBytesResult != nil {
		return m.uploadBytesResult, nil
	}
	return &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/stub")}, nil
}

func (m *mockIntentAPI) DownloadBytes(_ context.Context, mxcURL id.ContentURI) ([]byte, error) {
	m.downloadBytesCalled++
	m.lastDownloadBytesMXC = mxcURL
	return m.downloadBytesResult, m.downloadBytesErr
}

func (m *mockIntentAPI) SendReceipt(_ context.Context, _ id.RoomID, _ id.EventID, _ event.ReceiptType, _ interface{}) error {
	m.sendReceiptCalled++
	return m.sendReceiptErr
}

func (m *mockIntentAPI) SetReadMarkers(_ context.Context, _ id.RoomID, _ interface{}) error {
	m.setReadMarkersCalled++
	return m.setReadMarkersErr
}

func (m *mockIntentAPI) GetAccountData(_ context.Context, name string, _ interface{}) error {
	if m.getAccountDataByKey != nil {
		if err, ok := m.getAccountDataByKey[name]; ok {
			return err
		}
	}
	return m.getAccountDataErr
}

func (m *mockIntentAPI) SetAccountData(_ context.Context, name string, data interface{}) error {
	m.setAccountDataCalled++
	m.lastSetAccountDataName = name
	m.lastSetAccountDataContent = data
	return m.setAccountDataErr
}

func (m *mockIntentAPI) GetRoomAccountData(_ context.Context, _ id.RoomID, _ string, _ interface{}) error {
	return m.getRoomAccountDataErr
}

func (m *mockIntentAPI) Messages(_ context.Context, _ id.RoomID, _, _ string, _ mautrix.Direction, _ *mautrix.FilterPart, _ int) (*mautrix.RespMessages, error) {
	return m.messagesResult, m.messagesErr
}

func (m *mockIntentAPI) CreateFilter(_ context.Context, _ *mautrix.Filter) (*mautrix.RespCreateFilter, error) {
	return m.createFilterResult, m.createFilterErr
}

func (m *mockIntentAPI) SyncRequest(_ context.Context, _ int, _, _ string, _ bool, _ event.Presence) (*mautrix.RespSync, error) {
	return m.syncRequestResult, m.syncRequestErr
}

func (m *mockIntentAPI) GetEvent(_ context.Context, _ id.RoomID, _ id.EventID) (*event.Event, error) {
	return m.getEventResult, m.getEventErr
}

func (m *mockIntentAPI) Whoami(_ context.Context) (*mautrix.RespWhoami, error) {
	return m.whoamiResult, m.whoamiErr
}

func (m *mockIntentAPI) BuildClientURL(urlPath ...any) string {
	m.lastBuildClientURLParts = urlPath
	return m.buildClientURLResult
}

func (m *mockIntentAPI) MakeRequest(_ context.Context, method string, httpURL string, _ any, _ any) ([]byte, error) {
	m.makeRequestCalled++
	m.lastMakeRequestMethod = method
	m.lastMakeRequestURL = httpURL
	return nil, m.makeRequestErr
}

// ============================================================================
// Mock appserviceAPI
// ============================================================================

type mockAppserviceAPI struct {
	botIntent          intentAPI
	intents            map[id.UserID]intentAPI
	botMXID            id.UserID
	homeserverDomain   string
	setMembershipCalls []setMembershipCall
	setMembershipErr   error
}

var _ appserviceAPI = (*mockAppserviceAPI)(nil)

func (m *mockAppserviceAPI) BotIntent() intentAPI {
	return m.botIntent
}

func (m *mockAppserviceAPI) BotMXID() id.UserID {
	return m.botMXID
}

func (m *mockAppserviceAPI) Intent(userID id.UserID) intentAPI {
	if intent, ok := m.intents[userID]; ok {
		return intent
	}
	// Panic on unexpected user IDs so tests must explicitly map all expected users.
	panic("mockAppserviceAPI.Intent called with unmapped user ID: " + userID.String())
}

func (m *mockAppserviceAPI) Start()                       {}
func (m *mockAppserviceAPI) Stop()                        {}
func (m *mockAppserviceAPI) HomeserverDomain() string     { return m.homeserverDomain }
func (m *mockAppserviceAPI) Host() *appservice.HostConfig { return &appservice.HostConfig{} }
func (m *mockAppserviceAPI) Router() *http.ServeMux       { return http.NewServeMux() }
func (m *mockAppserviceAPI) Events() <-chan *event.Event  { return make(<-chan *event.Event) }
func (m *mockAppserviceAPI) SetMembership(_ context.Context, roomID id.RoomID, userID id.UserID, membership event.Membership) error {
	m.setMembershipCalls = append(m.setMembershipCalls, setMembershipCall{roomID, userID, membership})
	return m.setMembershipErr
}

type setMembershipCall struct {
	RoomID     id.RoomID
	UserID     id.UserID
	Membership event.Membership
}

// ============================================================================
// Test helper
// ============================================================================

// newFullTestAdapter creates a fully testable adapter with mock as + admin.
func newFullTestAdapter(as appserviceAPI, admin adminAPI) *MautrixAdapter {
	return &MautrixAdapter{
		as:       as,
		admin:    admin,
		idMapper: domain.NewIDMapper("test.local"),
		logger:   &adapterMockLogger{},
	}
}

// testActorID is a fixed UUID used across tests for the primary actor.
var testActorID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

// testActorID2 is a second fixed UUID for multi-user tests.
var testActorID2 = uuid.MustParse("660e8400-e29b-41d4-a716-446655440001")

// testActor creates a domain.Actor with the given UUID and optional display name.
func testActor(id uuid.UUID, displayName string) domain.Actor {
	return domain.Actor{
		ID:          id,
		DisplayName: displayName,
	}
}

// intentTestIDMapper is a shared IDMapper for constructing Matrix IDs in intent tests.
var intentTestIDMapper = domain.NewIDMapper("test.local")

// expectedUserID returns the Matrix user ID for a test actor UUID.
func expectedUserID(actorID uuid.UUID) id.UserID {
	return intentTestIDMapper.UserID(actorID)
}

// newMockAS creates a mockAppserviceAPI where both botIntent and any user intent
// are mapped to the given intents. botIntent is the intent used for BotIntent().
// userIntents maps specific user IDs to their intents.
func newMockAS(botIntent intentAPI, userIntents map[id.UserID]intentAPI) *mockAppserviceAPI {
	return &mockAppserviceAPI{
		botIntent:        botIntent,
		intents:          userIntents,
		botMXID:          "@bot:test.local",
		homeserverDomain: "test.local",
	}
}

// ============================================================================
// EnsureUser
// ============================================================================

func TestEnsureUser_Success(t *testing.T) {
	intent := &mockIntentAPI{}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	userID, err := a.EnsureUser(context.Background(), testActor(testActorID, "Alice"))
	require.NoError(t, err)
	assert.Equal(t, expectedUserID(testActorID), userID)
	assert.Equal(t, 1, intent.ensureRegisteredCalled)
	assert.Equal(t, 1, intent.setDisplayNameCalled)
	assert.Equal(t, "Alice", intent.lastDisplayName)
}

func TestEnsureUser_RegistrationError(t *testing.T) {
	intent := &mockIntentAPI{
		ensureRegisteredErr: errors.New("registration failed"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.EnsureUser(context.Background(), testActor(testActorID, "Alice"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ensure user registered")
}

func TestEnsureUser_NoDisplayName(t *testing.T) {
	intent := &mockIntentAPI{}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	userID, err := a.EnsureUser(context.Background(), testActor(testActorID, ""))
	require.NoError(t, err)
	assert.Equal(t, expectedUserID(testActorID), userID)
	assert.Equal(t, 0, intent.setDisplayNameCalled, "should not set display name when empty")
}

// ============================================================================
// SetUserProfile
// ============================================================================

func TestSetUserProfile_Success(t *testing.T) {
	intent := &mockIntentAPI{}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	actor := domain.Actor{
		ID:          testActorID,
		DisplayName: "Alice Updated",
		AvatarURL:   "mxc://test.local/avatar123",
	}
	err := a.SetUserProfile(context.Background(), actor)
	require.NoError(t, err)
	// EnsureUser sets display name once, then SetUserProfile sets it again = 2 calls
	assert.Equal(t, 2, intent.setDisplayNameCalled)
	assert.Equal(t, "Alice Updated", intent.lastDisplayName)
	assert.Equal(t, 1, intent.setAvatarURLCalled)
}

func TestSetUserProfile_DisplayNameError(t *testing.T) {
	intent := &mockIntentAPI{
		setDisplayNameErr: errors.New("display name error"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	actor := domain.Actor{
		ID:          testActorID,
		DisplayName: "Alice",
		AvatarURL:   "mxc://test.local/avatar",
	}
	// SetUserProfile logs warnings but does not return errors for profile failures
	// EnsureUser also tries SetDisplayName, so 2 calls total
	err := a.SetUserProfile(context.Background(), actor)
	require.NoError(t, err)
	assert.Equal(t, 2, intent.setDisplayNameCalled)
}

func TestSetUserProfile_AvatarError(t *testing.T) {
	intent := &mockIntentAPI{
		setAvatarURLErr: errors.New("avatar error"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	actor := domain.Actor{
		ID:          testActorID,
		DisplayName: "Alice",
		AvatarURL:   "mxc://test.local/avatar",
	}
	err := a.SetUserProfile(context.Background(), actor)
	require.NoError(t, err)
	assert.Equal(t, 1, intent.setAvatarURLCalled)
}

func TestSetUserProfile_ClearAvatar(t *testing.T) {
	intent := &mockIntentAPI{}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	actor := domain.Actor{
		ID:          testActorID,
		DisplayName: "Alice",
		AvatarURL:   "", // empty means no avatar update
	}
	err := a.SetUserProfile(context.Background(), actor)
	require.NoError(t, err)
	assert.Equal(t, 0, intent.setAvatarURLCalled, "should not call SetAvatarURL when empty")
}

// ============================================================================
// SendMessage
// ============================================================================

func TestSendMessage_Success(t *testing.T) {
	intent := &mockIntentAPI{
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$msg1"},
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	eventID, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "Hello", nil, "")
	require.NoError(t, err)
	assert.Equal(t, id.EventID("$msg1"), eventID)
	// Text goes through the unified SendMessageEvent path (never SendText).
	assert.Equal(t, 0, intent.sendTextCalled)
	require.Equal(t, 1, intent.sendMessageEventCalled)
	assert.Equal(t, id.RoomID("!room:test.local"), intent.lastSendMsgEventRoomID)
	content, ok := intent.lastSendMsgEventContent.(*event.MessageEventContent)
	require.True(t, ok)
	assert.Equal(t, event.MsgText, content.MsgType)
	assert.Equal(t, "Hello", content.Body)
}

func TestSendMessage_Error(t *testing.T) {
	intent := &mockIntentAPI{
		sendMessageEventErr: errors.New("send failed"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "Hello", nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send message")
}

// ============================================================================
// SendReply
// ============================================================================

func TestSendReply_Success(t *testing.T) {
	intent := &mockIntentAPI{
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$reply1"},
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	eventID, err := a.SendReply(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "reply text", "$thread-root", nil, "")
	require.NoError(t, err)
	assert.Equal(t, id.EventID("$reply1"), eventID)
	assert.Equal(t, 1, intent.sendMessageEventCalled)
	assert.Equal(t, id.RoomID("!room:test.local"), intent.lastSendMsgEventRoomID)
	assert.Equal(t, event.EventMessage, intent.lastSendMsgEventType)

	// Verify thread relation content
	content, ok := intent.lastSendMsgEventContent.(*event.MessageEventContent)
	require.True(t, ok)
	assert.Equal(t, "reply text", content.Body)
	assert.Equal(t, event.RelThread, content.RelatesTo.Type)
	assert.Equal(t, id.EventID("$thread-root"), content.RelatesTo.EventID)
}

func TestSendReply_Error(t *testing.T) {
	intent := &mockIntentAPI{
		sendMessageEventErr: errors.New("send reply failed"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.SendReply(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "reply", "$thread", nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send reply")
}

// ============================================================================
// RedactEvent
// ============================================================================

func TestRedactEvent_Success(t *testing.T) {
	intent := &mockIntentAPI{
		redactEventResult: &mautrix.RespSendEvent{EventID: "$redact1"},
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.RedactEvent(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "$event1", "spam")
	require.NoError(t, err)
	assert.Equal(t, 1, intent.redactEventCalled)
	assert.Equal(t, id.RoomID("!room:test.local"), intent.lastRedactRoomID)
	assert.Equal(t, id.EventID("$event1"), intent.lastRedactEventID)
}

func TestRedactEvent_Error(t *testing.T) {
	intent := &mockIntentAPI{
		redactEventErr: errors.New("redact failed"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.RedactEvent(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "$event1", "spam")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to redact event")
}

// ============================================================================
// SendReaction
// ============================================================================

func TestSendReaction_Success(t *testing.T) {
	intent := &mockIntentAPI{
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$reaction1"},
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	eventID, err := a.SendReaction(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "$msg1", "\U0001F44D")
	require.NoError(t, err)
	assert.Equal(t, id.EventID("$reaction1"), eventID)
	assert.Equal(t, 1, intent.sendMessageEventCalled)
	assert.Equal(t, event.EventReaction, intent.lastSendMsgEventType)

	// Verify reaction content
	content, ok := intent.lastSendMsgEventContent.(*event.ReactionEventContent)
	require.True(t, ok)
	assert.Equal(t, id.EventID("$msg1"), content.RelatesTo.EventID)
	assert.Equal(t, "\U0001F44D", content.RelatesTo.Key)
	assert.Equal(t, event.RelAnnotation, content.RelatesTo.Type)
}

func TestSendReaction_Error(t *testing.T) {
	intent := &mockIntentAPI{
		sendMessageEventErr: errors.New("reaction failed"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.SendReaction(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "$msg1", "\U0001F44D")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send reaction")
}

// ============================================================================
// ResolveAlias
// ============================================================================

func TestResolveAlias_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		resolveAliasResult: &mautrix.RespAliasResolve{
			RoomID: "!resolved:test.local",
		},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	roomID, err := a.ResolveAlias(context.Background(), "#myroom:test.local")
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!resolved:test.local"), roomID)
	assert.Equal(t, 1, botIntent.resolveAliasCalled)
}

func TestResolveAlias_NotFound(t *testing.T) {
	botIntent := &mockIntentAPI{
		resolveAliasErr: errors.New("alias not found"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.ResolveAlias(context.Background(), "#unknown:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve alias")
}

// ============================================================================
// DeleteAlias
// ============================================================================

func TestDeleteAlias_Success(t *testing.T) {
	botIntent := &mockIntentAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.DeleteAlias(context.Background(), "#old:test.local")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.deleteAliasCalled)
}

func TestDeleteAlias_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		deleteAliasErr: errors.New("delete failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.DeleteAlias(context.Background(), "#old:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete alias")
}

// ============================================================================
// KickUser
// ============================================================================

func TestKickUser_Success(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		// Bot is a member of the room
		getRoomMembersResult: []string{"@bot:test.local", "@user:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.KickUser(context.Background(), "!room:test.local", "@baduser:test.local", "rule violation")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.kickUserCalled)
	assert.Equal(t, id.RoomID("!room:test.local"), botIntent.lastKickUserRoomID)
	assert.Equal(t, id.UserID("@baduser:test.local"), botIntent.lastKickUserReq.UserID)
	assert.Equal(t, "rule violation", botIntent.lastKickUserReq.Reason)
}

func TestKickUser_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		kickUserErr: errors.New("kick failed"),
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.KickUser(context.Background(), "!room:test.local", "@user:test.local", "reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to kick user")
}

// ============================================================================
// SetRoomAlias
// ============================================================================

func TestSetRoomAlias_Success(t *testing.T) {
	botIntent := &mockIntentAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.SetRoomAlias(context.Background(), "!room:test.local", "#myalias:test.local")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.createAliasCalled)
	assert.Equal(t, id.RoomAlias("#myalias:test.local"), botIntent.lastCreateAliasAlias)
	assert.Equal(t, id.RoomID("!room:test.local"), botIntent.lastCreateAliasRoomID)
}

func TestSetRoomAlias_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		createAliasErr: errors.New("alias exists"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.SetRoomAlias(context.Background(), "!room:test.local", "#dup:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create room alias")
}

// ============================================================================
// InviteUser (direct join, no invite event)
// ============================================================================

func TestInviteUser_Success(t *testing.T) {
	inviteeIntent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{
		// For leaveBotIfNotNeeded - bot will leave since ghost is present
		leaveRoomErr: nil,
	}
	admin := &mockAdminAPI{
		// For getLatestEventID
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{ID: "$latest1"},
			},
		},
		// For leaveBotIfNotNeeded — bot is member, ghost is member
		getRoomMembersResult: []string{
			"@bot:test.local",
			expectedUserID(testActorID).String(),
		},
		// For isSpaceRoom
		getStateEventContentResult: map[string]interface{}{}, // not a space
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): inviteeIntent,
	})
	a := newFullTestAdapter(as, admin)

	err := a.InviteUser(
		context.Background(),
		"!room:test.local",
		testActor(testActorID2, "Inviter"),
		testActor(testActorID, "Invitee"),
	)
	require.NoError(t, err)
	assert.Equal(t, 1, inviteeIntent.ensureRegisteredCalled)
	assert.Equal(t, 1, inviteeIntent.ensureJoinedCalled)
}

func TestInviteUser_JoinError(t *testing.T) {
	inviteeIntent := &mockIntentAPI{
		ensureJoinedErr: errors.New("join failed"),
	}
	as := newMockAS(&mockIntentAPI{}, map[id.UserID]intentAPI{
		expectedUserID(testActorID): inviteeIntent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.InviteUser(
		context.Background(),
		"!room:test.local",
		testActor(testActorID2, "Inviter"),
		testActor(testActorID, "Invitee"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to join user to room")
}

// ============================================================================
// UpdateRoomState
// ============================================================================

func TestUpdateRoomState_NameChange(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		// Bot is in the room
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(intent, nil)
	a := newFullTestAdapter(as, admin)

	name := "New Room Name"
	err := a.UpdateRoomState(context.Background(), "!room:test.local", testActor(testActorID, ""), &name, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendStateEventCalled)
	assert.Equal(t, event.StateRoomName, intent.lastSendStateEventType)
}

func TestUpdateRoomState_TopicChange(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(intent, nil)
	a := newFullTestAdapter(as, admin)

	topic := "New Topic"
	err := a.UpdateRoomState(context.Background(), "!room:test.local", testActor(testActorID, ""), nil, &topic, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendStateEventCalled)
	assert.Equal(t, event.StateTopic, intent.lastSendStateEventType)
}

func TestUpdateRoomState_AvatarChange(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(intent, nil)
	a := newFullTestAdapter(as, admin)

	avatar := "mxc://test.local/newavatar"
	err := a.UpdateRoomState(context.Background(), "!room:test.local", testActor(testActorID, ""), nil, nil, &avatar, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendStateEventCalled)
	assert.Equal(t, event.StateRoomAvatar, intent.lastSendStateEventType)
}

func TestUpdateRoomState_JoinRuleChange(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(intent, nil)
	a := newFullTestAdapter(as, admin)

	joinRule := "invite"
	err := a.UpdateRoomState(context.Background(), "!room:test.local", testActor(testActorID, ""), nil, nil, nil, &joinRule)
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendStateEventCalled)
	assert.Equal(t, event.StateJoinRules, intent.lastSendStateEventType)
}

func TestUpdateRoomState_NilFields_NoChanges(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(intent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.UpdateRoomState(context.Background(), "!room:test.local", testActor(testActorID, ""), nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, intent.sendStateEventCalled, "no state events should be sent for nil fields")
}

func TestUpdateRoomState_MultipleFields(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(intent, nil)
	a := newFullTestAdapter(as, admin)

	name := "Name"
	topic := "Topic"
	err := a.UpdateRoomState(context.Background(), "!room:test.local", testActor(testActorID, ""), &name, &topic, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, intent.sendStateEventCalled, "should send state events for name and topic")
}

func TestUpdateRoomState_NameError(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventErr: errors.New("state event error"),
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(intent, nil)
	a := newFullTestAdapter(as, admin)

	name := "New Name"
	err := a.UpdateRoomState(context.Background(), "!room:test.local", testActor(testActorID, ""), &name, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set room name")
}

// ============================================================================
// SetCustomState
// ============================================================================

func TestSetCustomState_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		// Bot is in the room
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	state := map[string]map[string]interface{}{
		"io.alkemio.custom": {"key": "value"},
	}
	err := a.SetCustomState(context.Background(), "!room:test.local", state)
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.sendStateEventCalled)
	assert.Equal(t, "io.alkemio.custom", botIntent.lastSendStateEventType.Type)
}

func TestSetCustomState_InvalidPrefix(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	state := map[string]map[string]interface{}{
		"m.custom.state": {"key": "value"},
	}
	err := a.SetCustomState(context.Background(), "!room:test.local", state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "io.alkemio. prefix")
}

func TestSetCustomState_SendError(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventErr: errors.New("send state failed"),
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	state := map[string]map[string]interface{}{
		"io.alkemio.type": {"key": "val"},
	}
	err := a.SetCustomState(context.Background(), "!room:test.local", state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set custom state")
}

// ============================================================================
// SetRoomDirectoryVisibility
// ============================================================================

func TestSetRoomDirectoryVisibility_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		buildClientURLResult: "https://test.local/_matrix/client/v3/directory/list/room/!room:test.local",
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.SetRoomDirectoryVisibility(context.Background(), "!room:test.local", true)
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.makeRequestCalled)
	assert.Equal(t, http.MethodPut, botIntent.lastMakeRequestMethod)
}

func TestSetRoomDirectoryVisibility_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		buildClientURLResult: "https://test.local/_matrix/client/v3/directory/list/room/!room:test.local",
		makeRequestErr:       errors.New("403 forbidden"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.SetRoomDirectoryVisibility(context.Background(), "!room:test.local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set room directory visibility")
}

// ============================================================================
// GetSpaceDetails
// ============================================================================

func TestGetSpaceDetails_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{
			Aliases: []id.RoomAlias{
				id.RoomAlias("#" + testActorID.String() + ":test.local"),
			},
		},
	}
	admin := &mockAdminAPI{
		getStateEventContentResult: map[string]interface{}{
			"name": "My Space",
		},
		getCustomStateResult: map[string]map[string]interface{}{
			"io.alkemio.meta": {"version": "1"},
		},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	space, err := a.GetSpaceDetails(context.Background(), "!space:test.local")
	require.NoError(t, err)
	require.NotNil(t, space)
	assert.Equal(t, id.RoomID("!space:test.local"), space.ID)
}

func TestGetSpaceDetails_AdminError(t *testing.T) {
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{
			Aliases: []id.RoomAlias{},
		},
	}
	admin := &mockAdminAPI{
		getStateEventContentErr: errors.New("admin api error"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	// GetSpaceDetails gracefully handles admin errors (returns empty fields)
	space, err := a.GetSpaceDetails(context.Background(), "!space:test.local")
	require.NoError(t, err)
	require.NotNil(t, space)
	assert.Equal(t, "", space.Name)
}

// ============================================================================
// CreateSpace
// ============================================================================

func TestCreateSpace_Success(t *testing.T) {
	memberIntent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newspace:test.local"},
	}
	admin := &mockAdminAPI{
		// For autoJoinAndMarkRead -> getLatestEventID
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{ID: "$latest1"},
			},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): memberIntent,
	})
	a := newFullTestAdapter(as, admin)

	contextID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440002")
	members := []domain.Actor{testActor(testActorID, "Alice")}

	roomID, err := a.CreateSpace(context.Background(), contextID, "Space Name", "Space Topic", "", "invite", members)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!newspace:test.local"), roomID)
	assert.Equal(t, 1, botIntent.createRoomCalled)
	// Verify the creation request had the space type
	require.NotNil(t, botIntent.lastCreateRoomReq)
	assert.Equal(t, "m.space", botIntent.lastCreateRoomReq.CreationContent["type"])
}

func TestCreateSpace_CreateError(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomErr: errors.New("create space failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	contextID := uuid.MustParse("770e8400-e29b-41d4-a716-446655440002")
	_, err := a.CreateSpace(context.Background(), contextID, "Space", "", "", "invite", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create space")
}

// ============================================================================
// CreateRoomWithAlias
// ============================================================================

func TestCreateRoomWithAlias_Success(t *testing.T) {
	memberIntent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
	}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{ID: "$latest1"},
			},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): memberIntent,
	})
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	members := []domain.Actor{testActor(testActorID, "Alice")}

	roomID, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "community",
		"Room Name", "Room Topic", "", "", nil, members,
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!newroom:test.local"), roomID)
	assert.Equal(t, 1, botIntent.createRoomCalled)
	assert.Equal(t, 1, botIntent.createAliasCalled, "should set the room alias")
	assert.Equal(t, 1, botIntent.leaveRoomCalled, "bot should leave after members joined")
}

func TestCreateRoomWithAlias_CreateError(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomErr: errors.New("create room failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	_, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "community",
		"Room", "", "", "", nil, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create room with alias")
}

func TestCreateRoomWithAlias_AliasError(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
		createAliasErr:   errors.New("alias conflict"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	_, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "community",
		"Room", "", "", "", nil, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set alias on room")
}

func TestCreateRoomWithAlias_NoMembers_BotStays(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
	}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	roomID, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "community",
		"Room", "", "", "", nil, nil, // no initial members
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!newroom:test.local"), roomID)
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "bot should stay when no members joined")
}

func TestCreateRoomWithAlias_DirectRoom(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!dm:test.local"},
	}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	roomID, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "direct",
		"", "", "", "", nil, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!dm:test.local"), roomID)
	require.NotNil(t, botIntent.lastCreateRoomReq)
	assert.Equal(t, "trusted_private_chat", botIntent.lastCreateRoomReq.Preset)
	assert.True(t, botIntent.lastCreateRoomReq.IsDirect)
}

func TestCreateRoomWithAlias_DirectRoom_SetsAccountData(t *testing.T) {
	user1ID := expectedUserID(testActorID)
	user2ID := expectedUserID(testActorID2)

	user1Intent := &mockIntentAPI{
		getAccountDataByKey: map[string]error{
			"m.direct": mautrix.MNotFound,
		},
	}
	user2Intent := &mockIntentAPI{
		getAccountDataByKey: map[string]error{
			"m.direct": mautrix.MNotFound,
		},
	}
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!dm:test.local"},
	}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{{ID: "$latest1"}},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		user1ID: user1Intent,
		user2ID: user2Intent,
	})
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	members := []domain.Actor{
		testActor(testActorID, "Alice"),
		testActor(testActorID2, "Bob"),
	}

	roomID, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "direct",
		"", "", "", "", nil, members,
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!dm:test.local"), roomID)

	assert.Equal(t, 1, user1Intent.setAccountDataCalled)
	assert.Equal(t, "m.direct", user1Intent.lastSetAccountDataName)

	assert.Equal(t, 1, user2Intent.setAccountDataCalled)
	assert.Equal(t, "m.direct", user2Intent.lastSetAccountDataName)
}

func TestCreateRoomWithAlias_DirectRoom_AccountDataIdempotent(t *testing.T) {
	user1ID := expectedUserID(testActorID)
	user2ID := expectedUserID(testActorID2)

	user1Intent := &mockIntentAPI{}
	user2Intent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!dm:test.local"},
	}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{{ID: "$latest1"}},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		user1ID: user1Intent,
		user2ID: user2Intent,
	})
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	members := []domain.Actor{
		testActor(testActorID, "Alice"),
		testActor(testActorID2, "Bob"),
	}

	_, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "direct",
		"", "", "", "", nil, members,
	)
	require.NoError(t, err)

	// GetAccountData returns nil error (success) with empty data, so the room
	// is not a duplicate — SetAccountData should still be called.
	assert.Equal(t, 1, user1Intent.setAccountDataCalled)
	assert.Equal(t, 1, user2Intent.setAccountDataCalled)
}

func TestCreateRoomWithAlias_DirectRoom_TransientError_NoWrite(t *testing.T) {
	user1ID := expectedUserID(testActorID)
	user2ID := expectedUserID(testActorID2)

	user1Intent := &mockIntentAPI{
		getAccountDataByKey: map[string]error{
			"m.direct": mautrix.MForbidden,
		},
	}
	user2Intent := &mockIntentAPI{
		getAccountDataByKey: map[string]error{
			"m.direct": mautrix.MForbidden,
		},
	}
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!dm:test.local"},
	}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{{ID: "$latest1"}},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		user1ID: user1Intent,
		user2ID: user2Intent,
	})
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	members := []domain.Actor{
		testActor(testActorID, "Alice"),
		testActor(testActorID2, "Bob"),
	}

	_, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "direct",
		"", "", "", "", nil, members,
	)
	require.NoError(t, err)

	// Transient error on GetAccountData → should NOT write to avoid data loss
	assert.Equal(t, 0, user1Intent.setAccountDataCalled)
	assert.Equal(t, 0, user2Intent.setAccountDataCalled)
}

// ============================================================================
// AddSpaceChild
// ============================================================================

func TestAddSpaceChild_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$child1"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.AddSpaceChild(context.Background(), "!space:test.local", "!child:test.local", "01", true)
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.sendStateEventCalled)
	assert.Equal(t, event.StateSpaceChild, botIntent.lastSendStateEventType)
	assert.Equal(t, "!child:test.local", botIntent.lastSendStateEventStateKey)

	content, ok := botIntent.lastSendStateEventContent.(*event.SpaceChildEventContent)
	require.True(t, ok)
	assert.Equal(t, "01", content.Order)
	assert.True(t, content.Suggested)
	assert.Contains(t, content.Via, "test.local")
}

func TestAddSpaceChild_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventErr: errors.New("state event failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.AddSpaceChild(context.Background(), "!space:test.local", "!child:test.local", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add space child")
}

// ============================================================================
// SetSpaceParent
// ============================================================================

func TestSetSpaceParent_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$parent1"},
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.SetSpaceParent(context.Background(), "!child:test.local", "!parent:test.local")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.sendStateEventCalled)
	assert.Equal(t, event.StateSpaceParent, botIntent.lastSendStateEventType)
	assert.Equal(t, "!parent:test.local", botIntent.lastSendStateEventStateKey)

	content, ok := botIntent.lastSendStateEventContent.(*event.SpaceParentEventContent)
	require.True(t, ok)
	assert.True(t, content.Canonical)
	assert.Contains(t, content.Via, "test.local")
}

func TestSetSpaceParent_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventErr: errors.New("set parent failed"),
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.SetSpaceParent(context.Background(), "!child:test.local", "!parent:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set space parent")
}

// ============================================================================
// InviteToSpace
// ============================================================================

func TestInviteToSpace_Success(t *testing.T) {
	inviteeIntent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{ID: "$latest1"},
			},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): inviteeIntent,
	})
	a := newFullTestAdapter(as, admin)

	err := a.InviteToSpace(context.Background(), "!space:test.local", testActor(testActorID, "Alice"))
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.inviteUserCalled, "bot should invite the user")
	assert.Equal(t, 1, inviteeIntent.ensureJoinedCalled, "invitee should auto-join")
}

func TestInviteToSpace_EnsureJoinedError(t *testing.T) {
	inviteeIntent := &mockIntentAPI{
		ensureJoinedErr: errors.New("join failed"),
	}
	botIntent := &mockIntentAPI{}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): inviteeIntent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.InviteToSpace(context.Background(), "!space:test.local", testActor(testActorID, "Alice"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to join space after invite")
}

// ============================================================================
// KickFromSpace
// ============================================================================

func TestKickFromSpace_Success(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.KickFromSpace(context.Background(), "!space:test.local", "@user:test.local", "removed")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.kickUserCalled)
}

func TestKickFromSpace_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		kickUserErr: errors.New("kick from space failed"),
	}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.KickFromSpace(context.Background(), "!space:test.local", "@user:test.local", "reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to kick user")
}

// ============================================================================
// autoJoinAndMarkRead
// ============================================================================

func TestAutoJoinAndMarkRead_Success(t *testing.T) {
	member1Intent := &mockIntentAPI{}
	member2Intent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{ID: "$latest1"},
			},
		},
	}
	user1ID := expectedUserID(testActorID)
	user2ID := expectedUserID(testActorID2)
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		user1ID: member1Intent,
		user2ID: member2Intent,
	})
	a := newFullTestAdapter(as, admin)

	count := a.autoJoinAndMarkRead(context.Background(), "!room:test.local", []id.UserID{user1ID, user2ID})
	assert.Equal(t, 2, count)
	assert.Equal(t, 1, member1Intent.ensureJoinedCalled)
	assert.Equal(t, 1, member2Intent.ensureJoinedCalled)
}

func TestAutoJoinAndMarkRead_PartialJoinFailures(t *testing.T) {
	member1Intent := &mockIntentAPI{}
	member2Intent := &mockIntentAPI{
		ensureJoinedErr: errors.New("join failed"),
	}
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{ID: "$latest1"},
			},
		},
	}
	user1ID := expectedUserID(testActorID)
	user2ID := expectedUserID(testActorID2)
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		user1ID: member1Intent,
		user2ID: member2Intent,
	})
	a := newFullTestAdapter(as, admin)

	count := a.autoJoinAndMarkRead(context.Background(), "!room:test.local", []id.UserID{user1ID, user2ID})
	assert.Equal(t, 1, count, "only one member should have joined successfully")
	assert.Equal(t, 1, member1Intent.ensureJoinedCalled)
	assert.Equal(t, 1, member2Intent.ensureJoinedCalled)
}

func TestAutoJoinAndMarkRead_AllJoinsFail(t *testing.T) {
	failingIntent := &mockIntentAPI{
		ensureJoinedErr: errors.New("join failed"),
	}
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{}
	user1ID := expectedUserID(testActorID)
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		user1ID: failingIntent,
	})
	a := newFullTestAdapter(as, admin)

	count := a.autoJoinAndMarkRead(context.Background(), "!room:test.local", []id.UserID{user1ID})
	assert.Equal(t, 0, count)
}

func TestAutoJoinAndMarkRead_EmptyInvites(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	count := a.autoJoinAndMarkRead(context.Background(), "!room:test.local", []id.UserID{})
	assert.Equal(t, 0, count)
}

// ============================================================================
// leaveBotIfNotNeeded
// ============================================================================

func TestLeaveBotIfNotNeeded_Leaves_WhenGhostPresent(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		// Room members include bot and a ghost user (UUID-formatted localpart)
		getRoomMembersResult: []string{
			"@bot:test.local",
			expectedUserID(testActorID).String(),
		},
		// Not a space
		getStateEventContentResult: map[string]interface{}{},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	a.leaveBotIfNotNeeded(context.Background(), "!room:test.local")
	assert.Equal(t, 1, botIntent.leaveRoomCalled, "bot should leave when ghost is present")
}

func TestLeaveBotIfNotNeeded_Stays_NoGhost(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		// Only bot and a non-ghost external user
		getRoomMembersResult: []string{
			"@bot:test.local",
			"@external-user:other.server",
		},
		getStateEventContentResult: map[string]interface{}{},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	a.leaveBotIfNotNeeded(context.Background(), "!room:test.local")
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "bot should stay when no ghost user is present")
}

func TestLeaveBotIfNotNeeded_Stays_IsSpace(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{
			"@bot:test.local",
			expectedUserID(testActorID).String(),
		},
		// This is a space room
		getStateEventContentResult: map[string]interface{}{"type": "m.space"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	a.leaveBotIfNotNeeded(context.Background(), "!space:test.local")
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "bot should stay in spaces")
}

func TestLeaveBotIfNotNeeded_Stays_BotNotMember(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		// Bot is NOT in the room
		getRoomMembersResult: []string{
			expectedUserID(testActorID).String(),
		},
		getStateEventContentResult: map[string]interface{}{},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	a.leaveBotIfNotNeeded(context.Background(), "!room:test.local")
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "bot should not try to leave if not a member")
}

func TestLeaveBotIfNotNeeded_MemberFetchError(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMembersErr:          errors.New("member fetch failed"),
		getStateEventContentResult: map[string]interface{}{},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	a.leaveBotIfNotNeeded(context.Background(), "!room:test.local")
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "should not leave on member fetch error")
}

// ============================================================================
// getIntentForRoom
// ============================================================================

func TestGetIntentForRoom_BotIsMember(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local", "@other:test.local"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	intent := a.getIntentForRoom(context.Background(), "!room:test.local")
	assert.Equal(t, botIntent, intent, "should return bot intent when bot is a member")
}

func TestGetIntentForRoom_FallbackToGhost(t *testing.T) {
	botIntent := &mockIntentAPI{}
	ghostIntent := &mockIntentAPI{}
	ghostUserID := expectedUserID(testActorID)
	admin := &mockAdminAPI{
		// Bot is NOT a member, but a ghost user is
		getRoomMembersResult: []string{
			ghostUserID.String(),
		},
		// For power levels lookup
		getStateEventContentResult: map[string]interface{}{
			"users": map[string]interface{}{
				ghostUserID.String(): float64(50),
			},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		ghostUserID: ghostIntent,
	})
	a := newFullTestAdapter(as, admin)

	intent := a.getIntentForRoom(context.Background(), "!room:test.local")
	assert.Equal(t, ghostIntent, intent, "should return ghost intent when bot is not in room")
}

func TestGetIntentForRoom_AdminJoinFallback(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		// No members at all (or only non-ghost external users)
		getRoomMembersResult: []string{"@external:other.server"},
		getStateEventContentResult: map[string]interface{}{
			"users": map[string]interface{}{},
		},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	intent := a.getIntentForRoom(context.Background(), "!room:test.local")
	assert.Equal(t, botIntent, intent, "should return bot intent as last resort after admin-join")
	assert.Equal(t, 1, len(admin.joinRoomCalls), "should admin-join the bot")
	// Verify StateStore was synced to prevent EnsureJoined from creating duplicate join
	assert.Equal(t, 1, len(as.setMembershipCalls), "should sync StateStore after admin join")
	assert.Equal(t, id.RoomID("!room:test.local"), as.setMembershipCalls[0].RoomID)
	assert.Equal(t, event.MembershipJoin, as.setMembershipCalls[0].Membership)
}

// ============================================================================
// GetAllJoinedRooms
// ============================================================================

func TestGetAllJoinedRooms_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		joinedRoomsResult: &mautrix.RespJoinedRooms{
			JoinedRooms: []id.RoomID{"!room1:test.local", "!room2:test.local"},
		},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	rooms, err := a.GetAllJoinedRooms(context.Background())
	require.NoError(t, err)
	assert.Len(t, rooms, 2)
	assert.Equal(t, id.RoomID("!room1:test.local"), rooms[0])
}

func TestGetAllJoinedRooms_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		joinedRoomsErr: errors.New("not connected"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.GetAllJoinedRooms(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get joined rooms")
}

// ============================================================================
// HomeserverDomain
// ============================================================================

func TestHomeserverDomain(t *testing.T) {
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	assert.Equal(t, "test.local", a.HomeserverDomain())
}

// ============================================================================
// GetRoomAliases
// ============================================================================

func TestGetRoomAliases_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{
			Aliases: []id.RoomAlias{"#room1:test.local", "#room2:test.local"},
		},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	aliases, err := a.GetRoomAliases(context.Background(), "!room:test.local")
	require.NoError(t, err)
	assert.Len(t, aliases, 2)
	assert.Equal(t, "#room1:test.local", aliases[0])
}

func TestGetRoomAliases_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		getAliasesErr: errors.New("alias lookup failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.GetRoomAliases(context.Background(), "!room:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get aliases for room")
}

// ============================================================================
// UpdateSpaceState
// ============================================================================

func TestUpdateSpaceState_AllFields(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	name := "Space Name"
	topic := "Space Topic"
	avatar := "mxc://test.local/avatar"
	joinRule := "public"
	err := a.UpdateSpaceState(context.Background(), "!space:test.local", &name, &topic, &avatar, &joinRule)
	require.NoError(t, err)
	// name + topic + avatar each call setOrRedactState (sendStateEvent), plus join rule
	assert.Equal(t, 4, botIntent.sendStateEventCalled)
}

func TestUpdateSpaceState_NilFields(t *testing.T) {
	botIntent := &mockIntentAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.UpdateSpaceState(context.Background(), "!space:test.local", nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, botIntent.sendStateEventCalled)
}

func TestUpdateSpaceState_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventErr: errors.New("state error"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	name := "Name"
	err := a.UpdateSpaceState(context.Background(), "!space:test.local", &name, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set space name")
}

// ============================================================================
// Disconnect
// ============================================================================

func TestDisconnect(t *testing.T) {
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.Disconnect()
	require.NoError(t, err)
}

// ============================================================================
// SetRoomDirectoryVisibility (private)
// ============================================================================

func TestSetRoomDirectoryVisibility_Private(t *testing.T) {
	botIntent := &mockIntentAPI{
		buildClientURLResult: "https://test.local/_matrix/client/v3/directory/list/room/!room:test.local",
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.SetRoomDirectoryVisibility(context.Background(), "!room:test.local", false)
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.makeRequestCalled)
}

// ============================================================================
// GetRoomDetails (uses admin + intent for aliases)
// ============================================================================

func TestGetRoomDetails_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{
			Aliases: []id.RoomAlias{
				id.RoomAlias("#" + testActorID.String() + ":test.local"),
			},
		},
	}
	admin := &mockAdminAPI{
		getStateEventContentResult: map[string]interface{}{
			"name": "Test Room",
		},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	room, err := a.GetRoomDetails(context.Background(), "!room:test.local")
	require.NoError(t, err)
	require.NotNil(t, room)
	assert.Equal(t, id.RoomID("!room:test.local"), room.ID)
}

// ============================================================================
// CreateRoomWithAlias with joinRule
// ============================================================================

func TestCreateRoomWithAlias_WithJoinRule(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
	}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	roomID, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "community",
		"Room", "", "", "invite", nil, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!newroom:test.local"), roomID)
	require.NotNil(t, botIntent.lastCreateRoomReq)
	assert.Equal(t, "private_chat", botIntent.lastCreateRoomReq.Preset, "non-direct with joinRule should use private_chat preset")
}

func TestCreateRoomWithAlias_WithAvatar(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
	}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	roomID, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "community",
		"Room", "", "mxc://test.local/avatar", "", nil, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!newroom:test.local"), roomID)
	require.NotNil(t, botIntent.lastCreateRoomReq)
	assert.NotEmpty(t, botIntent.lastCreateRoomReq.InitialState, "should have initial state with avatar")
}

func TestCreateRoomWithAlias_WithCustomState(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
	}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	customState := map[string]map[string]interface{}{
		"io.alkemio.meta": {"version": "1"},
	}
	roomID, err := a.CreateRoomWithAlias(
		context.Background(), alkemioRoomID, "community",
		"Room", "", "", "", customState, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!newroom:test.local"), roomID)
	require.NotNil(t, botIntent.lastCreateRoomReq)
	assert.NotEmpty(t, botIntent.lastCreateRoomReq.InitialState, "should have initial state with custom events")
}

// ============================================================================
// GetSpaceChildren (uses intent.State)
// ============================================================================

func TestGetSpaceChildren_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		stateResult: mautrix.RoomStateMap{
			event.StateSpaceChild: {
				"!child1:test.local": &event.Event{
					Content: event.Content{
						Parsed: &event.SpaceChildEventContent{
							Via:       []string{"test.local"},
							Order:     "01",
							Suggested: true,
						},
					},
				},
			},
		},
	}
	admin := &mockAdminAPI{
		// For isSpaceRoom check on the child
		getStateEventContentResult: map[string]interface{}{},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	children, err := a.GetSpaceChildren(context.Background(), "!space:test.local")
	require.NoError(t, err)
	assert.Len(t, children, 1)
	assert.Equal(t, "!child1:test.local", children[0].ChildID)
	assert.Equal(t, "01", children[0].Order)
	assert.True(t, children[0].Suggested)
}

func TestGetSpaceChildren_StateError(t *testing.T) {
	botIntent := &mockIntentAPI{
		stateErr: errors.New("state fetch failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.GetSpaceChildren(context.Background(), "!space:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get space state")
}

func TestGetSpaceChildren_NoChildren(t *testing.T) {
	botIntent := &mockIntentAPI{
		stateResult: mautrix.RoomStateMap{},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	children, err := a.GetSpaceChildren(context.Background(), "!space:test.local")
	require.NoError(t, err)
	assert.Empty(t, children)
}

// ============================================================================
// setOrRedactState
// ============================================================================

func TestSetOrRedactState_NonEmptyValue_SetsState(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	err := a.setOrRedactState(context.Background(), intent, "!room:test.local", event.StateRoomName, "New Name")
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendStateEventCalled)
	content, ok := intent.lastSendStateEventContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "New Name", content["name"])
}

func TestSetOrRedactState_EmptyValue_ClearsState(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	err := a.setOrRedactState(context.Background(), intent, "!room:test.local", event.StateRoomName, "")
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendStateEventCalled)
	content, ok := intent.lastSendStateEventContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "", content["name"])
}

func TestSetOrRedactState_Topic(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	err := a.setOrRedactState(context.Background(), intent, "!room:test.local", event.StateTopic, "My Topic")
	require.NoError(t, err)
	content, ok := intent.lastSendStateEventContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "My Topic", content["topic"])
}

func TestSetOrRedactState_Avatar(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$state1"},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	err := a.setOrRedactState(context.Background(), intent, "!room:test.local", event.StateRoomAvatar, "mxc://test/avatar")
	require.NoError(t, err)
	assert.Equal(t, 1, intent.sendStateEventCalled)
}

func TestSetOrRedactState_Error(t *testing.T) {
	intent := &mockIntentAPI{
		sendStateEventErr: errors.New("state error"),
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	err := a.setOrRedactState(context.Background(), intent, "!room:test.local", event.StateRoomName, "Name")
	require.Error(t, err)
}

// ============================================================================
// findGhostIntentInRoom
// ============================================================================

func TestFindGhostIntentInRoom_FindsHighestPL(t *testing.T) {
	ghostUserID1 := expectedUserID(testActorID)
	ghostUserID2 := expectedUserID(testActorID2)
	ghost1Intent := &mockIntentAPI{}
	ghost2Intent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{
			"@bot:test.local",
			ghostUserID1.String(),
			ghostUserID2.String(),
		},
		getStateEventContentResult: map[string]interface{}{
			"users": map[string]interface{}{
				ghostUserID1.String(): float64(50),
				ghostUserID2.String(): float64(100),
			},
		},
	}
	as := newMockAS(&mockIntentAPI{}, map[id.UserID]intentAPI{
		ghostUserID1: ghost1Intent,
		ghostUserID2: ghost2Intent,
	})
	a := newFullTestAdapter(as, admin)

	intent, pl := a.findGhostIntentInRoom(context.Background(), "!room:test.local")
	assert.Equal(t, ghost2Intent, intent, "should return ghost with highest power level")
	assert.Equal(t, float64(100), pl)
}

func TestFindGhostIntentInRoom_NoGhosts(t *testing.T) {
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{"@bot:test.local", "@external:other.server"},
		getStateEventContentResult: map[string]interface{}{
			"users": map[string]interface{}{},
		},
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	intent, pl := a.findGhostIntentInRoom(context.Background(), "!room:test.local")
	assert.Nil(t, intent, "should return nil when no ghost users")
	assert.Equal(t, float64(-1), pl)
}

func TestFindGhostIntentInRoom_MemberFetchError(t *testing.T) {
	admin := &mockAdminAPI{
		getRoomMembersErr: errors.New("fetch error"),
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	intent, pl := a.findGhostIntentInRoom(context.Background(), "!room:test.local")
	assert.Nil(t, intent)
	assert.Equal(t, float64(-1), pl)
}

func TestGetIntentForRoom_LowPLGhost_AdminJoinFallback(t *testing.T) {
	botIntent := &mockIntentAPI{}
	ghostIntent := &mockIntentAPI{}
	ghostUserID := expectedUserID(testActorID)
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{
			ghostUserID.String(),
		},
		getStateEventContentResult: map[string]interface{}{
			"users": map[string]interface{}{
				ghostUserID.String(): float64(10),
			},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		ghostUserID: ghostIntent,
	})
	a := newFullTestAdapter(as, admin)

	intent := a.getIntentForRoom(context.Background(), "!room:test.local")
	assert.Equal(t, botIntent, intent, "should admin-join bot when ghost PL < 50")
	assert.Len(t, admin.joinRoomCalls, 1)
}

func TestGetIntentForRoom_CustomMinPL(t *testing.T) {
	botIntent := &mockIntentAPI{}
	ghostIntent := &mockIntentAPI{}
	ghostUserID := expectedUserID(testActorID)
	admin := &mockAdminAPI{
		getRoomMembersResult: []string{
			ghostUserID.String(),
		},
		getStateEventContentResult: map[string]interface{}{
			"users": map[string]interface{}{
				ghostUserID.String(): float64(10),
			},
		},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		ghostUserID: ghostIntent,
	})
	a := newFullTestAdapter(as, admin)

	// Ghost has PL 10 — sufficient if caller only needs PL 0
	intent := a.getIntentForRoom(context.Background(), "!room:test.local", 0)
	assert.Equal(t, ghostIntent, intent, "should use ghost when PL >= minPL")
	assert.Len(t, admin.joinRoomCalls, 0, "should not admin-join bot")
}
