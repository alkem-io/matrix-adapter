# RMQ Command Contracts

**Spec**: [spec.md](../spec.md)  
**Data Model**: [data-model.md](../data-model.md)  
**Status**: ✅ Implemented

## Overview

All 14 RabbitMQ command contracts for the Matrix Adapter protocol. Each command follows the request-response pattern over RabbitMQ.

---

## Transport Details

- **Exchange**: Direct exchange
- **Routing Key**: Command topic (e.g., `communication.room.create`)
- **Reply-To**: Correlation ID based routing for responses
- **Content-Type**: `application/json`
- **Encoding**: UTF-8

---

## Command Reference

### 1. communication.room.create

Creates a new communication room.

| Property | Value |
|----------|-------|
| Topic | `communication.room.create` |
| Request | `CreateRoomRequest` |
| Response | `CreateRoomResponse` |
| Idempotent | Yes (if room exists, returns success) |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "community",
  "name": "Project Alpha Discussion",
  "topic": "Discuss Project Alpha milestones",
  "initial_members": [
    "660e8400-e29b-41d4-a716-446655440001",
    "770e8400-e29b-41d4-a716-446655440002"
  ]
}
```

**Success Response**:
```json
{
  "success": true
}
```

**Error Response**:
```json
{
  "success": false,
  "error": {
    "code": "INVALID_PARAM",
    "message": "direct rooms require exactly 2 initial_members"
  }
}
```

---

### 2. communication.room.get

Retrieves room details including members and all messages.

| Property | Value |
|----------|-------|
| Topic | `communication.room.get` |
| Request | `GetRoomRequest` |
| Response | `GetRoomResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Success Response**:
```json
{
  "success": true,
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "display_name": "Project Alpha Discussion",
  "member_actor_ids": [
    "660e8400-e29b-41d4-a716-446655440001",
    "770e8400-e29b-41d4-a716-446655440002"
  ],
  "messages": [
    {
      "id": "$event123",
      "content": "Hello team!",
      "sender_actor_id": "660e8400-e29b-41d4-a716-446655440001",
      "timestamp": "2025-12-02T10:00:00Z",
      "reactions": [],
      "thread_id": null
    }
  ]
}
```

---

### 3. communication.room.update

Updates room metadata.

| Property | Value |
|----------|-------|
| Topic | `communication.room.update` |
| Request | `UpdateRoomRequest` |
| Response | `UpdateRoomResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Project Alpha - Phase 2",
  "topic": "Phase 2 planning"
}
```

---

### 4. communication.room.delete

Deletes a room (kicks all members, removes alias).

| Property | Value |
|----------|-------|
| Topic | `communication.room.delete` |
| Request | `DeleteRoomRequest` |
| Response | `DeleteRoomResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "reason": "Project completed"
}
```

---

### 5. communication.room.list

Lists all rooms (admin operation with pagination).

| Property | Value |
|----------|-------|
| Topic | `communication.room.list` |
| Request | `ListRoomsRequest` |
| Response | `ListRoomsResponse` |

**Request Example**:
```json
{
  "limit": 50,
  "cursor": ""
}
```

**Success Response**:
```json
{
  "success": true,
  "alkemio_room_ids": [
    "550e8400-e29b-41d4-a716-446655440000",
    "550e8400-e29b-41d4-a716-446655440001"
  ],
  "next_cursor": "abc123"
}
```

---

### 6. communication.message.send

Sends a message to a room.

| Property | Value |
|----------|-------|
| Topic | `communication.message.send` |
| Request | `SendMessageRequest` |
| Response | `SendMessageResponse` |

**Request Example** (new message):
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "sender_actor_id": "660e8400-e29b-41d4-a716-446655440001",
  "content": "Hello team! Here's the update."
}
```

**Request Example** (threaded reply):
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "sender_actor_id": "660e8400-e29b-41d4-a716-446655440001",
  "content": "Thanks for the update!",
  "parent_message_id": "$event123"
}
```

**Success Response**:
```json
{
  "success": true,
  "message_id": "$event456",
  "timestamp": "2025-12-02T10:05:00Z"
}
```

---

### 7. communication.message.get

Retrieves details of a specific message.

| Property | Value |
|----------|-------|
| Topic | `communication.message.get` |
| Request | `GetMessageRequest` |
| Response | `GetMessageResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_id": "$event123"
}
```

**Success Response**:
```json
{
  "success": true,
  "message": {
    "id": "$event123",
    "content": "Hello team!",
    "sender_actor_id": "660e8400-e29b-41d4-a716-446655440001",
    "timestamp": "2025-12-02T10:00:00Z",
    "reactions": [
      {
        "id": "$reaction789",
        "emoji": "👍",
        "sender_actor_id": "770e8400-e29b-41d4-a716-446655440002",
        "timestamp": "2025-12-02T10:01:00Z"
      }
    ],
    "thread_id": null
  }
}
```

---

### 8. communication.message.delete

Deletes (redacts) a message.

| Property | Value |
|----------|-------|
| Topic | `communication.message.delete` |
| Request | `DeleteMessageRequest` |
| Response | `DeleteMessageResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_id": "$event123",
  "sender_actor_id": "660e8400-e29b-41d4-a716-446655440001",
  "reason": "Message contained error"
}
```

---

### 9. communication.reaction.add

Adds an emoji reaction to a message.

