package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

// ---------------------------------------------------------------------------
// Mock: arMockMatrixPort — covers only methods used by ActorService and
// ReadReceiptService; every other MatrixPort method is a no-op stub.
// ---------------------------------------------------------------------------

type arMockMatrixPort struct {
	// EnsureUser behaviour
	ensureUserResult id.UserID
	ensureUserErr    error
	ensureUserCalled bool

	// SetUserProfile behaviour
	setUserProfileErr    error
	setUserProfileCalled bool

	// SendReadReceipt behaviour
	sendReadReceiptErr    error
	sendReadReceiptCalled bool
	sendReadReceiptActor  domain.Actor
	sendReadReceiptRoom   id.RoomID
	sendReadReceiptEvent  id.EventID
	sendReadReceiptThread *id.EventID

	// GetUnreadCounts behaviour
	getUnreadCountsResult *domain.UnreadCountSummary
	getUnreadCountsErr    error
	getUnreadCountsCalled bool
}

// --- Methods under test ---------------------------------------------------

func (m *arMockMatrixPort) EnsureUser(_ context.Context, _ domain.Actor) (id.UserID, error) {
	m.ensureUserCalled = true
	return m.ensureUserResult, m.ensureUserErr
}

func (m *arMockMatrixPort) SetUserProfile(_ context.Context, _ domain.Actor) error {
	m.setUserProfileCalled = true
	return m.setUserProfileErr
}

func (m *arMockMatrixPort) SendReadReceipt(_ context.Context, actor domain.Actor, roomID id.RoomID, eventID id.EventID, threadRootID *id.EventID) error {
	m.sendReadReceiptCalled = true
	m.sendReadReceiptActor = actor
	m.sendReadReceiptRoom = roomID
	m.sendReadReceiptEvent = eventID
	m.sendReadReceiptThread = threadRootID
	return m.sendReadReceiptErr
}

func (m *arMockMatrixPort) GetUnreadCounts(_ context.Context, _ domain.Actor, _ id.RoomID, _ []id.EventID) (*domain.UnreadCountSummary, error) {
	m.getUnreadCountsCalled = true
	return m.getUnreadCountsResult, m.getUnreadCountsErr
}

// --- No-op stubs for the rest of the MatrixPort interface ------------------

