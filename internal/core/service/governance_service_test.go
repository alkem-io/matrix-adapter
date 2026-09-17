package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

// govMockPort embeds the space mock and adds call tracking for every
// governance primitive the repair orchestration drives.
type govMockPort struct {
	*mockSpaceMatrixPort

	governanceState    *domain.RoomGovernanceState
	governanceStateErr error
	roomVersion        string
	botPresence        domain.BotPresence

	applyLadderCalls    []domain.LadderOptions
	applyLadderDryRuns  []bool
	applyLadderChanged  bool
	setAccessCalls      []domain.RoomAccessState
	setGovStateCalls    []govStateCall
	setRoomAliasCalls   []string
	addSpaceChildCalls  int
	setSpaceParentCalls int
}

type govStateCall struct {
	Entity     *domain.EntityMarker
	Governance *domain.GovernanceMarker
}

func (m *govMockPort) ApplyLadder(_ context.Context, _ id.RoomID, _ domain.RoomClass, opts domain.LadderOptions, dryRun bool) (bool, error) {
	m.applyLadderCalls = append(m.applyLadderCalls, opts)
	m.applyLadderDryRuns = append(m.applyLadderDryRuns, dryRun)
	return m.applyLadderChanged, nil
}

func (m *govMockPort) EnsureBotAdmin(_ context.Context, _ id.RoomID) (domain.BotPresence, error) {
	return m.botPresence, nil
}

func (m *govMockPort) GetRoomVersion(_ context.Context, _ id.RoomID) (string, error) {
	if m.roomVersion == "" {
		return "10", nil
	}
	return m.roomVersion, nil
}

func (m *govMockPort) GetRoomGovernanceState(_ context.Context, _ id.RoomID) (*domain.RoomGovernanceState, error) {
	if m.governanceStateErr != nil {
		return nil, m.governanceStateErr
	}
	if m.governanceState != nil {
		return m.governanceState, nil
	}
	return &domain.RoomGovernanceState{JoinRule: "invite", HistoryVisibility: "shared", GuestAccess: "forbidden"}, nil
}

func (m *govMockPort) SetRoomAccessState(_ context.Context, _ id.RoomID, access domain.RoomAccessState) error {
	m.setAccessCalls = append(m.setAccessCalls, access)
	return nil
}

func (m *govMockPort) SetGovernanceState(_ context.Context, _ id.RoomID, entity *domain.EntityMarker, governance *domain.GovernanceMarker) error {
	m.setGovStateCalls = append(m.setGovStateCalls, govStateCall{Entity: entity, Governance: governance})
	return nil
}

func (m *govMockPort) SetRoomAlias(_ context.Context, _ id.RoomID, alias string) error {
	m.setRoomAliasCalls = append(m.setRoomAliasCalls, alias)
	return nil
}

func (m *govMockPort) AddSpaceChild(_ context.Context, _ id.RoomID, _ id.RoomID, _ string, _ bool) error {
	m.addSpaceChildCalls++
	return nil
}

func (m *govMockPort) SetSpaceParent(_ context.Context, _ id.RoomID, _ id.RoomID) error {
	m.setSpaceParentCalls++
	return nil
}

var (
	govRoomUUID  = uuid.MustParse("880e8400-e29b-41d4-a716-446655440003")
	govSpaceUUID = uuid.MustParse("990e8400-e29b-41d4-a716-446655440004")
)

func newGovMock() *govMockPort {
	testMapper := domain.NewIDMapper("test.local")
	return &govMockPort{
		mockSpaceMatrixPort: &mockSpaceMatrixPort{
			resolveAliasResults: map[string]id.RoomID{
				testMapper.RoomAlias(govRoomUUID):   "!room:test.local",
				testMapper.SpaceAlias(govSpaceUUID): "!space:test.local",
			},
		},
		botPresence: domain.BotPresence{Joined: true, PowerOK: true},
	}
}

