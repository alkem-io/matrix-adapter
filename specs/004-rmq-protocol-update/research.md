# Research: RMQ Protocol Update

**Spec**: [spec.md](spec.md)  
**Status**: ✅ Implemented (research archived)

## Executive Summary

Gap analysis and implementation notes for updating the Matrix Adapter to the new protocol. All identified changes have been implemented.

Key changes implemented:

1. **Renaming topics** from legacy pattern (`room.create`) to new pattern (`communication.room.create`)
2. **Replacing DTOs** with new structures using explicit UUID types
3. **Adding new commands** (room.list, reaction.get, message.get)
4. **Updating batch operations** to return per-room results instead of simple arrays
5. **Enhancing room delete** to kick all members and remove aliases

---

## Current Architecture Analysis

### Layer Structure (Hexagonal)

```
cmd/adapter/main.go          → Entry point
internal/app/app.go          → Application wiring
internal/config/config.go    → Configuration
internal/core/
  ├── domain/model.go        → Domain entities (Actor, Room, Message)
  ├── ports/                 → Interfaces
  │   ├── matrix.go          → MatrixPort interface
  │   ├── queue.go           → QueuePort interface
  │   └── logger.go          → Logger interface
  └── service/               → Business logic
      ├── actor_service.go   → Actor operations
      ├── room_service.go    → Room operations
      ├── admin_service.go   → Admin operations
      └── event_service.go   → Event handling
internal/infrastructure/
  ├── matrix/mautrix.go      → Matrix SDK adapter
  ├── queue/                 → RabbitMQ handlers
  │   ├── router.go          → Topic subscriptions
  │   ├── handler_*.go       → Command handlers
  │   └── errors.go          → Error mapping
  └── logger/zap.go          → Logging adapter
pkg/dto/                     → Public DTOs (contract)
```

### Current Route Mapping (router.go)

| Current Topic | Handler | Notes |
|--------------|---------|-------|
| `room.create` | `room.HandleCreate` | ✅ Exists, needs rename |
| `room.delete` | `room.HandleDelete` | ✅ Exists, needs rename + logic change |
| `room.details` | `room.HandleGetDetails` | ✅ Exists, needs rename to `room.get` |
| `room.members` | `room.HandleGetMembers` | ⚠️ Merge into `room.get` response |
| `room.updateState` | `room.HandleUpdateState` | ✅ Exists, needs rename to `room.update` |
| `room.message.send` | `room.HandleSendMessage` | ✅ Exists, needs rename to `message.send` |
| `room.message.details` | `room.HandleMessageDetails` | ✅ Exists, needs rename to `message.get` |
| `room.message.sendReply` | `room.HandleSendReply` | ⚠️ Merge into `message.send` (ParentMessageID) |
| `room.message.delete` | `room.HandleDeleteMessage` | ✅ Exists, needs rename |
| `room.message.addReaction` | `room.HandleAddReaction` | ✅ Exists, needs rename |
| `room.message.removeReaction` | `room.HandleRemoveReaction` | ✅ Exists, needs rename |
| `actor.register` | `actor.HandleRegister` | ✅ Exists, rename to `actor.sync` |
| `actor.addToRooms` | `actor.HandleAddToRooms` | ✅ Exists, rename to `room.member.batch.add` |
| `actor.removeFromRooms` | `actor.HandleRemoveFromRooms` | ✅ Exists, rename to `room.member.batch.remove` |
| `actor.rooms` | `actor.HandleGetRooms` | ❌ Remove (not in new spec) |
| `actor.rooms.direct` | `actor.HandleGetDirectRooms` | ❌ Remove (not in new spec) |
| `actor.startDirectMessaging` | `actor.HandleStartDirectMessaging` | ⚠️ Merge into `room.create` (Type=direct) |
| `actor.stopDirectMessaging` | `actor.HandleStopDirectMessaging` | ❌ Remove (use room.delete) |
| `admin.allRooms` | `admin.HandleGetAllRooms` | ✅ Exists, rename to `room.list` |
| `admin.replicateRoomMembership` | `admin.HandleReplicateRoomMembership` | ❌ Remove (not in new spec) |

### Missing Commands (New in Protocol)

| New Topic | Existing Implementation | Action Required |
|-----------|------------------------|-----------------|
| `communication.reaction.get` | ❌ None | Add new handler + service method |
| `communication.room.list` (paginated) | Partial (`admin.allRooms`) | Add pagination support |

---

## Current DTO Analysis

### pkg/dto/base.go

```go
type BaseMatrixAdapterEventPayload struct {
    TriggeredBy string `json:"triggeredBy"`  // Actor UUID as string
}
```

**Issue**: Uses `TriggeredBy` for all commands. New protocol uses explicit fields:
- `SenderActorID` for messages
- `ActorID` for batch operations
- No triggeredBy in most commands

### pkg/dto/error.go