func (m *arMockMatrixPort) Connect(_ context.Context) error { return nil }
func (m *arMockMatrixPort) Disconnect() error               { return nil }
func (m *arMockMatrixPort) HomeserverDomain() string        { return "test.local" }
func (m *arMockMatrixPort) CreateRoomWithAlias(_ context.Context, _ uuid.UUID, _, _, _, _, _ string, _ map[string]map[string]interface{}, _ []domain.Actor) (id.RoomID, error) {
	return "", nil
}
func (m *arMockMatrixPort) InviteUser(_ context.Context, _ id.RoomID, _ domain.Actor, _ domain.Actor) error {
	return nil
}
func (m *arMockMatrixPort) GetRoomDetails(_ context.Context, _ id.RoomID) (*domain.Room, error) {
	return nil, nil
}
func (m *arMockMatrixPort) GetRoomMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return nil, nil
}
func (m *arMockMatrixPort) UpdateRoomState(_ context.Context, _ id.RoomID, _ domain.Actor, _, _, _, _ *string) error {
	return nil
}
func (m *arMockMatrixPort) SetRoomDirectoryVisibility(_ context.Context, _ id.RoomID, _ bool) error {
	return nil
}
func (m *arMockMatrixPort) SetCustomState(_ context.Context, _ id.RoomID, _ map[string]map[string]interface{}) error {
	return nil
}
func (m *arMockMatrixPort) GetCustomState(_ context.Context, _ id.RoomID, _ []string) (map[string]map[string]interface{}, error) {
	return nil, nil
}
func (m *arMockMatrixPort) ResolveAlias(_ context.Context, _ string) (id.RoomID, error) {
	return "", nil
}
func (m *arMockMatrixPort) DeleteAlias(_ context.Context, _ string) error { return nil }
func (m *arMockMatrixPort) KickUser(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return nil
}
func (m *arMockMatrixPort) SendMessage(_ context.Context, _ id.RoomID, _ domain.Actor, _ string, _ []domain.Attachment) (id.EventID, error) {
	return "", nil
}
func (m *arMockMatrixPort) SendReply(_ context.Context, _ id.RoomID, _ domain.Actor, _ string, _ id.EventID, _ []domain.Attachment) (id.EventID, error) {
	return "", nil
}
func (m *arMockMatrixPort) RedactEvent(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) error {
	return nil
}
func (m *arMockMatrixPort) SendReaction(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) (id.EventID, error) {
	return "", nil
}
func (m *arMockMatrixPort) GetMessage(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Message, error) {
	return nil, nil
}
func (m *arMockMatrixPort) GetRoomMessages(_ context.Context, _ id.RoomID) ([]domain.Message, error) {
	return nil, nil
}
func (m *arMockMatrixPort) GetLastMessage(_ context.Context, _ id.RoomID) (*domain.Message, error) {
	return nil, nil
}
func (m *arMockMatrixPort) GetBatchLastMessages(_ context.Context, _ []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	return nil, nil
}
func (m *arMockMatrixPort) GetReactionEventID(_ context.Context, _ id.RoomID, _ id.EventID, _ string, _ domain.Actor) (id.EventID, error) {
	return "", nil
}
func (m *arMockMatrixPort) GetReaction(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Reaction, error) {
	return nil, nil
}
func (m *arMockMatrixPort) GetThreadMessages(_ context.Context, _ id.RoomID, _ id.EventID) ([]domain.Message, error) {
	return nil, nil
}
func (m *arMockMatrixPort) FindExistingDirectRoom(_ context.Context, _ domain.Actor, _ domain.Actor) (id.RoomID, error) {
	return "", domain.ErrNotFound
}
func (m *arMockMatrixPort) SetRoomAlias(_ context.Context, _ id.RoomID, _ string) error { return nil }
func (m *arMockMatrixPort) GetAllJoinedRooms(_ context.Context) ([]id.RoomID, error) {
	return nil, nil
}
func (m *arMockMatrixPort) CreateSpace(_ context.Context, _ uuid.UUID, _, _, _ string, _ string, _ []domain.Actor) (id.RoomID, error) {
	return "", nil
}
func (m *arMockMatrixPort) GetSpaceDetails(_ context.Context, _ id.RoomID) (*domain.Space, error) {
	return nil, nil
}
func (m *arMockMatrixPort) GetSpaceMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return nil, nil
}
func (m *arMockMatrixPort) UpdateSpaceState(_ context.Context, _ id.RoomID, _, _, _, _ *string) error {
	return nil
}
func (m *arMockMatrixPort) GetSpaceChildren(_ context.Context, _ id.RoomID) ([]domain.SpaceChild, error) {
	return nil, nil
}
func (m *arMockMatrixPort) AddSpaceChild(_ context.Context, _ id.RoomID, _ id.RoomID, _ string, _ bool) error {
	return nil
}
func (m *arMockMatrixPort) SetSpaceParent(_ context.Context, _ id.RoomID, _ id.RoomID) error {
	return nil
}
func (m *arMockMatrixPort) InviteToSpace(_ context.Context, _ id.RoomID, _ domain.Actor) error {
	return nil
}
func (m *arMockMatrixPort) KickFromSpace(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return nil
}
func (m *arMockMatrixPort) GetBatchUnreadCounts(_ context.Context, _ domain.Actor, _ []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error) {
	return nil, nil
}

// ===========================================================================
// ActorService tests
// ===========================================================================

