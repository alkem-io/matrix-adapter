package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/testutil"
)

// mockMatrixPort is a minimal test double for MatrixPort.
// Only methods used by CreateRoomWithAlkemioID and UpdateRoomMetadata are implemented.
type mockMatrixPort struct {
	// ResolveAlias behavior
	resolveAliasErr error

	// CreateRoomWithAlias captures
	createRoomCalled   bool
	createRoomJoinRule string
	createRoomType     string
	createRoomErr      error

	// UpdateRoomState captures
	updateRoomCalled   bool
	updateRoomJoinRule *string
	updateRoomErr      error
}

func (m *mockMatrixPort) Connect(_ context.Context) error { return nil }
func (m *mockMatrixPort) Disconnect() error               { return nil }
func (m *mockMatrixPort) HomeserverDomain() string        { return "test.local" }
func (m *mockMatrixPort) EnsureUser(_ context.Context, _ domain.Actor) (id.UserID, error) {
	return "@bot:test.local", nil
}
func (m *mockMatrixPort) SetUserProfile(_ context.Context, _ domain.Actor) error { return nil }

func (m *mockMatrixPort) CreateRoomWithAlias(_ context.Context, _ uuid.UUID, roomType string, _, _, _ string, joinRule string, _ map[string]map[string]interface{}, _ []domain.Actor) (id.RoomID, error) {
	m.createRoomCalled = true
	m.createRoomJoinRule = joinRule
	m.createRoomType = roomType
	if m.createRoomErr != nil {
		return "", m.createRoomErr
	}
	return "!room:test.local", nil
}

func (m *mockMatrixPort) SetRoomDirectoryVisibility(_ context.Context, _ id.RoomID, _ bool) error {
	return nil
}
func (m *mockMatrixPort) SetCustomState(_ context.Context, _ id.RoomID, _ map[string]map[string]interface{}) error {
	return nil
}
func (m *mockMatrixPort) GetCustomState(_ context.Context, _ id.RoomID, _ []string) (map[string]map[string]interface{}, error) {
	return nil, nil
}
func (m *mockMatrixPort) ResolveAlias(_ context.Context, _ string) (id.RoomID, error) {
	if m.resolveAliasErr != nil {
		return "", m.resolveAliasErr
	}
	return "!room:test.local", nil
}

func (m *mockMatrixPort) UpdateRoomState(_ context.Context, _ id.RoomID, _ domain.Actor, _, _, _ *string, joinRule *string) error {
	m.updateRoomCalled = true
	m.updateRoomJoinRule = joinRule
	return m.updateRoomErr
}

// Stub implementations for remaining MatrixPort interface methods
func (m *mockMatrixPort) InviteUser(_ context.Context, _ id.RoomID, _ domain.Actor, _ domain.Actor) error {
	return nil
}
func (m *mockMatrixPort) GetRoomDetails(_ context.Context, _ id.RoomID) (*domain.Room, error) {
	return nil, nil
}
func (m *mockMatrixPort) GetRoomMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return nil, nil
}
func (m *mockMatrixPort) DeleteAlias(_ context.Context, _ string) error { return nil }
func (m *mockMatrixPort) KickUser(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return nil
}
func (m *mockMatrixPort) SendMessage(_ context.Context, _ id.RoomID, _ domain.Actor, _ string) (id.EventID, error) {
	return "", nil
}
func (m *mockMatrixPort) SendReply(_ context.Context, _ id.RoomID, _ domain.Actor, _ string, _ id.EventID) (id.EventID, error) {
	return "", nil
}
func (m *mockMatrixPort) RedactEvent(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) error {
	return nil
}
func (m *mockMatrixPort) SendReaction(_ context.Context, _ id.RoomID, _ domain.Actor, _ id.EventID, _ string) (id.EventID, error) {
	return "", nil
}
func (m *mockMatrixPort) GetMessage(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Message, error) {
	return nil, nil
}
func (m *mockMatrixPort) GetRoomMessages(_ context.Context, _ id.RoomID) ([]domain.Message, error) {
	return nil, nil
}
func (m *mockMatrixPort) GetLastMessage(_ context.Context, _ id.RoomID) (*domain.Message, error) {
	return nil, nil
}
func (m *mockMatrixPort) GetBatchLastMessages(_ context.Context, _ []id.RoomID) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	return nil, nil
}
func (m *mockMatrixPort) GetReactionEventID(_ context.Context, _ id.RoomID, _ id.EventID, _ string, _ domain.Actor) (id.EventID, error) {
	return "", nil
}
func (m *mockMatrixPort) GetReaction(_ context.Context, _ id.RoomID, _ id.EventID) (*domain.Reaction, error) {
	return nil, nil
}
func (m *mockMatrixPort) GetThreadMessages(_ context.Context, _ id.RoomID, _ id.EventID) ([]domain.Message, error) {
	return nil, nil
}
func (m *mockMatrixPort) FindExistingDirectRoom(_ context.Context, _ domain.Actor, _ domain.Actor) (id.RoomID, error) {
	return "", domain.ErrNotFound
}
func (m *mockMatrixPort) SetRoomAlias(_ context.Context, _ id.RoomID, _ string) error { return nil }
func (m *mockMatrixPort) GetAllJoinedRooms(_ context.Context) ([]id.RoomID, error) {
	return nil, nil
}
func (m *mockMatrixPort) CreateSpace(_ context.Context, _ uuid.UUID, _, _, _ string, _ string, _ []domain.Actor) (id.RoomID, error) {
	return "", nil
}
func (m *mockMatrixPort) GetSpaceDetails(_ context.Context, _ id.RoomID) (*domain.Space, error) {
	return nil, nil
}
func (m *mockMatrixPort) GetSpaceMembers(_ context.Context, _ id.RoomID) ([]id.UserID, error) {
	return nil, nil
}
func (m *mockMatrixPort) UpdateSpaceState(_ context.Context, _ id.RoomID, _, _, _, _ *string) error {
	return nil
}
func (m *mockMatrixPort) GetSpaceChildren(_ context.Context, _ id.RoomID) ([]domain.SpaceChild, error) {
	return nil, nil
}
func (m *mockMatrixPort) AddSpaceChild(_ context.Context, _ id.RoomID, _ id.RoomID, _ string, _ bool) error {
	return nil
}
func (m *mockMatrixPort) SetSpaceParent(_ context.Context, _ id.RoomID, _ id.RoomID) error {
	return nil
}
func (m *mockMatrixPort) InviteToSpace(_ context.Context, _ id.RoomID, _ domain.Actor) error {
	return nil
}
func (m *mockMatrixPort) KickFromSpace(_ context.Context, _ id.RoomID, _ id.UserID, _ string) error {
	return nil
}
func (m *mockMatrixPort) SendReadReceipt(_ context.Context, _ domain.Actor, _ id.RoomID, _ id.EventID, _ *id.EventID) error {
	return nil
}
func (m *mockMatrixPort) GetUnreadCounts(_ context.Context, _ domain.Actor, _ id.RoomID, _ []id.EventID) (*domain.UnreadCountSummary, error) {
	return nil, nil
}
func (m *mockMatrixPort) GetBatchUnreadCounts(_ context.Context, _ domain.Actor, _ []id.RoomID) (map[id.RoomID]int, map[id.RoomID]error) {
	return nil, nil
}

