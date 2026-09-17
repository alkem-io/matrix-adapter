package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

// Defaults of the report-first repair protocol (spec FR-022).
const (
	// defaultRepairWriteInterval paces state writes at 5/s against the
	// rate-limited homeserver (plan: reuse 061's budget pattern).
	defaultRepairWriteInterval = 200 * time.Millisecond
	// minRestrictedRoomVersion is the first room version supporting the
	// space-scoped (restricted) join rule.
	minRestrictedRoomVersion = 8
)

// GovernanceService converges rooms and spaces to the governance ladder:
// report-first (dry-run) repair with compare-before-write, write counting,
// pacing, and a divergence record per repaired aspect.
type GovernanceService struct {
	matrix        ports.MatrixPort
	logger        ports.Logger
	idMapper      *domain.IDMapper
	writeInterval time.Duration
	sleep         func(time.Duration) // injectable for tests
}

// NewGovernanceService creates a new instance of GovernanceService.
func NewGovernanceService(matrix ports.MatrixPort, logger ports.Logger, idMapper *domain.IDMapper) *GovernanceService {
	return &GovernanceService{
		matrix:        matrix,
		logger:        logger,
		idMapper:      idMapper,
		writeInterval: defaultRepairWriteInterval,
		sleep:         time.Sleep,
	}
}

// RepairRoomParams carries one room repair (topic communication.room.governance.repair).
type RepairRoomParams struct {
	AlkemioRoomID   uuid.UUID
	JoinRule        string
	ParentContextID *uuid.UUID
	CustomState     map[string]map[string]interface{}
	Visibility      string // "shared" | "world_readable"
	IsDirect        bool
	DryRun          bool
}

// RepairSpaceParams carries one space repair (topic communication.space.governance.repair).
type RepairSpaceParams struct {
	AlkemioContextID uuid.UUID
	CustomState      map[string]map[string]interface{}
	ElevatedActorIDs []uuid.UUID
	DryRun           bool
}

// repairRun accumulates one repair invocation's outcome and paces its writes.
type repairRun struct {
	service *GovernanceService
	outcome domain.RepairOutcome
	dryRun  bool
}

func (r *repairRun) wrote() {
	r.outcome.Writes++
	if !r.dryRun {
		r.service.sleep(r.service.writeInterval)
	}
}

// RepairRoom converges one room to the governance ladder (contract
// room-governance-ladder §2.2): power levels (guests preserved), join rules,
// history visibility, guest access, identity marker, both aliases, m.direct,
// and the governance marker last. A converged room produces zero writes.
func (s *GovernanceService) RepairRoom(ctx context.Context, params RepairRoomParams) domain.RepairOutcome {
	run := &repairRun{service: s, dryRun: params.DryRun}
	run.outcome.Scanned = 1
	run.outcome.DryRun = params.DryRun
	entityID := params.AlkemioRoomID.String()

	// 1. The alias IS the identity — a room without one is unresolved.
	roomID, err := s.matrix.ResolveAlias(ctx, s.idMapper.RoomAlias(params.AlkemioRoomID))
	if err != nil {
		run.outcome.Unresolved = append(run.outcome.Unresolved, domain.RepairProblem{ID: entityID, Reason: "no-room"})
		return run.outcome
	}

	// 2. Bot presence and power.
	if !s.ensureBot(ctx, run, roomID, entityID) {
		return run.outcome
	}

	// 3. Room version caps the declared mode (never upgrade in place — FR-009).
	joinRule, membershipMode, skipped := s.resolveDeclaredMode(ctx, roomID, entityID, params)
	if skipped {
		run.outcome.SkippedPreVersion = append(run.outcome.SkippedPreVersion, entityID)
	}

	current, err := s.matrix.GetRoomGovernanceState(ctx, roomID)
	if err != nil {
		run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
		return run.outcome
	}

	// 4. Compare-before-write repairs, then the governance marker last.
	repaired := s.repairRoomAspects(ctx, run, roomID, entityID, current, joinRule, params)
	repaired = s.finalizeGovernanceMarker(ctx, run, roomID, entityID, membershipMode, repaired, current)

	if repaired {
		run.outcome.Repaired = 1
	}
	return run.outcome
}

