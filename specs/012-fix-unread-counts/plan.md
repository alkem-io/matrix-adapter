# Implementation Plan: Fix Unreliable Unread Message Counts

**Branch**: `012-fix-unread-counts` | **Date**: 2026-03-19 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/012-fix-unread-counts/spec.md`

## Summary

Replace Synapse's unreliable `notification_count` (stale after read receipts due to element-hq/synapse#18111) with self-calculated unread counts. The adapter will:
1. Retrieve the user's read receipt position from `/sync` ephemeral events
2. Count messages after that position using progressive batch fetching via the Messages API
3. Fall back to Synapse's `notification_count` when self-calculation isn't possible (receipt not found in 200 events, no ephemeral data)

All changes are confined to `internal/infrastructure/matrix/mautrix.go` — no interface, DTO, or handler changes.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: mautrix-go v0.26.0 → v0.26.4 (update), Watermill (RabbitMQ), Zap (logging)
**Storage**: N/A (no persistence changes)
**Testing**: Go `testing` + `testify`, mocked `MatrixPort` interface
**Target Platform**: Linux server (Docker/K8s)
**Project Type**: Web service (Matrix adapter microservice)
**Performance Goals**: Batch unread counts for 10 rooms within 5 seconds
**Constraints**: Progressive fetch capped at 200 events per room; semaphore-limited concurrency for batch operations
**Scale/Scope**: 4-10 rooms per batch request typically

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | PASS | All changes in `internal/infrastructure/matrix/mautrix.go`. No service/handler imports of mautrix-go SDK. |
| 2. Event-Driven State Sync | PASS | No new synchronous blocking. Batch operations use parallel goroutines with semaphore. |
| 3. Microservice Contract Stability | PASS | No changes to DTOs, command payloads, or response formats. |
| 4. Matrix Client Lifecycle | PASS | Uses existing `intent.EnsureRegistered()` pattern. No new client creation. |
| 5. Observability | PASS | Structured debug logging with roomID, actorID, eventID context. Fallback events logged. |
| 6. Pragmatic Testing | PASS | Unit tests mock `MatrixPort`. No new integration tests needed (reusing existing SDK methods). |
| 7. Go Service Source of Truth | PASS | No DTO changes → no `make generate` needed. |
| 8. Secure Credentials | PASS | No credential handling changes. |
| 9. Container Determinism | PASS | Only mautrix-go patch version bump in go.mod. |
| 10. Simplicity | PASS | Reuses existing progressive fetch pattern. No speculative API surface expansion. |

**Post-Phase 1 re-check**: All gates still pass. The design adds no new external interfaces, no new domain types, and no new SDK method categories.

## Project Structure

### Documentation (this feature)

```text
specs/012-fix-unread-counts/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: API research and decisions
├── data-model.md        # Phase 1: No new entities
├── quickstart.md        # Phase 1: Developer guide
└── checklists/
    └── requirements.md  # Spec quality checklist
```

### Source Code (files modified)

```text
internal/infrastructure/matrix/
└── mautrix.go           # Rewritten GetUnreadCounts + GetBatchUnreadCounts
                         # New helper: extractUserReadReceipt()
                         # New helper: countUnreadMessages()

go.mod                   # mautrix-go v0.26.0 → v0.26.4
go.sum                   # Updated checksums
```

**Structure Decision**: No new files or directories. Changes are entirely within the existing Matrix adapter implementation file, consistent with the project's architecture.

## Implementation Design

### Algorithm: Self-Calculated Unread Counts

#### Step 1: Get receipt positions via /sync

Modify the existing `/sync` call (already used in both methods) to include ephemeral events:

```
Filter change:
  Ephemeral.Limit: 0  →  Ephemeral.Types: ["m.receipt"] (no limit)
```

Parse `syncResp.Rooms.Join[roomID].Ephemeral.Events` for `m.receipt` events to extract the user's read receipt event ID per room.

#### Step 2: Count unread messages via progressive fetch

For each room (parallel with semaphore for batch):

```
extractUserReadReceipt(ephemeralEvents, userID) → receiptEventID | nil
  - Iterate m.receipt events
  - Find entry where userID matches and receiptType is m.read
  - Return the associated eventID (or nil if not found)

countUnreadMessages(ctx, intent, roomID, receiptEventID, userID) → count, found
  - Progressive batch sizes: [5, 10, 20, 50, 200]
  - For each batch: intent.Messages(backward)
  - For each event in batch:
    - If event.ID == receiptEventID → return count, true (found receipt)
    - If event.Type == m.room.message AND event.Sender != userID → count++
  - If 200 events scanned without finding receipt → return count-so-far, false
  - If no more events → return count, true (counted all messages)
```

#### Step 3: Assemble results with fallback

```
For each room:
  if receiptEventID != nil:
    count, found = countUnreadMessages(receiptEventID)
    if found → use count
    else → use notification_count from sync (fallback)
  else if no receipt in ephemeral:
    count, found = countUnreadMessages(nil)  // count ALL non-self messages
    if found → use count
    else → use notification_count from sync (fallback)
```

### New Helper Functions

#### `extractUserReadReceipt`
- Input: `[]event.Event` (ephemeral events from sync), `id.UserID`
- Output: `*id.EventID` (pointer: nil = no receipt found, non-nil = receipt target event ID)
- Logic: Iterate receipt events, find user's `m.read` receipt, return event ID with latest timestamp

#### `countUnreadMessages`
- Input: `context.Context`, `*appservice.IntentAPI`, `id.RoomID`, `*id.EventID` (receipt target: nil = count all), `id.UserID` (to exclude self)
- Output: `int` (count of non-self messages found), `bool` (true = receipt found or all events scanned; false = 200-event cap hit without finding receipt)
- Logic: Progressive batch fetch backward, count m.room.message events after receipt position. When cap is hit, returns the count accumulated so far (caller decides whether to use it or fall back).

### Batch Operation Flow

```
GetBatchUnreadCounts(ctx, actor, roomIDs):
  1. Create /sync filter: Rooms=roomIDs, Timeline.Limit=0, Ephemeral enabled
  2. SyncRequest → syncResp (contains ephemeral + notification_counts)
  3. For each roomID (parallel, sem=10):
     a. receipt = extractUserReadReceipt(ephemeral, userID)
     b. fallback = syncResp notification_count
     c. count, found = countUnreadMessages(ctx, intent, roomID, receipt, userID)
     d. if found → results[roomID] = count
        else → results[roomID] = fallback
  4. Return results
```

## Complexity Tracking

No constitution violations. No complexity justifications needed.
