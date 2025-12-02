# Tasks: Protocol V3 Implementation

**Status**: ✅ Complete (77/77 tasks)  
**Completed**: 2025-12-03

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)

---

## Phase 1: Setup

**Purpose**: Add new types and foundational DTOs required by all user stories

- [X] T001 Add AlkemioContextID type with JSON marshal/unmarshal methods in pkg/dto/types.go
- [X] T002 [P] Add JoinRule enum with constants (public, invite, restricted) in pkg/dto/types.go
- [X] T003 [P] Create pkg/dto/space.go with SpaceChildDto struct
- [X] T004 [P] Create pkg/dto/hierarchy.go with SetParentRequest and SetParentResponse structs
- [X] T005 Verify compilation with `go build ./...`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Domain Model

- [X] T006 Add Space and SpaceChild domain structs in internal/core/domain/model.go

### Matrix Port Interface

- [X] T007 Add Space-related interface methods to internal/core/ports/matrix.go:
  - CreateSpace, UpdateSpace, DeleteSpace, GetSpaceDetails, GetSpaceChildren
  - SetSpaceChild, RemoveSpaceChild, SetSpaceParent, RemoveSpaceParent
  - SetRoomJoinRule, SetRoomAvatar

### Space Service Scaffold

- [X] T008 Create internal/core/service/space_service.go with:
  - SpaceService struct with matrix port and logger
  - NewSpaceService constructor
  - buildSpaceAlias helper method

### Space Handler Scaffold

- [X] T009 Create internal/infrastructure/queue/handler_space.go with:
  - SpaceHandler struct with space service and matrix port
  - NewSpaceHandler constructor

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Space Lifecycle Management (Priority: P1) 🎯 MVP

**Goal**: Create, update, delete, and get Matrix Spaces with idempotent alias-based mapping

**Independent Test**: Create a Space, verify with get, update metadata, then delete

### Matrix Adapter Implementation for US1

- [X] T010 [US1] Implement CreateSpace in internal/infrastructure/matrix/mautrix.go (creates room with type=m.space and alias)
- [X] T011 [P] [US1] Implement GetSpaceDetails in internal/infrastructure/matrix/mautrix.go (fetches name, topic, avatar)
- [X] T012 [P] [US1] Implement GetSpaceChildren in internal/infrastructure/matrix/mautrix.go (queries m.space.child state events)
- [X] T013 [P] [US1] Implement UpdateSpace in internal/infrastructure/matrix/mautrix.go (updates state events)
- [X] T014 [P] [US1] Implement DeleteSpace in internal/infrastructure/matrix/mautrix.go (kicks members, removes alias)
- [X] T015 [P] [US1] Implement SetRoomAvatar in internal/infrastructure/matrix/mautrix.go (sets m.room.avatar state)

### Space DTOs for US1

- [X] T016 [US1] Add CreateSpaceRequest and CreateSpaceResponse to pkg/dto/space.go
- [X] T017 [P] [US1] Add UpdateSpaceRequest and UpdateSpaceResponse to pkg/dto/space.go
- [X] T018 [P] [US1] Add DeleteSpaceRequest and DeleteSpaceResponse to pkg/dto/space.go
- [X] T019 [P] [US1] Add GetSpaceRequest and GetSpaceResponse to pkg/dto/space.go

### Space Service Methods for US1

- [X] T020 [US1] Implement CreateSpaceWithAlkemioID in internal/core/service/space_service.go (idempotent via alias lookup)
- [X] T021 [P] [US1] Implement GetSpaceWithChildren in internal/core/service/space_service.go
- [X] T022 [P] [US1] Implement UpdateSpaceMetadata in internal/core/service/space_service.go
- [X] T023 [P] [US1] Implement DeleteSpaceFully in internal/core/service/space_service.go

### Queue Handlers for US1

