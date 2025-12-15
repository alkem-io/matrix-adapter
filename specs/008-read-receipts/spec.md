# Feature Specification: Message Events and Read Receipts

**Feature Branch**: `008-read-receipts`
**Status**: ✅ Implemented
**Completed**: 2025-12-15

## Overview

This feature adds comprehensive message event tracking and read receipt management to the Matrix adapter, enabling the Alkemio platform to track message activity and user read status at both room and thread levels.

## Implemented Capabilities

### Incoming Commands (Server → Adapter)

| Command | Topic | Description |
|---------|-------|-------------|
| Mark Message Read | `communication.message.read` | Mark messages as read for a user (room or thread level) |
| Get Unread Counts | `communication.room.unread_counts.get` | Query unread message counts |

### Outgoing Events (Adapter → Server)

| Event | Topic | Description |
|-------|-------|-------------|
| Read Receipt Updated | `matrix.room.receipt.updated` | User's read position changed |
| Message Edited | `matrix.room.message.edited` | Message content modified via `m.replace` |
| Message Redacted | `matrix.room.message.redacted` | Message permanently deleted |
| Room Created | `matrix.room.created` | New room created in Matrix |
| Room Member Updated | `matrix.room.member.updated` | Membership state changed (join/invite/leave/ban) |

## Key Design Decisions

1. **Stateless Adapter**: All read receipt state stored in Matrix homeserver (no adapter-side caching)
2. **Thread Independence**: Room-level and thread-level receipts tracked independently
3. **On-Demand Calculation**: Unread counts calculated by querying Matrix homeserver
4. **Matrix-Native Receipts**: Uses `m.read` for room-level, `m.read.thread` for thread-level

## Matrix Event Handling

| Matrix Event | Adapter Action |
|--------------|----------------|
| `m.room.message` with `m.replace` | Emit `MessageEditedEvent` |
| `m.room.redaction` | Emit `MessageRedactedEvent` |
| `m.room.create` | Emit `RoomCreatedEvent` |
| `m.room.member` (join/invite) | Emit `RoomMemberUpdatedEvent` |
| `m.receipt` | Emit `ReadReceiptUpdatedEvent` |

## Thread Context

All message-related events include thread context when applicable:
- `MatrixThreadID` field contains the thread root event ID
- `nil` indicates room-level (main timeline) activity

## Files Modified/Created

### New Files
- `internal/core/domain/read_receipt.go` - Domain models
- `internal/core/service/read_receipt_service.go` - Business logic
- `internal/infrastructure/queue/handler_read_receipt.go` - Command handlers
- `pkg/dto/read_receipt.go` - DTOs for commands/events

### Modified Files
- `internal/core/ports/matrix.go` - Added receipt methods
- `internal/core/service/event_service.go` - Event publishing
- `internal/infrastructure/matrix/mautrix.go` - Receipt implementation
- `internal/infrastructure/matrix/listener.go` - Event listeners
- `internal/infrastructure/queue/router.go` - Handler registration
- `internal/infrastructure/queue/topics.go` - New topics

## Out of Scope

- Push notification delivery
- Read receipt privacy controls
- Typing indicators / presence
- Delivery receipts
- Batch mark-as-read operations
- Nested threading (only single-level supported)
