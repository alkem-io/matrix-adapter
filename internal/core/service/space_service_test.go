package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/testutil"
)

// ============================================================================
// Mock: mockSpaceMatrixPort
// ============================================================================

type mockSpaceMatrixPort struct {
	// ResolveAlias behavior — keyed by alias string
	resolveAliasResults map[string]id.RoomID
	resolveAliasErr     error

	// CreateSpace captures
	createSpaceCalled   bool
	createSpaceRoomID   id.RoomID
	createSpaceErr      error
	createSpaceJoinRule string

	// GetSpaceDetails
	getSpaceDetailsResult *domain.Space
	getSpaceDetailsErr    error

	// GetSpaceMembers
	getSpaceMembersResult []id.UserID
	getSpaceMembersErr    error

	// GetSpaceChildren
	getSpaceChildrenResult []domain.SpaceChild
	getSpaceChildrenErr    error

	// UpdateSpaceState
	updateSpaceStateCalled bool
	updateSpaceStateErr    error

	// DeleteAlias
	deleteAliasCalled bool
	deleteAliasErr    error

	// KickFromSpace
	kickFromSpaceCalled int
	kickFromSpaceErr    error

	// GetAllJoinedRooms
	getAllJoinedRoomsResult []id.RoomID
	getAllJoinedRoomsErr    error

	// AddSpaceChild / SetSpaceParent
	addSpaceChildCalled  bool
	setSpaceParentCalled bool

	// InviteToSpace
	inviteToSpaceCalled int
	inviteToSpaceErr    error

	// SetRoomDirectoryVisibility
	setRoomDirectoryVisibilityCalled bool

	// SetCustomState
	setCustomStateCalled bool
}

// resolveAlias looks up the alias in the results map; if not found uses resolveAliasErr.
func (m *mockSpaceMatrixPort) ResolveAlias(_ context.Context, alias string) (id.RoomID, error) {
	if m.resolveAliasResults != nil {
		if roomID, ok := m.resolveAliasResults[alias]; ok {
			return roomID, nil
		}
	}
	if m.resolveAliasErr != nil {
		return "", m.resolveAliasErr
	}
	return "", domain.NewSpaceNotFoundError(alias)
}

func (m *mockSpaceMatrixPort) CreateSpace(_ context.Context, _ uuid.UUID, _, _, _ string, joinRule string, _ []domain.Actor) (id.RoomID, error) {
	m.createSpaceCalled = true
	m.createSpaceJoinRule = joinRule
	if m.createSpaceErr != nil {
		return "", m.createSpaceErr
	}
	if m.createSpaceRoomID != "" {
		return m.createSpaceRoomID, nil
	}
	return "!space:test.local", nil
}

func (m *mockSpaceMatrixPort) GetSpaceDetails(_ context.Context, _ id.RoomID) (*domain.Space, error) {
	if m.getSpaceDetailsErr != nil {
		return nil, m.getSpaceDetailsErr
	}
	if m.getSpaceDetailsResult != nil {
		return m.getSpaceDetailsResult, nil
	}
	return &domain.Space{Alias: "#00000000-0000-0000-0000-000000000000:test.local"}, nil
}

func (m *mockSpaceMatrixPort) GetSpaceMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	if m.getSpaceMembersErr != nil {
		return nil, m.getSpaceMembersErr
	}
	return m.getSpaceMembersResult, nil
}

func (m *mockSpaceMatrixPort) GetSpaceChildren(_ context.Context, _ id.RoomID) ([]domain.SpaceChild, error) {
	if m.getSpaceChildrenErr != nil {
		return nil, m.getSpaceChildrenErr
	}
	return m.getSpaceChildrenResult, nil
}

func (m *mockSpaceMatrixPort) UpdateSpaceState(_ context.Context, _ id.RoomID, _, _, _, _ *string) error {
	m.updateSpaceStateCalled = true
	return m.updateSpaceStateErr
}

func (m *mockSpaceMatrixPort) DeleteAlias(_ context.Context, _ string) error {
	m.deleteAliasCalled = true
	return m.deleteAliasErr
}

