package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

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
	ensureJoinedErrQueue   []error // consumed first, one per call
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

	// Optional per-call results for fan-out failure tests.
	sendMessageEventResults []*mautrix.RespSendEvent
	sendMessageEventErrs    []error

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
	redactEventIDs             []id.EventID
	redactReasons              []string
	redactEventErrs            map[id.EventID]error
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
	// lastSendMsgEventDeadline is the deadline carried by the context handed to
	// SendMessageEvent, zero when it carries none. The AMQP/watermill message
	// context has NO deadline, so on the media path a non-zero value here can
	// only come from fanOutAttachments' whole-message budget.
	lastSendMsgEventDeadline time.Time
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
	sendStateEventTypes       []event.Type
	sendStateEventContents    []any
	buildClientURLResult      string

	// Media
	uploadBytesCalled   int
	lastUploadBytesData []byte
	lastUploadBytesType string
	// lastUploadContentLength is the raw ReqUploadMedia.ContentLength field.
	// It is NOT what goes on the wire — see lastUploadWireContentLength.
	lastUploadContentLength int64
	// lastUploadWireContentLength is the net/http request ContentLength mautrix
	// would actually derive from this ReqUploadMedia (-1 = chunked, i.e. NO
	// Content-Length header). Asserting the raw field instead lets a request that
	// Synapse rejects ("Request must specify a Content-Length") pass vacuously.
	lastUploadWireContentLength int64
	// Which ReqUploadMedia branch the caller used. The project's hard
	// requirement is that blob bytes STREAM (Content, an io.Reader) with
	// constant memory and are never buffered whole (ContentBytes), so tests
	// must be able to assert the branch — folding both into
	// lastUploadBytesData alone would let a re-introduced io.ReadAll pass.
	lastUploadContentWasReader bool
	lastUploadUsedContentBytes bool
}

// mautrixWireContentLength reproduces mautrix v0.28.0's mapping from
// ReqUploadMedia to the net/http request's ContentLength — the ONLY thing that
// decides whether the upload carries a "Content-Length" header or goes out
// chunked (which Synapse rejects outright).
//
// Client.UploadMedia builds FullRequest{RequestBytes: ContentBytes,
// RequestBody: Content, RequestLength: ContentLength} and
// FullRequest.compileRequest then:
//   - checks RequestBytes FIRST and, when non-nil, sets reqLen = len(RequestBytes)
//     (so an EMPTY non-nil slice yields a real 0);
//   - otherwise, on the RequestBody branch, starts at reqLen = -1 and overrides it
//     only when RequestLength > 0 — RequestLength == 0 just logs a warning, so the
//     request goes out with NO Content-Length.
//
// TestUploadRequestWireLength_MautrixContract pins this model against the real
// mautrix + net/http stack, so it cannot silently drift from the library.
func mautrixWireContentLength(req mautrix.ReqUploadMedia) int64 {
	switch {
	case req.ContentBytes != nil:
		return int64(len(req.ContentBytes))
	case req.Content != nil:
		if req.ContentLength > 0 {
			return req.ContentLength
		}
		return -1
	default:
		return 0
	}
}

var _ intentAPI = (*mockIntentAPI)(nil)

func (m *mockIntentAPI) EnsureRegistered(_ context.Context) error {
	m.ensureRegisteredCalled++
	return m.ensureRegisteredErr
}

func (m *mockIntentAPI) EnsureJoined(_ context.Context, roomID id.RoomID, _ ...appservice.EnsureJoinedParams) error {
	m.ensureJoinedCalled++
	m.lastEnsureJoinedRoomID = roomID
	if len(m.ensureJoinedErrQueue) > 0 {
		err := m.ensureJoinedErrQueue[0]
		m.ensureJoinedErrQueue = m.ensureJoinedErrQueue[1:]
		return err
	}
	return m.ensureJoinedErr
}

func (m *mockIntentAPI) SendMessageEvent(ctx context.Context, roomID id.RoomID, eventType event.Type, contentJSON any, extra ...mautrix.ReqSendEvent) (*mautrix.RespSendEvent, error) {
	callIndex := m.sendMessageEventCalled
	m.sendMessageEventCalled++
	m.lastSendMsgEventDeadline, _ = ctx.Deadline()
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
	result, err := m.sendMessageEventResult, m.sendMessageEventErr
	if callIndex < len(m.sendMessageEventResults) {
		result = m.sendMessageEventResults[callIndex]
	}
	if callIndex < len(m.sendMessageEventErrs) {
		err = m.sendMessageEventErrs[callIndex]
	}
	return result, err
}

