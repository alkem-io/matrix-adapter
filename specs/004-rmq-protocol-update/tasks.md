# Tasks: RMQ Protocol Update

**Status**: ✅ **Complete** (81/81 tasks)  
**Completed**: 2025-12-02

**Input**: Design documents from `/specs/004-rmq-protocol-update/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅

**Tests**: Not explicitly requested - tests will be added during implementation where needed for validation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Go service**: `internal/`, `pkg/`, `cmd/` at repository root
- **TypeScript lib**: `lib/src/dto/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Foundation types and error handling that all user stories depend on

- [X] T001 Create type aliases in pkg/dto/types.go: AlkemioRoomID and AlkemioActorID as UUID types; MessageID and ReactionID as string types (opaque, map to Matrix Event IDs). Note: Use `time.Time` for timestamps; Go's JSON marshaler uses RFC3339 format by default.
- [X] T002 [P] Update error codes (INVALID_PARAM, ROOM_NOT_FOUND, ACTOR_NOT_FOUND, MATRIX_ERROR, INTERNAL_ERROR, NOT_ALLOWED) in pkg/dto/error.go
- [X] T003 [P] Create BaseResponse struct with Success and Error fields in pkg/dto/base.go
- [X] T004 [P] Add Reaction domain entity in internal/core/domain/model.go
- [X] T005 Update Room domain entity to include AlkemioID field in internal/core/domain/model.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### MatrixPort Interface Extensions

- [X] T006 Add ResolveAlias(ctx, alias string) method to MatrixPort interface in internal/core/ports/matrix.go
- [X] T007 [P] Add DeleteAlias(ctx, alias string) method to MatrixPort interface in internal/core/ports/matrix.go
- [X] T008 [P] Add KickUser(ctx, roomID, userID, reason string) method to MatrixPort interface in internal/core/ports/matrix.go
- [X] T009 [P] Add GetRoomMessages(ctx, roomID) method to MatrixPort interface in internal/core/ports/matrix.go
- [X] T010 [P] Add GetReaction(ctx, roomID, reactionID) method to MatrixPort interface in internal/core/ports/matrix.go

### Matrix Adapter Implementations