func TestActorService_SyncActor_Success(t *testing.T) {
	matrix := &arMockMatrixPort{
		ensureUserResult: "@user-abc:test.local",
	}
	logger := &testutil.MockLogger{}
	svc := NewActorService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	err := svc.SyncActor(context.Background(), actorID, "Alice", "mxc://test.local/avatar.png")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.ensureUserCalled {
		t.Fatal("expected EnsureUser to be called")
	}
	if !matrix.setUserProfileCalled {
		t.Fatal("expected SetUserProfile to be called")
	}
}

func TestActorService_SyncActor_EnsureUserError(t *testing.T) {
	ensureErr := errors.New("homeserver unreachable")
	matrix := &arMockMatrixPort{
		ensureUserErr: ensureErr,
	}
	logger := &testutil.MockLogger{}
	svc := NewActorService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	err := svc.SyncActor(context.Background(), actorID, "Alice", "")

	if err == nil {
		t.Fatal("expected error when EnsureUser fails")
	}
	if !errors.Is(err, ensureErr) {
		t.Fatalf("expected underlying error %v, got: %v", ensureErr, err)
	}
	if !matrix.ensureUserCalled {
		t.Fatal("expected EnsureUser to be called")
	}
	if matrix.setUserProfileCalled {
		t.Fatal("SetUserProfile should NOT be called when EnsureUser fails")
	}
}

func TestActorService_SyncActor_SetUserProfileError(t *testing.T) {
	profileErr := errors.New("profile update failed")
	matrix := &arMockMatrixPort{
		ensureUserResult:  "@user-abc:test.local",
		setUserProfileErr: profileErr,
	}
	logger := &testutil.MockLogger{}
	svc := NewActorService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	err := svc.SyncActor(context.Background(), actorID, "Alice", "mxc://test.local/avatar.png")

	if err == nil {
		t.Fatal("expected error when SetUserProfile fails")
	}
	if !errors.Is(err, profileErr) {
		t.Fatalf("expected underlying error %v, got: %v", profileErr, err)
	}
	if !matrix.ensureUserCalled {
		t.Fatal("expected EnsureUser to be called")
	}
	if !matrix.setUserProfileCalled {
		t.Fatal("expected SetUserProfile to be called")
	}
}

// ===========================================================================
// ReadReceiptService tests
// ===========================================================================

func TestReadReceiptService_MarkMessageRead_Success(t *testing.T) {
	matrix := &arMockMatrixPort{}
	logger := &testutil.MockLogger{}
	svc := NewReadReceiptService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	roomID := id.RoomID("!room123:test.local")
	eventID := id.EventID("$event456")

	err := svc.MarkMessageRead(context.Background(), actorID, roomID, eventID, nil)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.sendReadReceiptCalled {
		t.Fatal("expected SendReadReceipt to be called")
	}
	if matrix.sendReadReceiptActor.ID != actorID {
		t.Errorf("expected actor ID %s, got %s", actorID, matrix.sendReadReceiptActor.ID)
	}
	if matrix.sendReadReceiptRoom != roomID {
		t.Errorf("expected room ID %s, got %s", roomID, matrix.sendReadReceiptRoom)
	}
	if matrix.sendReadReceiptEvent != eventID {
		t.Errorf("expected event ID %s, got %s", eventID, matrix.sendReadReceiptEvent)
	}
	if matrix.sendReadReceiptThread != nil {
		t.Errorf("expected nil thread root ID, got %v", matrix.sendReadReceiptThread)
	}
}