func newGovService(mock *govMockPort) *GovernanceService {
	svc := NewGovernanceService(mock, &testutil.MockLogger{}, domain.NewIDMapper("test.local"))
	svc.sleep = func(time.Duration) {} // no pacing in tests
	return svc
}

// convergedRoomState is a room already exactly at the governance target.
func convergedRoomState(entityID string, entityType string) *domain.RoomGovernanceState {
	return &domain.RoomGovernanceState{
		JoinRule:          "invite",
		HistoryVisibility: "shared",
		GuestAccess:       "forbidden",
		Entity:            &domain.EntityMarker{EntityID: entityID, EntityType: entityType},
		Governance:        &domain.GovernanceMarker{LadderVersion: 1, MembershipMode: domain.MembershipModePlatform, AppliedAt: 1, RoomVersion: "10"},
		Aliases: []string{
			"#" + entityID + ":test.local",
			"#t_" + entityID + ":test.local",
		},
	}
}

func TestRepair_Converged_NoWrites(t *testing.T) {
	mock := newGovMock()
	mock.governanceState = convergedRoomState(govRoomUUID.String(), "thread")
	mock.applyLadderChanged = false
	svc := newGovService(mock)

	outcome := svc.RepairRoom(context.Background(), RepairRoomParams{AlkemioRoomID: govRoomUUID})

	if outcome.Writes != 0 {
		t.Errorf("writes = %d, want 0 for a converged room", outcome.Writes)
	}
	if outcome.Repaired != 0 {
		t.Errorf("repaired = %d, want 0", outcome.Repaired)
	}
	if len(mock.setAccessCalls) != 0 || len(mock.setGovStateCalls) != 0 || len(mock.setRoomAliasCalls) != 0 {
		t.Error("a converged room must produce zero state writes")
	}
	if outcome.Scanned != 1 {
		t.Errorf("scanned = %d, want 1", outcome.Scanned)
	}
}

func TestRepair_DryRun_ZeroWrites(t *testing.T) {
	mock := newGovMock()
	// Everything drifted: default (non-converged) governance state + ladder drift.
	mock.applyLadderChanged = true
	svc := newGovService(mock)

	outcome := svc.RepairRoom(context.Background(), RepairRoomParams{AlkemioRoomID: govRoomUUID, DryRun: true})

	if !outcome.DryRun {
		t.Error("outcome must carry dry_run")
	}
	if outcome.Writes == 0 {
		t.Error("dry run must COUNT the writes it would perform")
	}
	if len(mock.setAccessCalls) != 0 || len(mock.setGovStateCalls) != 0 || len(mock.setRoomAliasCalls) != 0 {
		t.Error("dry run must write nothing")
	}
	for _, dryRun := range mock.applyLadderDryRuns {
		if !dryRun {
			t.Error("ApplyLadder must run in dry-run mode")
		}
	}
}

func TestRepair_PreVersion_Skipped(t *testing.T) {
	parent := govSpaceUUID
	mock := newGovMock()
	mock.roomVersion = "6" // predates the restricted join rule
	mock.governanceState = convergedRoomState(govRoomUUID.String(), "thread")
	svc := newGovService(mock)

	outcome := svc.RepairRoom(context.Background(), RepairRoomParams{
		AlkemioRoomID:   govRoomUUID,
		JoinRule:        "restricted",
		ParentContextID: &parent,
	})

	if len(outcome.SkippedPreVersion) != 1 {
		t.Fatalf("skipped_pre_version = %v, want the room listed", outcome.SkippedPreVersion)
	}
	// The room keeps platform-driven membership: no restricted write attempted.
	for _, access := range mock.setAccessCalls {
		if access.JoinRule != nil && *access.JoinRule == "restricted" {
			t.Error("a pre-version room must never be switched to restricted (and never upgraded)")
		}
	}
}