- [X] T011 Implement ResolveAlias method in internal/infrastructure/matrix/mautrix.go
- [X] T012 [P] Implement DeleteAlias method in internal/infrastructure/matrix/mautrix.go
- [X] T013 [P] Implement KickUser method in internal/infrastructure/matrix/mautrix.go
- [X] T014 Implement GetRoomMessages method in internal/infrastructure/matrix/mautrix.go
- [X] T015 Implement GetReaction method in internal/infrastructure/matrix/mautrix.go
- [X] T016 Update CreateRoom to set room alias (#<UUID>:<homeserver>) in internal/infrastructure/matrix/mautrix.go

### Error Mapping

- [X] T017 Update error mapping from Matrix SDK errors to new error codes in internal/infrastructure/queue/errors.go

**Checkpoint**: Foundation ready - Matrix adapter supports all new operations, user story implementation can now begin

---

## Phase 3: User Story 1 - Room Lifecycle Management (Priority: P1) 🎯 MVP

**Goal**: Create, update, retrieve, and delete communication rooms using Alkemio UUIDs

**Independent Test**: Send room create/update/get/delete commands via RMQ and verify room state changes

### DTOs for User Story 1

- [X] T018 [P] [US1] Create CreateRoomRequest and CreateRoomResponse DTOs in pkg/dto/room.go
- [X] T019 [P] [US1] Create GetRoomRequest and GetRoomResponse DTOs in pkg/dto/room.go
- [X] T020 [P] [US1] Create UpdateRoomRequest and UpdateRoomResponse DTOs in pkg/dto/room.go
- [X] T021 [P] [US1] Create DeleteRoomRequest and DeleteRoomResponse DTOs in pkg/dto/room.go
- [X] T022 [P] [US1] Create ListRoomsRequest and ListRoomsResponse DTOs in pkg/dto/room.go

### Service Layer for User Story 1

- [X] T023 [US1] Refactor RoomService.CreateRoom for idempotent creation with alias lookup in internal/core/service/room_service.go. Uses T011 ResolveAlias via MatrixPort interface.
- [X] T024 [US1] Implement RoomService.GetRoom with messages and member mapping in internal/core/service/room_service.go. Note: Log warning if message count > 1000 for future pagination tracking; no limit enforced in v2.
- [X] T025 [US1] Implement RoomService.UpdateRoom for name/topic/visibility in internal/core/service/room_service.go
- [X] T026 [US1] Implement RoomService.DeleteRoom (kick all, leave, remove alias) in internal/core/service/room_service.go
- [X] T027 [US1] Implement RoomService.ListRooms with pagination in internal/core/service/room_service.go

### Handlers for User Story 1

- [X] T028 [US1] Rewrite HandleCreateRoom for communication.room.create in internal/infrastructure/queue/handler_room.go
- [X] T029 [US1] Rewrite HandleGetRoom for communication.room.get in internal/infrastructure/queue/handler_room.go
- [X] T030 [US1] Rewrite HandleUpdateRoom for communication.room.update in internal/infrastructure/queue/handler_room.go
- [X] T031 [US1] Rewrite HandleDeleteRoom for communication.room.delete in internal/infrastructure/queue/handler_room.go
- [X] T032 [US1] Create HandleListRooms for communication.room.list in internal/infrastructure/queue/handler_room.go

### Router for User Story 1

- [X] T033 [US1] Update router with room topics (communication.room.*) in internal/infrastructure/queue/router.go

**Checkpoint**: Room lifecycle fully functional - can create, get, update, delete, and list rooms with Alkemio UUIDs

---

## Phase 4: User Story 2 - Message Operations (Priority: P1)

**Goal**: Send, retrieve, and delete messages in rooms with threading support

**Independent Test**: With at least one room, send messages, retrieve details, and delete messages

### DTOs for User Story 2

- [X] T034 [P] [US2] Create ReactionDto struct in pkg/dto/reaction.go (needed by MessageDto.Reactions field)
- [X] T035 [P] [US2] Create MessageDto struct in pkg/dto/message.go (imports ReactionDto)
- [X] T036 [P] [US2] Create SendMessageRequest and SendMessageResponse DTOs in pkg/dto/message.go
- [X] T037 [P] [US2] Create GetMessageRequest and GetMessageResponse DTOs in pkg/dto/message.go
- [X] T038 [P] [US2] Create DeleteMessageRequest and DeleteMessageResponse DTOs in pkg/dto/message.go

### Service Layer for User Story 2

- [X] T039 [US2] Implement RoomService.SendMessage with threading support in internal/core/service/room_service.go
- [X] T040 [US2] Implement RoomService.GetMessage with reactions in internal/core/service/room_service.go
- [X] T041 [US2] Implement RoomService.DeleteMessage (redaction) in internal/core/service/room_service.go

### Handlers for User Story 2

- [X] T042 [US2] Create HandleSendMessage for communication.message.send in internal/infrastructure/queue/handler_room.go
- [X] T043 [US2] Create HandleGetMessage for communication.message.get in internal/infrastructure/queue/handler_room.go
- [X] T044 [US2] Create HandleDeleteMessage for communication.message.delete in internal/infrastructure/queue/handler_room.go

### Router for User Story 2

- [X] T045 [US2] Update router with message topics (communication.message.*) in internal/infrastructure/queue/router.go

**Checkpoint**: Message operations fully functional - can send, retrieve, and delete messages with threading

---

## Phase 5: User Story 3 - Reaction Operations (Priority: P2)

**Goal**: Add, remove, and retrieve emoji reactions on messages

**Independent Test**: With a message in a room, add a reaction, get it, and remove it

### DTOs for User Story 3

> Note: ReactionDto already created in T034 (US2 dependency)

- [X] T046 [P] [US3] Create AddReactionRequest and AddReactionResponse DTOs in pkg/dto/reaction.go
- [X] T047 [P] [US3] Create RemoveReactionRequest and RemoveReactionResponse DTOs in pkg/dto/reaction.go
- [X] T048 [P] [US3] Create GetReactionRequest and GetReactionResponse DTOs in pkg/dto/reaction.go

### Service Layer for User Story 3

- [X] T049 [US3] Implement RoomService.AddReaction in internal/core/service/room_service.go
- [X] T050 [US3] Implement RoomService.RemoveReaction in internal/core/service/room_service.go
- [X] T051 [US3] Implement RoomService.GetReaction in internal/core/service/room_service.go

### Handlers for User Story 3

- [X] T052 [US3] Create HandleAddReaction for communication.reaction.add in internal/infrastructure/queue/handler_room.go
- [X] T053 [US3] Create HandleRemoveReaction for communication.reaction.remove in internal/infrastructure/queue/handler_room.go
- [X] T054 [US3] Create HandleGetReaction for communication.reaction.get in internal/infrastructure/queue/handler_room.go

### Router for User Story 3

- [X] T055 [US3] Update router with reaction topics (communication.reaction.*) in internal/infrastructure/queue/router.go

**Checkpoint**: Reaction operations fully functional - can add, get, and remove reactions

---

## Phase 6: User Story 4 - Batch Membership Operations (Priority: P2)

**Goal**: Add or remove an actor from multiple rooms with per-room results

**Independent Test**: Create multiple rooms, use batch add/remove for an actor across all rooms

### DTOs for User Story 4

- [X] T056 [P] [US4] Create RoomOperationResult struct in pkg/dto/batch.go
- [X] T057 [P] [US4] Create BatchAddMemberRequest and BatchAddMemberResponse DTOs in pkg/dto/batch.go
- [X] T058 [P] [US4] Create BatchRemoveMemberRequest and BatchRemoveMemberResponse DTOs in pkg/dto/batch.go

### Service Layer for User Story 4

- [X] T059 [US4] Refactor ActorService.AddToRooms to return Results map in internal/core/service/actor_service.go. Handle edge case: empty room list returns success with empty Results map.
- [X] T060 [US4] Refactor ActorService.RemoveFromRooms to return Results map in internal/core/service/actor_service.go. Handle edge case: empty room list returns success with empty Results map.

### Handlers for User Story 4

- [X] T061 [US4] Rewrite HandleBatchAddMember for communication.room.member.batch.add in internal/infrastructure/queue/handler_room.go
- [X] T062 [US4] Rewrite HandleBatchRemoveMember for communication.room.member.batch.remove in internal/infrastructure/queue/handler_room.go

### Router for User Story 4

- [X] T063 [US4] Update router with batch membership topics in internal/infrastructure/queue/router.go

**Checkpoint**: Batch membership operations fully functional with per-room results

---

## Phase 7: User Story 5 - Actor Profile Synchronization (Priority: P2)

**Goal**: Sync actor profiles (display name, avatar) to Matrix using Intent API

**Independent Test**: Send SyncActorRequest and verify Matrix user profile is created/updated

### DTOs for User Story 5

- [X] T064 [P] [US5] Create SyncActorRequest and SyncActorResponse DTOs in pkg/dto/actor.go

### Service Layer for User Story 5

- [X] T065 [US5] Refactor ActorService for sync operation (replace register pattern) in internal/core/service/actor_service.go

### Handlers for User Story 5

- [X] T066 [US5] Rewrite HandleSyncActor for communication.actor.sync in internal/infrastructure/queue/handler_actor.go

### Router for User Story 5

- [X] T067 [US5] Update router with actor sync topic in internal/infrastructure/queue/router.go

**Checkpoint**: Actor sync fully functional - can create and update Matrix user profiles

---

---

## Phase 8: Cleanup & Removal

**Purpose**: Remove deprecated handlers and routes

- [X] T068 Remove deprecated actor.rooms handler from internal/infrastructure/queue/handler_actor.go
- [X] T069 [P] Remove deprecated actor.rooms.direct handler from internal/infrastructure/queue/handler_actor.go
- [X] T070 [P] Remove deprecated actor.startDirectMessaging handler from internal/infrastructure/queue/handler_actor.go
- [X] T071 [P] Remove deprecated actor.stopDirectMessaging handler from internal/infrastructure/queue/handler_actor.go
- [X] T072 [P] Remove deprecated admin.replicateRoomMembership handler from internal/infrastructure/queue/handler_admin.go
- [X] T073 Remove deprecated routes from internal/infrastructure/queue/router.go
- [X] T074 Remove deprecated DTOs (old payloads) from pkg/dto/*.go

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: TypeScript generation, documentation, and final validation

- [X] T075 Run `make generate` to regenerate TypeScript definitions in lib/src/dto/generated.ts
- [X] T076 [P] Update README.md with new protocol documentation
- [X] T077 [P] Update MatrixAdapterProtocol.md to mark as implemented
- [X] T078 Run `make lint` and fix any issues
- [X] T079 Run `make test` and ensure all tests pass
- [X] T080 Run `make build` and verify binary builds correctly
- [X] T081 Run quickstart.md validation scenarios

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 - BLOCKS all user stories
- **User Story 1-5 (Phases 3-7)**: All depend on Foundational phase completion
  - US1 and US2 are both P1 priority - start US1 first as foundation for rooms
  - US2 can start once room DTOs exist (T018-T022)
  - US3-US5 can proceed after their dependencies are met
- **Cleanup (Phase 8)**: Can start after all user story handlers are updated
- **Polish (Phase 9)**: Depends on all implementation phases being complete

### User Story Dependencies

| Story | Depends On | Notes |
|-------|------------|-------|
| US1 (Rooms) | Foundational | Core foundation - start first; includes admin room.list |
| US2 (Messages) | US1 room DTOs | Needs room structures for message operations |
| US3 (Reactions) | T034 ReactionDto | ReactionDto created in US2 phase |
| US4 (Batch) | US1 room DTOs | Needs room structures for batch operations |
| US5 (Actor) | Foundational | Independent of room/message stories |

### Parallel Opportunities

- **Phase 1**: T002, T003, T004 can run in parallel (different files)
- **Phase 2**: T007, T008, T009, T010 can run in parallel (interface additions)
- **Phase 2**: T012, T013 can run in parallel (implementation of parallel interface changes)
- **Phase 3+**: All DTO tasks marked [P] within a story can run in parallel
- **Different stories**: US1 and US5 can be worked on in parallel by different developers

---

## Parallel Example: Phase 1 Setup

```bash
# Launch all parallel setup tasks together:
T001: Create UUID type aliases in pkg/dto/types.go
# Then in parallel:
T002: Update error codes in pkg/dto/error.go
T003: Create BaseResponse struct in pkg/dto/base.go
T004: Add Reaction domain entity in internal/core/domain/model.go
```

## Parallel Example: User Story 1 DTOs

```bash
# Launch all room DTOs in parallel:
T018: CreateRoomRequest/Response in pkg/dto/room.go
T019: GetRoomRequest/Response in pkg/dto/room.go
T020: UpdateRoomRequest/Response in pkg/dto/room.go
T021: DeleteRoomRequest/Response in pkg/dto/room.go
T022: ListRoomsRequest/Response in pkg/dto/room.go
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 Only)

1. Complete Phase 1: Setup (T001-T005)
2. Complete Phase 2: Foundational (T006-T017)
3. Complete Phase 3: User Story 1 - Rooms (T018-T033)
4. Complete Phase 4: User Story 2 - Messages (T034-T044)
5. **STOP and VALIDATE**: Test room and message operations independently
6. Deploy if ready - this provides core communication capability

### Full Protocol Implementation

1. Complete MVP (above)
2. Add Phase 5: User Story 3 - Reactions (T046-T055)
3. Add Phase 6: User Story 4 - Batch Ops (T056-T063)
4. Add Phase 7: User Story 5 - Actor Sync (T064-T067)
5. Complete Phase 8: Cleanup (T068-T074)
6. Complete Phase 9: Polish (T075-T081)

### Estimated Effort by Phase

| Phase | Tasks | Estimate |
|-------|-------|----------|
| Phase 1: Setup | T001-T005 | 2h |
| Phase 2: Foundational | T006-T017 | 5h |
| Phase 3: US1 Rooms | T018-T033 | 5h |
| Phase 4: US2 Messages | T034-T045 | 4h |
| Phase 5: US3 Reactions | T046-T055 | 2.5h |
| Phase 6: US4 Batch | T056-T063 | 2.5h |
| Phase 7: US5 Actor | T064-T067 | 1h |
| Phase 8: Cleanup | T068-T074 | 1h |
| Phase 9: Polish | T075-T081 | 1.5h |
| **Total** | **81 tasks** | **~24.5h** |

---

## Notes

- [P] tasks = different files, no dependencies within that phase
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Run `make lint` and `make test` frequently during implementation