- [X] T024 [US1] Implement HandleCreateSpace in internal/infrastructure/queue/handler_space.go
- [X] T025 [P] [US1] Implement HandleGetSpace in internal/infrastructure/queue/handler_space.go
- [X] T026 [P] [US1] Implement HandleUpdateSpace in internal/infrastructure/queue/handler_space.go
- [X] T027 [P] [US1] Implement HandleDeleteSpace in internal/infrastructure/queue/handler_space.go

### Route Registration for US1

- [X] T028 [US1] Register Space CRUD routes in internal/infrastructure/queue/router.go:
  - communication.space.create
  - communication.space.get
  - communication.space.update
  - communication.space.delete

**Checkpoint**: Space CRUD operations functional and testable

---

## Phase 4: User Story 2 - Room-Space Hierarchy Management (Priority: P1)

**Goal**: Establish and modify parent-child relationships between Rooms/Spaces

**Independent Test**: Create Space, create Room, set Room as child of Space, verify hierarchy, change parent, orphan

### Matrix Adapter Implementation for US2

- [X] T029 [US2] Implement SetSpaceChild in internal/infrastructure/matrix/mautrix.go (sends m.space.child state event)
- [X] T030 [P] [US2] Implement RemoveSpaceChild in internal/infrastructure/matrix/mautrix.go (removes m.space.child event)
- [X] T031 [P] [US2] Implement SetSpaceParent in internal/infrastructure/matrix/mautrix.go (sends m.space.parent state event)
- [X] T032 [P] [US2] Implement RemoveSpaceParent in internal/infrastructure/matrix/mautrix.go (removes m.space.parent event)

### Service Methods for US2

- [X] T033 [US2] Implement SetParent in internal/core/service/space_service.go:
  - Validate exactly one child ID provided
  - Validate parent is a Space (type=m.space) before setting hierarchy; return INVALID_PARAM if not
  - Resolve child and parent aliases
  - Set m.space.child in parent, m.space.parent in child
  - Handle parent removal (orphaning)

### Queue Handler for US2

- [X] T034 [US2] Implement HandleSetParent in internal/infrastructure/queue/handler_space.go with validation:
  - Check exactly one of child_room_id or child_context_id is set
  - Return INVALID_PARAM if validation fails

### Route Registration for US2

- [X] T035 [US2] Register hierarchy route in internal/infrastructure/queue/router.go:
  - communication.hierarchy.set_parent

**Checkpoint**: Hierarchy management fully functional

---

## Phase 5: User Story 3 - Extended Room Creation (Priority: P2)

**Goal**: Extend room creation to support avatar, parent context, and join rules

**Independent Test**: Create room with avatar_url, parent_context_id, and join_rule=restricted; verify settings

### Matrix Adapter Extension for US3

- [X] T036 [US3] Implement SetRoomJoinRule in internal/infrastructure/matrix/mautrix.go:
  - Handle public, invite join rules
  - Handle restricted with parent space allow list

### DTO Updates for US3

- [X] T037 [US3] Extend CreateRoomRequest in pkg/dto/room.go with:
  - AvatarURL string
  - ParentContextID *AlkemioContextID
  - JoinRule JoinRule

### Matrix Adapter Extension for US3

- [X] T038 [US3] Extend CreateRoomWithAlias in internal/infrastructure/matrix/mautrix.go to:
  - Set avatar after room creation
  - Set join rule after room creation
  - Add room as child of parent space if provided

### Service Extension for US3

- [X] T039 [US3] Extend CreateRoomWithAlkemioID in internal/core/service/room_service.go to:
  - Accept avatar, parent context, join rule params
  - Call matrix adapter with new fields
  - Validate restricted requires parent

### Handler Extension for US3

- [X] T040 [US3] Extend HandleCreateRoom in internal/infrastructure/queue/handler_room.go to:
  - Parse new fields from request
  - Pass to service
  - Validate restricted join rule has parent

**Checkpoint**: Room creation with V3 fields working

---

