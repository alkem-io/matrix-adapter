# RabbitMQ Contracts: DM Room Creation

**Feature**: 006-room-creation-control | **Date**: 2025-12-06

## Overview

Defines the RabbitMQ topic for the DM room request notification. Room creation uses the existing `communication.room.create` topic.

---

## 1. Topics

### 1.1 communication.room.dm.requested

**Direction**: Adapter → Alkemio Server  
**Type**: Event (notification)  
**Trigger**: Ghost user attempts to create DM room via Element

**Purpose**: Notifies Alkemio Server that a user wants to start a DM conversation. Server decides whether to authorize and responds via standard `communication.room.create`.

**Message Schema**:
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "DMRequestedEvent",
  "type": "object",
  "required": ["initiator_actor_id", "target_actor_id", "timestamp"],
  "properties": {
    "initiator_actor_id": {
      "type": "string",
      "format": "uuid",
      "description": "Alkemio actor ID of the user requesting the DM",
      "example": "550e8400-e29b-41d4-a716-446655440000"
    },
    "target_actor_id": {
      "type": "string",
      "format": "uuid",
      "description": "Alkemio actor ID of the intended recipient",
      "example": "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
    },
    "timestamp": {
      "type": "string",
      "format": "date-time",
      "description": "ISO 8601 timestamp when request was received"
    },
    "correlation_id": {
      "type": "string",
      "description": "Optional correlation ID for request tracing"
    }
  }
}
```

**Example Message**:
```json
{
  "initiator_actor_id": "550e8400-e29b-41d4-a716-446655440000",
  "target_actor_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  "timestamp": "2024-01-15T10:30:00.000Z",
  "correlation_id": "req-abc-123"
}
```

---

### 1.2 Room Creation via Existing Topic

**Direction**: Alkemio Server → Adapter  
**Topic**: `communication.room.create` (existing)  
**Type**: Command (request/response)  
**Trigger**: Alkemio Server authorizes DM room creation

**Purpose**: Server uses the **standard room creation flow** with `type: "direct"` to create the DM room.

**Request Payload** (existing `CreateRoomRequest`):
```json
{
  "alkemio_room_id": "dm-room-uuid-generated-by-server",
  "type": "direct",
  "initial_members": [
    "550e8400-e29b-41d4-a716-446655440000",
    "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
  ]
}
```

**Notes**:
- `alkemio_room_id`: Server generates this (can use deterministic uuid v5 from sorted actor pair for idempotency)
- `type: "direct"`: Indicates DM room (sets `is_direct=true` in Matrix)
- `initial_members`: Both actors - adapter invites both to the room
- `name`, `topic`: Ignored for direct rooms (DMs don't have display names)

**Response**: Standard `BaseResponse` with `success: true`

---

## 2. Exchange & Routing

**Exchange**: `alkemio` (existing direct exchange)

**New Routing Key**:
| Topic | Routing Key | Direction | Queue |
|-------|-------------|-----------|-------|
| `communication.room.dm.requested` | `communication.room.dm.requested` | Adapter → Server | Alkemio Server queue |

**Existing Routing Key** (reused for DM creation):
| Topic | Routing Key | Direction | Queue |
|-------|-------------|-----------|-------|
| `communication.room.create` | `communication.room.create` | Server → Adapter | Adapter queue |

**Queue Configuration**:
- Durable: Yes
- Auto-delete: No
- Arguments: Standard (no TTL, no DLX for MVP)

---

## 3. Message Properties

| Property | Value | Notes |
|----------|-------|-------|
| `content_type` | `application/json` | All messages are JSON |
| `delivery_mode` | 2 (persistent) | Survive broker restart |
| `correlation_id` | Passthrough | For request/response matching |
| `reply_to` | Caller's queue | For RPC-style commands |
| `timestamp` | Unix epoch | Message creation time |

---

## 4. Error Codes

Room creation errors use the standard error codes from `communication.room.create`:

| Code | HTTP Equiv | Description |
|------|------------|-------------|
| `INVALID_PAYLOAD` | 400 | Malformed JSON or missing required fields |
| `USER_NOT_FOUND` | 404 | Actor ID does not map to Matrix user |
| `ROOM_CREATION_FAILED` | 500 | Matrix API returned error |
| `INTERNAL_ERROR` | 500 | Unexpected adapter error |

---

## 5. Sequence Diagram

```
┌─────────┐     ┌──────────────┐     ┌─────────────┐     ┌────────────┐
│ Element │     │Synapse Module│     │   Adapter   │     │Alkemio Svr │
└────┬────┘     └──────┬───────┘     └──────┬──────┘     └─────┬──────┘
     │                 │                    │                  │
     │ Create DM       │                    │                  │
     │ (m.room.create) │                    │                  │
     │────────────────►│                    │                  │
     │                 │                    │                  │
     │                 │ Webhook POST       │                  │
     │                 │ /dm-request        │                  │
     │                 │───────────────────►│                  │
     │                 │                    │                  │
     │                 │          200 OK    │                  │
     │                 │◄───────────────────│                  │
     │                 │                    │                  │
     │   FORBIDDEN     │                    │ Publish Event    │
     │◄────────────────│                    │ dm.requested     │
     │                 │                    │─────────────────►│
     │                 │                    │                  │
     │                 │                    │                  │ Authorize
     │                 │                    │                  │────┐
     │                 │                    │                  │    │
     │                 │                    │                  │◄───┘
     │                 │                    │                  │
     │                 │                    │   Command        │
     │                 │                    │   room.create    │
     │                 │                    │   (type=direct)  │
     │                 │                    │◄─────────────────│
     │                 │                    │                  │
     │                 │                    │ Create Room      │
     │                 │                    │ (AppService)     │
     │                 │                    │────┐             │
     │                 │                    │    │             │
     │                 │                    │◄───┘             │
     │                 │                    │                  │
     │                 │                    │   BaseResponse   │
     │                 │                    │   {success:true} │
     │                 │                    │─────────────────►│
     │                 │                    │                  │
     │ Invited to DM   │                    │                  │
     │◄────────────────────────────────────────────────────────│
     │                 │                    │                  │
```

---

## 6. Backward Compatibility

- **New Topic**: Only `communication.room.dm.requested` is new (adapter → server)
- **Reused Topic**: `communication.room.create` with `type: "direct"` - no changes needed
- **Version Strategy**: Payload changes will follow semantic versioning
- **Migration**: Alkemio Server must:
  1. Subscribe to `communication.room.dm.requested` events
  2. Send `communication.room.create` with `type: "direct"` when authorizing DMs

---

## 7. Implementation Notes

### Why Reuse `communication.room.create`?

1. **Simplicity**: No new handler needed in adapter - existing `HandleCreateRoom` already supports `type: "direct"`
2. **Consistency**: Same pattern for all room types
3. **Fewer DTOs**: No `CreateDMRoomCommand`/`CreateDMRoomResponse` needed
4. **Existing Tests**: Room creation already tested

### Server Responsibilities

- Generate `alkemio_room_id` for the DM room (deterministic uuid v5 recommended for idempotency)
- Send both actor IDs in `initial_members` array
- Handle case where DM room already exists (idempotent creation or lookup)

````
