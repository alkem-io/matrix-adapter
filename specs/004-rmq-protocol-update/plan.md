# Implementation Plan: RMQ Protocol Update

**Branch**: `004-rmq-protocol-update` | **Spec**: [spec.md](spec.md)  
**Status**: ✅ Completed | **Completed**: 2025-12-02

## Summary

Updated the Matrix Adapter RMQ protocol to conform to `MatrixAdapterProtocol.md`. Key changes:

1. **Renaming all RMQ topics** to use `communication.` prefix
2. **Replacing DTOs** with new structures using explicit UUID types and Alkemio-native identifiers
3. **Adding room alias-based mapping** (`#<UUID>:<homeserver>`) for idempotent room creation
4. **Updating batch operations** to return per-room results map instead of simple arrays
5. **Enhancing room delete** to kick all members and remove room aliases

No backward compatibility is required.

## Technical Context

**Language/Version**: Go 1.25  
**Primary Dependencies**: mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)  
**Storage**: Matrix room aliases (no external DB)  
**Testing**: Go `testing` package, testify  
**Target Platform**: Linux container (Docker)
**Project Type**: Single service (hexagonal architecture)  
**Performance Goals**: < 2 seconds response time per command  
**Constraints**: Fail fast on Matrix errors (no internal retries)  
**Scale/Scope**: 14 RMQ command types, ~15 DTO structs, ~5 handler files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | ✅ Pass | All Matrix SDK usage stays in `internal/infrastructure/matrix` |
| 2. Event-Driven State Sync | ✅ Pass | Using Watermill for all RMQ operations |
| 3. Microservice Contract Stability | ⚠️ Justified | Breaking change intentional - no backward compat per spec |
| 4. Matrix Client Lifecycle | ✅ Pass | Using Intent API for Application Service pattern |
| 5. Observability | ✅ Pass | Structured logging with Zap maintained |
| 6. Pragmatic Testing | ✅ Pass | Unit tests mock Matrix boundary via ports |
| 7. Go Service as Source of Truth | ✅ Pass | DTOs in `pkg/dto`, TypeScript generated |
| 8. Secure Credential Management | ✅ Pass | No changes to credential handling |
| 9. Container Determinism | ✅ Pass | No changes to Docker/deployment |
| 10. Simplicity | ✅ Pass | Implementing only specified protocol commands |

**Justified Violations**:
- Principle 3 (Contract Stability): Breaking change is explicitly required by the spec. The Alkemio Server will be updated concurrently.

## Project Structure

### Documentation (this feature)

```text
specs/004-rmq-protocol-update/
├── plan.md              # This file
├── research.md          # Gap analysis and implementation notes
├── data-model.md        # New DTO structures
├── quickstart.md        # Developer integration guide
├── contracts/
│   └── rmq-commands.md  # All 14 command contracts
├── checklists/
│   └── requirements.md  # Quality checklist
└── tasks.md             # Implementation tasks (Phase 2)
```

### Source Code (repository root)

```text
# Go Hexagonal Architecture (existing structure)
cmd/
├── adapter/main.go          # Entry point (no changes)
└── gen-events/main.go       # TypeScript generator

internal/
├── app/app.go               # Wiring (minor changes for new handlers)
├── config/config.go         # Config (no changes)
├── core/
│   ├── domain/
│   │   └── model.go         # Add Reaction entity
│   ├── ports/
│   │   └── matrix.go        # Add new interface methods
│   └── service/
│       ├── room_service.go  # New methods for room operations
│       └── actor_service.go # Refactor for sync operation
└── infrastructure/
    ├── matrix/
    │   └── mautrix.go       # Add alias ops, kick, reaction get
    ├── queue/
    │   ├── router.go        # New topic names
    │   ├── handler_room.go  # Updated handlers
    │   ├── handler_message.go  # NEW: message handlers
    │   ├── handler_reaction.go # NEW: reaction handlers
    │   ├── handler_actor.go    # Simplified (sync only)
    │   └── errors.go        # New error codes
    └── logger/zap.go        # No changes

pkg/dto/
├── types.go             # NEW: AlkemioRoomID, AlkemioActorID, etc.
├── error.go             # Updated error codes
├── room.go              # Rewritten for new protocol
├── message.go           # Rewritten for new protocol
├── reaction.go          # NEW: reaction DTOs
├── actor.go             # Simplified for sync
└── batch.go             # NEW: batch operation DTOs

lib/src/dto/
└── generated.ts         # Regenerated from Go
```

**Structure Decision**: Maintain existing hexagonal architecture. Split handlers by domain (room, message, reaction, actor) for clarity.

## Implementation Phases

### Phase 1: Foundation (DTOs & Error Handling)

**Scope**: New type system and error codes

| Task | Files | Est. |
|------|-------|------|
| Define UUID type aliases | `pkg/dto/types.go` | 0.5h |
| Update error codes | `pkg/dto/error.go` | 0.5h |
| Create room DTOs | `pkg/dto/room.go` | 1h |
| Create message DTOs | `pkg/dto/message.go` | 1h |
| Create reaction DTOs | `pkg/dto/reaction.go` | 0.5h |
| Create batch DTOs | `pkg/dto/batch.go` | 0.5h |
| Update actor DTOs | `pkg/dto/actor.go` | 0.5h |

