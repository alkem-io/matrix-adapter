# Data Model: RMQ Events Extension

**Feature**: 007-rmq-events-extension  
**Date**: 2025-12-07  
**Status**: ✅ Implemented in `pkg/dto/`

## New DTOs

### Outgoing Events (Adapter → Server)

#### ReactionAddedEvent

Published when a user adds a reaction to a message.

```go
// ReactionAddedEvent is published when a user adds a reaction to a message.
// Topic: communication.reaction.added
type ReactionAddedEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    ReactionID    ReactionID     `json:"reaction_id"`
    Emoji         string         `json:"emoji"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     int64          `json:"timestamp"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| alkemio_room_id | UUID | Alkemio room identifier |
| message_id | string | Matrix event ID of the target message |
| reaction_id | string | Matrix event ID of the reaction |
| emoji | string | The reaction emoji |
| sender_actor_id | UUID | Alkemio actor ID who added the reaction |
| timestamp | int64 | Unix timestamp in milliseconds |

---

#### ReactionRemovedEvent

Published when a user removes a reaction from a message.

```go
// ReactionRemovedEvent is published when a user removes a reaction from a message.
// Topic: communication.reaction.removed
type ReactionRemovedEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    ReactionID    ReactionID     `json:"reaction_id"`
    Emoji         string         `json:"emoji"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     int64          `json:"timestamp"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| alkemio_room_id | UUID | Alkemio room identifier |
| message_id | string | Matrix event ID of the target message |
| reaction_id | string | Matrix event ID of the removed reaction |
| emoji | string | The reaction emoji/key that was removed (empty string if unavailable from redaction lookup) |
| sender_actor_id | UUID | Alkemio actor ID who removed the reaction |
| timestamp | int64 | Unix timestamp in milliseconds |

---

#### RoomMemberLeftEvent

Published when a user leaves or is removed from a room.

```go
// RoomMemberLeftEvent is published when a user leaves or is kicked from a room.
// Topic: communication.room.member.left
type RoomMemberLeftEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    ActorID       AlkemioActorID `json:"actor_id"`
    Reason        string         `json:"reason,omitempty"`
    Timestamp     int64          `json:"timestamp"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| alkemio_room_id | UUID | Alkemio room identifier |
| actor_id | UUID | Alkemio actor ID who left/was removed |
| reason | string | Optional reason (for kicks/bans) |
| timestamp | int64 | Unix timestamp in milliseconds |

---

### Commands (Server → Adapter)

#### GetRoomMembersRequest / GetRoomMembersResponse

Query the current members of a room.

```go
// GetRoomMembersRequest retrieves the list of members in a room.
// Topic: communication.room.members.get
type GetRoomMembersRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}

// GetRoomMembersResponse returns the list of joined member actor IDs.
type GetRoomMembersResponse struct {
    BaseResponse   `tstype:",extends"`
    AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
    MemberActorIDs []AlkemioActorID `json:"member_actor_ids"`
}
```

| Request Field | Type | Description |
|---------------|------|-------------|
| alkemio_room_id | UUID | Alkemio room identifier |

| Response Field | Type | Description |
|----------------|------|-------------|
| alkemio_room_id | UUID | Echo of request room ID |
| member_actor_ids | UUID[] | List of joined member Alkemio actor IDs |

---

#### GetThreadMessagesRequest / GetThreadMessagesResponse

Retrieve all messages in a thread.

```go
// GetThreadMessagesRequest retrieves messages in a thread.
// Topic: communication.thread.messages.get
type GetThreadMessagesRequest struct {
    AlkemioRoomID   AlkemioRoomID `json:"alkemio_room_id"`
    ThreadRootID    MessageID     `json:"thread_root_id"`
}

// GetThreadMessagesResponse returns thread messages.
type GetThreadMessagesResponse struct {
    BaseResponse  `tstype:",extends"`
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    ThreadRootID  MessageID     `json:"thread_root_id"`
    Messages      []MessageDto  `json:"messages"`
}
```

| Request Field | Type | Description |
|---------------|------|-------------|
| alkemio_room_id | UUID | Alkemio room identifier |
| thread_root_id | string | Matrix event ID of the thread root message |

| Response Field | Type | Description |
|----------------|------|-------------|
| alkemio_room_id | UUID | Echo of request room ID |
| thread_root_id | string | Echo of thread root ID |
| messages | MessageDto[] | All messages in the thread (root message first, then replies in chronological order) |

---

## Topic Constants

Add to `pkg/dto/commands.go`:

```go
const (
    // Existing topics...
    
    // Outgoing Events (Adapter → Server)
    TopicReactionAdded     = "communication.reaction.added"
    TopicReactionRemoved   = "communication.reaction.removed"
    TopicRoomMemberLeft    = "communication.room.member.left"
    
    // Commands (Server → Adapter)
    TopicRoomMembersGet    = "communication.room.members.get"
    TopicThreadMessagesGet = "communication.thread.messages.get"
)
```

---

## Entity Relationships

```
Room
├── Members (via GetRoomMembers)
│   └── Actor (UUID)
├── Messages
│   ├── Reactions
│   │   ├── ReactionAddedEvent (published on add)
│   │   └── ReactionRemovedEvent (published on remove)
│   └── Thread
│       └── Messages (via GetThreadMessages)
└── MembershipChanges
    └── RoomMemberLeftEvent (published on leave/kick/ban)
```

---

## Validation Rules

### GetRoomMembersRequest
- `alkemio_room_id`: Required, valid UUID

### GetThreadMessagesRequest
- `alkemio_room_id`: Required, valid UUID
- `thread_root_id`: Required, non-empty string (Matrix event ID format)

### Outgoing Events
- All `AlkemioRoomID` fields: Derived from Matrix room alias
- All `AlkemioActorID` fields: Parsed from Matrix user ID localpart
- All `Timestamp` fields: Unix milliseconds from Matrix event

---

## Error Codes

| Code | Scenario |
|------|----------|
| `ROOM_NOT_FOUND` | Room doesn't exist or bot not joined |
| `MESSAGE_NOT_FOUND` | Thread root message doesn't exist |
| `INVALID_ROOM_ID` | Malformed Alkemio room ID |
| `INVALID_MESSAGE_ID` | Malformed Matrix event ID |