| Property | Value |
|----------|-------|
| Topic | `communication.reaction.add` |
| Request | `AddReactionRequest` |
| Response | `AddReactionResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_id": "$event123",
  "sender_actor_id": "770e8400-e29b-41d4-a716-446655440002",
  "emoji": "👍"
}
```

**Success Response**:
```json
{
  "success": true,
  "reaction_id": "$reaction789"
}
```

---

### 10. communication.reaction.remove

Removes a reaction.

| Property | Value |
|----------|-------|
| Topic | `communication.reaction.remove` |
| Request | `RemoveReactionRequest` |
| Response | `RemoveReactionResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "reaction_id": "$reaction789",
  "sender_actor_id": "770e8400-e29b-41d4-a716-446655440002"
}
```

---

### 11. communication.reaction.get

Retrieves details of a specific reaction.

| Property | Value |
|----------|-------|
| Topic | `communication.reaction.get` |
| Request | `GetReactionRequest` |
| Response | `GetReactionResponse` |

**Request Example**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "reaction_id": "$reaction789"
}
```

**Success Response**:
```json
{
  "success": true,
  "reaction": {
    "id": "$reaction789",
    "emoji": "👍",
    "sender_actor_id": "770e8400-e29b-41d4-a716-446655440002",
    "timestamp": "2025-12-02T10:01:00Z"
  }
}
```

---

### 12. communication.room.member.batch.add

Adds an actor to multiple rooms (partial success supported).

| Property | Value |
|----------|-------|
| Topic | `communication.room.member.batch.add` |
| Request | `BatchAddMemberRequest` |
| Response | `BatchAddMemberResponse` |

**Request Example**:
```json
{
  "actor_id": "660e8400-e29b-41d4-a716-446655440001",
  "alkemio_room_ids": [
    "550e8400-e29b-41d4-a716-446655440000",
    "550e8400-e29b-41d4-a716-446655440001"
  ]
}
```

**Success Response** (partial success):
```json
{
  "success": true,
  "results": {
    "550e8400-e29b-41d4-a716-446655440000": {
      "success": true
    },
    "550e8400-e29b-41d4-a716-446655440001": {
      "success": false,
      "error": {
        "code": "ROOM_NOT_FOUND",
        "message": "Room does not exist"
      }
    }
  }
}
```

**Error Response** (actor not found):
```json
{
  "success": false,
  "error": {
    "code": "ACTOR_NOT_FOUND",
    "message": "Actor 660e8400-e29b-41d4-a716-446655440001 does not exist"
  }
}
```

---

### 13. communication.room.member.batch.remove

Removes an actor from multiple rooms.

| Property | Value |
|----------|-------|
| Topic | `communication.room.member.batch.remove` |
| Request | `BatchRemoveMemberRequest` |
| Response | `BatchRemoveMemberResponse` |

**Request Example**:
```json
{
  "actor_id": "660e8400-e29b-41d4-a716-446655440001",
  "alkemio_room_ids": [
    "550e8400-e29b-41d4-a716-446655440000"
  ],
  "reason": "User left project"
}
```

---

### 14. communication.actor.sync

Ensures an actor exists in Matrix and updates their profile.

| Property | Value |
|----------|-------|
| Topic | `communication.actor.sync` |
| Request | `SyncActorRequest` |
| Response | `SyncActorResponse` |
| Idempotent | Yes |

**Request Example**:
```json
{
  "actor_id": "660e8400-e29b-41d4-a716-446655440001",
  "display_name": "Jane Doe",
  "avatar_url": "https://example.com/avatar.png"
}
```

**Success Response**:
```json
{
  "success": true
}
```

---

## Error Code Reference

| Code | Description | Retryable |
|------|-------------|-----------|
| `INVALID_PARAM` | Request validation failed | No |
| `ROOM_NOT_FOUND` | Room does not exist | No |
| `ACTOR_NOT_FOUND` | Actor does not exist | No |
| `MATRIX_ERROR` | Matrix homeserver error | Yes (by server) |
| `INTERNAL_ERROR` | Unexpected adapter error | Yes (by server) |
| `NOT_ALLOWED` | Operation not permitted | No |

---

## Topic Migration Map

| Old Topic | New Topic |
|-----------|-----------|
| `room.create` | `communication.room.create` |
| `room.details` | `communication.room.get` |
| `room.updateState` | `communication.room.update` |
| `room.delete` | `communication.room.delete` |
| `admin.allRooms` | `communication.room.list` |
| `room.message.send` | `communication.message.send` |
| `room.message.sendReply` | `communication.message.send` (with parent_message_id) |
| `room.message.details` | `communication.message.get` |
| `room.message.delete` | `communication.message.delete` |
| `room.message.addReaction` | `communication.reaction.add` |
| `room.message.removeReaction` | `communication.reaction.remove` |
| (new) | `communication.reaction.get` |
| `actor.addToRooms` | `communication.room.member.batch.add` |
| `actor.removeFromRooms` | `communication.room.member.batch.remove` |
| `actor.register` | `communication.actor.sync` |
| `actor.startDirectMessaging` | `communication.room.create` (type=direct) |
| `actor.stopDirectMessaging` | `communication.room.delete` |
| `actor.rooms` | (removed) |
| `actor.rooms.direct` | (removed) |
| `admin.replicateRoomMembership` | (removed) |
