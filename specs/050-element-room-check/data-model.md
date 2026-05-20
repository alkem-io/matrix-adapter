# Data Model: Element-Initiated Conversation Creation (Synchronous Check)

**Feature**: 050-element-room-check | **Date**: 2026-05-20

## Entities

### CheckRoomRequest (Adapter → Server via RabbitMQ)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `creator_actor_id` | string (UUID) | yes | Alkemio actor UUID of the room creator |
| `member_actor_ids` | []string (UUID) | yes | Alkemio actor UUIDs of invited members (extracted from Matrix user IDs) |
| `is_direct` | bool | yes | True for DM (exactly 1 member), false for group |

### CheckRoomResponse (Server → Adapter via RabbitMQ)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `allow` | bool | yes | Whether room creation is permitted |
| `alkemio_room_id` | string (UUID) | if allow=true | UUID assigned by server for the new Conversation entity |
| `reason` | string | if allow=false | Human-readable rejection reason (e.g., "messaging disabled", "duplicate DM") |

### CheckRoomHTTPRequest (Synapse Module → Adapter HTTP)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `creator` | string | yes | Matrix user ID of room creator (format: `@{uuid}:{domain}`) |
| `members` | []string | yes | Matrix user IDs of invited members |
| `is_direct` | bool | yes | True for DM, false for group |

### CheckRoomHTTPResponse (Adapter → Synapse Module HTTP)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `allow` | bool | yes | Whether room creation is permitted |
| `alkemio_room_id` | string (UUID) | if allow=true | UUID for the Conversation entity |
| `reason` | string | if allow=false | Human-readable rejection reason |

### GetRoomInfoRequest (Adapter → Server via RabbitMQ)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `alkemio_room_id` | string (UUID) | yes | The server-side room UUID |

### GetRoomInfoResponse (Server → Adapter via RabbitMQ)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `alkemio_room_id` | string (UUID) | yes | Echo of requested room UUID |
| `type` | string | yes | Server-side room type: `conversation_direct`, `conversation_group` (extensible) |
| `is_direct` | bool | yes | Convenience flag: true when type is `conversation_direct` |
| `members` | []RoomInfoMember | yes | Members assigned to this room on the server side |

### RoomInfoMember

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `actor_id` | string (UUID) | yes | Alkemio actor UUID |
| `display_name` | string | yes | Display name for the actor |

### AlkemioPendingContent (io.alkemio.pending state event)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `alkemio_room_id` | string (UUID) | yes | UUID assigned during the check, used for reconciliation |

## State Transitions

### Room Lifecycle (Element-Initiated)

```
[Element: createRoom request]
    │
    ▼
[Synapse Module: on_create_room]
    │
    ├─ Block (403) ← server rejects (consent/dedup)
    ├─ Block (503) ← adapter/server unavailable
    │
    ▼ Allow
[Synapse: Room Created]
    state: io.alkemio.pending = {alkemio_room_id: uuid}
    state: io.alkemio.visibility = {visible: true}
    members: [creator only]
    power_levels: {users_default: 50, creator: 100}
    │
    ▼
[Adapter: ANY event from room (m.room.create, message, reaction, etc.)]
    resolveOrReconcile → resolveAlkemioRoomID → uuid.Nil (no alias)
    detect io.alkemio.pending → trigger reconciliation (sync.Map dedup)
    │
    ├─ Reconciliation fails (server unreachable, timeout)
    │   io.alkemio.pending remains → retry on next event
    │
    ▼ Reconciliation succeeds
[Reconciliation]
    1. Bot admin-join
    2. Get room info from server (RMQ request-reply)
    3. EnsureJoined for each member
    4. Set m.direct account data (if DM)
    5. Set power levels (bot=100, creator→50, users_default=50)
    6. Set alias via SetRoomAlias
    7. Bot leaves
    │
    ▼
[Reconciled Room]
    state: has alias (Alkemio UUID-based)
    state: io.alkemio.pending still present (harmless)
    members: [creator + all server-assigned members]
    power_levels: {users_default: 50}
    │
    ▼
[Adapter: emit RoomCreatedEvent]
    → Server: fires GraphQL subscription
    → Alkemio UI: real-time update
```

## Relationships

- **CheckRoomRequest** → produces → **CheckRoomResponse** (RabbitMQ request-reply)
- **CheckRoomResponse.alkemio_room_id** → injected as → **AlkemioPendingContent.alkemio_room_id** (via Synapse module)
- **AlkemioPendingContent.alkemio_room_id** → used in → **GetRoomInfoRequest** (during reconciliation)
- **GetRoomInfoResponse.members** → joined via → **EnsureJoined** (during reconciliation)
- **Reconciliation completion** → emits → **RoomCreatedEvent** (existing DTO, `communication.room.created`)