// repairRoomAspects converges the ladder, access state, identity marker,
// thread alias and m.direct bookkeeping of one room.
func (s *GovernanceService) repairRoomAspects(
	ctx context.Context, run *repairRun, roomID id.RoomID, entityID string,
	current *domain.RoomGovernanceState, joinRule string, params RepairRoomParams,
) bool {
	repaired := false
	class := domain.ClassConversation
	if params.ParentContextID != nil && !params.IsDirect {
		class = domain.ClassThread
	}

	// 4a. Power levels (wholesale, guests preserved inside ApplyLadder).
	changed, err := s.matrix.ApplyLadder(ctx, roomID, class, domain.LadderOptions{}, params.DryRun)
	if err != nil {
		run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
		return repaired
	}
	if changed {
		domain.LogDivergence(s.logger, roomID.String(), entityID, domain.DivergenceLadder, "power levels drifted from ladder v1")
		run.wrote()
		repaired = true
	}

	// 4b. Join rules / history visibility / guest access (compare-before-write).
	if s.repairAccess(ctx, run, roomID, entityID, current, joinRule, params.Visibility) {
		repaired = true
	}

	// 4c. Identity marker.
	if s.repairEntityMarker(ctx, run, roomID, entityID, current, params) {
		repaired = true
	}

	// 4d. Both aliases (the canonical #<uuid> resolved above; ensure #t_<uuid>).
	if s.repairThreadAlias(ctx, run, roomID, params.AlkemioRoomID, current) {
		repaired = true
	}

	// 4e. m.direct for direct rooms (account data, idempotent, uncounted).
	if params.IsDirect && !params.DryRun {
		if err := s.matrix.EnsureDirectRoomMarked(ctx, roomID); err != nil {
			s.logger.Warn("Repair: failed to ensure m.direct", "room_id", roomID, "error", err)
		}
	}
	return repaired
}

// finalizeGovernanceMarker writes io.alkemio.governance last, when anything
// was repaired or the marker is absent, and returns the final repaired flag.
func (s *GovernanceService) finalizeGovernanceMarker(
	ctx context.Context, run *repairRun, roomID id.RoomID, entityID, membershipMode string,
	repaired bool, current *domain.RoomGovernanceState,
) bool {
	if !repaired && current.Governance != nil {
		return repaired
	}
	if !run.dryRun {
		roomVersion, _ := s.matrix.GetRoomVersion(ctx, roomID)
		if err := s.matrix.SetGovernanceState(ctx, roomID, nil, &domain.GovernanceMarker{
			LadderVersion:  1,
			MembershipMode: membershipMode,
			AppliedAt:      time.Now().UnixMilli(),
			RoomVersion:    roomVersion,
		}); err != nil {
			run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
			return repaired
		}
	}
	run.wrote()
	return true
}

// ensureBot establishes the bot; an unresolvable room is reported, never an error.
func (s *GovernanceService) ensureBot(ctx context.Context, run *repairRun, roomID id.RoomID, entityID string) bool {
	presence, err := s.matrix.EnsureBotAdmin(ctx, roomID)
	if err != nil {
		run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
		return false
	}
	if presence.UnresolvedReason != "" {
		domain.LogDivergence(s.logger, roomID.String(), entityID, domain.DivergenceBotPresence, presence.UnresolvedReason)
		run.outcome.Unresolved = append(run.outcome.Unresolved, domain.RepairProblem{ID: entityID, Reason: presence.UnresolvedReason})
		return false
	}
	return true
}

// resolveDeclaredMode caps the declared join rule by room-version capability.
// Returns the effective join rule, membership mode, and whether the room was
// skipped as pre-version.
func (s *GovernanceService) resolveDeclaredMode(
	ctx context.Context, roomID id.RoomID, entityID string, params RepairRoomParams,
) (string, string, bool) {
	if params.JoinRule != "restricted" || params.ParentContextID == nil {
		return "invite", domain.MembershipModePlatform, false
	}
	version, err := s.matrix.GetRoomVersion(ctx, roomID)
	if err != nil || !roomVersionSupportsRestricted(version) {
		if err != nil {
			s.logger.Warn("Repair: failed to read room version, staying platform-driven",
				"room_id", roomID, "error", err)
			return "invite", domain.MembershipModePlatform, false
		}
		domain.LogDivergence(s.logger, roomID.String(), entityID, domain.DivergencePreVersion,
			"room version "+version+" predates the restricted join rule; membership stays platform-driven")
		return "invite", domain.MembershipModePlatform, true
	}
	return "restricted", domain.MembershipModeSpace, false
}