```go
const (
    ErrorCodeInvalidPayload   = "INVALID_PAYLOAD"
    ErrorCodeValidationError  = "VALIDATION_ERROR"
    ErrorCodeNotFound         = "NOT_FOUND"
    ErrorCodePermissionDenied = "PERMISSION_DENIED"
    ErrorCodeMatrixError      = "MATRIX_ERROR"
    ErrorCodeInternalError    = "INTERNAL_ERROR"
)
```

**Gap**: New protocol defines different codes:
- `INVALID_PARAM` (replaces `INVALID_PAYLOAD` + `VALIDATION_ERROR`)
- `ROOM_NOT_FOUND` (more specific than `NOT_FOUND`)
- `ACTOR_NOT_FOUND` (more specific than `NOT_FOUND`)
- `NOT_ALLOWED` (replaces `PERMISSION_DENIED`)
- `ErrorResponse.Details` field missing

### Current DTOs vs New Protocol

| Current DTO | New DTO | Changes |
|------------|---------|---------|
| `RoomCreatePayload` | `CreateRoomRequest` | Add `AlkemioRoomID`, `Type`, `InitialMembers`, `Topic`; remove `Metadata` |
| `RoomCreateResponsePayload` | `CreateRoomResponse` | Remove `RoomID` return (server provides it) |
| `RoomDetailsPayload` | `GetRoomRequest` | Rename `RoomID` → `AlkemioRoomID` |
| `RoomDetailsResponse` | `GetRoomResponse` | Add `MemberActorIDs`, `Messages[]` |
| `RoomDeletePayload` | `DeleteRoomRequest` | Add `Reason` |
| `RoomMessageSendPayload` | `SendMessageRequest` | Add `ParentMessageID` for threads |
| `RoomMessageSendResponse` | `SendMessageResponse` | Rename `EventID` → `MessageID`, add `Timestamp` |
| `ActorAddToRoomsPayload` | `BatchAddMemberRequest` | Rename fields, use UUID types |
| `ActorAddToRoomsResponsePayload` | `BatchAddMemberResponse` | Change `FailedRooms[]` → `Results map[string]RoomOperationResult` |

---

## Matrix Adapter Analysis (mautrix.go)

### Current Implementation

- **User provisioning**: Uses `Intent.EnsureRegistered()` - ✅ Correct for Application Service
- **Room creation**: Does NOT set room alias currently - ❌ Need to add `#<UUID>:domain` alias
- **Room lookup**: No alias resolution - ❌ Need to add `ResolveAlias()` for idempotency
- **Room deletion**: Only leaves/forgets - ❌ Need to kick all members + remove alias
- **GetRoomDetails**: Returns Matrix IDs - ❌ Need to map to Alkemio actor IDs
- **GetMessage**: Missing reactions - ❌ Need to fetch reactions with message

### Missing MatrixPort Methods

```go
// Needed for new protocol
ResolveAlias(ctx, alias string) (id.RoomID, error)
DeleteAlias(ctx, alias string) error
KickUser(ctx, roomID, userID, reason string) error
GetRoomMessages(ctx, roomID, limit int) ([]domain.Message, error)
GetReaction(ctx, roomID, reactionID id.EventID) (*domain.Reaction, error)
```

---

## Decisions Made (from Clarifications)

1. **Room ID mapping**: Use room aliases `#<UUID>:<homeserver>` (no external DB)
2. **Retry policy**: No internal retries; fail fast with structured error
3. **Message limit**: Return all messages in `room.get` (no pagination)
4. **Room deletion**: Kick all members, leave, remove alias
5. **Actor provisioning**: Use Intent API (Application Service auto-provision)

---

## Implementation Risks

| Risk | Mitigation |
|------|------------|
| Breaking existing integrations | No backward compat required (per spec) |
| Room alias collisions | Use full UUID, idempotent creation |
| Large message payloads | Noted in spec - may need future pagination |
| Reaction ID tracking | Use Matrix event ID directly |

---

## Technology Choices (Confirmed)

- **Go 1.25**: Current toolchain
- **mautrix-go**: Matrix SDK (Application Service mode)
- **Watermill**: RabbitMQ message routing
- **tygo**: Go-to-TypeScript generation for `lib/`
- **Zap**: Structured logging

---

## File Change Summary

### New Files
- `pkg/dto/room_v2.go` (or update existing)
- `pkg/dto/message_v2.go` (or update existing)
- `pkg/dto/types.go` (UUID type aliases)

### Modified Files
- `pkg/dto/error.go` - New error codes
- `pkg/dto/base.go` - Remove `BaseMatrixAdapterEventPayload`
- `internal/infrastructure/queue/router.go` - New topic names
- `internal/infrastructure/queue/handler_room.go` - Updated handlers
- `internal/infrastructure/queue/handler_actor.go` - Renamed/merged handlers
- `internal/infrastructure/queue/handler_admin.go` - Move to room handler
- `internal/infrastructure/matrix/mautrix.go` - Alias operations, kick, reactions
- `internal/core/ports/matrix.go` - New interface methods
- `internal/core/service/room_service.go` - New methods
- `internal/core/domain/model.go` - Reaction entity

### Deleted Files
- None (refactor in place)

### TypeScript Library
- `lib/src/dto/generated.ts` - Regenerate via `make generate`
