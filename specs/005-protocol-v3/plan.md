# Implementation Plan: Protocol V3 Implementation

**Branch**: `005-protocol-v3` | **Spec**: [spec.md](spec.md)  
**Status**: ✅ Completed | **Completed**: 2025-12-03

## Summary

Implement Matrix Adapter Protocol V3 changes: Add `AlkemioContextID` type and `JoinRule` enum, implement 8 new Space-related commands (Space CRUD, Space membership batch operations, hierarchy management via `set_parent`), extend room creation/update with `avatar_url`, `parent_context_id`, and `join_rule` fields. No backward compatibility required—remove deprecated `Limit` field from `ListRoomsRequest`.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: `mautrix-go` (Matrix SDK), `Watermill` (RabbitMQ), `zap` (Logging), `tygo` (TS generation)
**Storage**: Room/Space aliases for ID mapping (no external DB)
**Testing**: Go `testing` package, `testify` for assertions, mock interfaces in `internal/core/ports`
**Target Platform**: Linux container (Docker)
**Project Type**: Single Go service with TypeScript library generation
**Performance Goals**: <2s response time for typical Space operations
**Constraints**: MSC1772 (Spaces) and MSC3083 (Restricted Rooms) Matrix homeserver support required
**Scale/Scope**: 8 new RabbitMQ command handlers, ~15 new DTO types, ~10 new Matrix port methods

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **1. Adapter-First Domain Isolation** | ✅ PASS | All new Matrix SDK interactions will be in `internal/infrastructure/matrix`. New `MatrixPort` methods for Space operations. |
| **2. Event-Driven State Synchronization** | ✅ PASS | Commands follow existing RabbitMQ pattern via Watermill. No blocking sync calls. |
| **3. Microservice Contract Stability** | ✅ PASS | Breaking change (`Limit` removal) is documented; V3 is a coordinated release with Alkemio Server. New commands documented in protocol spec. |
| **4. Matrix Client Lifecycle Management** | ✅ PASS | Follows existing Intent pattern via AppService. No new client types introduced. |
| **5. Observability with Matrix Context** | ✅ PASS | All new handlers will log `contextID`, `roomID`, `userID` using existing `zap` patterns. |
| **6. Pragmatic Testing with SDK Boundaries** | ✅ PASS | Unit tests will mock `MatrixPort` interface. Integration tests deferred unless complex SDK flows emerge. |
| **7. Go Service as Source of Truth** | ✅ PASS | All new DTOs in `pkg/dto/`. Run `make generate` to produce TypeScript. |
| **8. Secure Matrix Credential Management** | ✅ PASS | No new credential handling. Uses existing AppService token flow. |
| **9. Container Determinism and Configuration** | ✅ PASS | No new configuration. Uses existing homeserver URL. |
| **10. Simplicity and Matrix API Coverage** | ✅ PASS | Only implements Space features explicitly required by V3 protocol. No speculative API wrapping. |

## Project Structure

### Documentation (this feature)

```text
specs/005-protocol-v3/
├── plan.md              # This file
├── research.md          # Phase 0: Matrix Spaces research
├── data-model.md        # Phase 1: Domain model changes
├── quickstart.md        # Phase 1: Developer quickstart
├── contracts/           # Phase 1: RMQ command contracts
│   └── space-commands.md
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
pkg/dto/                           # DTOs (Source of Truth)
├── types.go                       # + AlkemioContextID, JoinRule
├── room.go                        # + avatar_url, parent_context_id, join_rule fields
├── space.go                       # NEW: Space request/response DTOs
└── hierarchy.go                   # NEW: SetParentRequest/Response

internal/core/domain/
└── model.go                       # + Space domain model

internal/core/ports/
└── matrix.go                      # + Space-related interface methods

internal/core/service/
├── room_service.go                # Extended for new room fields
└── space_service.go               # NEW: Space business logic

internal/infrastructure/matrix/
└── mautrix.go                     # + Space SDK operations

internal/infrastructure/queue/
├── router.go                      # + 8 new topic subscriptions
├── handler_room.go                # Extended for new room fields
└── handler_space.go               # NEW: Space command handlers

lib/src/dto/
└── generated.ts                   # Auto-generated via tygo

lib/src/
└── matrix.adapter.event.type.ts   # + 8 new event types
```

**Structure Decision**: Follows existing hexagonal architecture. New Space operations parallel existing Room pattern. New files (`space.go`, `hierarchy.go`, `handler_space.go`, `space_service.go`) isolate Space concerns while reusing infrastructure.

## Complexity Tracking

> No constitution violations. All changes follow established patterns.

## Post-Implementation Improvements

