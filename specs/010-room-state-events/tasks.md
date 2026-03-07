# Tasks: Room State Events

**Input**: Design documents from `/specs/010-room-state-events/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

---

## Phase 1: Foundational (Shared Domain Changes)

**Purpose**: Domain model enhancement needed by both user stories

- [x] T001 Add `AvatarURL string` field to `Room` struct in `internal/core/domain/model.go`

**Checkpoint**: Domain model updated, project compiles

---

## Phase 2: User Story 1 - Room Avatar in Queries (Priority: P1)

**Goal**: Room detail query responses include the room's avatar URL, matching existing space query behavior

**Independent Test**: Send a `communication.room.get` command for a room with an avatar set and verify the response includes the `avatar_url` field with the correct `mxc://` content URI

### Implementation for User Story 1

- [x] T002 [P] [US1] Add `AvatarURL string` field with `json:"avatar_url,omitempty"` tag to both `GetRoomResponse` and `GetRoomAsUserResponse` in `pkg/dto/room.go`
- [x] T003 [P] [US1] Add avatar state event fetch to `GetRoomDetails()` in `internal/infrastructure/matrix/mautrix.go` (mirror `GetSpaceDetails` pattern: `intent.StateEvent` with `event.StateRoomAvatar` → `event.RoomAvatarEventContent` → `string(avatarContent.URL)`)
- [x] T004 [US1] Populate `AvatarURL: room.AvatarURL` in `HandleGetRoom()` response in `internal/infrastructure/queue/handler_room.go`
- [x] T005 [US1] Populate `AvatarURL: room.AvatarURL` in `HandleGetRoomAsUser()` response in `internal/infrastructure/queue/handler_room.go`

**Checkpoint**: Room queries return avatar URL. Verify with `make build && make test`

---

## Phase 3: User Story 2 - Inbound Room Update Events (Priority: P2)

**Goal**: Adapter detects room name/avatar/topic state changes in Matrix and publishes `communication.room.updated` events to RabbitMQ

**Independent Test**: Change a room's name, avatar, or topic in Matrix and verify a `RoomUpdatedEvent` is published to the message queue with the correct Alkemio room ID and only the changed property populated

### Implementation for User Story 2

- [x] T006 [P] [US2] Add `RoomUpdatedEvent` domain struct to `internal/core/domain/model.go` with fields: `AlkemioRoomID uuid.UUID`, `DisplayName *string`, `AvatarURL *string`, `Topic *string`, `Timestamp time.Time`
- [x] T007 [P] [US2] Add `TopicRoomUpdated = "communication.room.updated"` constant to outgoing event topics in `pkg/dto/commands.go`
- [x] T008 [P] [US2] Add `RoomUpdatedEvent` DTO struct to `pkg/dto/event.go` with fields: `AlkemioRoomID AlkemioRoomID`, `DisplayName *string`, `AvatarURL *string`, `Topic *string`, `Timestamp int64` (pointer fields use `json:",omitempty"`)
- [x] T009 [US2] Add `{Topic: TopicRoomUpdated, RequestType: "", ResponseType: "RoomUpdatedEvent"}` entry to `OutgoingEventRegistry` in `pkg/dto/commands.go`
- [x] T010 [US2] Add `HandleRoomUpdated(evt domain.RoomUpdatedEvent) error` method to `EventService` in `internal/core/service/event_service.go` — converts domain event to DTO and publishes to `TopicRoomUpdated` (follow `HandleRoomCreated` pattern)
- [x] T011 [US2] Add `OnRoomUpdated func(evt domain.RoomUpdatedEvent) error` field to `EventHandlers` struct in `internal/infrastructure/matrix/listener.go`
- [x] T012 [US2] Add cases for `event.StateRoomName`, `event.StateRoomAvatar`, `event.StateTopic` to `processEvent()` switch in `internal/infrastructure/matrix/listener.go` — each calls a shared `handleRoomStateEvent()` handler
- [x] T013 [US2] Implement `handleRoomStateEvent(evt *event.Event)` in `internal/infrastructure/matrix/listener.go` — resolves Alkemio room ID via `resolveAlkemioRoomID()` in goroutine, extracts changed property from event content, calls `OnRoomUpdated` with only the changed field populated. Self-event filtering is inherited from `processEvent()` bot sender guard. For avatar removal (empty URL), set `*string` pointer to empty string (not nil) so the server can clear it; nil means "property unchanged"
- [x] T014 [US2] Wire `OnRoomUpdated: eventService.HandleRoomUpdated` in `SetEventHandlers()` call in `internal/app/app.go`

