# Tasks: Fix Unreliable Unread Message Counts

**Input**: Design documents from `/specs/012-fix-unread-counts/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md

**Tests**: Not explicitly requested in spec. Test tasks omitted.

**Organization**: Tasks grouped by user story. All changes are in a single file (`internal/infrastructure/matrix/mautrix.go`) plus dependency update, so parallelism is limited within stories but stories themselves share the same file and must be sequential.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Dependency update and preparation

- [x] T001 Update mautrix-go from v0.26.0 to v0.26.4 in go.mod and run `go mod tidy` to update go.sum
- [x] T002 Verify build passes after dependency update with `make build`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Helper functions that all user stories depend on

**CRITICAL**: No user story work can begin until this phase is complete

- [x] T003 Implement `extractUserReadReceipt` helper function in `internal/infrastructure/matrix/mautrix.go` — takes ephemeral events slice (`[]event.Event`) and user ID (`id.UserID`), iterates `m.receipt` events parsed as `*event.ReceiptEventContent`, finds the user's `m.read` receipt entry, returns `*id.EventID` (pointer: nil = no receipt found, non-nil = receipt target event ID with latest timestamp if multiple entries exist).
- [x] T004 Implement `countUnreadMessages` helper function in `internal/infrastructure/matrix/mautrix.go` — takes context, intent, room ID, `*id.EventID` (receipt target: nil = count all non-self messages), and `id.UserID` to exclude. Uses progressive batch fetching (sizes: `[]int{5, 10, 20, 50, 200}`) via `intent.Messages(ctx, roomID, from, "", mautrix.DirectionBackward, nil, batchSize)`. For each event: if receipt is non-nil and `event.ID == *receiptEventID` return (count, true); if `event.Type == event.EventMessage && event.Sender != userID` increment count. Returns `(count-so-far, false)` if 200 events scanned without finding receipt — caller decides whether to use partial count or Synapse fallback. Returns `(count, true)` if receipt found or no more events to fetch. Include structured debug logging with room_id, receipt_event_id, events_scanned, and count.
- [x] T005 Implement `extractSynapseNotificationCount` helper function in `internal/infrastructure/matrix/mautrix.go` — takes a `mautrix.SyncJoinedRoom` and extracts the notification count (prefer `MSC2654UnreadCount` over `UnreadNotifications.NotificationCount`). This encapsulates the existing fallback logic.

**Checkpoint**: Foundation ready — helper functions tested via compilation, user story implementation can begin

---

## Phase 3: User Story 1 - Accurate Unread Count After Reading Messages (Priority: P1) MVP

**Goal**: Single-room `GetUnreadCounts` self-calculates unread count from read receipt position and message timeline instead of trusting Synapse's `notification_count`.

**Independent Test**: Send a read receipt for a room, request unread count, verify it matches messages after receipt position excluding self-sent.

### Implementation for User Story 1

- [x] T006 [US1] Rewrite `GetUnreadCounts` method in `internal/infrastructure/matrix/mautrix.go` — modify the sync filter to include ephemeral events (change `Ephemeral.Limit: 0` to include `m.receipt` type events). After sync, call `extractUserReadReceipt` to get `*id.EventID` from ephemeral. Call `extractSynapseNotificationCount` to get fallback value. If receipt non-nil: call `countUnreadMessages` with receipt pointer; if `found` is true use self-calculated count, else use Synapse fallback (log as "receipt not found in 200 events — possible redaction or large gap"). If receipt is nil: call `countUnreadMessages` with nil (count all non-self messages); if `found` is true use that count, else use Synapse fallback. Set `summary.RoomUnreadCount` to the determined value. Preserve existing thread-level stub behavior (empty map, debug log).
- [x] T007 [US1] Add structured debug logging to `GetUnreadCounts` in `internal/infrastructure/matrix/mautrix.go` — log receipt event ID found (or "none"), calculation method used ("self-calculated" vs "synapse-fallback"), final count, events scanned. Include room_id, actor_id, receipt_event_id fields.
- [x] T008 [US1] Verify `GetUnreadCounts` builds and edge cases compile correctly with `make build`

**Checkpoint**: Single-room unread counts are self-calculated. Can be tested independently via the `communication.room.unread_counts.get` queue topic.

---

## Phase 4: User Story 2 - Batch Unread Counts Across Multiple Rooms (Priority: P1)

**Goal**: Multi-room `GetBatchUnreadCounts` self-calculates unread counts for all rooms using parallel progressive fetching with semaphore-limited concurrency.

**Independent Test**: Set up 3-5 rooms with known states, request batch unread counts, verify each room's count is correct.

### Implementation for User Story 2

- [x] T009 [US2] Rewrite `GetBatchUnreadCounts` method in `internal/infrastructure/matrix/mautrix.go` — modify sync filter to include ephemeral events (same filter change as US1). After single sync for all rooms, launch parallel goroutines (semaphore-limited to 10) for each room. Each goroutine: call `extractUserReadReceipt` to get `*id.EventID`, get fallback via `extractSynapseNotificationCount`, call `countUnreadMessages`, determine final count — use self-calculated count if `found` is true, else Synapse fallback (with distinguishing debug log: "receipt not found in 200 events — possible redaction or large gap" vs "no receipt — fallback after 200-event cap"). Collect results via channel (reuse existing `result` struct and channel pattern from `GetBatchLastMessages`).
- [x] T010 [US2] Add per-room debug logging in `GetBatchUnreadCounts` in `internal/infrastructure/matrix/mautrix.go` — for each room log: receipt_event_id (or "none"), calculation_method ("self-calculated" / "synapse-fallback"), unread_count, events_scanned. Keep existing summary log with rooms_requested, rooms_found, rooms_failed.
- [x] T011 [US2] Verify `GetBatchUnreadCounts` builds correctly with `make build`

**Checkpoint**: Batch unread counts are self-calculated. Can be tested via the `communication.room.batch.unread_counts.get` queue topic.

---

## Phase 5: User Story 3 - Unread Count for Room with No Read History (Priority: P2)

**Goal**: Rooms where the user has never sent a read receipt correctly report all non-self messages as unread.

**Independent Test**: Create room with messages, user has no receipt, verify unread count equals total non-self messages.

### Implementation for User Story 3

- [x] T012 [US3] Verify "no receipt" path in `countUnreadMessages` helper in `internal/infrastructure/matrix/mautrix.go` — when called with `nil` receipt event ID, the function should count ALL `m.room.message` events not from the user until events are exhausted or 200-event cap is hit. Add a debug log distinguishing "no receipt — counting all messages" from "has receipt — counting after position". This path should already work from T004 but verify and add explicit log.
- [x] T013 [US3] Verify "no receipt" path in both `GetUnreadCounts` and `GetBatchUnreadCounts` in `internal/infrastructure/matrix/mautrix.go` — when `extractUserReadReceipt` returns nil, both methods should pass nil to `countUnreadMessages`. If `found` is false (200-event cap hit), use Synapse fallback (design limitation for rooms with >200 messages and no receipt). Add debug log: "No read receipt found for user in room, counting all messages".

**Checkpoint**: All user stories functional. Rooms with no read history correctly report unread counts.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Cleanup, validation, and edge case hardening

- [x] T014 Remove the previous per-room debug logging added earlier in this session (the logging added before the spec was created) from `GetBatchUnreadCounts` and `GetBatchLastMessages` in `internal/infrastructure/matrix/mautrix.go` — these are superseded by the new structured logging in T007 and T010
- [x] T015 Run `make lint` to verify no linting issues in `internal/infrastructure/matrix/mautrix.go`
- [x] T016 Run `make build` for final compilation check
- [x] T017 Run `make test` to verify existing tests still pass (no regressions)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 (updated mautrix-go)
- **US1 (Phase 3)**: Depends on Phase 2 (helper functions)
- **US2 (Phase 4)**: Depends on Phase 2 (helper functions). Independent of US1 but modifies same file — execute sequentially.
- **US3 (Phase 5)**: Depends on Phase 2 (helper functions). Verifies paths already implemented in T004.
- **Polish (Phase 6)**: Depends on all user stories complete

### User Story Dependencies

- **User Story 1 (P1)**: Depends on foundational helpers (T003-T005). No dependency on other stories.
- **User Story 2 (P1)**: Depends on foundational helpers (T003-T005). Same file as US1 — execute after US1.
- **User Story 3 (P2)**: Depends on foundational helpers (T003-T005). Verification of existing paths — execute after US1 and US2.

### Within Each User Story

- Implementation tasks are sequential (same file: `mautrix.go`)
- Build verification after each story

### Parallel Opportunities

- T001 (go.mod update) is independent of spec/plan reading
- T003, T004, T005 could theoretically be parallel but all modify `mautrix.go` — execute sequentially
- T014 and T015 can run in parallel (different concerns)
- US1, US2, US3 all modify the same file — must be sequential

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T002)
2. Complete Phase 2: Foundational helpers (T003-T005)
3. Complete Phase 3: User Story 1 (T006-T008)
4. **STOP and VALIDATE**: Test single-room unread counts via queue
5. Deploy/demo if ready

### Full Delivery

1. Setup → Foundational → US1 → US2 → US3 → Polish
2. Each story builds on the same helpers, adding scope
3. US1 = single-room calculation
4. US2 = batch calculation with parallel fetching
5. US3 = verification of no-receipt edge case
6. Polish = cleanup and validation

---

## Notes

- All implementation tasks modify `internal/infrastructure/matrix/mautrix.go` — no parallelism within phases
- No new files created — all changes within existing adapter
- No interface/DTO/handler changes — purely infrastructure layer
- Existing tests should continue to pass (mock-based, don't test internal calculation)
- The `countUnreadMessages` helper handles both "has receipt" and "no receipt" cases via the nil check on receipt event ID
- Fallback to Synapse's `notification_count` is always available from the same sync response — zero additional API cost