## Phase 6: User Story 4 - Space Membership Batch Operations (Priority: P2)

**Goal**: Add/remove actor from multiple Spaces in a single operation

**Independent Test**: Create 3 Spaces, batch add actor, verify membership, batch remove

### DTOs for US4

- [X] T041 [US4] Add BatchAddSpaceMemberRequest and BatchAddSpaceMemberResponse to pkg/dto/space.go
- [X] T042 [P] [US4] Add BatchRemoveSpaceMemberRequest and BatchRemoveSpaceMemberResponse to pkg/dto/space.go

### Service Methods for US4

- [X] T043 [US4] Implement BatchAddSpaceMembers in internal/core/service/space_service.go:
  - Iterate AlkemioContextIDs
  - Resolve each to Matrix room ID
  - Invite/join actor
  - Collect per-space results
- [X] T044 [P] [US4] Implement BatchRemoveSpaceMembers in internal/core/service/space_service.go:
  - Iterate AlkemioContextIDs
  - Resolve each to Matrix room ID
  - Kick actor
  - Collect per-space results

### Queue Handlers for US4

- [X] T045 [US4] Implement HandleBatchAddSpaceMember in internal/infrastructure/queue/handler_space.go
- [X] T046 [P] [US4] Implement HandleBatchRemoveSpaceMember in internal/infrastructure/queue/handler_space.go

### Route Registration for US4

- [X] T047 [US4] Register Space batch routes in internal/infrastructure/queue/router.go:
  - communication.space.member.batch.add
  - communication.space.member.batch.remove

**Checkpoint**: Space membership batch operations working with partial success

---

## Phase 7: User Story 5 - Space Listing (Priority: P3)

**Goal**: List all Spaces with cursor-based pagination

**Independent Test**: Create 5 Spaces, list all, verify all returned

### DTOs for US5

- [X] T048 [US5] Add ListSpacesRequest and ListSpacesResponse to pkg/dto/space.go

### Service Method for US5

- [X] T049 [US5] Implement ListSpaces in internal/core/service/space_service.go:
  - Get all joined rooms from Matrix
  - Filter to Spaces (check creation content for m.space type)
  - Extract AlkemioContextIDs from aliases
  - Return with cursor support

### Queue Handler for US5

- [X] T050 [US5] Implement HandleListSpaces in internal/infrastructure/queue/handler_space.go

### Route Registration for US5

- [X] T051 [US5] Register Space list route in internal/infrastructure/queue/router.go:
  - communication.space.list

**Checkpoint**: Space listing operational

---

## Phase 8: User Story 6 - Updated Room Operations (Priority: P2)

**Goal**: Extend room update to support avatar and join rule changes

**Independent Test**: Create room, update avatar_url and join_rule, verify changes

### DTO Updates for US6

- [X] T052 [US6] Extend UpdateRoomRequest in pkg/dto/room.go with:
  - AvatarURL *string
  - JoinRule *JoinRule

### Breaking Change for US6

- [X] T053 [US6] Remove Limit field from ListRoomsRequest in pkg/dto/room.go

### Service Extension for US6

- [X] T054 [US6] Extend UpdateRoomMetadata in internal/core/service/room_service.go to:
  - Accept avatar_url and join_rule params
  - Call SetRoomAvatar if provided
  - Call SetRoomJoinRule if provided

### Handler Extension for US6

- [X] T055 [US6] Extend HandleUpdateRoom in internal/infrastructure/queue/handler_room.go to:
  - Parse avatar_url and join_rule from request
  - Pass to service

### Handler Update for US6

- [X] T056 [US6] Update HandleListRooms in internal/infrastructure/queue/handler_room.go:
  - Remove limit parameter handling (breaking change)

**Checkpoint**: Room update with V3 fields working, ListRooms updated

---

## Phase 9: TypeScript Generation & Polish

**Purpose**: Generate TypeScript library and finalize

### TypeScript Event Types