func TestRepair_RestrictedRequiresParent(t *testing.T) {
	mock := newGovMock()
	mock.governanceState = convergedRoomState(govRoomUUID.String(), "thread")
	svc := newGovService(mock)

	// Declared restricted but NO parent: stays invite (platform-driven).
	outcome := svc.RepairRoom(context.Background(), RepairRoomParams{
		AlkemioRoomID: govRoomUUID,
		JoinRule:      "restricted",
	})

	if len(outcome.SkippedPreVersion) != 0 {
		t.Error("no parent is not a version skip")
	}
	for _, access := range mock.setAccessCalls {
		if access.JoinRule != nil && *access.JoinRule == "restricted" {
			t.Error("restricted without a resolvable parent must not be written")
		}
	}
}

func TestRepair_ElevatedRecomputed(t *testing.T) {
	adminActor := uuid.MustParse("aaaa1111-0000-4000-8000-000000000001")
	mock := newGovMock()
	mock.governanceState = convergedRoomState(govSpaceUUID.String(), "space")
	mock.governanceState.Governance.MembershipMode = domain.MembershipModeProjected
	mock.applyLadderChanged = true
	svc := newGovService(mock)

	outcome := svc.RepairSpace(context.Background(), RepairSpaceParams{
		AlkemioContextID: govSpaceUUID,
		ElevatedActorIDs: []uuid.UUID{adminActor},
	})

	if outcome.Repaired != 1 {
		t.Errorf("repaired = %d, want 1", outcome.Repaired)
	}
	if len(mock.applyLadderCalls) != 1 {
		t.Fatalf("ApplyLadder calls = %d, want 1", len(mock.applyLadderCalls))
	}
	elevated := mock.applyLadderCalls[0].Elevated
	if len(elevated) != 1 || elevated[0] != domain.NewIDMapper("test.local").UserID(adminActor) {
		t.Errorf("elevated = %v, want exactly the recomputed admin entry", elevated)
	}
}

func TestRepairSpace_NoHierarchyWrites(t *testing.T) {
	parent := govRoomUUID.String()
	mock := newGovMock()
	state := convergedRoomState(govSpaceUUID.String(), "space")
	state.Governance.MembershipMode = domain.MembershipModeProjected
	// The marker names a parent but no m.space.parent exists — hierarchy drift.
	state.Entity.ParentID = &parent
	state.SpaceParents = nil
	mock.governanceState = state
	svc := newGovService(mock)

	svc.RepairSpace(context.Background(), RepairSpaceParams{
		AlkemioContextID: govSpaceUUID,
		CustomState: map[string]map[string]interface{}{
			"io.alkemio.entity": {"entityId": govSpaceUUID.String(), "entityType": "space", "parentId": parent},
		},
	})

	if mock.addSpaceChildCalls != 0 || mock.setSpaceParentCalls != 0 {
		t.Error("space repair must never write hierarchy links (061 owns convergence)")
	}
}

func TestRepair_NoRoom_Unresolved(t *testing.T) {
	mock := newGovMock()
	mock.resolveAliasResults = nil
	mock.resolveAliasErr = errors.New("M_NOT_FOUND")
	svc := newGovService(mock)

	outcome := svc.RepairRoom(context.Background(), RepairRoomParams{AlkemioRoomID: govRoomUUID})

	if len(outcome.Unresolved) != 1 || outcome.Unresolved[0].Reason != "no-room" {
		t.Errorf("unresolved = %v, want the no-room entry", outcome.Unresolved)
	}
}

func TestRepair_BotUnreachable_Unresolved(t *testing.T) {
	mock := newGovMock()
	mock.botPresence = domain.BotPresence{UnresolvedReason: "bot-unreachable"}
	svc := newGovService(mock)

	outcome := svc.RepairRoom(context.Background(), RepairRoomParams{AlkemioRoomID: govRoomUUID})

	if len(outcome.Unresolved) != 1 || outcome.Unresolved[0].Reason != "bot-unreachable" {
		t.Errorf("unresolved = %v, want bot-unreachable", outcome.Unresolved)
	}
	if len(mock.applyLadderCalls) != 0 {
		t.Error("no writes may be attempted when the bot cannot be established")
	}
}
