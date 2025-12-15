# Data Model: Read Receipts & Message Events

**Feature**: 008-read-receipts
**Date**: 2025-12-12

## Domain Entities

### ReadReceipt

Represents a user's read status in a room or thread.

| Field | Type | Description |
|-------|------|-------------|
| `RoomID` | `string` | Matrix Room ID |
| `UserID` | `string` | Matrix User ID (reader) |
| `EventID` | `string` | ID of the last read message |
| `ThreadID` | `*string` | Thread root ID (optional, nil for main timeline) |
| `Timestamp` | `int64` | Unix timestamp (ms) of the receipt |

### UnreadCount

Represents the number of unread messages for a user.

| Field | Type | Description |
|-------|------|-------------|
| `RoomID` | `string` | Matrix Room ID |
| `ThreadID` | `*string` | Thread root ID (optional) |
| `Count` | `int` | Number of unread messages |
| `Highlighted` | `bool` | Whether there are mentions/highlights |

## DTOs (Data Transfer Objects)

These definitions map to `pkg/dto` and will generate TypeScript types.

### Commands

#### MarkMessageReadRequest

Command to mark a message as read.

```go
type MarkMessageReadRequest struct {
    ActorID       uuid.UUID `json:"actor_id"`
    AlkemioRoomID uuid.UUID `json:"alkemio_room_id"`
    MessageID     string    `json:"message_id"`
    ThreadRootID  *string   `json:"thread_root_id,omitempty"` // Optional: if marking a thread message
}
```

#### GetUnreadCountsRequest

Command to get unread counts for a user in a room.

```go
type GetUnreadCountsRequest struct {
    ActorID       uuid.UUID `json:"actor_id"`
    AlkemioRoomID uuid.UUID `json:"alkemio_room_id"`
    ThreadRootIDs []string  `json:"thread_root_ids,omitempty"` // Optional: specific threads to query
}
```

#### UnreadCountsResponse

Response for unread counts.

```go
type UnreadCountsResponse struct {
    RoomUnreadCount   int                  `json:"room_unread_count"`
    ThreadUnreadCounts map[string]int      `json:"thread_unread_counts,omitempty"` // Map[ThreadID]Count
}
```

### Events

#### ReadReceiptEvent

Emitted when a read receipt is received from Matrix.

```go
type ReadReceiptEvent struct {
    RoomID    string    `json:"room_id"`
    UserID    string    `json:"user_id"`
    EventID   string    `json:"event_id"`
    ThreadID  *string   `json:"thread_id,omitempty"`
    Timestamp int64     `json:"timestamp"`
}
```

#### MessageEditedEvent

Emitted when a message is edited (`m.replace`).

```go
type MessageEditedEvent struct {
    OriginalEventID string    `json:"original_event_id"`
    NewEventID      string    `json:"new_event_id"`
    RoomID          string    `json:"room_id"`
    SenderID        string    `json:"sender_id"`
    NewContent      string    `json:"new_content"`
    Timestamp       int64     `json:"timestamp"`
}
```

#### MessageRedactedEvent

Emitted when a message is redacted.

```go
type MessageRedactedEvent struct {
    RedactedEventID string    `json:"redacted_event_id"`
    RedactionEventID string   `json:"redaction_event_id"`
    RoomID          string    `json:"room_id"`
    RedactorID      string    `json:"redactor_id"`
    Reason          string    `json:"reason,omitempty"`
    Timestamp       int64     `json:"timestamp"`
}
```

#### RoomCreatedEvent

Emitted when a room is created (via `m.room.create`).

```go
type RoomCreatedEvent struct {
    RoomID    string    `json:"room_id"`
    CreatorID string    `json:"creator_id"`
    Timestamp int64     `json:"timestamp"`
}
```

#### RoomMemberUpdatedEvent

Emitted when a user's membership status changes (`m.room.member`).

```go
type RoomMemberUpdatedEvent struct {
    RoomID     string `json:"room_id"`
    MemberID   string `json:"member_id"`   // Actor whose membership changed
    Membership string `json:"membership"`  // join, leave, invite, ban, knock
    SenderID   string `json:"sender_id"`   // Actor who performed the action
    Timestamp  int64  `json:"timestamp"`
}
```

## State Transitions

### Read Receipt State

1. **Initial**: No receipt exists.
2. **Update**: User sends `m.read` or `m.read.thread` receipt for Event A.
   - Homeserver updates user's read marker to Event A.
   - Adapter receives `m.receipt` event -> Emits `ReadReceiptEvent`.
3. **Advance**: User sends receipt for Event B (where B > A).
   - Marker moves to B.
   - Adapter emits new `ReadReceiptEvent`.
4. **Regression**: User sends receipt for Event A (where A < B).
   - Matrix typically ignores this (receipts only move forward).
   - Adapter does nothing.

### Message Lifecycle (Edit/Redact)

1. **Created**: `m.room.message` -> `MessageEvent` emitted.
2. **Edited**: `m.room.message` with `m.replace` -> `MessageEditedEvent` emitted.
   - Original message content is effectively replaced in UI.
   - Read receipts for original message remain valid.
3. **Redacted**: `m.room.redaction` -> `MessageRedactedEvent` emitted.
   - Content removed.
   - Read receipts pointing to this event ID remain valid (user "read" the redacted message).