func (m *mockSpaceMatrixPort) KickFromSpace(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	m.kickFromSpaceCalled++
	return m.kickFromSpaceErr
}

func (m *mockSpaceMatrixPort) GetAllJoinedRooms(_ context.Context) ([]id.RoomID, error) {
	if m.getAllJoinedRoomsErr != nil {
		return nil, m.getAllJoinedRoomsErr
	}
	return m.getAllJoinedRoomsResult, nil
}

func (m *mockSpaceMatrixPort) AddSpaceChild(_ context.Context, _ id.RoomID, _ id.RoomID, _ string, _ bool) error {
	m.addSpaceChildCalled = true
	return nil
}

func (m *mockSpaceMatrixPort) SetSpaceParent(_ context.Context, _ id.RoomID, _ id.RoomID) error {
	m.setSpaceParentCalled = true
	return nil
}

func (m *mockSpaceMatrixPort) InviteToSpace(_ context.Context, _ id.RoomID, _ domain.Actor) error {
	m.inviteToSpaceCalled++
	return m.inviteToSpaceErr
}

func (m *mockSpaceMatrixPort) SetRoomDirectoryVisibility(_ context.Context, _ id.RoomID, _ bool) error {
	m.setRoomDirectoryVisibilityCalled = true
	return nil
}

func (m *mockSpaceMatrixPort) SetCustomState(_ context.Context, _ id.RoomID, _ map[string]map[string]interface{}) error {
	m.setCustomStateCalled = true
	return nil
}

// --- Stub implementations for remaining MatrixPort interface methods ---

func (m *mockSpaceMatrixPort) Connect(_ context.Context) error { return nil }
func (m *mockSpaceMatrixPort) Disconnect() error               { return nil }
func (m *mockSpaceMatrixPort) HomeserverDomain() string        { return "test.local" }
func (m *mockSpaceMatrixPort) EnsureUser(_ context.Context, _ domain.Actor) (id.UserID, error) {
	return "@bot:test.local", nil
}
func (m *mockSpaceMatrixPort) SetUserProfile(_ context.Context, _ domain.Actor) error { return nil }
func (m *mockSpaceMatrixPort) CreateRoomWithAlias(_ context.Context, _ uuid.UUID, _, _, _, _, _ string, _ map[string]map[string]interface{}, _ []domain.Actor) (id.RoomID, error) {
	return "", nil
}
func (m *mockSpaceMatrixPort) InviteUser(_ context.Context, _ id.RoomID, _ domain.Actor, _ domain.Actor) error {
	return nil
}
func (m *mockSpaceMatrixPort) GetRoomDetails(_ context.Context, _ id.RoomID) (*domain.Room, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) GetRoomMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) UpdateRoomState(_ context.Context, _ id.RoomID, _ domain.Actor, _, _, _, _ *string) error {
	return nil
}
func (m *mockSpaceMatrixPort) GetCustomState(_ context.Context, _ id.RoomID, _ []string) (map[string]map[string]interface{}, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) KickUser(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return nil
}
func (m *mockSpaceMatrixPort) SendMessage(_ context.Context, _ id.RoomID, _ domain.Actor, _ string) (id.EventID, error) {
	return "", nil
}
func (m *mockSpaceMatrixPort) SendReply(_ context.Context, _ id.RoomID, _ domain.Actor, _ string, _ id.EventID) (id.EventID, error) {
	return "", nil
}
func (m *mockSpaceMatrixPort) RedactEvent(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) error {
	return nil
}
func (m *mockSpaceMatrixPort) SendReaction(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) (id.EventID, error) {
	return "", nil
}
func (m *mockSpaceMatrixPort) GetMessage(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Message, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) GetRoomMessages(_ context.Context, _ id.RoomID) ([]domain.Message, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) GetLastMessage(_ context.Context, _ id.RoomID) (*domain.Message, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) GetBatchLastMessages(_ context.Context, _ []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) GetReactionEventID(_ context.Context, _ id.RoomID, _ id.EventID, _ string, _ domain.Actor) (id.EventID, error) {
	return "", nil
}
func (m *mockSpaceMatrixPort) GetReaction(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Reaction, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) GetThreadMessages(_ context.Context, _ id.RoomID, _ id.EventID) ([]domain.Message, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) FindExistingDirectRoom(_ context.Context, _ domain.Actor, _ domain.Actor) (id.RoomID, error) {
	return "", domain.ErrNotFound
}
func (m *mockSpaceMatrixPort) SetRoomAlias(_ context.Context, _ id.RoomID, _ string) error {
	return nil
}
func (m *mockSpaceMatrixPort) SendReadReceipt(_ context.Context, _ domain.Actor, _ id.RoomID, _ id.EventID, _ *id.EventID) error {
	return nil
}
func (m *mockSpaceMatrixPort) GetUnreadCounts(_ context.Context, _ domain.Actor, _ id.RoomID, _ []id.EventID) (*domain.UnreadCountSummary, error) {
	return nil, nil
}
func (m *mockSpaceMatrixPort) GetBatchUnreadCounts(_ context.Context, _ domain.Actor, _ []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error) {
	return nil, nil
}