// ============================================================================
// Tests for CreateRoomWithAlkemioID — joinRule handling (SC-004)
// ============================================================================

func TestCreateRoom_JoinRulePublic(t *testing.T) {
	matrix := &mockMatrixPort{
		resolveAliasErr: domain.NewRoomNotFoundError("not found"),
	}
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper("test.local")
	svc := NewRoomService(matrix, logger, idMapper)

	err := svc.CreateRoomWithAlkemioID(
		context.Background(),
		uuid.New(),
		"community",
		"Test Room", "", "",
		"public",
		nil, nil, nil,
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.createRoomCalled {
		t.Fatal("expected CreateRoomWithAlias to be called")
	}
	if matrix.createRoomJoinRule != "public" {
		t.Errorf("expected joinRule 'public', got '%s'", matrix.createRoomJoinRule)
	}
}

func TestCreateRoom_JoinRuleInvite(t *testing.T) {
	matrix := &mockMatrixPort{
		resolveAliasErr: domain.NewRoomNotFoundError("not found"),
	}
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper("test.local")
	svc := NewRoomService(matrix, logger, idMapper)

	err := svc.CreateRoomWithAlkemioID(
		context.Background(),
		uuid.New(),
		"community",
		"Test Room", "", "",
		"invite",
		nil, nil, nil,
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if matrix.createRoomJoinRule != "invite" {
		t.Errorf("expected joinRule 'invite', got '%s'", matrix.createRoomJoinRule)
	}
}

func TestCreateRoom_JoinRuleOmitted(t *testing.T) {
	matrix := &mockMatrixPort{
		resolveAliasErr: domain.NewRoomNotFoundError("not found"),
	}
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper("test.local")
	svc := NewRoomService(matrix, logger, idMapper)

	err := svc.CreateRoomWithAlkemioID(
		context.Background(),
		uuid.New(),
		"community",
		"Test Room", "", "",
		"",
		nil, nil, nil,
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if matrix.createRoomJoinRule != "" {
		t.Errorf("expected empty joinRule (default behavior), got '%s'", matrix.createRoomJoinRule)
	}
}

func TestCreateRoom_DirectMessage_JoinRuleIgnored(t *testing.T) {
	matrix := &mockMatrixPort{
		resolveAliasErr: domain.NewRoomNotFoundError("not found"),
	}
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper("test.local")
	svc := NewRoomService(matrix, logger, idMapper)

	err := svc.CreateRoomWithAlkemioID(
		context.Background(),
		uuid.New(),
		"direct",
		"", "", "",
		"public", // should be ignored for DM rooms
		nil, nil, nil,
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if matrix.createRoomJoinRule != "" {
		t.Errorf("expected empty joinRule for DM room, got '%s'", matrix.createRoomJoinRule)
	}
}

// ============================================================================
// Tests for UpdateRoomMetadata — joinRule handling (SC-004)
// ============================================================================

func TestUpdateRoom_JoinRuleProvided(t *testing.T) {
	matrix := &mockMatrixPort{}
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper("test.local")
	svc := NewRoomService(matrix, logger, idMapper)

	joinRule := "public"
	err := svc.UpdateRoomMetadata(
		context.Background(),
		uuid.New(),
		nil, nil, nil,
		&joinRule,
		nil, nil,
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !matrix.updateRoomCalled {
		t.Fatal("expected UpdateRoomState to be called")
	}
	if matrix.updateRoomJoinRule == nil || *matrix.updateRoomJoinRule != "public" {
		t.Errorf("expected joinRule pointer to 'public', got %v", matrix.updateRoomJoinRule)
	}
}

func TestUpdateRoom_JoinRuleOmitted(t *testing.T) {
	matrix := &mockMatrixPort{}
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper("test.local")
	svc := NewRoomService(matrix, logger, idMapper)

	err := svc.UpdateRoomMetadata(
		context.Background(),
		uuid.New(),
		nil, nil, nil,
		nil, // joinRule omitted
		nil, // isPublic omitted
		nil, // visible omitted
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if matrix.updateRoomJoinRule != nil {
		t.Errorf("expected nil joinRule when omitted, got '%s'", *matrix.updateRoomJoinRule)
	}
}