func (m *mockIntentAPI) SendStateEvent(_ context.Context, roomID id.RoomID, eventType event.Type, stateKey string, contentJSON any, _ ...mautrix.ReqSendEvent) (*mautrix.RespSendEvent, error) {
	m.sendStateEventCalled++
	m.lastSendStateEventRoomID = roomID
	m.lastSendStateEventType = eventType
	m.lastSendStateEventStateKey = stateKey
	m.lastSendStateEventContent = contentJSON
	m.sendStateEventTypes = append(m.sendStateEventTypes, eventType)
	m.sendStateEventContents = append(m.sendStateEventContents, contentJSON)
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

func (m *mockIntentAPI) RedactEvent(
	_ context.Context, roomID id.RoomID, eventID id.EventID, extra ...mautrix.ReqRedact,
) (*mautrix.RespSendEvent, error) {
	m.redactEventCalled++
	m.lastRedactRoomID = roomID
	m.lastRedactEventID = eventID
	m.redactEventIDs = append(m.redactEventIDs, eventID)
	if len(extra) > 0 {
		m.redactReasons = append(m.redactReasons, extra[0].Reason)
	}
	if err := m.redactEventErrs[eventID]; err != nil {
		return m.redactEventResult, err
	}
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

// UploadMedia drains req.Content into the mock's captured data. A read error
// from the streaming reader (e.g. the oversize cap tripping) is surfaced so
// sendAttachment can classify it. The branch actually taken is recorded so
// tests can assert the bytes STREAMED rather than being buffered.
//
// It reproduces the two ways a bad upload actually fails in production, both
// keyed off the WIRE length mautrix derives (mautrixWireContentLength), never
// the raw ReqUploadMedia.ContentLength field:
//   - a declared length that disagrees with the body → net/http's
//     "http: ContentLength=%d with Body length %d";
//   - no Content-Length at all (chunked) → Synapse's M_UNKNOWN 400.
//
// Asserting the raw field instead would let a request Synapse rejects outright
// pass here vacuously.
func (m *mockIntentAPI) UploadMedia(_ context.Context, req mautrix.ReqUploadMedia) (*mautrix.RespMediaUpload, error) {
	m.uploadBytesCalled++
	m.lastUploadBytesType = req.ContentType
	m.lastUploadContentLength = req.ContentLength
	wireLen := mautrixWireContentLength(req)
	m.lastUploadWireContentLength = wireLen
	m.lastUploadContentWasReader = req.Content != nil
	m.lastUploadUsedContentBytes = req.ContentBytes != nil
	// compileRequest prefers RequestBytes, so a non-nil ContentBytes is what goes
	// on the wire even if Content is also set.
	if req.ContentBytes == nil && req.Content != nil {
		data, err := io.ReadAll(req.Content)
		m.lastUploadBytesData = data
		if err != nil {
			return nil, err
		}
		// Mirror net/http's transfer-length contract, which is how a declared
		// length that disagrees with the body actually surfaces in production:
		// the transport fails the request with
		// "http: ContentLength=%d with Body length %d" (and, when the body is
		// SHORT, only after the homeserver has already stored the truncated
		// prefix). Reproducing it here keeps "declared length must be the true
		// length" an enforced contract instead of an untested comment.
		if wireLen >= 0 && int64(len(data)) != wireLen {
			return nil, fmt.Errorf("http: ContentLength=%d with Body length %d", wireLen, len(data))
		}
	} else {
		m.lastUploadBytesData = req.ContentBytes
	}
	// A negative wire length means mautrix sends the upload CHUNKED with no
	// Content-Length header, which Synapse refuses. Reproduce the real rejection
	// rather than accepting an upload production would never have completed.
	if wireLen < 0 {
		return nil, fmt.Errorf("M_UNKNOWN (HTTP 400): Request must specify a Content-Length")
	}
	if m.uploadBytesErr != nil {
		return nil, m.uploadBytesErr
	}
	if m.uploadBytesResult != nil {
		return m.uploadBytesResult, nil
	}
	return &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/stub")}, nil
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

// testIDMapper is the package-wide test IDMapper. Its domain matches every
// fixture mxc:// url and Matrix user id in these tests ("test.local"), so those
// references count as LOCAL to our homeserver.
var testIDMapper = domain.NewIDMapper("test.local")

// expectedUserID returns the Matrix user ID for a test actor UUID.
func expectedUserID(actorID uuid.UUID) id.UserID {
	return testIDMapper.UserID(actorID)
}

// adminWithBot returns an admin mock where the bot is a joined, power-100
// member of every room — the precondition every governed bot write needs.
func adminWithBot(members ...id.UserID) *mockAdminAPI {
	return &mockAdminAPI{
		getRoomMemberIDsResult: append([]id.UserID{"@bot:test.local"}, members...),
		getStateEventContentResult: map[string]interface{}{
			"users": map[string]interface{}{"@bot:test.local": float64(100)},
		},
	}
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
		sendTextResult: &mautrix.RespSendEvent{EventID: "$msg1"},
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	eventID, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "Hello", nil)
	require.NoError(t, err)
	assert.Equal(t, "$msg1", eventID.ID)
	assert.Equal(t, 1, intent.sendTextCalled)
	assert.Equal(t, 0, intent.sendMessageEventCalled)
	assert.Equal(t, id.RoomID("!room:test.local"), intent.lastSendTextRoomID)
	assert.Equal(t, "Hello", intent.lastSendTextContent)
}

func TestSendMessage_Error(t *testing.T) {
	intent := &mockIntentAPI{
		sendTextErr: errors.New("send failed"),
	}
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "Hello", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, intent.sendTextErr)
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

	eventID, err := a.SendReply(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "reply text", "$thread-root", nil)
	require.NoError(t, err)
	assert.Equal(t, "$reply1", eventID.ID)
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

	_, err := a.SendReply(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), "reply", "$thread", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, intent.sendMessageEventErr)
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

func TestKickUser_UsesBot(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := adminWithBot("@baduser:test.local")
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.KickUser(context.Background(), "!room:test.local", "@baduser:test.local", "rule violation")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.kickUserCalled, "kick must be performed by the bot")
	assert.Equal(t, id.RoomID("!room:test.local"), botIntent.lastKickUserRoomID)
	assert.Equal(t, id.UserID("@baduser:test.local"), botIntent.lastKickUserReq.UserID)
	assert.Equal(t, "rule violation", botIntent.lastKickUserReq.Reason)
	// No ghost intent was ever requested: mockAppserviceAPI.Intent panics on
	// unmapped ids and this test maps none.
}