// Compile-time interface check.
var _ ports.MatrixPort = (*mockSpaceMatrixPort)(nil)

// ============================================================================
// Helpers
// ============================================================================

// spaceIDMapper is a shared IDMapper for constructing Matrix IDs in space tests.
var spaceIDMapper = domain.NewIDMapper("test.local")

func newSpaceService(matrix *mockSpaceMatrixPort) *SpaceService {
	return NewSpaceService(matrix, &testutil.MockLogger{}, domain.NewIDMapper("test.local"))
}

func mustAlias(contextID uuid.UUID) string {
	return spaceIDMapper.SpaceAlias(contextID)
}

// ============================================================================
// CreateSpace Tests
// ============================================================================

func TestCreateSpace_Success(t *testing.T) {
	contextID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr:   domain.NewSpaceNotFoundError("not found"),
		createSpaceRoomID: "!newspace:test.local",
	}
	svc := newSpaceService(matrix)

	err := svc.CreateSpace(context.Background(), contextID, "My Space", "A topic", "", "invite", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.createSpaceCalled {
		t.Fatal("expected CreateSpace to be called")
	}
}

func TestCreateSpace_Idempotent_AlreadyExists(t *testing.T) {
	contextID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!existing:test.local",
		},
	}
	svc := newSpaceService(matrix)

	err := svc.CreateSpace(context.Background(), contextID, "My Space", "", "", "invite", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected idempotent success, got: %v", err)
	}
	if matrix.createSpaceCalled {
		t.Fatal("expected CreateSpace NOT to be called when space already exists")
	}
}

func TestCreateSpace_AliasCheckError(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr: errors.New("network timeout"),
	}
	svc := newSpaceService(matrix)

	err := svc.CreateSpace(context.Background(), uuid.New(), "Space", "", "", "", nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for alias check failure")
	}
	if !contains(err.Error(), "failed to check space alias") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCreateSpace_CreateError(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr: domain.NewSpaceNotFoundError("not found"),
		createSpaceErr:  errors.New("matrix unavailable"),
	}
	svc := newSpaceService(matrix)

	err := svc.CreateSpace(context.Background(), uuid.New(), "Space", "", "", "invite", nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for create failure")
	}
	if !contains(err.Error(), "failed to create space") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCreateSpace_WithParent(t *testing.T) {
	contextID := uuid.New()
	parentID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(parentID): "!parent:test.local",
		},
		resolveAliasErr:   domain.NewSpaceNotFoundError("not found"),
		createSpaceRoomID: "!child:test.local",
	}
	svc := newSpaceService(matrix)

	err := svc.CreateSpace(context.Background(), contextID, "Child Space", "", "", "invite", nil, nil, &parentID, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.addSpaceChildCalled {
		t.Error("expected AddSpaceChild to be called for parent hierarchy")
	}
	if !matrix.setSpaceParentCalled {
		t.Error("expected SetSpaceParent to be called for parent hierarchy")
	}
}

