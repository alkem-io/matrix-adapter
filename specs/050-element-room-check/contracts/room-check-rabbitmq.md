# Contract: Room Check RabbitMQ Command

**Direction**: Adapter → Server (request-reply)
**Topic**: `communication.room.check`
**Pattern**: AMQP RPC (temporary exclusive reply queue, correlation_id)

## Request (Adapter publishes)

```json
{
  "creator_actor_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "member_actor_ids": ["xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"],
  "is_direct": true
}
```

| Field | Type | Description |
|-------|------|-------------|
| `creator_actor_id` | string (UUID) | Alkemio actor UUID of the room creator |
| `member_actor_ids` | []string (UUID) | Alkemio actor UUIDs of invited members |
| `is_direct` | bool | true for DM, false for group |

## Response (Server replies)

### Allowed

```json
{
  "allow": true,
  "alkemio_room_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

### Rejected

```json
{
  "allow": false,
  "reason": "duplicate DM already exists between these users"
}
```

## Server Responsibilities

On receiving a check request, the server:
1. Validates all actor IDs exist
2. Checks messaging consent for each member
3. For DMs (`is_direct: true`): checks for existing DM between creator and member
4. If all checks pass: creates Conversation entity, assigns UUID
5. Returns allow/deny with UUID or reason

## Timeout

Adapter waits up to 3 seconds for the server reply. If no reply arrives, the HTTP endpoint returns 504 to the Synapse module.
