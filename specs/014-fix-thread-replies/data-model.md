# Data Model: Fix Thread Reply Formatting and Message Ordering

**Date**: 2026-03-30
**Status**: No changes (retrofit bug fix)

## Entities

No new entities introduced. This feature modifies the behavior of two existing operations without changing any data structures.

### Existing Entities (unchanged)

| Entity | Location | Change |
|--------|----------|--------|
| `domain.Message` | `internal/core/domain/` | No structural changes. `ThreadID` field already exists. |
| `event.MessageEventContent` | mautrix-go SDK type | No changes. `RelatesTo` struct already supports `Type`, `EventID`, `InReplyTo`, and `IsFallingBack` fields. |
| `RespRelations` | `internal/infrastructure/matrix/` | No changes. Already used for relations API responses. |

## Relationships

No new relationships. The existing thread-root-to-reply relationship (via `m.thread` / `m.in_reply_to`) is now correctly expressed in the outgoing event payload.

## State Transitions

N/A — no state changes introduced.