func TestCreateSpace_WithIsPublic(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr:   domain.NewSpaceNotFoundError("not found"),
		createSpaceRoomID: "!space:test.local",
	}
	svc := newSpaceService(matrix)

	isPublic := true
	err := svc.CreateSpace(context.Background(), uuid.New(), "Public Space", "", "", "public", &isPublic, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.setRoomDirectoryVisibilityCalled {
		t.Error("expected SetRoomDirectoryVisibility to be called when isPublic is set")
	}
}

func TestCreateSpace_WithCustomState(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr:   domain.NewSpaceNotFoundError("not found"),
		createSpaceRoomID: "!space:test.local",
	}
	svc := newSpaceService(matrix)

	customState := map[string]map[string]interface{}{
		"io.alkemio.metadata": {"key": "value"},
	}
	err := svc.CreateSpace(context.Background(), uuid.New(), "Space", "", "", "invite", nil, customState, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.setCustomStateCalled {
		t.Error("expected SetCustomState to be called when customState is provided")
	}
}

func TestCreateSpace_DefaultJoinRuleInvite(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr:   domain.NewSpaceNotFoundError("not found"),
		createSpaceRoomID: "!space:test.local",
	}
	svc := newSpaceService(matrix)

	// Empty joinRule should default to "invite"
	err := svc.CreateSpace(context.Background(), uuid.New(), "Space", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.createSpaceCalled {
		t.Fatal("expected CreateSpace to be called")
	}
	// Assert the joinRule was defaulted to "invite" by the service layer.
	if matrix.createSpaceJoinRule != "invite" {
		t.Errorf("expected joinRule 'invite', got %q", matrix.createSpaceJoinRule)
	}
}

// ============================================================================
// GetSpace Tests
// ============================================================================

func TestGetSpace_Success(t *testing.T) {
	contextID := uuid.New()
	memberActorID := uuid.New()
	memberMatrixID := spaceIDMapper.UserID(memberActorID)
	spaceAlias := mustAlias(contextID)

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceAlias: "!space:test.local",
		},
		getSpaceDetailsResult: &domain.Space{
			ID:    "!space:test.local",
			Name:  "Test Space",
			Alias: spaceAlias,
		},
		getSpaceMembersResult: []id.UserID{memberMatrixID},
		getSpaceChildrenResult: []domain.SpaceChild{
			{ChildID: "!child1:test.local", IsSpace: false},
		},
	}
	svc := newSpaceService(matrix)

	space, err := svc.GetSpace(context.Background(), contextID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if space == nil {
		t.Fatal("expected non-nil space")
	}
	if space.AlkemioContextID != contextID {
		t.Errorf("expected AlkemioContextID %s, got %s", contextID, space.AlkemioContextID)
	}
	if len(space.MemberIDs) != 1 {
		t.Fatalf("expected 1 member, got %d", len(space.MemberIDs))
	}
	if space.MemberIDs[0] != memberActorID {
		t.Errorf("expected member %s, got %s", memberActorID, space.MemberIDs[0])
	}
	if len(space.Children) != 1 {
		t.Errorf("expected 1 child, got %d", len(space.Children))
	}
}

func TestGetSpace_NotFound(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr: domain.NewSpaceNotFoundError("not found"),
	}
	svc := newSpaceService(matrix)

	_, err := svc.GetSpace(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for space not found")
	}
	if !errors.Is(err, domain.ErrSpaceNotFound) {
		t.Errorf("expected ErrSpaceNotFound, got: %v", err)
	}
}

