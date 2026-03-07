# Implementation Plan: Room State Events

**Branch**: `010-room-state-events` | **Date**: 2026-03-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/010-room-state-events/spec.md`

## Summary

Add room avatar URL to room query responses (mirroring existing space pattern) and implement inbound room update event publishing when room name, avatar, or topic state changes in Matrix. All changes follow established codebase patterns with no new dependencies or architectural decisions.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)
**Storage**: N/A (no new persistence)
**Testing**: Go `testing` package, `testify`
**Target Platform**: Linux server (Docker container)
**Project Type**: Single service (hexagonal architecture)
**Performance Goals**: Normal event processing latency (same as existing events)
**Constraints**: Backward-compatible contract changes only
**Scale/Scope**: 3 new state event handlers, 1 new DTO, 2 enhanced DTOs, ~12 files modified

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | PASS | All mautrix-go calls stay in `internal/infrastructure/matrix/`. State event handling in listener.go, avatar fetch in mautrix.go. Services use domain types only. |
| 2. Event-Driven State Sync | PASS | Room updated events propagate through Watermill via EventService → QueuePort.Publish. No synchronous polling. |
| 3. Microservice Contract Stability | PASS | Adding optional `avatar_url` field is backward-compatible. New `communication.room.updated` topic is additive. DTO registered in OutgoingEventRegistry for TS generation. |
| 4. Matrix Client Lifecycle | PASS | No new client creation. Uses existing bot intent for state reads. |
| 5. Observability with Matrix Context | PASS | All new handlers include structured logging with room IDs and event context. Warning logs for unmapped rooms. |
| 6. Pragmatic Testing | PASS | Unit tests mock MatrixPort interface. Tests cover avatar fetch, event handling, self-event filtering edge cases. |
| 7. Go as Source of Truth | PASS | DTO changes in `pkg/dto/`, TypeScript regenerated via `make generate`. |
| 8. Secure Credentials | PASS | No credential handling involved. |
| 9. Container Determinism | PASS | No new dependencies. |
| 10. Simplicity | PASS | Only implementing what's specified: avatar in queries + 3 state event listeners. No speculative wrapping. |

**Gate Result**: ALL PASS — no violations.

## Project Structure

### Documentation (this feature)

```text
specs/010-room-state-events/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── room-state-events.md
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── core/
│   ├── domain/
│   │   └── model.go                  # Add AvatarURL to Room, add RoomUpdatedEvent
│   └── service/
│       └── event_service.go          # Add HandleRoomUpdated method
├── infrastructure/
│   ├── matrix/
│   │   ├── mautrix.go                # Add avatar fetch to GetRoomDetails
│   │   └── listener.go              # Add state event handlers + OnRoomUpdated callback
│   └── queue/
│       └── handler_room.go           # Populate AvatarURL in responses
├── app/
│   └── app.go                        # Wire OnRoomUpdated handler
pkg/
├── dto/
│   ├── room.go                       # Add AvatarURL to GetRoomResponse, GetRoomAsUserResponse
│   ├── event.go                      # Add RoomUpdatedEvent DTO
│   └── commands.go                   # Add TopicRoomUpdated constant + registry entry
lib/                                  # Regenerated via make generate
```

## Implementation Tasks

### Task 1: Add AvatarURL to Room domain model
**Files**: `internal/core/domain/model.go`
**Change**: Add `AvatarURL string` field to the `Room` struct (after `Topic`).
**Test**: Verify compilation. No behavior change yet.

### Task 2: Add avatar fetch to GetRoomDetails
**Files**: `internal/infrastructure/matrix/mautrix.go`
**Change**: In `GetRoomDetails()`, add avatar state event fetch using the same pattern as `GetSpaceDetails()`:
```go
var avatarContent event.RoomAvatarEventContent
if err := intent.StateEvent(ctx, roomID, event.StateRoomAvatar, "", &avatarContent); err == nil {
    avatarURL = string(avatarContent.URL)
}
```
Set `room.AvatarURL = avatarURL` in the returned struct.
**Test**: Unit test mocking MatrixPort — verify GetRoomDetails returns avatar when present and empty when absent.

### Task 3: Add AvatarURL to GetRoomResponse and GetRoomAsUserResponse DTOs
**Files**: `pkg/dto/room.go`
**Change**: Add `AvatarURL string \`json:"avatar_url,omitempty"\`` to both `GetRoomResponse` and `GetRoomAsUserResponse`.
**Test**: Verify JSON serialization includes/omits avatar_url correctly.

### Task 4: Populate AvatarURL in room query handlers
**Files**: `internal/infrastructure/queue/handler_room.go`
**Change**: In `HandleGetRoom()`, add `AvatarURL: room.AvatarURL` to the response struct. In `HandleGetRoomAsUser()`, add the same field.
**Test**: Unit test verifying handler returns avatar_url from domain model.

### Task 5: Add RoomUpdatedEvent domain type
**Files**: `internal/core/domain/model.go`
**Change**: Add `RoomUpdatedEvent` struct with `AlkemioRoomID uuid.UUID`, `DisplayName *string`, `AvatarURL *string`, `Topic *string`, `Timestamp time.Time`.
**Test**: Verify compilation.

### Task 6: Add TopicRoomUpdated constant and RoomUpdatedEvent DTO
**Files**: `pkg/dto/commands.go`, `pkg/dto/event.go`
**Change**:
- Add `TopicRoomUpdated = "communication.room.updated"` to outgoing event topics
- Add `RoomUpdatedEvent` DTO struct with pointer fields for optional properties
- Add entry to `OutgoingEventRegistry`
**Test**: Verify compilation and TS generation (`make generate`).

### Task 7: Add HandleRoomUpdated to EventService
**Files**: `internal/core/service/event_service.go`
**Change**: Add `HandleRoomUpdated(evt domain.RoomUpdatedEvent) error` method that converts domain event to DTO and publishes to `TopicRoomUpdated`.
**Test**: Unit test mocking QueuePort — verify correct topic and payload.

### Task 8: Add state event handlers to listener
**Files**: `internal/infrastructure/matrix/listener.go`
**Change**:
- Add `OnRoomUpdated func(evt domain.RoomUpdatedEvent) error` to `EventHandlers` struct
- Add cases for `event.StateRoomName`, `event.StateRoomAvatar`, `event.StateTopic` in `processEvent()` switch
- Implement `handleRoomStateEvent()` that: resolves Alkemio room ID, extracts changed property from event content, calls `OnRoomUpdated` with only the changed field populated
**Test**: Unit test verifying each state event type produces correct domain event with only the changed property set.

### Task 9: Wire OnRoomUpdated in app initialization
**Files**: `internal/app/app.go`
**Change**: Add `OnRoomUpdated: eventService.HandleRoomUpdated` to `SetEventHandlers()` call.
**Test**: Verify compilation. Integration verified by full flow.

### Task 10: Regenerate TypeScript library
**Command**: `make generate`
**Verify**: TypeScript lib includes `RoomUpdatedEvent` interface, `avatar_url` in `GetRoomResponse`/`GetRoomAsUserResponse`, and `COMMUNICATION_ROOM_UPDATED` topic constant.

### Task 11: Run full test suite and lint
**Command**: `make test && make lint && make build`
**Verify**: All tests pass, no lint errors, binary builds successfully.