func TestReadReceiptService_MarkMessageRead_WithThreadRootID(t *testing.T) {
	matrix := &arMockMatrixPort{}
	logger := &testutil.MockLogger{}
	svc := NewReadReceiptService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	roomID := id.RoomID("!room123:test.local")
	eventID := id.EventID("$event456")
	threadRootID := id.EventID("$thread789")

	err := svc.MarkMessageRead(context.Background(), actorID, roomID, eventID, &threadRootID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.sendReadReceiptCalled {
		t.Fatal("expected SendReadReceipt to be called")
	}
	if matrix.sendReadReceiptThread == nil {
		t.Fatal("expected thread root ID to be set, got nil")
	}
	if *matrix.sendReadReceiptThread != threadRootID {
		t.Errorf("expected thread root ID %s, got %s", threadRootID, *matrix.sendReadReceiptThread)
	}
}

func TestReadReceiptService_MarkMessageRead_Error(t *testing.T) {
	receiptErr := errors.New("receipt send failed")
	matrix := &arMockMatrixPort{
		sendReadReceiptErr: receiptErr,
	}
	logger := &testutil.MockLogger{}
	svc := NewReadReceiptService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	roomID := id.RoomID("!room123:test.local")
	eventID := id.EventID("$event456")

	err := svc.MarkMessageRead(context.Background(), actorID, roomID, eventID, nil)

	if err == nil {
		t.Fatal("expected error when SendReadReceipt fails")
	}
	if !errors.Is(err, receiptErr) {
		t.Fatalf("expected wrapped error to contain %v, got: %v", receiptErr, err)
	}
	if !matrix.sendReadReceiptCalled {
		t.Fatal("expected SendReadReceipt to be called")
	}
}

func TestReadReceiptService_GetUnreadCounts_Success(t *testing.T) {
	threadRoot := id.EventID("$thread001")
	expected := &domain.UnreadCountSummary{
		RoomUnreadCount: 5,
		ThreadUnreadCounts: map[id.EventID]int{
			threadRoot: 2,
		},
	}
	matrix := &arMockMatrixPort{
		getUnreadCountsResult: expected,
	}
	logger := &testutil.MockLogger{}
	svc := NewReadReceiptService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	roomID := id.RoomID("!room123:test.local")
	threadRootIDs := []id.EventID{threadRoot}

	summary, err := svc.GetUnreadCounts(context.Background(), actorID, roomID, threadRootIDs)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.getUnreadCountsCalled {
		t.Fatal("expected GetUnreadCounts to be called")
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.RoomUnreadCount != 5 {
		t.Errorf("expected room unread count 5, got %d", summary.RoomUnreadCount)
	}
	if summary.ThreadUnreadCounts[threadRoot] != 2 {
		t.Errorf("expected thread unread count 2, got %d", summary.ThreadUnreadCounts[threadRoot])
	}
}

func TestReadReceiptService_GetUnreadCounts_NoThreads(t *testing.T) {
	expected := &domain.UnreadCountSummary{
		RoomUnreadCount:    3,
		ThreadUnreadCounts: nil,
	}
	matrix := &arMockMatrixPort{
		getUnreadCountsResult: expected,
	}
	logger := &testutil.MockLogger{}
	svc := NewReadReceiptService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	roomID := id.RoomID("!room123:test.local")

	summary, err := svc.GetUnreadCounts(context.Background(), actorID, roomID, nil)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if summary.RoomUnreadCount != 3 {
		t.Errorf("expected room unread count 3, got %d", summary.RoomUnreadCount)
	}
}

func TestReadReceiptService_GetUnreadCounts_Error(t *testing.T) {
	countsErr := errors.New("sync failed")
	matrix := &arMockMatrixPort{
		getUnreadCountsErr: countsErr,
	}
	logger := &testutil.MockLogger{}
	svc := NewReadReceiptService(matrix, logger)

	actorID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	roomID := id.RoomID("!room123:test.local")

	summary, err := svc.GetUnreadCounts(context.Background(), actorID, roomID, nil)

	if err == nil {
		t.Fatal("expected error when GetUnreadCounts fails")
	}
	if !errors.Is(err, countsErr) {
		t.Fatalf("expected wrapped error to contain %v, got: %v", countsErr, err)
	}
	if summary != nil {
		t.Errorf("expected nil summary on error, got %+v", summary)
	}
	if !matrix.getUnreadCountsCalled {
		t.Fatal("expected GetUnreadCounts to be called")
	}
}