**Checkpoint**: State event changes in Matrix produce `communication.room.updated` events. Verify with `make build && make test`

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: TypeScript generation, full validation

- [x] T015 Regenerate TypeScript library via `make generate` — verify `RoomUpdatedEvent` interface, `avatar_url` in `GetRoomResponse`/`GetRoomAsUserResponse`, and `TopicRoomUpdated` constant appear in `lib/src/dto/generated.ts`
- [x] T016 Run full validation: `make test && make lint && make build`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Foundational (Phase 1)**: No dependencies — start immediately
- **User Story 1 (Phase 2)**: Depends on T001 (domain model)
- **User Story 2 (Phase 3)**: Depends on T001 (domain model). Independent of US1
- **Polish (Phase 4)**: Depends on both US1 and US2 completion

### User Story Dependencies

- **User Story 1 (P1)**: Can start after T001. No dependency on US2
- **User Story 2 (P2)**: Can start after T001. No dependency on US1. Can run in parallel with US1

### Within Each User Story

**US1**:
- T002, T003 can run in parallel (different files)
- T004, T005 depend on T002 (DTO fields) and T003 (avatar fetch)

**US2**:
- T006, T007, T008 can run in parallel (different files)
- T009 depends on T007 (topic constant)
- T010 depends on T006 (domain type) and T007/T008 (topic + DTO)
- T011 depends on T006 (domain type)
- T012, T013 depend on T011 (EventHandlers struct)
- T014 depends on T010 (EventService method) and T011 (EventHandlers field)

### Parallel Opportunities

- T002 + T003 (US1: DTO fields + avatar fetch — different files)
- T006 + T007 + T008 (US2: domain type + topic constant + DTO — different files)
- US1 and US2 phases can run in parallel after T001

---

## Parallel Example: User Story 1

```
# Launch independent US1 tasks together:
Task T002: "Add AvatarURL to GetRoomResponse and GetRoomAsUserResponse in pkg/dto/room.go"
Task T003: "Add avatar fetch to GetRoomDetails in internal/infrastructure/matrix/mautrix.go"

# Then sequentially:
Task T004: "Populate AvatarURL in HandleGetRoom"
Task T005: "Populate AvatarURL in HandleGetRoomAsUser"
```

## Parallel Example: User Story 2

```
# Launch independent US2 tasks together:
Task T006: "Add RoomUpdatedEvent domain struct in internal/core/domain/model.go"
Task T007: "Add TopicRoomUpdated constant in pkg/dto/commands.go"
Task T008: "Add RoomUpdatedEvent DTO in pkg/dto/event.go"

# Then sequentially:
Task T009: "Add OutgoingEventRegistry entry"
Task T010: "Add HandleRoomUpdated to EventService"
Task T011: "Add OnRoomUpdated to EventHandlers struct"
Task T012: "Add state event cases to processEvent() switch"
Task T013: "Implement handleRoomStateEvent handler"
Task T014: "Wire OnRoomUpdated in app.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Foundational (T001)
2. Complete Phase 2: User Story 1 (T002-T005)
3. **STOP and VALIDATE**: `make build && make test` — room queries return avatar
4. Can deploy immediately for server to consume

### Incremental Delivery

1. T001 → Foundation ready
2. T002-T005 → US1 complete → Room queries include avatar (MVP)
3. T006-T014 → US2 complete → State change events flowing
4. T015-T016 → TypeScript lib updated, full validation

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- No new dependencies needed — all patterns exist in codebase
- Self-event filtering is already handled by `processEvent()` top-level check (FR-009)
- `GetRoomAsUser` avatar support flows automatically through `GetRoomWithMessages` → `GetRoomDetails`