**Exit Criteria**: 
- All DTOs compile
- `make lint` passes
- Unit tests for type marshaling

### Phase 2: Matrix Adapter Extensions

**Scope**: New Matrix operations for room aliases and reactions

| Task | Files | Est. |
|------|-------|------|
| Add ResolveAlias method | `internal/infrastructure/matrix/mautrix.go` | 1h |
| Add DeleteAlias method | `internal/infrastructure/matrix/mautrix.go` | 0.5h |
| Add KickUser method | `internal/infrastructure/matrix/mautrix.go` | 0.5h |
| Add GetRoomMessages method | `internal/infrastructure/matrix/mautrix.go` | 1h |
| Add GetReaction method | `internal/infrastructure/matrix/mautrix.go` | 1h |
| Update CreateRoom for aliases | `internal/infrastructure/matrix/mautrix.go` | 1h |
| Update MatrixPort interface | `internal/core/ports/matrix.go` | 0.5h |
| Add Reaction domain entity | `internal/core/domain/model.go` | 0.5h |

**Exit Criteria**:
- All new methods implemented
- Interface matches implementation
- Unit tests with mocked Matrix responses

### Phase 3: Service Layer Updates

**Scope**: Business logic for new operations

| Task | Files | Est. |
|------|-------|------|
| Refactor RoomService.CreateRoom | `internal/core/service/room_service.go` | 1h |
| Add RoomService.DeleteRoom (full) | `internal/core/service/room_service.go` | 1h |
| Add RoomService.GetRoomWithMessages | `internal/core/service/room_service.go` | 1h |
| Add RoomService.ListRooms | `internal/core/service/room_service.go` | 0.5h |
| Add reaction service methods | `internal/core/service/room_service.go` | 1h |
| Refactor ActorService for sync | `internal/core/service/actor_service.go` | 0.5h |
| Update batch operations | `internal/core/service/actor_service.go` | 1h |

**Exit Criteria**:
- All service methods pass unit tests
- Error mapping follows new error codes

### Phase 4: Handler & Router Updates

**Scope**: RMQ integration with new topics

| Task | Files | Est. |
|------|-------|------|
| Update router with new topics | `internal/infrastructure/queue/router.go` | 0.5h |
| Rewrite room handlers | `internal/infrastructure/queue/handler_room.go` | 2h |
| Create message handlers | `internal/infrastructure/queue/handler_message.go` | 1.5h |
| Create reaction handlers | `internal/infrastructure/queue/handler_reaction.go` | 1h |
| Simplify actor handlers | `internal/infrastructure/queue/handler_actor.go` | 0.5h |
| Update error mapping | `internal/infrastructure/queue/errors.go` | 0.5h |
| Remove deprecated handlers | Various | 0.5h |

**Exit Criteria**:
- All 14 commands handled
- Integration tests pass with mock RMQ

### Phase 5: TypeScript Library & Cleanup

**Scope**: Generate TypeScript, update docs

| Task | Files | Est. |
|------|-------|------|
| Run tygo generation | `make generate` | 0.25h |
| Update README | `README.md` | 0.5h |
| Update MatrixAdapterProtocol.md | Root | 0.25h |
| Final lint & test | All | 0.5h |

**Exit Criteria**:
- TypeScript types match Go DTOs
- `make build` passes
- `make test` passes
- Documentation updated

## Risk Register

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Room alias conflicts | Medium | Low | Use full UUIDs, idempotent creation |
| Large message payloads | Medium | Medium | Document in spec, future pagination |
| Matrix SDK version issues | Low | Low | Lock mautrix-go version |
| Alkemio Server not ready | High | Low | Coordinate deployment timing |

## Dependencies

| Dependency | Type | Status |
|------------|------|--------|
| mautrix-go | Library | Stable |
| Watermill | Library | Stable |
| Alkemio Server update | External | Required concurrent |
| Synapse homeserver | Infrastructure | Available |

## Complexity Tracking

| Justified Complexity | Reason | Simpler Alternative Rejected |
|---------------------|--------|------------------------------|
| Room alias mapping | Protocol requires Alkemio UUIDs | External DB adds infrastructure |
| Per-room results map | Better error handling for batches | Simple arrays lose error context |

## Estimated Total Effort

| Phase | Estimate |
|-------|----------|
| Phase 1: DTOs | 4.5h |
| Phase 2: Matrix Adapter | 6h |
| Phase 3: Services | 6h |
| Phase 4: Handlers | 6.5h |
| Phase 5: Cleanup | 1.5h |
| **Total** | **~24.5h** |

> Note: Admin room listing (communication.room.list) is consolidated into US1 Room Lifecycle.

## Post-Implementation

- [X] Update Alkemio Server to use new protocol → Pending server team
- [X] Deploy both services together → Ready for deployment
- [ ] Monitor for unexpected errors → Post-deploy
- [X] Update documentation referencing old topics → README and MatrixAdapterProtocol.md updated
