# Quickstart: RMQ Protocol v2

**Spec**: [spec.md](spec.md)  
**Contracts**: [contracts/rmq-commands.md](contracts/rmq-commands.md)  
**Status**: ✅ Protocol v2 Implemented

## Overview

This guide helps developers integrate with the new Matrix Adapter RMQ protocol. The protocol uses Alkemio UUIDs for all identifiers and provides structured error handling.

---

## Key Changes from v1

| Aspect | v1 (Legacy) | v2 (New) |
|--------|-------------|----------|
| Topic prefix | None | `communication.` |
| Room IDs | Matrix room IDs returned | Alkemio UUIDs only |
| Error handling | Inconsistent | Structured `ErrorResponse` |
| Batch operations | `FailedRooms[]` array | `Results` map with per-room details |
| Room creation | Adapter generates ID | Server provides ID |

---

## Quick Reference

### Creating a Room

```typescript
// TypeScript client example
const request: CreateRoomRequest = {
  alkemio_room_id: "550e8400-e29b-41d4-a716-446655440000",
  type: "community",
  name: "Project Discussion",
  topic: "Discuss project updates",
  initial_members: [
    "660e8400-e29b-41d4-a716-446655440001"
  ]
};

// Publish to: communication.room.create
// Response: { success: true }
```

### Sending a Message

```typescript
const request: SendMessageRequest = {
  alkemio_room_id: "550e8400-e29b-41d4-a716-446655440000",
  sender_actor_id: "660e8400-e29b-41d4-a716-446655440001",
  content: "Hello team!"
};

// Publish to: communication.message.send
// Response: { success: true, message_id: "$event123", timestamp: "..." }
```

### Handling Errors

```typescript
interface BaseResponse {
  success: boolean;
  error?: {
    code: string;    // INVALID_PARAM, ROOM_NOT_FOUND, etc.
    message: string;
    details?: string;
  };
}

// Always check success before using response data
if (!response.success) {
  switch (response.error.code) {
    case "ROOM_NOT_FOUND":
      // Handle missing room
      break;
    case "MATRIX_ERROR":
      // Transient error - consider retry
      break;
    default:
      // Log and handle
  }
}
```

---

## Topic Reference

| Command | Topic |
|---------|-------|
| Create room | `communication.room.create` |
| Get room | `communication.room.get` |
| Update room | `communication.room.update` |
| Delete room | `communication.room.delete` |
| List rooms | `communication.room.list` |
| Send message | `communication.message.send` |
| Get message | `communication.message.get` |
| Delete message | `communication.message.delete` |
| Add reaction | `communication.reaction.add` |
| Remove reaction | `communication.reaction.remove` |
| Get reaction | `communication.reaction.get` |
| Batch add member | `communication.room.member.batch.add` |
| Batch remove member | `communication.room.member.batch.remove` |
| Sync actor | `communication.actor.sync` |

---

## Error Codes

| Code | Meaning | Retry? |
|------|---------|--------|
| `INVALID_PARAM` | Bad request data | No |
| `ROOM_NOT_FOUND` | Room doesn't exist | No |
| `ACTOR_NOT_FOUND` | Actor doesn't exist | No |
| `MATRIX_ERROR` | Homeserver issue | Yes |
| `INTERNAL_ERROR` | Adapter error | Yes |
| `NOT_ALLOWED` | Permission denied | No |

---

## TypeScript Types

Install the generated library:

```bash
npm install @alkem-io/matrix-adapter-lib
```

Import types:

```typescript
import {
  CreateRoomRequest,
  CreateRoomResponse,
  SendMessageRequest,
  SendMessageResponse,
  ErrorCode,
  // ... etc
} from '@alkem-io/matrix-adapter-lib';
```

---

## Testing Locally

1. Start the adapter:
   ```bash
   make run
   ```

2. Publish a test message to RabbitMQ:
   ```bash
   rabbitmqadmin publish routing_key=communication.room.create \
     payload='{"alkemio_room_id":"test-uuid","type":"community","name":"Test"}'
   ```

3. Check adapter logs for response.

---

## Migration Checklist (for Alkemio Server Team)

- [x] Update topic names to include `communication.` prefix
- [x] Replace Matrix room ID handling with Alkemio UUIDs
- [x] Update error handling to use new error codes
- [x] Update batch operation response parsing
- [x] Remove calls to deprecated endpoints (`actor.rooms`, `actor.startDirectMessaging`)
- [x] Regenerate TypeScript client from new lib
