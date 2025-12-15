# Implementation Plan: Message Events and Read Receipts

**Branch**: `008-read-receipts`
**Status**: ✅ Implemented
**Completed**: 2025-12-15

## Summary

Implemented comprehensive read receipt tracking and message event notifications. The system tracks unread messages at room and thread levels independently, emits RMQ notifications for message edits/redactions/room creation/membership changes, and provides commands for marking messages as read and querying unread counts.

## Technical Stack

- **Language**: Go 1.25
- **Matrix SDK**: mautrix-go v0.26.0
- **Messaging**: Watermill (RabbitMQ)
- **Storage**: Matrix homeserver only (stateless adapter)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Alkemio Server                              │
└─────────────────────────────────────────────────────────────────┘
                              ↕ RabbitMQ
┌─────────────────────────────────────────────────────────────────┐
│  pkg/dto/                   │  internal/infrastructure/queue/   │
│  - read_receipt.go (DTOs)   │  - handler_read_receipt.go        │
│  - commands.go (topics)     │  - router.go                      │
└─────────────────────────────┴───────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────────┐
│  internal/core/service/     │  internal/core/ports/             │
│  - read_receipt_service.go  │  - matrix.go (interface)          │
│  - event_service.go         │                                   │
└─────────────────────────────┴───────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────────┐
│  internal/infrastructure/matrix/                                │
│  - mautrix.go (SDK implementation)                              │
│  - listener.go (event handlers)                                 │
└─────────────────────────────────────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────────┐
│                      Matrix Homeserver                          │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Summary

### New Commands

| Command | Handler | Service Method |
|---------|---------|----------------|
| `MarkMessageRead` | `HandleMarkMessageRead` | `ReadReceiptService.MarkRead` |
| `GetUnreadCounts` | `HandleGetUnreadCounts` | `ReadReceiptService.GetUnreadCounts` |

### New Event Listeners

| Matrix Event | Listener | Published Event |
|--------------|----------|-----------------|
| `m.receipt` | `handleReceiptEvent` | `ReadReceiptUpdatedEvent` |
| `m.room.message` (replace) | `handleMessageEvent` | `MessageEditedEvent` |
| `m.room.redaction` | `handleRedactionEvent` | `MessageRedactedEvent` |
| `m.room.create` | `handleRoomCreateEvent` | `RoomCreatedEvent` |
| `m.room.member` | `handleMembershipEvent` | `RoomMemberUpdatedEvent` |

### Key Patterns

1. **ID Mapping**: All Alkemio ↔ Matrix ID conversions use `domain.IDMapper`
2. **Async Processing**: Event handlers use goroutines to avoid blocking Matrix sync
3. **Thread Context**: Thread root ID extracted from `m.relates_to` when present
4. **Error Handling**: Structured errors with codes defined in `pkg/dto`

## Constitution Compliance

All 10 principles followed:
- ✅ Adapter-First Domain Isolation
- ✅ Event-Driven State Synchronization
- ✅ Microservice Contract Stability
- ✅ Matrix Client Lifecycle Management
- ✅ Observability with Matrix Context
- ✅ Pragmatic Testing with SDK Boundaries
- ✅ Go Service as Source of Truth
- ✅ Secure Matrix Credential Management
- ✅ Container Determinism
- ✅ Simplicity and Matrix API Coverage