func TestKickUser_AlreadyLeft_NoOp(t *testing.T) {
	botIntent := &mockIntentAPI{
		kickUserErr: errors.New("M_FORBIDDEN: target not in room"),
	}
	// The target is NOT among the members — the failed kick is a no-op success.
	admin := adminWithBot()
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	err := a.KickUser(context.Background(), "!room:test.local", "@gone:test.local", "cleanup")
	require.NoError(t, err, "kicking a user who already left must succeed as a no-op")
	assert.Equal(t, 1, botIntent.kickUserCalled)
}

func TestKickUser_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		kickUserErr: errors.New("kick failed"),
	}
	// The target IS still a member — the failure is real and surfaces.
	admin := adminWithBot("@user:test.local")
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
		leaveRoomErr: nil,
	}
	admin := &mockAdminAPI{
		// For getLatestEventID
		getRoomMessagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{ID: "$latest1"},
			},
		},
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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
	admin := adminWithBot()
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

	roomID, err := a.CreateSpace(
		context.Background(),
		domain.CreateSpaceParams{AlkemioContextID: contextID, Name: "Space Name", Topic: "Space Topic", JoinRule: "invite", InitialMembers: members},
	)
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
	_, err := a.CreateSpace(
		context.Background(),
		domain.CreateSpaceParams{AlkemioContextID: contextID, Name: "Space", JoinRule: "invite"},
	)
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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "community", Name: "Room Name", Topic: "Room Topic", InitialMembers: members},
	)
	require.NoError(t, err)
	assert.Equal(t, id.RoomID("!newroom:test.local"), roomID)
	assert.Equal(t, 1, botIntent.createRoomCalled)
	assert.Equal(t, 1, botIntent.createAliasCalled, "should set the room alias")
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "the bot never leaves governed rooms")
}

