# RMQ Events: Message & Room Updates

**Feature**: 008-read-receipts
**Date**: 2025-12-12

## Read Receipt Updated

Emitted when a user's read receipt is updated in Matrix.

**Topic**: `matrix.room.receipt.updated`

### Payload

```json
{
  "room_id": "matrix-room-id",
  "user_id": "matrix-user-id",
  "event_id": "matrix-event-id",
  "thread_id": "matrix-thread-id", // Optional
  "timestamp": 1678900000000
}
```

---

## Message Edited

Emitted when a message is edited (`m.replace`).

**Topic**: `matrix.room.message.edited`

### Payload

```json
{
  "original_event_id": "matrix-event-id-original",
  "new_event_id": "matrix-event-id-edit",
  "room_id": "matrix-room-id",
  "sender_id": "matrix-user-id",
  "new_content": "Edited message content",
  "timestamp": 1678900000000
}
```

---

## Message Redacted

Emitted when a message is redacted.

**Topic**: `matrix.room.message.redacted`

### Payload

```json
{
  "redacted_event_id": "matrix-event-id-target",
  "redaction_event_id": "matrix-event-id-redaction",
  "room_id": "matrix-room-id",
  "redactor_id": "matrix-user-id",
  "reason": "Spam", // Optional
  "timestamp": 1678900000000
}
```

---

## Room Created

Emitted when a room is created.

**Topic**: `matrix.room.created`

### Payload

```json
{
  "room_id": "matrix-room-id",
  "creator_id": "matrix-user-id",
  "timestamp": 1678900000000
}
```

---

## Room Member Updated

Emitted when a user's membership status changes.

**Topic**: `matrix.room.member.updated`

### Payload

```json
{
  "room_id": "matrix-room-id",
  "user_id": "matrix-user-id",
  "membership": "join", // join, leave, invite, ban, knock
  "sender_id": "matrix-user-id", // Who performed the action
  "timestamp": 1678900000000
}
```