// repairAccess converges join rules, history visibility and guest access.
func (s *GovernanceService) repairAccess(
	ctx context.Context, run *repairRun, roomID id.RoomID, entityID string,
	current *domain.RoomGovernanceState, joinRule, visibility string,
) bool {
	access := domain.RoomAccessState{}
	if visibility == "" {
		visibility = "shared"
	}

	if current.JoinRule != joinRule {
		rule := joinRule
		access.JoinRule = &rule
	}
	if joinRule == "restricted" {
		// The allow entry must point at the current parent space room.
		// (ParentContextID resolution failure keeps joinRule "invite" upstream.)
		access.JoinRuleAllowRoom = current.JoinRuleAllowRoom
	}
	if current.HistoryVisibility != visibility {
		v := visibility
		access.HistoryVisibility = &v
	}
	if current.GuestAccess != "forbidden" {
		guest := "forbidden"
		access.GuestAccess = &guest
	}

	if access.JoinRule == nil && access.HistoryVisibility == nil && access.GuestAccess == nil {
		return false
	}
	domain.LogDivergence(s.logger, roomID.String(), entityID, domain.DivergenceLadder,
		"access state drifted (join rule / history visibility / guest access)")
	if !run.dryRun {
		if err := s.matrix.SetRoomAccessState(ctx, roomID, access); err != nil {
			run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
			return false
		}
	}
	for _, set := range []bool{access.JoinRule != nil, access.HistoryVisibility != nil, access.GuestAccess != nil} {
		if set {
			run.wrote()
		}
	}
	return true
}

// repairEntityMarker converges io.alkemio.entity (compare-before-write).
func (s *GovernanceService) repairEntityMarker(
	ctx context.Context, run *repairRun, roomID id.RoomID, entityID string,
	current *domain.RoomGovernanceState, params RepairRoomParams,
) bool {
	want := entityMarkerFromCustomState(params.CustomState)
	if want == nil {
		var parentID *string
		if params.ParentContextID != nil {
			parent := params.ParentContextID.String()
			parentID = &parent
		}
		want = &domain.EntityMarker{EntityID: entityID, EntityType: "thread", ParentID: parentID}
	}
	return s.repairMarker(ctx, run, roomID, entityID, current, want, "io.alkemio.entity missing or drifted")
}

// repairThreadAlias ensures the #t_<uuid> alias exists.
func (s *GovernanceService) repairThreadAlias(
	ctx context.Context, run *repairRun, roomID id.RoomID, alkemioRoomID uuid.UUID,
	current *domain.RoomGovernanceState,
) bool {
	threadAlias := s.idMapper.ThreadRoomAlias(alkemioRoomID)
	for _, alias := range current.Aliases {
		if alias == threadAlias {
			return false
		}
	}
	domain.LogDivergence(s.logger, roomID.String(), alkemioRoomID.String(), domain.DivergenceAlias, "#t_ alias missing")
	if !run.dryRun {
		if err := s.matrix.SetRoomAlias(ctx, roomID, threadAlias); err != nil {
			// The registration namespace may not have rolled out yet (spec edge
			// case): record, keep repairing the rest.
			domain.LogDivergence(s.logger, roomID.String(), alkemioRoomID.String(), domain.DivergenceAlias,
				"failed to create #t_ alias: "+err.Error())
			run.outcome.Unresolved = append(run.outcome.Unresolved,
				domain.RepairProblem{ID: alkemioRoomID.String(), Reason: "t-alias-refused"})
			return false
		}
	}
	run.wrote()
	return true
}

