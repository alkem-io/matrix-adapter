# Contract: Get Room Info RabbitMQ Command

**Direction**: Adapter → Server (request-reply)
**Topic**: `communication.room.info`
**Pattern**: AMQP RPC (temporary exclusive reply queue, correlation_id)

## Request (Adapter publishes)

```json
{
  "alkemio_room_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `alkemio_room_id` | string (UUID) | UUID of the room entity on the server side |

## Response (Server replies)

```json
{
  "alkemio_room_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "type": "conversation_direct",
  "is_direct": true,
  "members": [
    {
      "actor_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
      "display_name": "Jane Doe"
    },
    {
      "actor_id": "yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy",
      "display_name": "John Smith"
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `alkemio_room_id` | string (UUID) | Echo of the requested room UUID |
| `type` | string | Server-side room type: `conversation_direct`, `conversation_group` (extensible for future room types) |
| `is_direct` | bool | Convenience flag: true when type is `conversation_direct` |
| `members` | []object | All members assigned to this room on the server side |
| `members[].actor_id` | string (UUID) | Alkemio actor UUID |
| `members[].display_name` | string | Actor display name (used by adapter for EnsureUser) |

Future fields (not implemented now): `avatar_url`, `name`, `topic`, etc.

## Usage

Called during reconciliation (after `io.alkemio.pending` is detected on `m.room.create`). The adapter uses this response to:
1. `EnsureUser` for each member (register ghost user if needed)
2. `EnsureJoined` for each member (direct join, no invite events)
3. Set `m.direct` account data if `is_direct: true`
4. Use `type` for any type-specific reconciliation logic

## Timeout

Adapter waits up to 3 seconds. On timeout, reconciliation is aborted but `io.alkemio.pending` remains, allowing retry on the next event in that room.
