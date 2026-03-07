# Contracts: Room State Events

**Feature**: 010-room-state-events
**Date**: 2026-03-06

## Modified Contracts

### communication.room.get (enhanced response)

**Direction**: Server → Adapter (request) / Adapter → Server (response)
**Change**: Response includes new `avatar_url` field

**Request** (unchanged):
```json
{
  "alkemio_room_id": "uuid-string"
}
```

**Response** (enhanced):
```json
{
  "success": true,
  "alkemio_room_id": "uuid-string",
  "display_name": "Room Name",
  "avatar_url": "mxc://matrix.org/abc123",
  "member_actor_ids": ["uuid-1", "uuid-2"],
  "messages": [...]
}
```

**Notes**: `avatar_url` is omitted when the room has no avatar. This is backward-compatible — existing consumers that don't read `avatar_url` are unaffected.

### communication.room.get.as_user (enhanced response)

**Direction**: Server → Adapter (request) / Adapter → Server (response)
**Change**: Response includes new `avatar_url` field (same as above)

**Response** (enhanced):
```json
{
  "success": true,
  "alkemio_room_id": "uuid-string",
  "display_name": "Room Name",
  "avatar_url": "mxc://matrix.org/abc123",
  "member_actor_ids": ["uuid-1", "uuid-2"],
  "messages": [...],
  "last_read_event_id": "$event-id",
  "unread_count": 3
}
```

## New Contracts

### communication.room.updated (new outbound event)

**Direction**: Adapter → Server (event published to RabbitMQ)
**Trigger**: Matrix state events `m.room.name`, `m.room.avatar`, `m.room.topic` on managed rooms

**Payload**:
```json
{
  "alkemio_room_id": "uuid-string",
  "display_name": "New Room Name",
  "avatar_url": "mxc://matrix.org/newavatar",
  "topic": "New topic description",
  "timestamp": 1709769600000
}
```

**Field behavior**:
- `alkemio_room_id`: Always present (required)
- `timestamp`: Always present (required, Unix milliseconds)
- `display_name`: Present only when room name changed; omitted otherwise
- `avatar_url`: Present only when avatar changed; omitted otherwise (empty string = avatar removed)
- `topic`: Present only when topic changed; omitted otherwise

**One event per state change**: Each Matrix state event produces one `RoomUpdatedEvent` with only the changed property populated.

**Filtering**: Events from the appservice bot are excluded (self-event loop prevention).

**Error conditions**:
- Room ID unresolvable (no Alkemio mapping): Event skipped, warning logged. No retry — best effort.
- Event content parsing failure: Event skipped, error logged.

## Backward Compatibility

| Contract                        | Breaking? | Notes                                           |
|---------------------------------|-----------|-------------------------------------------------|
| communication.room.get          | No        | New optional field added to response             |
| communication.room.get.as_user  | No        | New optional field added to response             |
| communication.room.updated      | N/A       | New event — consumers must subscribe explicitly  |