func TestCreateRoomWithAlias_CreateError(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomErr: errors.New("create room failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	_, err := a.CreateRoomWithAlias(
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "community", Name: "Room"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create room with alias")
}

func TestCreateRoomWithAlias_ThreadAliasError_Divergence(t *testing.T) {
	// The canonical #<uuid> alias is atomic (RoomAliasName in the create
	// request); only the best-effort #t_<uuid> alias goes through CreateAlias.
	// Its failure (e.g. the registration namespace not yet rolled out) is a
	// recorded divergence, never a creation failure (spec edge case).
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
		createAliasErr:   errors.New("M_EXCLUSIVE: alias not in namespace"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	roomID, err := a.CreateRoomWithAlias(
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "community", Name: "Room"},
	)
	require.NoError(t, err, "a refused #t_ alias must not fail the creation")
	assert.Equal(t, id.RoomID("!newroom:test.local"), roomID)
	assert.Equal(t, 1, botIntent.createAliasCalled, "the #t_ alias was attempted")
	assert.Equal(t, id.RoomAlias("#t_880e8400-e29b-41d4-a716-446655440003:test.local"), botIntent.lastCreateAliasAlias)
}

func TestCreateRoomWithAlias_BotStays(t *testing.T) {
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!newroom:test.local"},
	}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	alkemioRoomID := uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	roomID, err := a.CreateRoomWithAlias(
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "community", Name: "Room"},
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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "direct"},
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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "direct", InitialMembers: members},
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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "direct", InitialMembers: members},
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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "direct", InitialMembers: members},
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

func TestSetSpaceParent_UsesBot(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$parent1"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, adminWithBot())

	err := a.SetSpaceParent(context.Background(), "!child:test.local", "!parent:test.local")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.sendStateEventCalled, "m.space.parent must be written by the bot")
	assert.Equal(t, event.StateSpaceParent, botIntent.lastSendStateEventType)
	assert.Equal(t, "!parent:test.local", botIntent.lastSendStateEventStateKey)
}

func TestSetSpaceParent_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventErr: errors.New("state denied"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, adminWithBot())

	err := a.SetSpaceParent(context.Background(), "!child:test.local", "!parent:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set space parent")
}

// ============================================================================
// GetSpaceParents
// ============================================================================

// spaceParentStateJSON builds one raw m.space.parent state event as the
// Synapse admin API would return it, for GetSpaceParents fixtures.
func spaceParentStateJSON(t *testing.T, stateKey string, via []string, canonical bool) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{
		"type":      "m.space.parent",
		"state_key": stateKey,
		"content": map[string]interface{}{
			"via":       via,
			"canonical": canonical,
		},
	})
	require.NoError(t, err)
	return raw
}

func TestGetSpaceParents_ReturnsCurrentCanonicalParent(t *testing.T) {
	botIntent := &mockIntentAPI{}
	as := newMockAS(botIntent, nil)
	admin := &mockAdminAPI{
		getRoomStateResult: []json.RawMessage{
			spaceParentStateJSON(t, "!parent:test.local", []string{"test.local"}, true),
		},
	}
	a := newFullTestAdapter(as, admin)

	parents, err := a.GetSpaceParents(context.Background(), "!child:test.local")
	require.NoError(t, err)
	assert.Equal(t, []id.RoomID{"!parent:test.local"}, parents)
	// The read must never touch a client intent — it must issue no join.
	assert.Equal(t, 0, botIntent.ensureJoinedCalled)
	assert.Equal(t, 0, botIntent.sendStateEventCalled)
}

