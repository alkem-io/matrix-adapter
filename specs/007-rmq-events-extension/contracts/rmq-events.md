# RMQ Contract: Events Extension

**Feature**: 007-rmq-events-extension  
**Protocol Version**: V3.1 (extension of V3)  
**Status**: ✅ Implemented

## Overview

This contract defines 5 new RMQ topics extending the Matrix Adapter protocol:
- 3 outgoing events (Adapter → Server)
- 2 incoming commands (Server → Adapter)

All topics follow the existing `communication.*` namespace pattern.

---

## Outgoing Events (Adapter → Server)

### communication.reaction.added

**Direction**: Adapter → Server  
**Trigger**: Matrix `m.reaction` event received

#### Payload

```json
{
  "alkemio_room_id": "uuid",
  "message_id": "$eventId",
  "reaction_id": "$reactionEventId",
  "emoji": "👍",
  "sender_actor_id": "uuid",
  "timestamp": 1702000000000
}
```

#### Sequencing
- Published immediately upon receiving `event.EventReaction` in listener
- No acknowledgment expected

---

### communication.reaction.removed

**Direction**: Adapter → Server  
**Trigger**: Matrix `m.room.redaction` event targeting a reaction

#### Payload

```json
{
  "alkemio_room_id": "uuid",
  "message_id": "$targetEventId",
  "reaction_id": "$redactedReactionId",
  "emoji": "👍",
  "sender_actor_id": "uuid",
  "timestamp": 1702000000000
}
```

#### Sequencing
- Published upon receiving `event.EventRedaction` where target is a reaction
- Requires lookup of original reaction to get `message_id` and `emoji`

---

### communication.room.member.left

**Direction**: Adapter → Server  
**Trigger**: Matrix `m.room.member` state event with membership "leave" or "ban"

#### Payload

```json
{
  "alkemio_room_id": "uuid",
  "actor_id": "uuid",
  "reason": "optional reason",
  "timestamp": 1702000000000
}
```

#### Sequencing
- Published upon receiving `event.StateMember` with leave/ban membership
- Covers both voluntary leaves and kicks/bans

---

## Incoming Commands (Server → Adapter)

### communication.room.members.get

**Direction**: Server → Adapter  
**Response**: Via reply queue

#### Request

```json
{
  "alkemio_room_id": "uuid"
}
```

#### Response (Success)

```json
{
  "success": true,
  "alkemio_room_id": "uuid",
  "member_actor_ids": ["uuid1", "uuid2", "uuid3"]
}
```

#### Response (Error)

```json
{
  "success": false,
  "error": {
    "code": "ROOM_NOT_FOUND",
    "message": "Room not found or bot not joined"
  }
}
```

---

### communication.thread.messages.get

**Direction**: Server → Adapter  
**Response**: Via reply queue

#### Request

```json
{
  "alkemio_room_id": "uuid",
  "thread_id": "$eventId"
}
```

#### Response (Success)

```json
{
  "success": true,
  "alkemio_room_id": "uuid",
  "thread_id": "$eventId",
  "messages": [
    {
      "id": "$reply1",
      "body": "First reply",
      "sender_actor_id": "uuid",
      "timestamp": 1702000000000
    },
    {
      "id": "$reply2",
      "body": "Second reply",
      "sender_actor_id": "uuid",
      "timestamp": 1702000001000
    }
  ]
}
```

#### Response (Error)

```json
{
  "success": false,
  "error": {
    "code": "MESSAGE_NOT_FOUND",
    "message": "Thread root message not found"
  }
}
```

---

## Error Codes

| Code | HTTP Equivalent | Description |
|------|-----------------|-------------|
| `ROOM_NOT_FOUND` | 404 | Room doesn't exist or adapter not joined |
| `MESSAGE_NOT_FOUND` | 404 | Referenced message doesn't exist |
| `INVALID_ROOM_ID` | 400 | Malformed Alkemio room ID |
| `INVALID_MESSAGE_ID` | 400 | Malformed Matrix event ID |

---

## Topic Summary

| Topic | Direction | Type |
|-------|-----------|------|
| `communication.reaction.added` | Adapter → Server | Event |
| `communication.reaction.removed` | Adapter → Server | Event |
| `communication.room.member.left` | Adapter → Server | Event |
| `communication.room.members.get` | Server → Adapter | Command |
| `communication.thread.messages.get` | Server → Adapter | Command |

---

## Backward Compatibility

- All new topics are additive; no changes to existing V3 topics
- Server can subscribe to new topics incrementally
- No breaking changes to existing payloads