- [X] T057 Add 8 new event types to lib/src/matrix.adapter.event.type.ts:
  - COMMUNICATION_SPACE_CREATE
  - COMMUNICATION_SPACE_UPDATE
  - COMMUNICATION_SPACE_DELETE
  - COMMUNICATION_SPACE_GET
  - COMMUNICATION_SPACE_LIST
  - COMMUNICATION_SPACE_MEMBER_BATCH_ADD
  - COMMUNICATION_SPACE_MEMBER_BATCH_REMOVE
  - COMMUNICATION_HIERARCHY_SET_PARENT

### Generation & Validation

- [X] T058 Run `make generate` to produce TypeScript DTOs in lib/src/dto/generated.ts
- [X] T059 Verify generated TypeScript includes all new types (AlkemioContextID, JoinRule, Space DTOs)
- [X] T060 Run `make lint` to verify code quality
- [X] T061 Run `make test` to verify all tests pass
- [X] T062 Run `make build` to verify successful compilation

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 1: Setup ─────────────► Phase 2: Foundational ───┬──► Phase 3: US1 (Space CRUD)
                                                       │
                                                       ├──► Phase 4: US2 (Hierarchy)
                                                       │
                                                       ├──► Phase 5: US3 (Extended Room Create)
                                                       │
                                                       ├──► Phase 6: US4 (Space Membership Batch)
                                                       │
                                                       ├──► Phase 7: US5 (Space List)
                                                       │
                                                       └──► Phase 8: US6 (Extended Room Update)
                                                                           │
                                                                           ▼
                                                              Phase 9: TypeScript & Polish
```

### User Story Dependencies

- **US1 (Space CRUD)**: Independent, can start after Phase 2
- **US2 (Hierarchy)**: Logically depends on US1 (needs Spaces to exist) but code is independent
- **US3 (Extended Room Create)**: Independent, uses existing room infrastructure
- **US4 (Space Membership Batch)**: Depends on Space resolution from US1 infrastructure
- **US5 (Space List)**: Depends on Space creation from US1
- **US6 (Extended Room Update)**: Independent, extends existing room functionality

### Recommended Order (Single Developer)

1. Phase 1 → Phase 2 → Phase 3 (US1) → Phase 4 (US2) - Core P1 stories
2. Phase 5 (US3) + Phase 8 (US6) - Room extensions (P2)
3. Phase 6 (US4) + Phase 7 (US5) - Space auxiliary (P2/P3)
4. Phase 9 - Finalize

### Parallel Opportunities Per Phase

```bash
# Phase 1: All [P] tasks can run in parallel
T002 + T003 + T004

# Phase 3 (US1): Matrix adapters in parallel, then DTOs in parallel, then services
T011 + T012 + T013 + T014 + T015  # Matrix adapters
T016 + T017 + T018 + T019         # DTOs
T021 + T022 + T023                # Services (after T020)
T025 + T026 + T027                # Handlers (after T024)

# Phase 4 (US2): Matrix adapters in parallel
T029 + T030 + T031 + T032

