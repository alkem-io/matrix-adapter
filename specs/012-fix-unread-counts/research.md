# Research: Fix Unreliable Unread Message Counts

## Online Research Findings

### mautrix-go versions
- Current: v0.26.0 (2025-11-16)
- Latest: v0.26.4 (2026-03-16) — 4 patch releases behind
- **No receipt retrieval methods exist in any version** — all receipt methods are send-only (`SendReceipt`, `MarkRead`, `SetReadMarkers`)
- Recommendation: Update to v0.26.4 for bug fixes but no new receipt features expected

### Matrix Client-Server API — no GET receipts endpoint
- **Only POST exists**: `POST /_matrix/client/v3/rooms/{roomId}/receipt/{receiptType}/{eventId}` (send-only)
- **No GET endpoint** — this is a known spec gap (matrix-org/matrix-spec#250, open since 2017)
- The Matrix team suggested "Sliding Sync (sync v3)" would solve this but no endpoint was added

### `/sync` ephemeral receipts are DELTAS, not full state
- **Critical finding**: Receipts in `/sync` are deltas — clients must accumulate them over successive syncs
- An initial sync (no `since` token) includes recent ephemeral events but may not include all users' receipt positions
- This means relying solely on a fresh `/sync` for receipt positions is **unreliable**

### `m.fully_read` marker IS retrievable
- Stored as room account data, readable via: `GET /_matrix/client/v3/user/{userId}/rooms/{roomId}/account_data/m.fully_read`
- In mautrix-go: `intent.GetAccountData(ctx, "m.fully_read", &content)` — but this is per-user, not per-room (need room-scoped variant)
- The adapter already uses `intent.GetAccountData()` for `m.direct` (line 1322 of mautrix.go)
- **However**: `m.fully_read` is set by `SetReadMarkers` alongside `m.read`. The adapter's `SendReadReceipt` uses `intent.SendReceipt()` which only sets `m.read`, NOT `m.fully_read`
- **Action needed**: Update `SendReadReceipt` to also set `m.fully_read` via `intent.SetReadMarkers()`, or use a dual approach

## Decision 1: How to retrieve user's read receipt position

**Decision**: Use a two-layer approach:
1. **Primary**: `/sync` with ephemeral events enabled to get receipt positions (works for recent receipts)
2. **Validation**: If `/sync` ephemeral doesn't contain the user's receipt for a room, fall back to Synapse's `notification_count`

**Rationale**:
- The `/sync` ephemeral section is the only standard way to get receipt data
- For an initial sync, Synapse does include recent receipt state in ephemeral events (Synapse-specific behavior, not guaranteed by spec)
- Since the adapter sends read receipts on behalf of managed users (via `SendReceipt`), the receipts are always for appservice-managed users syncing against the same Synapse — initial sync ephemeral is reliable in this context
- The existing `GetBatchUnreadCounts` already does a `/sync` call — we change ephemeral filter from `Limit: 0` to include receipt events
- The receipt content (`event.ReceiptEventContent`) is a map: `eventID → receiptType → userID → Receipt`

**Alternatives considered**:
- **In-memory cache from appservice events**: Fast lookups but cold-start problem on restart. Good future optimization but not suitable as sole approach.
- **`m.fully_read` room account data**: Retrievable but adapter currently doesn't set it when sending receipts. Would require changing `SendReadReceipt` to use `SetReadMarkers`. Could be explored as enhancement.
- **Fetch full timeline and scan**: Wasteful — would fetch thousands of events just to find one receipt position.

## Decision 2: How to count unread messages after receipt position

**Decision**: Use progressive batch fetching via `intent.Messages()` backward from the latest event (batch sizes: 5, 10, 20, 50, 200), counting `m.room.message` events that are not sent by the requesting user, stopping when the receipt target event ID is found.

**Rationale**:
- Reuses the proven progressive fetch pattern from `GetLastMessage` (lines 905-944 of mautrix.go)
- Most rooms have few unread messages, so the first 5-event batch will often suffice
- The Messages API returns events in reverse chronological order when fetching backward, so we can count as we scan
- Falls back to Synapse's `notification_count` (from the same sync response) if receipt not found within 200 events

**Alternatives considered**:
- **Single large Messages fetch (1000 events)**: Wastes bandwidth for typical case (1-10 unread messages)
- **Include timeline in /sync response**: Would need high timeline limit, making sync response unnecessarily large for 10-room batch

## Decision 3: Architecture of the change

**Decision**: Modify only `internal/infrastructure/matrix/mautrix.go` — rewrite `GetUnreadCounts` and `GetBatchUnreadCounts` internals. No changes to ports, services, handlers, or DTOs.

**Rationale**:
- The `MatrixPort` interface signatures remain unchanged: `GetUnreadCounts` returns `*domain.UnreadCountSummary`, `GetBatchUnreadCounts` returns `map[id.RoomID]int`
- The change is entirely an implementation detail of how the adapter calculates counts
- Aligns with Constitution Principle 1 (Adapter-First Domain Isolation) — all Matrix SDK interactions stay within the infrastructure layer

**Alternatives considered**:
- **Add new port methods**: Unnecessary since the contract (input/output) doesn't change
- **Add caching layer in service**: Premature optimization — the progressive fetch is already efficient

## Decision 4: Fallback strategy

**Decision**: Two fallback triggers, both using Synapse's `notification_count` from the same `/sync` response:
1. Read receipt event ID not found in 200 fetched timeline events
2. No receipt found for the user in the `/sync` ephemeral data AND the user has been in the room (i.e., not the "no receipt ever" case from FR-005)

**Rationale**:
- The `/sync` response already contains `notification_count` as a byproduct — zero additional API cost for fallback
- Stale counts from Synapse are better than no counts or errors
- 200-event cap matches the existing `GetLastMessage` progressive fetch ceiling

## Decision 5: Handling the "no receipt" case

**Decision**: When no read receipt exists for the user in a room (never read any message), count all messages (up to 200) excluding the user's own messages via progressive fetch. If more than 200, fall back to Synapse's `notification_count`. This is an accepted design limitation — rooms with >200 messages and no receipt are uncommon in Alkemio's use case.

**Rationale**:
- FR-005 requires returning count of all non-self messages when no receipt exists
- Consistent with the 200-event progressive fetch cap
- New room joins (no receipt) typically have few messages in Alkemio's use case
- The `countUnreadMessages` helper returns `(count-so-far, false)` when cap is hit, allowing the caller to decide: use Synapse fallback

## Decision 6: mautrix-go version

**Decision**: Update mautrix-go from v0.26.0 to v0.26.4 as part of this feature.

**Rationale**:
- 4 patch releases with bug fixes available
- No breaking changes in patch versions
- Good hygiene to stay current with the SDK

## Technical Notes

### Receipt parsing from /sync ephemeral

The `/sync` response structure for ephemeral events:
```
syncResp.Rooms.Join[roomID].Ephemeral.Events[]
  → event.Type == event.EphemeralEventReceipt
  → event.Content.Parsed.(*event.ReceiptEventContent)
  → map[eventID] → map[receiptType] → map[userID] → Receipt{Timestamp, ThreadID}
```

To find the user's latest read position: iterate through all receipt entries, find the one where `userID` matches the requesting user and `receiptType` is `m.read`, then take the associated `eventID`. If multiple entries exist, use the one with the latest timestamp.

### Progressive fetch with Messages API

```go
intent.Messages(ctx, roomID, from, "", mautrix.DirectionBackward, nil, batchSize)
```
- Returns `resp.Chunk` (events) and `resp.End` (pagination token)
- Filter for `evt.Type == event.EventMessage` to count only messages
- Check `evt.Sender` against requesting user's Matrix ID to exclude self-sent
- Use `resp.End` as `from` for next batch
- Stop when: receipt event found, no more events (`resp.End == ""` or `len(resp.Chunk) < batchSize`), or 200 events fetched

### Algorithm overview

```
GetBatchUnreadCounts(actor, roomIDs):
  1. /sync with ephemeral enabled for all rooms → get receipts + notification_count fallbacks
  2. For each room (parallel, semaphore-limited):
     a. Extract user's receipt eventID from ephemeral
     b. Extract notification_count as fallback
     c. If receipt found:
        - Progressive fetch backward, count messages after receipt (exclude self)
        - If receipt eventID not found in 200 events → use notification_count
     d. If no receipt found:
        - Progressive fetch backward, count ALL messages (exclude self)
        - If > 200 events → use notification_count
  3. Return counts map
```