func TestGetSpace_DetailsError(t *testing.T) {
	contextID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!space:test.local",
		},
		getSpaceDetailsErr: errors.New("internal error"),
	}
	svc := newSpaceService(matrix)

	_, err := svc.GetSpace(context.Background(), contextID)
	if err == nil {
		t.Fatal("expected error for details failure")
	}
	if !contains(err.Error(), "failed to get space details") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetSpace_MembersError(t *testing.T) {
	contextID := uuid.New()
	spaceAlias := mustAlias(contextID)
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceAlias: "!space:test.local",
		},
		getSpaceDetailsResult: &domain.Space{
			ID:    "!space:test.local",
			Alias: spaceAlias,
		},
		getSpaceMembersErr: errors.New("members unavailable"),
	}
	svc := newSpaceService(matrix)

	_, err := svc.GetSpace(context.Background(), contextID)
	if err == nil {
		t.Fatal("expected error for members failure")
	}
	if !contains(err.Error(), "failed to get space members") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetSpace_ChildrenError_Graceful(t *testing.T) {
	contextID := uuid.New()
	spaceAlias := mustAlias(contextID)
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			spaceAlias: "!space:test.local",
		},
		getSpaceDetailsResult: &domain.Space{
			ID:    "!space:test.local",
			Alias: spaceAlias,
		},
		getSpaceMembersResult: []id.UserID{},
		getSpaceChildrenErr:   errors.New("children unavailable"),
	}
	svc := newSpaceService(matrix)

	space, err := svc.GetSpace(context.Background(), contextID)
	if err != nil {
		t.Fatalf("expected no error (graceful children failure), got: %v", err)
	}
	if space == nil {
		t.Fatal("expected non-nil space")
	}
	// Children should be an empty slice, not nil
	if space.Children == nil {
		t.Error("expected empty children slice, got nil")
	}
	if len(space.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(space.Children))
	}
}

// ============================================================================
// UpdateSpace Tests
// ============================================================================

func TestUpdateSpace_Success(t *testing.T) {
	contextID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!space:test.local",
		},
	}
	svc := newSpaceService(matrix)

	name := "Updated Name"
	err := svc.UpdateSpace(context.Background(), contextID, &name, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.updateSpaceStateCalled {
		t.Fatal("expected UpdateSpaceState to be called")
	}
}

func TestUpdateSpace_NotFound(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr: domain.NewSpaceNotFoundError("not found"),
	}
	svc := newSpaceService(matrix)

	name := "Updated"
	err := svc.UpdateSpace(context.Background(), uuid.New(), &name, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for space not found")
	}
	if !errors.Is(err, domain.ErrSpaceNotFound) {
		t.Errorf("expected ErrSpaceNotFound, got: %v", err)
	}
}

func TestUpdateSpace_UpdateError(t *testing.T) {
	contextID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!space:test.local",
		},
		updateSpaceStateErr: errors.New("state update failed"),
	}
	svc := newSpaceService(matrix)

	name := "Updated"
	err := svc.UpdateSpace(context.Background(), contextID, &name, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for update failure")
	}
	if !contains(err.Error(), "failed to update space") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateSpace_WithIsPublic(t *testing.T) {
	contextID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!space:test.local",
		},
	}
	svc := newSpaceService(matrix)

	isPublic := true
	err := svc.UpdateSpace(context.Background(), contextID, nil, nil, nil, nil, &isPublic, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.setRoomDirectoryVisibilityCalled {
		t.Error("expected SetRoomDirectoryVisibility to be called")
	}
}

func TestUpdateSpace_WithCustomState(t *testing.T) {
	contextID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!space:test.local",
		},
	}
	svc := newSpaceService(matrix)

	customState := map[string]map[string]interface{}{
		"io.alkemio.metadata": {"key": "value"},
	}
	err := svc.UpdateSpace(context.Background(), contextID, nil, nil, nil, nil, nil, customState)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.setCustomStateCalled {
		t.Error("expected SetCustomState to be called")
	}
}

// ============================================================================
// DeleteSpace Tests
// ============================================================================

