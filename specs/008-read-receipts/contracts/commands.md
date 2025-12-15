# RMQ Commands: Read Receipts

**Feature**: 008-read-receipts
**Date**: 2025-12-12

## MarkMessageRead

Marks a message as read for a specific user.

**Topic**: `matrix.room.message.read` (Command)
**Response Topic**: `matrix.room.message.read.response`

### Payload

```json
{
  "actor_id": "uuid-string",
  "alkemio_room_id": "uuid-string",
  "message_id": "matrix-event-id",
  "thread_root_id": "matrix-event-id" // Optional
}
```

### Response (Success)

```json
{
  "success": true
}
```

### Response (Error)

| Error Code | Description |
|------------|-------------|
| `invalid_param` | Missing required fields |
| `room_not_found` | Room UUID not found |
| `message_not_found` | Message ID does not exist in room |
| `matrix_error` | Upstream Matrix error |

---

## GetUnreadCounts

Retrieves unread message counts for a user in a room.

**Topic**: `matrix.room.unread_counts.get` (Command)
**Response Topic**: `matrix.room.unread_counts.get.response`

### Payload

```json
{
  "actor_id": "uuid-string",
  "alkemio_room_id": "uuid-string",
  "thread_root_ids": ["thread-id-1", "thread-id-2"] // Optional
}
```

### Response (Success)

```json
{
  "success": true,
  "data": {
    "room_unread_count": 5,
    "thread_unread_counts": {
      "thread-id-1": 2,
      "thread-id-2": 0
    }
  }
}
```

### Response (Error)

| Error Code | Description |
|------------|-------------|
| `invalid_param` | Missing required fields |
| `room_not_found` | Room UUID not found |
| `matrix_error` | Upstream Matrix error |
