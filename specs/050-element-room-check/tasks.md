# Tasks: Element-Initiated Conversation Creation (Synchronous Check)

**Input**: Design documents from `/specs/050-element-room-check/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: New DTOs, topic constants, custom event types, and QueuePort extension needed by all stories

- [X] T001 [P] Add `io.alkemio.pending` custom event type and content struct in `internal/infrastructure/matrix/custom_events.go` (register `StateAlkemioPending` with `AlkemioPendingContent{AlkemioRoomID string}` in mautrix TypeMap, same pattern as `StateAlkemioVisibility`)
- [X] T002 [P] Add `TopicRoomCheck` and `TopicRoomInfo` topic constants in `pkg/dto/commands.go` and corresponding re-export aliases in `internal/infrastructure/queue/topics.go`
- [X] T003 [P] Add DTOs for check flow in new file `pkg/dto/room_check.go`: `CheckRoomHTTPRequest`, `CheckRoomHTTPResponse`, `CheckRoomRequest`, `CheckRoomResponse`, `GetRoomInfoRequest`, `GetRoomInfoResponse`, `RoomInfoMember` (per data-model.md)
- [X] T004 [P] Add domain types for check and reconciliation in new file `internal/core/domain/room_check.go`: `RoomCheckRequest`, `RoomCheckResponse`, `RoomInfo`, `RoomInfoMember` structs
- [X] T005 Add `PublishAndWait(ctx context.Context, topic string, payload interface{}, timeout time.Duration) ([]byte, error)` method to `QueuePort` interface in `internal/core/ports/queue.go`
- [X] T006 Implement `PublishAndWait` in `internal/infrastructure/queue/watermill.go`: publish message with `reply_to` set to a temporary exclusive AMQP queue and `correlation_id`, consume reply with timeout from context, return response bytes (per research.md R1)

**Checkpoint**: Infrastructure ready — new types registered, RPC pattern available

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core services and endpoints that all user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T007 Implement `RoomCheckService` in new file `internal/core/service/room_check_service.go`: method `CheckRoom(ctx context.Context, req domain.RoomCheckRequest) (*domain.RoomCheckResponse, error)` that converts Matrix user IDs to actor UUIDs via IDMapper, calls `QueuePort.PublishAndWait` on `TopicRoomCheck` with 3s timeout, parses server response, returns allow/deny with UUID or reason
- [X] T008 Implement `ReconcileRoom` method on `MautrixAdapter` in `internal/infrastructure/matrix/mautrix.go`: signature `ReconcileRoom(ctx context.Context, roomID id.RoomID, alkemioRoomID uuid.UUID, creatorUserID id.UserID) error`. Uses `m.queuePort` field (set during init in T013) for RabbitMQ calls. Performs: (1) bot admin-join via `m.admin.JoinRoom`, (2) get room info from server via `m.queuePort.PublishAndWait` on `TopicRoomInfo` with 3s timeout and parse `GetRoomInfoResponse`, (3) `EnsureUser` + `EnsureJoined` for each member via member intents, (4) call `m.registerDirectRoomParticipants` if `roomInfo.IsDirect`, (5) set power levels via bot intent SendStateEvent: bot=100, creatorUserID→50, users_default=50, (6) set alias via `m.SetRoomAlias` with `m.idMapper.RoomAlias(alkemioRoomID)`, (7) bot leaves via `m.as.BotIntent().LeaveRoom`. Lives in infrastructure layer because it orchestrates 7+ mautrix-go SDK operations directly
- [X] T009 Implement helper `getRoomCreator(ctx context.Context, roomID id.RoomID) (id.UserID, error)` as a private method on `MautrixAdapter` in `internal/infrastructure/matrix/mautrix.go` (read `m.room.create` state event via admin API, extract `creator` field). Used by `ReconcileRoom` for power level adjustment and by the retry path when the original m.room.create event is no longer available
- [X] T010 Implement centralized `resolveOrReconcile` method on `MautrixAdapter` in `internal/infrastructure/matrix/listener.go`: when `resolveAlkemioRoomID` returns `uuid.Nil`, check for `io.alkemio.pending` state event via `GetCustomState(ctx, roomID, []string{"io.alkemio.pending"})`. If found: extract `alkemio_room_id` UUID, get creator via `getRoomCreator`, call `ReconcileRoom`, then on success call `OnRoomCreated` handler to emit `RoomCreatedEvent`. Use a `sync.Map` of room IDs to prevent concurrent reconciliation attempts for the same room. If reconciliation is already in progress for this room, return `uuid.Nil` (event dropped, will be in timeline once reconciliation completes). If not found, return `uuid.Nil` as before (silently drop)
- [X] T011 Update all event handlers in `internal/infrastructure/matrix/listener.go` that call `resolveAlkemioRoomID` to use `resolveOrReconcile` instead: `handleRoomCreateEvent`, `handleMessageEvent` (goroutine), `handleReactionEvent` (goroutine), `handleRedactionEvent` (goroutine), `handleReceiptEvent` (goroutine), `handleMembershipEvent` (goroutine), `handleRoomStateEvent` (goroutine). This ensures reconciliation is retried on ANY subsequent event from a room without an alias, not just `m.room.create`
- [X] T012 Implement HTTP handler `CheckRoomHandler` in new file `internal/infrastructure/http/check_room_handler.go`: `POST /_matrix/app/alkemio/check-room` endpoint, validates Bearer token (hs_token, constant-time comparison), parses `CheckRoomHTTPRequest` JSON, calls `RoomCheckService.CheckRoom`, returns `CheckRoomHTTPResponse` JSON. On timeout returns 504, on internal error returns 500 (per contracts/check-room-http.md)
- [X] T013 Wire everything in `internal/app/app.go`: (1) create `RoomCheckService` with queueAdapter, idMapper, logger, (2) create `CheckRoomHandler` with roomCheckService, idMapper, hsToken, logger and register on router, (3) add `queuePort ports.QueuePort` field to `MautrixAdapter` struct and set it during initialization (before listener starts) so `ReconcileRoom` can call `m.queuePort.PublishAndWait`. No separate ReconciliationService needed — reconciliation lives on MautrixAdapter

**Checkpoint**: Foundation ready — check endpoint operational, reconciliation wired to listener, user story implementation can begin

---

## Phase 3: User Story 1 — DM Creation from Element (Priority: P1) 🎯 MVP

**Goal**: Ghost user creates a DM in Element → check → approve → reconcile → both users chat

**Independent Test**: Log into Element as User A, start DM with User B, send message. Both users see the conversation. Conversation appears on Alkemio platform.

### Implementation

- [X] T014 [US1] Update Synapse module `on_create_room` in `synapse-modules/alkemio_room_control.py`: replace the existing DM notification flow (lines 329-352) with synchronous check logic: (1) save invite list from `request_content["invite"]`, (2) strip invites: `request_content["invite"] = []`, (3) HTTP POST to `{adapter_url}/_matrix/app/alkemio/check-room` with `{creator, members, is_direct}` using `SimpleHttpClient.post_json_get_json` with 3s timeout, (4) on allow: inject `power_level_content_override: {users_default: 50}`, inject `io.alkemio.visibility: {visible: true}` and `io.alkemio.pending: {alkemio_room_id: uuid}` into `initial_state`, return (allow creation), (5) on deny: `raise SynapseError(403, reason, Codes.FORBIDDEN)`, (6) on timeout/error: `raise SynapseError(503, "Service temporarily unavailable", Codes.UNKNOWN)`
- [X] T015 [US1] Handle DM-specific logic in `ReconcileRoom` in `internal/infrastructure/matrix/mautrix.go`: when `roomInfo.IsDirect` is true, call `m.registerDirectRoomParticipants` to set `m.direct` account data for both participants (reuse existing pattern from `CreateRoomWithAlias`). This is already part of T008's reconciliation sequence step (4) — this task verifies DM-specific behavior works correctly in the end-to-end flow
- [ ] T016 [US1] Verify DM dedup rejection works end-to-end: ensure the Synapse module correctly surfaces the server's rejection reason (e.g., "duplicate DM") as a `SynapseError(403)` so Element displays a meaningful error. No adapter code change needed — this is a verification that the check flow handles server-side `allow: false` correctly
- [ ] T017 [US1] Manual integration test: DM happy path per quickstart.md Scenario 1 — create DM from Element, verify both members joined, message exchanged, conversation visible on platform, no invite events in timeline, alias set, bot not in room

**Checkpoint**: DM creation from Element works end-to-end

---

## Phase 4: User Story 2 — Group Conversation from Element (Priority: P1)

**Goal**: Ghost user creates a group room in Element with multiple participants → check → approve → reconcile → all members chat

**Independent Test**: Log into Element as User A, create room with User B and C, send message. All three see the conversation.

### Implementation

- [X] T018 [US2] Extend Synapse module check flow in `synapse-modules/alkemio_room_control.py` to handle non-DM rooms: the check flow added in T014 should already handle groups (same code path, `is_direct` will be false when invite list > 1 or `is_direct` flag is false). Verify that community/non-DM room creation from ghost users goes through the check endpoint instead of being blocked. Remove the old block for non-DM rooms (lines 354-364) and route all ghost user room creation (DMs and groups) through the check flow
- [X] T019 [US2] Handle group-specific logic in `ReconcileRoom` in `internal/infrastructure/matrix/mautrix.go`: when `roomInfo.IsDirect` is false, skip `registerDirectRoomParticipants` (no `m.direct` account data for groups). Verify members with consent disabled are excluded from the member list returned by server (server's responsibility, adapter just joins whoever is in the response)
- [ ] T020 [US2] Manual integration test: Group happy path per quickstart.md Scenario 2 — create group from Element with 3 users, verify all joined, conversation visible on platform. Also test with 20 members to verify SC-002 (group creation with up to 20 members within 5 seconds)

**Checkpoint**: Both DM and group creation from Element work

---

## Phase 5: User Story 3 — Consent and Error Handling (Priority: P2)

**Goal**: Proper error messages in Element when creation is rejected or service unavailable; no orphaned rooms

**Independent Test**: Configure a user with messaging disabled, attempt DM → see 403 error. Stop adapter, attempt DM → see 503 error.

### Implementation

- [X] T021 [US3] Add structured error responses in `CheckRoomHandler` in `internal/infrastructure/http/check_room_handler.go`: ensure all error paths (invalid JSON → 400, unauthorized → 401, RMQ timeout → 504, internal error → 500) return JSON bodies per contract. Verify the Synapse module maps these HTTP errors to appropriate `SynapseError` codes
- [X] T022 [US3] Handle partial group rejection in Synapse module `synapse-modules/alkemio_room_control.py`: when server returns `allow: false` with reason "all members have messaging disabled", raise `SynapseError(403)`. When server allows with reduced member list (some have consent, some don't), the room is still created — non-consenting members simply won't appear in the server's member list during reconciliation
- [ ] T023 [US3] Manual integration test: Consent rejection per quickstart.md Scenario 4 — attempt DM with messaging-disabled user, verify 403 error in Element, no orphaned room
- [ ] T024 [US3] Manual integration test: Service unavailable per quickstart.md Scenario 5 — stop adapter, attempt room creation, verify 503 error, restart adapter, verify retry succeeds

**Checkpoint**: Error handling complete, production-ready error messages

---

## Phase 6: User Story 4 — Space and Child Room Blocking (Priority: P2)

**Goal**: Ghost users remain blocked from creating spaces or rooms inside spaces from Element

**Independent Test**: Attempt to create a space from Element → see 403 error.

### Implementation

- [X] T025 [US4] Verify space blocking is preserved in `synapse-modules/alkemio_room_control.py`: ensure the refactored `on_create_room` still blocks space creation (`m.space` room type) and room-inside-space creation (request has `m.space.parent` in initial_state) for ghost users BEFORE the check endpoint is called. Bot and server admins bypass all restrictions
- [ ] T026 [US4] Manual integration test: Space blocking per quickstart.md Scenario 6 — attempt space creation from Element as ghost user, verify 403 error. Verify bot can still create spaces

**Checkpoint**: Space governance preserved, no regressions

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Cleanup, deprecation, and validation

- [X] T027 [P] Deprecate old DM webhook: mark `DMWebhookHandler` and `DMService` in `internal/infrastructure/http/dm_webhook.go` and `internal/core/service/dm_service.go` as deprecated, and mark `DMWebhookPayload` in `pkg/dto/dm.go` as deprecated (add deprecation comments), but do NOT remove yet — the old flow may still be needed during server-side transition. Remove route registration from `app.go` only if server confirms it no longer sends DM webhooks
- [X] T028 [P] Add `TopicRoomCheck` and `TopicRoomInfo` entries to `CommandRegistry` in `pkg/dto/commands.go` (with `CheckRoomRequest`/`CheckRoomResponse` and `GetRoomInfoRequest`/`GetRoomInfoResponse` type mappings) so TypeScript lib generation picks them up
- [X] T029 [P] Run `make generate` to update TypeScript library in `lib/` with new DTOs and topic constants
- [X] T030 [P] Run `make lint` and fix any linting issues across all modified files
- [X] T031 Run `make test` and verify no regressions in existing tests
- [ ] T032 Run full quickstart.md validation checklist (all 6 scenarios + verification items)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 (needs DTOs, topic constants, QueuePort extension)
- **Phase 3 (US1 DM)**: Depends on Phase 2 (needs services, handler, listener wiring)
- **Phase 4 (US2 Group)**: Depends on Phase 3 (extends the same check flow; shares module + reconciliation code)
- **Phase 5 (US3 Errors)**: Depends on Phase 3 (needs working check flow to test error paths)
- **Phase 6 (US4 Spaces)**: Depends on Phase 3 (needs refactored Synapse module to verify space blocking preserved)
- **Phase 7 (Polish)**: Depends on Phases 3-6

### User Story Dependencies

- **US1 (DM)**: Foundation only — MVP, delivers core value
- **US2 (Group)**: Extends US1 (shares 95% of code, adds group-specific paths)
- **US3 (Errors)**: Builds on US1 check flow (adds error path coverage)
- **US4 (Spaces)**: Regression guard on US1 module refactor (verification, minimal code)

### Within Each Phase

- Tasks marked [P] can run in parallel
- Non-[P] tasks run sequentially in listed order

### Parallel Opportunities

**Phase 1** — All 4 tasks (T001-T004) touch different files, fully parallel:
```
T001: custom_events.go
T002: commands.go + topics.go
T003: pkg/dto/room_check.go (new)
T004: domain/room_check.go (new)
```
T005-T006 are sequential (interface then implementation).

**Phase 7** — T027-T030 touch different files, fully parallel.

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T006)
2. Complete Phase 2: Foundational (T007-T013)
3. Complete Phase 3: User Story 1 — DM (T014-T017)
4. **STOP and VALIDATE**: Test DM flow end-to-end
5. This alone delivers the core value proposition

### Incremental Delivery

1. Phase 1 + 2 → Infrastructure ready
2. Phase 3 (US1 DM) → Test → MVP deployed
3. Phase 4 (US2 Group) → Test → Group support added
4. Phase 5 (US3 Errors) → Test → Production-ready error handling
5. Phase 6 (US4 Spaces) → Test → Regression verification
6. Phase 7 (Polish) → Final cleanup and TypeScript lib update

---

## Notes

- Synapse module changes (T014, T018, T022, T025) are Python code in `synapse-modules/` — tested manually via Element
- The adapter has no integration test framework for live Synapse; manual testing per quickstart.md is the validation path
- `PublishAndWait` (T006) is the most technically novel piece — uses raw amqp091-go for the reply queue alongside existing Watermill publisher
- The old DM webhook flow (T027) is deprecated but not removed until server confirms migration complete
- Reconciliation logic lives on `MautrixAdapter` (infrastructure layer), not as a separate service — it orchestrates 7+ SDK operations directly (R8)
- `resolveOrReconcile` replaces `resolveAlkemioRoomID` in all event handlers — any event from a room without alias triggers reconciliation with per-room dedup via `sync.Map` (R9)
