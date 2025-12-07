# Implementation Plan: RMQ Events Extension

**Branch**: `007-rmq-events-extension` | **Spec**: [spec.md](spec.md)  
**Status**: ✅ Implemented | **Created**: 2025-12-07

## Summary

Extend the Matrix Adapter RMQ protocol with 5 new topics:
- **3 outgoing events** (Adapter → Server): reaction added/removed, member left
- **2 incoming commands** (Server → Adapter): get room members, get thread messages

This is an additive, non-breaking extension to the V3 protocol.

## Technical Context

**Language/Version**: Go 1.25  
**Primary Dependencies**: mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)  
**Storage**: Matrix room state (no external DB)  
**Testing**: Go `testing` package, testify  
**Target Platform**: Linux container (Docker)  
**Project Type**: Single service (hexagonal architecture)  
**Performance Goals**: Events < 500ms latency, queries < 500ms  
**Constraints**: No pagination for MVP (add later if needed)  
**Scale/Scope**: 5 new topics, ~8 new DTO structs, ~3 handler files

### Key Technical Decisions (from research.md)

1. **Thread Messages**: Use `Client.GetRelations()` with `RelThread` relation type
2. **Room Members**: Reuse existing `intent.JoinedMembers()` via `GetRoomMembers()` method
3. **Reaction Events**: Listen for `event.EventReaction` in listener; parse `RelatesTo`
4. **Redaction Events**: Listen for `event.EventRedaction`; lookup original reaction
5. **Membership Events**: Listen for `event.StateMember`; filter for leave/ban membership
6. **Actor ID Resolution**: Use existing `IDMapper.AlkemioActorID()` method; returns `uuid.Nil` for non-ghost users (which should be ignored)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | ✅ Pass | All Matrix SDK usage in `internal/infrastructure/matrix` |
| 2. Event-Driven State Sync | ✅ Pass | New events published via Watermill |
| 3. Microservice Contract Stability | ✅ Pass | Additive changes only, no breaking changes |
| 4. Matrix Client Lifecycle | ✅ Pass | Using existing Intent API patterns |
| 5. Observability | ✅ Pass | Structured logging for all new operations |
| 6. Pragmatic Testing | ✅ Pass | Unit tests mock Matrix boundary |
| 7. Go Service as Source of Truth | ✅ Pass | DTOs in `pkg/dto`, TypeScript generated |
| 8. Secure Credential Management | ✅ Pass | No credential changes |
| 9. Container Determinism | ✅ Pass | No deployment changes |
| 10. Simplicity | ✅ Pass | Only implementing specified features |

**Justified Violations**: None

## Project Structure

### Documentation (this feature)

```text
specs/007-rmq-events-extension/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Technical research findings
├── data-model.md        # New DTO structures
├── quickstart.md        # Developer guide
├── contracts/
│   └── rmq-events.md    # New RMQ contracts
├── checklists/
│   └── requirements.md  # Quality checklist
└── tasks.md             # Implementation tasks
```

### Source Code (modifications)

```text
# Modified files only (no new files)
pkg/dto/
├── event.go             # + ReactionAddedEvent, ReactionRemovedEvent, RoomMemberLeftEvent
├── message.go           # + GetThreadMessagesRequest/Response
├── room.go              # + GetRoomMembersRequest/Response
└── commands.go          # + Topic constants and registry entries

internal/infrastructure/matrix/
├── mautrix.go           # + GetThreadMessages method
└── listener.go          # + Reaction/redaction/membership event handlers

internal/infrastructure/queue/
├── router.go            # + New topic registrations
├── handler_room.go      # + HandleGetRoomMembers, HandleGetThreadMessages
└── topics.go            # + Topic aliases

internal/core/ports/
└── matrix.go            # + GetThreadMessages interface method
```

**Structure Decision**: Extend existing files. No new packages or files created.

## Implementation Phases

### Phase 1: DTOs & Contracts (Foundation)

**Scope**: Define new data structures and topic constants

| Task | Files | Est. |
|------|-------|------|
| Add outgoing event DTOs | `pkg/dto/event.go` | 0.5h |
| Add thread DTOs | `pkg/dto/message.go` | 0.5h |
| Add GetRoomMembers DTOs | `pkg/dto/room.go` | 0.5h |
| Add topic constants | `pkg/dto/commands.go` | 0.25h |
| Add topic aliases | `internal/infrastructure/queue/topics.go` | 0.25h |

**Exit Criteria**: 
- All DTOs compile and serialize correctly
- `make lint` passes

### Phase 2: Matrix Adapter (P1 Features)

**Scope**: Extend listener for reaction/membership events

| Task | Files | Est. |
|------|-------|------|
| Add reaction event handler | `listener.go` | 1h |
| Add redaction event handler | `listener.go` | 1h |
| Add membership event handler | `listener.go` | 1h |
| Update MatrixPort interface | `ports/matrix.go` | 0.25h |

**Exit Criteria**:
- Listener handles `EventReaction`, `EventRedaction`, `StateMember`
- Events published to correct topics
- Bot's own actions filtered out

### Phase 3: Command Handlers (P1)

**Scope**: GetRoomMembers command

| Task | Files | Est. |
|------|-------|------|
| Add GetRoomMembers handler | `handler_room.go` | 0.5h |
| Register route in router | `router.go` | 0.25h |
| Unit tests | `handler_room_test.go` | 0.5h |

**Exit Criteria**:
- Command returns joined member UUIDs
- Error handling for invalid rooms

### Phase 4: Thread Messages (P2)

**Scope**: GetThreadMessages command

| Task | Files | Est. |
|------|-------|------|
| Add GetThreadMessages method | `mautrix.go` | 1h |
| Add handler to existing file | `handler_room.go` | 0.5h |
| Register route | `router.go` | 0.25h |

**Exit Criteria**:
- Returns all thread replies using Matrix relations API
- Error handling for missing messages

### Phase 5: Integration & Cleanup

**Scope**: TypeScript generation, testing, docs

| Task | Files | Est. |
|------|-------|------|
| Run `make generate` | TypeScript lib | 0.25h |
| Manual integration testing | - | 1h |
| Update README | `README.md` | 0.25h |
| Final lint & tests | All | 0.5h |

**Exit Criteria**:
- TypeScript types match Go DTOs
- `make build` and `make test` pass
- All new topics documented

## Risk Register

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Redaction without original | Medium | Low | Log warning, publish partial event |
| High event volume | Medium | Medium | Document in spec, add throttling later |
| Thread API differences | Low | Low | Using standard Matrix relations API |
| Actor ID format changes | High | Very Low | Extract from localpart, validate UUID |

## Dependencies

| Dependency | Type | Status |
|------------|------|--------|
| mautrix-go | Library | Stable, GetRelations available |
| Feature 004 (Protocol V3) | Internal | Completed |
| Feature 006 (Room Control) | Internal | Completed |

## Complexity Tracking

No constitution violations. No justified complexity additions required.