func TestDeleteSpace_Success(t *testing.T) {
	contextID := uuid.New()
	member1 := spaceIDMapper.UserID(uuid.New())
	member2 := spaceIDMapper.UserID(uuid.New())

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!space:test.local",
		},
		getSpaceMembersResult: []id.UserID{member1, member2},
	}
	svc := newSpaceService(matrix)

	err := svc.DeleteSpace(context.Background(), contextID, "cleanup")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if matrix.kickFromSpaceCalled != 2 {
		t.Errorf("expected 2 kicks, got %d", matrix.kickFromSpaceCalled)
	}
	if !matrix.deleteAliasCalled {
		t.Error("expected DeleteAlias to be called")
	}
}

func TestDeleteSpace_NotFound_Idempotent(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr: domain.NewSpaceNotFoundError("not found"),
	}
	svc := newSpaceService(matrix)

	err := svc.DeleteSpace(context.Background(), uuid.New(), "cleanup")
	if err != nil {
		t.Fatalf("expected idempotent success for missing space, got: %v", err)
	}
}

func TestDeleteSpace_MembersKickFailures(t *testing.T) {
	contextID := uuid.New()
	member1 := spaceIDMapper.UserID(uuid.New())

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(contextID): "!space:test.local",
		},
		getSpaceMembersResult: []id.UserID{member1},
		kickFromSpaceErr:      errors.New("kick failed"),
	}
	svc := newSpaceService(matrix)

	// DeleteSpace logs kick failures but still succeeds
	err := svc.DeleteSpace(context.Background(), contextID, "cleanup")
	if err != nil {
		t.Fatalf("expected no error despite kick failure, got: %v", err)
	}
	if matrix.kickFromSpaceCalled != 1 {
		t.Errorf("expected 1 kick attempt, got %d", matrix.kickFromSpaceCalled)
	}
	if !matrix.deleteAliasCalled {
		t.Error("expected DeleteAlias to be called even after kick failures")
	}
}

// ============================================================================
// ListSpaces Tests
// ============================================================================

func TestListSpaces_Success(t *testing.T) {
	ctx1 := uuid.New()
	ctx2 := uuid.New()

	// We need a more nuanced mock for GetSpaceDetails that returns different
	// results per room. Use a wrapper mock with map-based approach.
	detailsByRoom := map[id.RoomID]*domain.Space{
		"!space1:test.local": {Alias: mustAlias(ctx1)},
		"!space2:test.local": {Alias: mustAlias(ctx2)},
	}

	// Use a wrapper mock
	wrapper := &mockSpaceMatrixPortWithDetailsByRoom{
		mockSpaceMatrixPort: mockSpaceMatrixPort{
			getAllJoinedRoomsResult: []id.RoomID{"!space1:test.local", "!space2:test.local"},
		},
		detailsByRoom: detailsByRoom,
	}
	svc := NewSpaceService(wrapper, &testutil.MockLogger{}, domain.NewIDMapper("test.local"))

	contextIDs, cursor, err := svc.ListSpaces(context.Background(), "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got: %s", cursor)
	}
	if len(contextIDs) != 2 {
		t.Fatalf("expected 2 context IDs, got %d", len(contextIDs))
	}

	// Verify both context IDs are present
	found := map[uuid.UUID]bool{}
	for _, cid := range contextIDs {
		found[cid] = true
	}
	if !found[ctx1] || !found[ctx2] {
		t.Errorf("expected both context IDs, got: %v", contextIDs)
	}
}

func TestListSpaces_Error(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		getAllJoinedRoomsErr: errors.New("rooms unavailable"),
	}
	svc := newSpaceService(matrix)

	_, _, err := svc.ListSpaces(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for rooms failure")
	}
	if !contains(err.Error(), "failed to list spaces") {
		t.Errorf("unexpected error: %v", err)
	}
}

// mockSpaceMatrixPortWithDetailsByRoom wraps mockSpaceMatrixPort to return
// different SpaceDetails per room ID (needed for ListSpaces).
type mockSpaceMatrixPortWithDetailsByRoom struct {
	mockSpaceMatrixPort
	detailsByRoom map[id.RoomID]*domain.Space
}