func TestGetSpaceParents_ReturnsEveryLiveParentForADualCanonicalChild(t *testing.T) {
	// A child can carry more than one live m.space.parent pointer — a
	// pre-existing dual-canonical-parent violation from before it was
	// recategorised. Repair needs to see the whole set, not an arbitrary
	// single member of it.
	admin := &mockAdminAPI{
		getRoomStateResult: []json.RawMessage{
			spaceParentStateJSON(t, "!old-parent:test.local", []string{"test.local"}, true),
			spaceParentStateJSON(t, "!new-parent:test.local", []string{"test.local"}, true),
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	parents, err := a.GetSpaceParents(context.Background(), "!child:test.local")
	require.NoError(t, err)
	assert.ElementsMatch(t, []id.RoomID{"!old-parent:test.local", "!new-parent:test.local"}, parents)
}

func TestGetSpaceParents_EmptyViaIsNotACurrentParent(t *testing.T) {
	// Empty via is the MSC1772 removal marker (mirrors GetSpaceChildStateKeys'
	// treatment of a removed m.space.child edge) — a stale, cleared pointer
	// must not be reported as a live parent.
	admin := &mockAdminAPI{
		getRoomStateResult: []json.RawMessage{
			spaceParentStateJSON(t, "!old-parent:test.local", []string{}, false),
		},
	}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	parents, err := a.GetSpaceParents(context.Background(), "!child:test.local")
	require.NoError(t, err)
	assert.Empty(t, parents)
}

func TestGetSpaceParents_NoParentSet(t *testing.T) {
	admin := &mockAdminAPI{getRoomStateResult: []json.RawMessage{}}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	parents, err := a.GetSpaceParents(context.Background(), "!child:test.local")
	require.NoError(t, err)
	assert.Empty(t, parents)
}

func TestGetSpaceParents_StateError(t *testing.T) {
	admin := &mockAdminAPI{getRoomStateErr: errors.New("state fetch failed")}
	a := newFullTestAdapter(newMockAS(&mockIntentAPI{}, nil), admin)

	_, err := a.GetSpaceParents(context.Background(), "!child:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get room state for parent pointers")
}

// ============================================================================
// ClearSpaceParent
// ============================================================================

func TestClearSpaceParent_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$cleared1"},
	}
	admin := adminWithBot()
	a := newFullTestAdapter(newMockAS(botIntent, nil), admin)

	err := a.ClearSpaceParent(context.Background(), "!child:test.local", "!stale-parent:test.local")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.sendStateEventCalled)
	assert.Equal(t, event.StateSpaceParent, botIntent.lastSendStateEventType)
	assert.Equal(t, "!stale-parent:test.local", botIntent.lastSendStateEventStateKey)

	content, ok := botIntent.lastSendStateEventContent.(*event.SpaceParentEventContent)
	require.True(t, ok)
	assert.Empty(t, content.Via)
}

func TestClearSpaceParent_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventErr: errors.New("clear parent failed"),
	}
	admin := adminWithBot()
	a := newFullTestAdapter(newMockAS(botIntent, nil), admin)

	err := a.ClearSpaceParent(context.Background(), "!child:test.local", "!stale-parent:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clear stale space parent pointer")
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

func TestKickFromSpace_UsesBot(t *testing.T) {
	botIntent := &mockIntentAPI{}
	admin := adminWithBot()
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
	// The target is still a member, so the failed kick is a real error.
	admin := adminWithBot("@user:test.local")
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
// Bot stays in rooms (069: the bot never leaves governed rooms)
// ============================================================================

func TestBotStaysInRoom_AfterCreate(t *testing.T) {
	memberIntent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{
		createRoomResult: &mautrix.RespCreateRoom{RoomID: "!stay:test.local"},
	}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{Chunk: []*event.Event{{ID: "$latest"}}},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): memberIntent,
	})
	a := newFullTestAdapter(as, admin)

	_, err := a.CreateRoomWithAlias(
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: uuid.MustParse("880e8400-e29b-41d4-a716-446655440003"), RoomType: "community", Name: "Room", InitialMembers: []domain.Actor{testActor(testActorID, "Alice")}},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "the bot never leaves governed rooms")
}

func TestBotStaysInRoom_AfterInvite(t *testing.T) {
	inviteeIntent := &mockIntentAPI{}
	botIntent := &mockIntentAPI{}
	admin := &mockAdminAPI{
		getRoomMessagesResult: &mautrix.RespMessages{Chunk: []*event.Event{{ID: "$latest"}}},
	}
	as := newMockAS(botIntent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): inviteeIntent,
	})
	a := newFullTestAdapter(as, admin)

	err := a.InviteUser(context.Background(), "!room:test.local", testActor(testActorID, "Invitee"))
	require.NoError(t, err)
	assert.Equal(t, 0, botIntent.leaveRoomCalled, "adding a member must not make the bot leave")
	assert.Equal(t, 1, inviteeIntent.ensureJoinedCalled)
}

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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "community", Name: "Room", JoinRule: "invite"},
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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "community", Name: "Room", AvatarURL: "mxc://test.local/avatar"},
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
		context.Background(),
		domain.CreateRoomParams{AlkemioRoomID: alkemioRoomID, RoomType: "community", Name: "Room", CustomState: customState},
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
// GetSpaceChildStateKeys (uses intent.State — no per-child admin call)
// ============================================================================