// RepairSpace converges one space room: ladder with recomputed class-S 75
// entries, access state, identity marker, governance marker. Hierarchy links
// (m.space.parent / m.space.child) are verified as a REPORT only — 061 owns
// hierarchy convergence and repair never writes them (T018).
func (s *GovernanceService) RepairSpace(ctx context.Context, params RepairSpaceParams) domain.RepairOutcome {
	run := &repairRun{service: s, dryRun: params.DryRun}
	run.outcome.Scanned = 1
	run.outcome.DryRun = params.DryRun
	entityID := params.AlkemioContextID.String()

	roomID, err := s.matrix.ResolveAlias(ctx, s.idMapper.SpaceAlias(params.AlkemioContextID))
	if err != nil {
		run.outcome.Unresolved = append(run.outcome.Unresolved, domain.RepairProblem{ID: entityID, Reason: "no-room"})
		return run.outcome
	}

	if !s.ensureBot(ctx, run, roomID, entityID) {
		return run.outcome
	}

	current, err := s.matrix.GetRoomGovernanceState(ctx, roomID)
	if err != nil {
		run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
		return run.outcome
	}

	repaired := false

	// Ladder with the recomputed elevated set (never preserved from the old event).
	elevated := make([]id.UserID, 0, len(params.ElevatedActorIDs))
	for _, actorID := range params.ElevatedActorIDs {
		elevated = append(elevated, s.idMapper.UserID(actorID))
	}
	changed, err := s.matrix.ApplyLadder(ctx, roomID, domain.ClassSpace, domain.LadderOptions{Elevated: elevated}, params.DryRun)
	if err != nil {
		run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
		return run.outcome
	}
	if changed {
		domain.LogDivergence(s.logger, roomID.String(), entityID, domain.DivergenceLadder, "space ladder drifted")
		run.wrote()
		repaired = true
	}

	// Access state: spaces keep invite; history shared; guests forbidden.
	if s.repairAccess(ctx, run, roomID, entityID, current, "invite", "shared") {
		repaired = true
	}

	// Identity marker.
	want := entityMarkerFromCustomState(params.CustomState)
	if want == nil {
		want = &domain.EntityMarker{EntityID: entityID, EntityType: "space", ParentID: nil}
	}
	if s.repairMarker(ctx, run, roomID, entityID, current, want, "space io.alkemio.entity missing or drifted") {
		repaired = true
	}

	// Hierarchy: report-only (061 owns convergence; upgrades/writes forbidden here).
	if want.ParentID != nil && len(current.SpaceParents) == 0 {
		domain.LogDivergence(s.logger, roomID.String(), entityID, domain.DivergenceMarker,
			"m.space.parent missing (hierarchy convergence is owned by syncSpaceHierarchy)")
	}

	repaired = s.finalizeGovernanceMarker(ctx, run, roomID, entityID, domain.MembershipModeProjected, repaired, current)

	if repaired {
		run.outcome.Repaired = 1
	}
	return run.outcome
}

// repairMarker converges io.alkemio.entity to the wanted marker (compare-before-write).
func (s *GovernanceService) repairMarker(
	ctx context.Context, run *repairRun, roomID id.RoomID, entityID string,
	current *domain.RoomGovernanceState, want *domain.EntityMarker, detail string,
) bool {
	if current.Entity != nil && entityMarkersEqual(current.Entity, want) {
		return false
	}
	domain.LogDivergence(s.logger, roomID.String(), entityID, domain.DivergenceMarker, detail)
	if !run.dryRun {
		if err := s.matrix.SetGovernanceState(ctx, roomID, want, nil); err != nil {
			run.outcome.Failed = append(run.outcome.Failed, domain.RepairProblem{ID: entityID, Reason: err.Error()})
			return false
		}
	}
	run.wrote()
	return true
}

// roomVersionSupportsRestricted reports whether a Matrix room version supports
// the space-scoped (restricted) join rule (room version >= 8; non-numeric
// versions are experimental and treated as unsupported).
func roomVersionSupportsRestricted(version string) bool {
	n := 0
	for _, r := range version {
		if r < '0' || r > '9' {
			return false
		}
		n = n*10 + int(r-'0')
	}
	return n >= minRestrictedRoomVersion
}

// entityMarkerFromCustomState extracts io.alkemio.entity from a custom_state map.
func entityMarkerFromCustomState(customState map[string]map[string]interface{}) *domain.EntityMarker {
	content, ok := customState["io.alkemio.entity"]
	if !ok {
		return nil
	}
	marker := &domain.EntityMarker{}
	if entityID, ok := content["entityId"].(string); ok {
		marker.EntityID = entityID
	}
	if entityType, ok := content["entityType"].(string); ok {
		marker.EntityType = entityType
	}
	if parentID, ok := content["parentId"].(string); ok && parentID != "" {
		marker.ParentID = &parentID
	}
	if marker.EntityID == "" {
		return nil
	}
	return marker
}

// entityMarkersEqual compares two identity markers semantically.
func entityMarkersEqual(a, b *domain.EntityMarker) bool {
	if a.EntityID != b.EntityID || a.EntityType != b.EntityType {
		return false
	}
	switch {
	case a.ParentID == nil && b.ParentID == nil:
		return true
	case a.ParentID != nil && b.ParentID != nil:
		return *a.ParentID == *b.ParentID
	default:
		return false
	}
}