func (m *mockSpaceMatrixPortWithDetailsByRoom) GetSpaceDetails(_ context.Context, roomID id.RoomID) (*domain.Space, error) {
	if space, ok := m.detailsByRoom[roomID]; ok {
		return space, nil
	}
	return nil, errors.New("space not found")
}

// ============================================================================
// SetParent Tests
// ============================================================================

func TestSetParent_Success_RoomChild(t *testing.T) {
	parentCtxID := uuid.New()
	childRoomUUID := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(parentCtxID):                 "!parent:test.local",
			spaceIDMapper.RoomAlias(childRoomUUID): "!child:test.local",
		},
	}
	svc := newSpaceService(matrix)

	err := svc.SetParent(context.Background(), childRoomUUID.String(), false, parentCtxID, "", false)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.addSpaceChildCalled {
		t.Error("expected AddSpaceChild to be called")
	}
	if !matrix.setSpaceParentCalled {
		t.Error("expected SetSpaceParent to be called")
	}
}

func TestSetParent_Success_SpaceChild(t *testing.T) {
	parentCtxID := uuid.New()
	childCtxID := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(parentCtxID): "!parent:test.local",
			mustAlias(childCtxID):  "!child:test.local",
		},
	}
	svc := newSpaceService(matrix)

	err := svc.SetParent(context.Background(), childCtxID.String(), true, parentCtxID, "01", true)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.addSpaceChildCalled {
		t.Error("expected AddSpaceChild to be called")
	}
	if !matrix.setSpaceParentCalled {
		t.Error("expected SetSpaceParent to be called")
	}
}

func TestSetParent_ParentNotFound(t *testing.T) {
	matrix := &mockSpaceMatrixPort{
		resolveAliasErr: domain.NewParentNotFoundError("not found"),
	}
	svc := newSpaceService(matrix)

	err := svc.SetParent(context.Background(), uuid.New().String(), false, uuid.New(), "", false)
	if err == nil {
		t.Fatal("expected error for parent not found")
	}
	if !errors.Is(err, domain.ErrParentNotFound) {
		t.Errorf("expected ErrParentNotFound, got: %v", err)
	}
}

func TestSetParent_ChildNotFound(t *testing.T) {
	parentCtxID := uuid.New()
	childRoomUUID := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(parentCtxID): "!parent:test.local",
			// child alias NOT present -> falls through to resolveAliasErr
		},
		resolveAliasErr: domain.NewChildNotFoundError("not found"),
	}
	svc := newSpaceService(matrix)

	err := svc.SetParent(context.Background(), childRoomUUID.String(), false, parentCtxID, "", false)
	if err == nil {
		t.Fatal("expected error for child not found")
	}
	if !errors.Is(err, domain.ErrChildNotFound) {
		t.Errorf("expected ErrChildNotFound, got: %v", err)
	}
}

func TestSetParent_InvalidChildUUID(t *testing.T) {
	parentCtxID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(parentCtxID): "!parent:test.local",
		},
	}
	svc := newSpaceService(matrix)

	err := svc.SetParent(context.Background(), "not-a-uuid", false, parentCtxID, "", false)
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
	if !contains(err.Error(), "invalid child") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetParent_InvalidChildUUID_Space(t *testing.T) {
	parentCtxID := uuid.New()
	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(parentCtxID): "!parent:test.local",
		},
	}
	svc := newSpaceService(matrix)

	err := svc.SetParent(context.Background(), "not-a-uuid", true, parentCtxID, "", false)
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
	if !contains(err.Error(), "invalid child space ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
// BatchAddMember Tests
// ============================================================================

func TestBatchAddMember_Success(t *testing.T) {
	ctx1 := uuid.New()
	ctx2 := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(ctx1): "!space1:test.local",
			mustAlias(ctx2): "!space2:test.local",
		},
	}
	svc := newSpaceService(matrix)

	results := svc.BatchAddMember(context.Background(), uuid.New(), []uuid.UUID{ctx1, ctx2})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for ctxID, err := range results {
		if err != nil {
			t.Errorf("expected nil error for %s, got: %v", ctxID, err)
		}
	}
	if matrix.inviteToSpaceCalled != 2 {
		t.Errorf("expected 2 invites, got %d", matrix.inviteToSpaceCalled)
	}
}