func TestGetSpaceChildStateKeys_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		stateResult: mautrix.RoomStateMap{
			event.StateSpaceChild: {
				"!live:test.local": &event.Event{
					Content: event.Content{
						Parsed: &event.SpaceChildEventContent{Via: []string{"test.local"}},
					},
				},
				"!ghost:test.local": &event.Event{
					// Empty via means this edge has already been removed (MSC1772) —
					// GetSpaceChildStateKeys must ignore it, exactly like GetSpaceChildren.
					Content: event.Content{
						Parsed: &event.SpaceChildEventContent{Via: []string{}},
					},
				},
			},
		},
	}
	admin := &mockAdminAPI{}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, admin)

	keys, err := a.GetSpaceChildStateKeys(context.Background(), "!space:test.local")
	require.NoError(t, err)
	assert.Equal(t, []string{"!live:test.local"}, keys)
	// No per-child admin lookup: unlike GetSpaceChildren, this call makes no
	// is-space classification request.
	assert.Equal(t, 0, admin.getStateEventContentCalled)
}

func TestGetSpaceChildStateKeys_StateError(t *testing.T) {
	botIntent := &mockIntentAPI{
		stateErr: errors.New("state fetch failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	_, err := a.GetSpaceChildStateKeys(context.Background(), "!space:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get space state")
}

func TestGetSpaceChildStateKeys_NoChildren(t *testing.T) {
	botIntent := &mockIntentAPI{
		stateResult: mautrix.RoomStateMap{},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	keys, err := a.GetSpaceChildStateKeys(context.Background(), "!space:test.local")
	require.NoError(t, err)
	assert.Empty(t, keys)
}

// ============================================================================
// RemoveSpaceChild
// ============================================================================

func TestRemoveSpaceChild_Success(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventResult: &mautrix.RespSendEvent{EventID: "$removed1"},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.RemoveSpaceChild(context.Background(), "!space:test.local", "!child:test.local")
	require.NoError(t, err)
	assert.Equal(t, 1, botIntent.sendStateEventCalled)
	assert.Equal(t, event.StateSpaceChild, botIntent.lastSendStateEventType)
	assert.Equal(t, "!child:test.local", botIntent.lastSendStateEventStateKey)

	content, ok := botIntent.lastSendStateEventContent.(*event.SpaceChildEventContent)
	require.True(t, ok)
	assert.Empty(t, content.Via, "empty via is the MSC1772 removal marker")
	assert.Empty(t, content.Order)
	assert.False(t, content.Suggested)
}

func TestRemoveSpaceChild_Error(t *testing.T) {
	botIntent := &mockIntentAPI{
		sendStateEventErr: errors.New("state event failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	err := a.RemoveSpaceChild(context.Background(), "!space:test.local", "!child:test.local")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove space child")
}

// ============================================================================
// ResolveAlkemioID
// ============================================================================

func TestResolveAlkemioID_ResolvesFromAlkemioPatternedAlias(t *testing.T) {
	alkemioID := uuid.New()
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{
			Aliases: []id.RoomAlias{id.RoomAlias("#" + alkemioID.String() + ":test.local")},
		},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	got, err := a.ResolveAlkemioID(context.Background(), "!child:test.local")
	require.NoError(t, err)
	assert.Equal(t, alkemioID, got)
}

func TestResolveAlkemioID_GhostEdgeHasNoAliasReturnsNil(t *testing.T) {
	// A deleted discussion's room has had its alias removed: GetAliases returns
	// none, exactly the state a ghost child edge is left in. That is a
	// confirmed answer, not a failure — nil error — because it is the only
	// answer allowed to drive a prune.
	botIntent := &mockIntentAPI{
		getAliasesResult: &mautrix.RespAliasList{Aliases: []id.RoomAlias{}},
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	got, err := a.ResolveAlkemioID(context.Background(), "!ghost:test.local")
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, got)
}

// TestResolveAlkemioID_LookupErrorIsReportedNotFlattenedToNil pins the
// distinction the prune path depends on: a failed alias lookup must not be
// indistinguishable from a room confirmed to have no alias, or a transient
// homeserver fault would present itself to the reconciler as proof that a
// live child edge is a prunable ghost.
func TestResolveAlkemioID_LookupErrorIsReportedNotFlattenedToNil(t *testing.T) {
	botIntent := &mockIntentAPI{
		getAliasesErr: errors.New("lookup failed"),
	}
	as := newMockAS(botIntent, nil)
	a := newFullTestAdapter(as, &mockAdminAPI{})

	got, err := a.ResolveAlkemioID(context.Background(), "!child:test.local")
	require.Error(t, err)
	assert.Equal(t, uuid.Nil, got)
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
// (ghost-intent fallback deleted in 069 — every governed write is bot-only)
// ============================================================================