# Phase 6 (US4): DTOs in parallel, services in parallel, handlers in parallel
T041 + T042
T043 + T044
T045 + T046
```

---

## Implementation Strategy

### MVP First (P1 Stories Only)

1. Complete Phase 1 + Phase 2 (Setup + Foundation)
2. Complete Phase 3: User Story 1 (Space CRUD)
3. Complete Phase 4: User Story 2 (Hierarchy)
4. **STOP and VALIDATE**: Test Space creation, hierarchy management
5. Run Phase 9 for TypeScript generation
6. Deploy/demo if ready

### Full Implementation

1. MVP above
2. Add Phase 5 (US3): Extended Room Creation
3. Add Phase 8 (US6): Extended Room Update
4. Add Phase 6 (US4): Space Membership Batch
5. Add Phase 7 (US5): Space Listing
6. Complete Phase 9: Full TypeScript regeneration and polish

---

## Notes

- All Matrix SDK calls MUST be in internal/infrastructure/matrix/ (Constitution Principle 1)
- All new DTOs go in pkg/dto/ (Constitution Principle 7)
- Run `make generate` after all DTO changes to update TypeScript
- Spaces use same alias pattern as Rooms: #<uuid>:<homeserver>
- Remember to log contextID, roomID, userID in all handlers (Constitution Principle 5)

---

## Phase 10: Post-Implementation Improvements

**Purpose**: Code quality and architecture refinements identified during implementation

### Topic Constants SSOT

- [X] T063 Create internal/infrastructure/queue/topics.go with centralized topic constants:
  - 22 topic constants organized by category (Room, Message, Reaction, Actor, Space, Hierarchy)
  - Single source of truth for RabbitMQ routing keys
- [X] T064 Update internal/infrastructure/queue/router.go to use topic constants instead of string literals

### Domain Model Helpers

- [X] T065 Add NewActor(id uuid.UUID) constructor in internal/core/domain/model.go
- [X] T066 Update 8 call sites in handler files to use domain.NewActor() helper

### Context Propagation Fix

- [X] T067 Update ports.MessageHandler type in internal/core/ports/queue.go:
  - Change signature to `func(ctx context.Context, payload []byte) (interface{}, error)`
- [X] T068 Update internal/infrastructure/queue/watermill.go to pass msg.Context() to handlers
- [X] T069 Update all 22 handlers to accept context.Context and propagate to service calls:
  - 14 handlers in handler_room.go
  - 7 handlers in handler_space.go
  - 1 handler in handler_actor.go

### Dead Code Removal

- [X] T070 Remove unused `matrix` field from SpaceHandler struct in handler_space.go
- [X] T071 Update app.go to pass only spaceService to NewSpaceHandler constructor

### TypeScript Generation Fix

- [X] T072 Update cmd/gen-events/main.go to parse topics.go for const declarations:
  - Changed from parsing router.go for string literals to parsing topics.go
  - Uses AST to find const string values
- [X] T073 Regenerate TypeScript library with `make generate` (23 events generated)

### Topic Naming Consistency

- [X] T074 Rename `message.received` to `communication.message.received` for consistency:
  - Add TopicMessageReceived constant to topics.go under new "Event Topics (Outbound)" section
  - Update event_service.go to use new topic name
  - Update README.md documentation
  - Regenerate TypeScript library

### Documentation Updates

- [X] T075 Update README.md with Protocol V3 commands:
  - Add Space commands (create, get, update, delete, list, batch add/remove member)
  - Add Hierarchy command (set_parent)
  - Add SPACE_NOT_FOUND error code
  - Update protocol reference to V3
- [X] T076 Update protocol documentation:
  - Add deprecation notice to MatrixAdapterProtocol_V2.md
  - Add status header and implementation reference to MatrixAdapterProtocol_V3.md
- [X] T077 Create comprehensive lib/README.md:
  - Installation instructions (npm/pnpm/yarn)
  - Usage examples for event types and DTOs
  - API reference with all 23 event types
  - Type aliases and constants documentation
  - Response handling patterns

**Checkpoint**: All code quality improvements verified with `make build`, `make test`, `make lint`

---

## Summary

| Category | Count |
|----------|-------|
| **Total Tasks** | 77 |
| **Setup Tasks** | 5 |
| **Foundational Tasks** | 4 |
| **US1 (Space Lifecycle) Tasks** | 19 |
| **US2 (Hierarchy) Tasks** | 7 |
| **US3 (Extended Room Create) Tasks** | 5 |
| **US4 (Space Membership Batch) Tasks** | 7 |
| **US5 (Space Listing) Tasks** | 4 |
| **US6 (Extended Room Update) Tasks** | 5 |
| **Polish Tasks** | 6 |
| **Post-Implementation Tasks** | 15 |
| **Parallel Opportunities** | 32 tasks marked [P] |