func TestBatchAddMember_PartialFailures(t *testing.T) {
	ctx1 := uuid.New()
	ctx2 := uuid.New() // this one will not be found

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(ctx1): "!space1:test.local",
			// ctx2 alias NOT present
		},
		resolveAliasErr: domain.NewSpaceNotFoundError("not found"),
	}
	svc := newSpaceService(matrix)

	results := svc.BatchAddMember(context.Background(), uuid.New(), []uuid.UUID{ctx1, ctx2})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[ctx1.String()] != nil {
		t.Errorf("expected nil error for ctx1, got: %v", results[ctx1.String()])
	}
	if !errors.Is(results[ctx2.String()], domain.ErrSpaceNotFound) {
		t.Errorf("expected ErrSpaceNotFound for ctx2, got: %v", results[ctx2.String()])
	}
}

func TestBatchAddMember_InviteError(t *testing.T) {
	ctx1 := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(ctx1): "!space1:test.local",
		},
		inviteToSpaceErr: errors.New("invite failed"),
	}
	svc := newSpaceService(matrix)

	results := svc.BatchAddMember(context.Background(), uuid.New(), []uuid.UUID{ctx1})
	if results[ctx1.String()] == nil {
		t.Error("expected error for invite failure")
	}
	if !contains(results[ctx1.String()].Error(), "invite failed") {
		t.Errorf("unexpected error: %v", results[ctx1.String()])
	}
}

// ============================================================================
// BatchRemoveMember Tests
// ============================================================================

func TestBatchRemoveMember_Success(t *testing.T) {
	ctx1 := uuid.New()
	ctx2 := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(ctx1): "!space1:test.local",
			mustAlias(ctx2): "!space2:test.local",
		},
	}
	svc := newSpaceService(matrix)

	results := svc.BatchRemoveMember(context.Background(), uuid.New(), []uuid.UUID{ctx1, ctx2}, "removed")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for ctxID, err := range results {
		if err != nil {
			t.Errorf("expected nil error for %s, got: %v", ctxID, err)
		}
	}
	if matrix.kickFromSpaceCalled != 2 {
		t.Errorf("expected 2 kicks, got %d", matrix.kickFromSpaceCalled)
	}
}

func TestBatchRemoveMember_PartialFailures(t *testing.T) {
	ctx1 := uuid.New()
	ctx2 := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(ctx1): "!space1:test.local",
			// ctx2 NOT present
		},
		resolveAliasErr: domain.NewSpaceNotFoundError("not found"),
	}
	svc := newSpaceService(matrix)

	results := svc.BatchRemoveMember(context.Background(), uuid.New(), []uuid.UUID{ctx1, ctx2}, "removed")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[ctx1.String()] != nil {
		t.Errorf("expected nil error for ctx1, got: %v", results[ctx1.String()])
	}
	if !errors.Is(results[ctx2.String()], domain.ErrSpaceNotFound) {
		t.Errorf("expected ErrSpaceNotFound for ctx2, got: %v", results[ctx2.String()])
	}
}

func TestBatchRemoveMember_KickError(t *testing.T) {
	ctx1 := uuid.New()

	matrix := &mockSpaceMatrixPort{
		resolveAliasResults: map[string]id.RoomID{
			mustAlias(ctx1): "!space1:test.local",
		},
		kickFromSpaceErr: errors.New("kick failed"),
	}
	svc := newSpaceService(matrix)

	results := svc.BatchRemoveMember(context.Background(), uuid.New(), []uuid.UUID{ctx1}, "removed")
	if results[ctx1.String()] == nil {
		t.Error("expected error for kick failure")
	}
	if !contains(results[ctx1.String()].Error(), "kick failed") {
		t.Errorf("unexpected error: %v", results[ctx1.String()])
	}
}

// ============================================================================
// Helper
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