*Completed after all 62 planned tasks. These address code quality and architecture refinements discovered during implementation.*

### Topic Constants (SSOT)

**Problem**: RabbitMQ topic strings were duplicated as literals in `router.go` and parsed by `gen-events` from the same file.

**Solution**: Created `internal/infrastructure/queue/topics.go` with 22 centralized topic constants organized by category (Room, Message, Reaction, Actor, Space, Hierarchy). Updated `router.go` to use constants.

**Files**:
- NEW: [internal/infrastructure/queue/topics.go](../../internal/infrastructure/queue/topics.go)
- MODIFIED: [internal/infrastructure/queue/router.go](../../internal/infrastructure/queue/router.go)

### Domain Model Helper

**Problem**: `domain.Actor{ID: memberID.UUID()}` pattern repeated across 8 handler call sites.

**Solution**: Added `domain.NewActor(id uuid.UUID)` constructor in `model.go`. Updated all call sites.

**Files**:
- MODIFIED: [internal/core/domain/model.go](../../internal/core/domain/model.go)

### Context Propagation Fix

**Problem**: Handlers used `context.Background()` instead of propagating the message context from Watermill. This breaks tracing, timeouts, and cancellation.

**Solution**: Updated `ports.MessageHandler` type signature to accept `context.Context`. Updated `watermill.go` to pass `msg.Context()`. Updated all 22 handlers across 3 files.

**Files**:
- MODIFIED: [internal/core/ports/queue.go](../../internal/core/ports/queue.go)
- MODIFIED: [internal/infrastructure/queue/watermill.go](../../internal/infrastructure/queue/watermill.go)
- MODIFIED: [internal/infrastructure/queue/handler_room.go](../../internal/infrastructure/queue/handler_room.go)
- MODIFIED: [internal/infrastructure/queue/handler_space.go](../../internal/infrastructure/queue/handler_space.go)
- MODIFIED: [internal/infrastructure/queue/handler_actor.go](../../internal/infrastructure/queue/handler_actor.go)

### Dead Code Removal

**Problem**: `SpaceHandler` struct had unused `matrix` field (MatrixPort was only used by SpaceService).

**Solution**: Removed `matrix` field from struct, updated constructor and `app.go`.

**Files**:
- MODIFIED: [internal/infrastructure/queue/handler_space.go](../../internal/infrastructure/queue/handler_space.go)
- MODIFIED: [internal/app/app.go](../../internal/app/app.go)

### TypeScript Generation Fix

**Problem**: After topic constants refactor, `gen-events` tool only found 1 event (was parsing string literals in `router.go`).

**Solution**: Rewrote `parseTopicConstants()` to parse `topics.go` using AST to find const declarations.

**Files**:
- MODIFIED: [cmd/gen-events/main.go](../../cmd/gen-events/main.go)

### Evaluated but Skipped

**Slice conversion helpers**: Evaluated adding generic slice mapping helpers. Decision: **SKIP**. Current `make, loop, append` patterns are idiomatic Go and immediately understandable. Adding helpers would add indirection without meaningful simplification.

### Topic Naming Consistency

**Problem**: Outbound event topic `message.received` didn't follow the `communication.<domain>.<action>` naming convention used by all other topics.

**Solution**: Renamed to `communication.message.received`. Added `TopicMessageReceived` constant under new "Event Topics (Outbound)" section in `topics.go`.

**Files**:
- MODIFIED: [internal/infrastructure/queue/topics.go](../../internal/infrastructure/queue/topics.go)
- MODIFIED: [internal/core/service/event_service.go](../../internal/core/service/event_service.go)
- MODIFIED: [README.md](../../README.md)

### Documentation Updates

**Problem**: Documentation was outdated - README missing V3 commands, protocol docs needed status headers, lib README was minimal.

**Solution**: Comprehensive documentation update:

1. **README.md**: Added Space commands, Hierarchy command, SPACE_NOT_FOUND error code, updated protocol reference to V3
2. **MatrixAdapterProtocol_V2.md**: Added deprecation notice with link to V3
3. **MatrixAdapterProtocol_V3.md**: Added status header (✅ Current v3.0.0) and implementation reference section
4. **lib/README.md**: Complete rewrite with installation, usage examples, API reference (23 events), type aliases, constants, and response handling patterns

**Files**:
- MODIFIED: [README.md](../../README.md)
- MODIFIED: [MatrixAdapterProtocol_V2.md](../../MatrixAdapterProtocol_V2.md)
- MODIFIED: [MatrixAdapterProtocol_V3.md](../../MatrixAdapterProtocol_V3.md)
- MODIFIED: [lib/README.md](../../lib/README.md)
