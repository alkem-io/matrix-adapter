# Matrix Adapter Protocol Specification (V3)

> **Status**: ✅ **Current** (v3.4.0)
> **Last Updated**: 2026-03-07

---

## Table of Contents

- [Implementation Reference](#implementation-reference)
- [Design Principles](#design-principles)
- [Data Types (Go)](#data-types-go)
- [Response Envelope](#response-envelope)
  - [Error Handling](#error-handling)
  - [Base Response](#base-response)
- [Commands & Events](#commands--events)
  - **Room Commands**
    - [1. Create Room](#1-create-room) - `communication.room.create`
    - [7. Get Room Details](#7-get-room-details) - `communication.room.get`
    - [32. Get Room As User](#32-get-room-as-user) - `communication.room.get.as_user`
    - [8. Update Room](#8-update-room) - `communication.room.update`
    - [9. Delete Room](#9-delete-room) - `communication.room.delete`
    - [12. List All Rooms (Admin)](#12-list-all-rooms-admin) - `communication.room.list`
  - **Message Commands**
    - [2. Send Message](#2-send-message) - `communication.message.send`
    - [5. Delete Message](#5-delete-message) - `communication.message.delete`
    - [13. Get Message Details](#13-get-message-details) - `communication.message.get`
  - **Reaction Commands**
    - [3. Add Reaction](#3-add-reaction) - `communication.reaction.add`
    - [4. Remove Reaction](#4-remove-reaction) - `communication.reaction.remove`
    - [14. Get Reaction Details](#14-get-reaction-details) - `communication.reaction.get`
  - **Room Membership Commands**
    - [6. Batch Room Membership (Add)](#6-batch-room-membership-add) - `communication.room.member.batch.add`
    - [10. Batch Room Membership (Remove)](#10-batch-room-membership-remove) - `communication.room.member.batch.remove`
    - [28. Get Room Members](#28-get-room-members) - `communication.room.members.get`
  - **Actor Commands**
    - [11. Sync Actor Profile](#11-sync-actor-profile) - `communication.actor.sync`
  - **Space Commands**
    - [15. Create Space](#15-create-space) - `communication.space.create`
    - [16. Update Space](#16-update-space) - `communication.space.update`
    - [17. Delete Space](#17-delete-space) - `communication.space.delete`
    - [21. Get Space Details](#21-get-space-details) - `communication.space.get`
    - [22. List All Spaces (Admin)](#22-list-all-spaces-admin) - `communication.space.list`
  - **Space Membership Commands**
    - [19. Batch Space Membership (Add)](#19-batch-space-membership-add) - `communication.space.member.batch.add`
    - [20. Batch Space Membership (Remove)](#20-batch-space-membership-remove) - `communication.space.member.batch.remove`
  - **Hierarchy Commands**
    - [18. Set Parent (Hierarchy)](#18-set-parent-hierarchy) - `communication.hierarchy.set_parent`
  - **Thread Commands**
    - [29. Get Thread Messages](#29-get-thread-messages) - `communication.thread.messages.get`
  - **Read Receipt Commands**
    - [30. Mark Message Read](#30-mark-message-read) - `communication.message.read`
    - [31. Get Unread Counts](#31-get-unread-counts) - `communication.room.unread_counts.get`
- [Outgoing Events](#outgoing-events)
  - [23. Message Received](#23-message-received-outgoing-event) - `communication.message.received`
  - [24. DM Requested](#24-dm-requested-outgoing-event) - `communication.room.dm.requested`
  - [25. Reaction Added](#25-reaction-added-outgoing-event) - `communication.reaction.added`
  - [26. Reaction Removed](#26-reaction-removed-outgoing-event) - `communication.reaction.removed`
  - [27. Room Member Left](#27-room-member-left-outgoing-event) - `communication.room.member.left`
  - [27a. Read Receipt Updated](#27a-read-receipt-updated-outgoing-event) - `communication.room.receipt.updated`
  - [27b. Message Edited](#27b-message-edited-outgoing-event) - `communication.message.edited`
  - [27c. Message Redacted](#27c-message-redacted-outgoing-event) - `communication.message.redacted`
  - [27d. Room Created](#27d-room-created-outgoing-event) - `communication.room.created`
  - [27e. Room Member Updated](#27e-room-member-updated-outgoing-event) - `communication.room.member.updated`
  - [27f. Room Updated](#27f-room-updated-outgoing-event) - `communication.room.updated`
- [HTTP Endpoints](#http-endpoints)
  - [Health Check](#health-check)
  - [DM Request Webhook](#dm-request-webhook)
- [Error Handling Strategy](#error-handling-strategy)
- [TS Library Export](#ts-library-export)

---

## Implementation Reference

- **Go DTOs**: `pkg/dto/` - Source of truth for all data structures
- **TypeScript Library**: `lib/src/dto/generated.ts` - Auto-generated from Go
- **Event Types**: `lib/src/matrix.adapter.event.type.ts` - Enum of all 37 topics
- **Topic Constants**: `internal/infrastructure/queue/topics.go` - Single source of truth

This document defines the communication protocol between the Alkemio Server and the Matrix Adapter service. The protocol is designed to be transport-agnostic but is currently implemented over RabbitMQ.

## Design Principles

1.  **Alkemio-Native**: All identifiers exchanged are Alkemio UUIDs (v4 or v7). The Adapter is responsible for mapping these to Matrix internal IDs.
2.  **Opaque Transport**: The Server does not know about Matrix Room IDs (`!abc:matrix.org`) or Event IDs (`$abc...`).
3.  **Structured Responses**: Every command returns a structured response indicating success, failure, or partial success.
4.  **Idempotency**: Operations should be idempotent where possible, relying on the deterministic Alkemio UUIDs.

## Data Types (Go)

The following types are used throughout the specification.

```go
import (
    "time"
    "github.com/google/uuid"
)

// AlkemioRoomID is a UUID v4 or v7 identifying a room in Alkemio.
type AlkemioRoomID uuid.UUID

// AlkemioContextID is a UUID v4 or v7 identifying a context (Space) in Alkemio.
// Currently maps to the Authorization Policy ID.
type AlkemioContextID uuid.UUID

// AlkemioActorID is a UUID v4 or v7 identifying an actor (user or bot) in Alkemio.
type AlkemioActorID uuid.UUID

// MessageID is an opaque string identifier for a message (mapped to Matrix Event ID).
type MessageID string

// ReactionID is an opaque string identifier for a reaction.
type ReactionID string
```

## Response Envelope

All responses from the Adapter MUST follow this structure.

### Error Handling

```go
type ErrorCode string

const (
    ErrCodeInvalidParam     ErrorCode = "INVALID_PARAM"
    ErrCodeRoomNotFound     ErrorCode = "ROOM_NOT_FOUND"
    ErrCodeSpaceNotFound    ErrorCode = "SPACE_NOT_FOUND"
    ErrCodeActorNotFound    ErrorCode = "ACTOR_NOT_FOUND"
    ErrCodeMessageNotFound  ErrorCode = "MESSAGE_NOT_FOUND"
    ErrCodeReactionNotFound ErrorCode = "REACTION_NOT_FOUND"
    ErrCodeMatrixError      ErrorCode = "MATRIX_ERROR"
    ErrCodeInternalError    ErrorCode = "INTERNAL_ERROR"
    ErrCodeNotAllowed       ErrorCode = "NOT_ALLOWED"
)

// ErrorResponse contains structured error information.
type ErrorResponse struct {
    Code    ErrorCode `json:"code"`
    Message string    `json:"message"`
    Details string    `json:"details,omitempty"` // Optional technical details
}
```

### Base Response

```go
// BaseResponse is the standard response structure for all command responses.
// Operations that return only status use this directly.
// Operations with additional data embed this struct.
type BaseResponse struct {
    Success bool           `json:"success"`
    Error   *ErrorResponse `json:"error,omitempty"`
}
```

---

## Commands & Events

### 1. Create Room

Creates a new communication channel. The Server provides the ID.

*   **Event Subject**: `communication.room.create`

#### Request Payload

```go
type RoomType string

const (
    RoomTypeCommunity RoomType = "community"
    RoomTypeDirect    RoomType = "direct"
)

type CreateRoomRequest struct {
    AlkemioRoomID   AlkemioRoomID     `json:"alkemio_room_id"`
    Type            RoomType          `json:"type"`
    Name            string            `json:"name,omitempty"` // Ignored for 'direct'
    InitialMembers  []AlkemioActorID  `json:"initial_members,omitempty"`
    Topic           string            `json:"topic,omitempty"`
    AvatarURL       string            `json:"avatar_url,omitempty"`
    ParentContextID *AlkemioContextID `json:"parent_context_id,omitempty"` // Optional parent Space
    JoinRule        JoinRule          `json:"join_rule,omitempty"`         // Defaults to 'invite'
}

type JoinRule string

const (
    JoinRulePublic     JoinRule = "public"
    JoinRuleInvite     JoinRule = "invite"
    JoinRuleRestricted JoinRule = "restricted" // Requires ParentContextID
)
```

#### Response Payload

Operations that only confirm success/failure return `BaseResponse` directly:

```go
// Returns BaseResponse - no additional data needed on success,
// as Server already knows the ID.
```

**Adapter Actions**:
1.  Check if mapping for `AlkemioRoomID` already exists. If so, return Success (Idempotency).
2.  If `Type == direct`:
    *   Ensure exactly 2 `InitialMembers`.
    *   Check if a Matrix DM exists for these users. If yes, map it.
    *   If no, create a new Matrix DM.
3.  If `Type == community`:
    *   Create a new Matrix Room.
    *   Invite/Join `InitialMembers`.
4.  Store `AlkemioRoomID <-> MatrixRoomID` mapping.

---

### 2. Send Message

Sends a text message to a room.

*   **Event Subject**: `communication.message.send`

#### Request Payload

```go
type SendMessageRequest struct {
    AlkemioRoomID   AlkemioRoomID  `json:"alkemio_room_id"`
    SenderActorID   AlkemioActorID `json:"sender_actor_id"`
    Content         string         `json:"content"` // Markdown supported
    ParentMessageID *MessageID     `json:"parent_message_id,omitempty"` // For threads/replies
}
```

#### Response Payload

```go
type SendMessageResponse struct {
    BaseResponse
    MessageID MessageID `json:"message_id"`
    Timestamp time.Time `json:"timestamp"` // UTC time recorded by Matrix
}
```

**Adapter Actions**:
1.  Resolve `SenderActorID` to Matrix User.
2.  Resolve `AlkemioRoomID` to Matrix Room ID.
3.  Send event to Matrix.
4.  Return the generated Matrix Event ID as `MessageID`.

---

### 3. Add Reaction

Adds an emoji reaction to a message.

*   **Event Subject**: `communication.reaction.add`

#### Request Payload

```go
type AddReactionRequest struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Emoji         string         `json:"emoji"`
}
```

#### Response Payload

```go
type AddReactionResponse struct {
    BaseResponse
    ReactionID ReactionID `json:"reaction_id"`
}
```

---

### 4. Remove Reaction

Removes a previously added reaction.

*   **Event Subject**: `communication.reaction.remove`

#### Request Payload

```go
type RemoveReactionRequest struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    ReactionID    ReactionID     `json:"reaction_id"` // ID returned from AddReaction
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

---

### 5. Delete Message

Redacts/deletes a message.

*   **Event Subject**: `communication.message.delete`

#### Request Payload

```go
type DeleteMessageRequest struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Reason        string         `json:"reason,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

---

### 6. Batch Room Membership (Add)

Adds a single actor to multiple rooms (e.g., joining a Space and all its sub-rooms). This operation supports **partial success**.

*   **Event Subject**: `communication.room.member.batch.add`

#### Request Payload

```go
type BatchAddMemberRequest struct {
    ActorID        AlkemioActorID  `json:"actor_id"`
    AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
}
```

#### Response Payload

```go
type BatchAddMemberResponse struct {
    BaseResponse
    // Results is a map of AlkemioRoomID (string representation) to the operation result.
    // Only populated if BaseResponse.Success is true (meaning the batch was processed).
    // If BaseResponse.Success is false, the entire batch failed (e.g. Actor not found).
    Results map[string]BaseResponse `json:"results,omitempty"`
}
```

**Adapter Actions**:
1.  Validate Actor exists. If not, return top-level Error.
2.  Iterate through `AlkemioRoomIDs`.
3.  For each room:
    *   Resolve Matrix Room ID.
    *   Attempt invite/join.
    *   Record success/failure in `Results`.
4.  Return `Success: true` (even if some rooms failed) with the detailed `Results` map.

---

### 7. Get Room Details

Retrieves current state of a room.

*   **Event Subject**: `communication.room.get`

#### Request Payload

```go
type GetRoomRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}
```

#### Response Payload

```go
type MessageDto struct {
    ID            MessageID      `json:"id"`
    Content       string         `json:"content"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     time.Time      `json:"timestamp"`
    Reactions     []ReactionDto  `json:"reactions"`
    ThreadID      *MessageID     `json:"thread_id,omitempty"`
}

type ReactionDto struct {
    ID            ReactionID     `json:"id"`
    Emoji         string         `json:"emoji"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     time.Time      `json:"timestamp"`
}

type GetRoomResponse struct {
    BaseResponse
    AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
    DisplayName    string           `json:"display_name"`
    AvatarURL      string           `json:"avatar_url,omitempty"`
    MemberActorIDs []AlkemioActorID `json:"member_actor_ids"`
    Messages       []MessageDto     `json:"messages"`
}
```

**Adapter Actions**:
1.  Resolve Matrix Room ID.
2.  Fetch state (members, recent messages).
3.  Map Matrix User IDs back to `AlkemioActorID` (using internal mapping or lookup).
4.  Return sanitized DTOs.

---

### 8. Update Room

Updates room metadata (Name, Topic, Visibility).

*   **Event Subject**: `communication.room.update`

#### Request Payload

```go
type UpdateRoomRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    Name          *string       `json:"name,omitempty"`
    Topic         *string       `json:"topic,omitempty"`
    AvatarURL     *string       `json:"avatar_url,omitempty"`
    IsPublic      *bool         `json:"is_public,omitempty"` // Maps to WorldReadable/GuestAccess
    JoinRule      *JoinRule     `json:"join_rule,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

---

### 9. Delete Room

Archives or deletes a room.

*   **Event Subject**: `communication.room.delete`

#### Request Payload

```go
type DeleteRoomRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    Reason        string        `json:"reason,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

---

### 10. Batch Room Membership (Remove)

Removes a single actor from multiple rooms.

*   **Event Subject**: `communication.room.member.batch.remove`

#### Request Payload

```go
type BatchRemoveMemberRequest struct {
    ActorID        AlkemioActorID  `json:"actor_id"`
    AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
    Reason         string          `json:"reason,omitempty"`
}
```

#### Response Payload

```go
type BatchRemoveMemberResponse struct {
    BaseResponse
    Results map[string]BaseResponse `json:"results,omitempty"`
}
```

---

### 11. Sync Actor Profile

Ensures an actor exists in Matrix and updates their profile (Display Name, Avatar).
Replaces `tryRegisterNewUser`.

*   **Event Subject**: `communication.actor.sync`

#### Request Payload

```go
type SyncActorRequest struct {
    ActorID     AlkemioActorID `json:"actor_id"`
    DisplayName string         `json:"display_name"`
    AvatarURL   string         `json:"avatar_url,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

---

### 12. List All Rooms (Admin)

Retrieves a list of all rooms known to the Adapter. Used for auditing and finding orphaned rooms.

*   **Event Subject**: `communication.room.list`

#### Request Payload

```go
type ListRoomsRequest struct {
    Cursor string `json:"cursor,omitempty"`
}
```

#### Response Payload

```go
type ListRoomsResponse struct {
    BaseResponse
    AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
    NextCursor     string          `json:"next_cursor,omitempty"`
}
```

---

### 13. Get Message Details

Retrieves details of a specific message, including its sender and reactions.

*   **Event Subject**: `communication.message.get`

#### Request Payload

```go
type GetMessageRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    MessageID     MessageID     `json:"message_id"`
}
```

#### Response Payload

```go
type GetMessageResponse struct {
    BaseResponse
    Message MessageDto `json:"message"`
}
```

---

### 14. Get Reaction Details

Retrieves details of a specific reaction, primarily to identify the sender.

*   **Event Subject**: `communication.reaction.get`

#### Request Payload

```go
type GetReactionRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    ReactionID    ReactionID    `json:"reaction_id"`
}
```

#### Response Payload

```go
type GetReactionResponse struct {
    BaseResponse
    Reaction ReactionDto `json:"reaction"`
}

---

### 15. Create Space

Creates a new Matrix Space (Context).

*   **Event Subject**: `communication.space.create`

#### Request Payload

```go
type CreateSpaceRequest struct {
    AlkemioContextID AlkemioContextID  `json:"alkemio_context_id"`
    Name             string            `json:"name"`
    Topic            string            `json:"topic,omitempty"`
    AvatarURL        string            `json:"avatar_url,omitempty"`
    ParentContextID  *AlkemioContextID `json:"parent_context_id,omitempty"`
    InitialMembers   []AlkemioActorID  `json:"initial_members,omitempty"`
    JoinRule         JoinRule          `json:"join_rule,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

**Adapter Actions**:
1.  Check if mapping for `AlkemioContextID` exists.
2.  Create Matrix Room with `type="m.space"`.
3.  If `ParentContextID` is provided, add the new space as a child of the parent space.
4.  Store `AlkemioContextID <-> MatrixRoomID` mapping.

---

### 16. Update Space

Updates space metadata.

*   **Event Subject**: `communication.space.update`

#### Request Payload

```go
type UpdateSpaceRequest struct {
    AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
    Name             *string          `json:"name,omitempty"`
    Topic            *string          `json:"topic,omitempty"`
    AvatarURL        *string          `json:"avatar_url,omitempty"`
    JoinRule         *JoinRule        `json:"join_rule,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

---

### 17. Delete Space

Deletes/Archives a space.

*   **Event Subject**: `communication.space.delete`

#### Request Payload

```go
type DeleteSpaceRequest struct {
    AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
    Reason           string           `json:"reason,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

---

### 18. Set Parent (Hierarchy)

Sets or changes the parent of a Room or Space. This manages the `m.space.child` state events.

*   **Event Subject**: `communication.hierarchy.set_parent`

#### Request Payload

```go
type SetParentRequest struct {
    // ChildID is the AlkemioRoomID (for rooms) or AlkemioContextID (for subspaces) to add as child.
    ChildID         string           `json:"child_id"`
    IsSpace         bool             `json:"is_space"`
    ParentContextID AlkemioContextID `json:"parent_context_id"`
    Order           string           `json:"order,omitempty"`
    Suggested       bool             `json:"suggested,omitempty"`
}
```

#### Response Payload

Returns `BaseResponse` directly.

**Adapter Actions**:
1.  Parse `ChildID` as UUID and resolve to Matrix Room ID (using `IsSpace` to determine alias pattern).
2.  Resolve Parent Space from `ParentContextID`.
3.  Send `m.space.child` event in Parent Space pointing to Child with `Order` and `Suggested`.
4.  Send `m.space.parent` event in Child pointing to Parent (canonical).

---

### 19. Batch Space Membership (Add)

Adds a single actor to multiple spaces.

*   **Event Subject**: `communication.space.member.batch.add`

#### Request Payload

```go
type BatchAddSpaceMemberRequest struct {
    ActorID           AlkemioActorID     `json:"actor_id"`
    AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
}
```

#### Response Payload

```go
type BatchAddSpaceMemberResponse struct {
    BaseResponse
    Results map[string]BaseResponse `json:"results,omitempty"`
}
```

---

### 20. Batch Space Membership (Remove)

Removes a single actor from multiple spaces.

*   **Event Subject**: `communication.space.member.batch.remove`

#### Request Payload

```go
type BatchRemoveSpaceMemberRequest struct {
    ActorID           AlkemioActorID     `json:"actor_id"`
    AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
    Reason            string             `json:"reason,omitempty"`
}
```

#### Response Payload

```go
type BatchRemoveSpaceMemberResponse struct {
    BaseResponse
    Results map[string]BaseResponse `json:"results,omitempty"`
}
```

---

### 21. Get Space Details

Retrieves details of a space, including its children (hierarchy).

*   **Event Subject**: `communication.space.get`

#### Request Payload

```go
type GetSpaceRequest struct {
    AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
}
```

#### Response Payload

```go
type SpaceChildDto struct {
    // ChildID is the AlkemioRoomID (for rooms) or AlkemioContextID (for subspaces).
    ChildID   string `json:"child_id"`
    IsSpace   bool   `json:"is_space"`
    Order     string `json:"order,omitempty"`
    Suggested bool   `json:"suggested,omitempty"`
}

type GetSpaceResponse struct {
    BaseResponse
    AlkemioContextID AlkemioContextID  `json:"alkemio_context_id"`
    DisplayName      string            `json:"display_name"`
    Topic            string            `json:"topic,omitempty"`
    AvatarURL        string            `json:"avatar_url,omitempty"`
    JoinRule         JoinRule          `json:"join_rule"`
    MemberActorIDs   []AlkemioActorID  `json:"member_actor_ids"`
    Children         []SpaceChildDto   `json:"children"`
    ParentContextID  *AlkemioContextID `json:"parent_context_id,omitempty"`
}
```

---

### 22. List All Spaces (Admin)

Retrieves a list of all spaces known to the Adapter.

*   **Event Subject**: `communication.space.list`

#### Request Payload

```go
type ListSpacesRequest struct {
    Cursor string `json:"cursor,omitempty"`
}
```

#### Response Payload

```go
type ListSpacesResponse struct {
    BaseResponse
    AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
    NextCursor        string             `json:"next_cursor,omitempty"`
}
```

---

## Outgoing Events

Events emitted by the Adapter to notify Alkemio Server of external actions.

### 23. Message Received (Outgoing Event)

Published when the Adapter receives a message in a room from a user.

*   **Event Subject**: `communication.message.received`

#### Payload

```go
// Message represents a Matrix message event (used in outgoing events).
type Message struct {
    ID        string     `json:"id"`
    Message   string     `json:"message"`
    ThreadID  *MessageID `json:"threadID,omitempty"`
    Sender    string     `json:"sender"`
    Timestamp int64      `json:"timestamp"`
    Reactions []Reaction `json:"reactions"`
}

// Reaction represents a reaction to a message (used in outgoing events).
type Reaction struct {
    ID        string `json:"id"`
    Emoji     string `json:"emoji"`
    Sender    string `json:"sender"`
    Timestamp int64  `json:"timestamp"`
    MessageID string `json:"messageId"`
}

type MessageReceivedPayload struct {
    RoomID      string  `json:"roomId"`
    RoomName    string  `json:"roomName"`
    Message     Message `json:"message"`
    ActorID     string  `json:"actorID"`
    CommunityID *string `json:"communityId,omitempty"`
}
```

---

### 24. DM Requested (Outgoing Event)

Published when a Synapse spam checker module requests approval for a DM room creation. This event is triggered via the Adapter's webhook endpoint.

*   **Event Subject**: `communication.room.dm.requested`

#### Payload

```go
type DMRequestedEvent struct {
    InitiatorActorID string `json:"initiator_actor_id"` // Alkemio UUID of initiating user
    TargetActorID    string `json:"target_actor_id"`    // Alkemio UUID of target user
}
```

**Source**: HTTP webhook from Synapse module at `POST /_matrix/app/alkemio/dm-request`

**Server Response**: If approved, Server sends `communication.room.create` with `type: "direct"` and both actors in `initial_members`.

---

### 25. Reaction Added (Outgoing Event)

Published when a user adds a reaction to a message.

*   **Event Subject**: `communication.reaction.added`

#### Payload

```go
type ReactionAddedEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    ReactionID    ReactionID     `json:"reaction_id"`
    Emoji         string         `json:"emoji"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     int64          `json:"timestamp"` // Unix milliseconds
}
```

---

### 26. Reaction Removed (Outgoing Event)

Published when a user removes a reaction from a message.

*   **Event Subject**: `communication.reaction.removed`

#### Payload

```go
type ReactionRemovedEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    ReactionID    ReactionID     `json:"reaction_id"`
    Emoji         string         `json:"emoji"` // May be empty if original reaction unavailable
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     int64          `json:"timestamp"` // Unix milliseconds
}
```

---

### 27. Room Member Left (Outgoing Event)

> **Deprecated**: This event is no longer emitted by the adapter. Use `RoomMemberUpdatedEvent` (27e) with `membership=leave` instead, which covers all membership transitions including leave/kick/ban. The DTO and topic constant remain in the codebase for backward compatibility but no events are published to this topic.

*   **Event Subject**: `communication.room.member.left`

#### Payload

```go
type RoomMemberLeftEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    ActorID       AlkemioActorID `json:"actor_id"`
    Reason        string         `json:"reason,omitempty"`
    Timestamp     int64          `json:"timestamp"` // Unix milliseconds
}
```

---

### 27a. Read Receipt Updated (Outgoing Event)

Published when a user's read receipt is updated (message marked as read).

*   **Event Subject**: `communication.room.receipt.updated`

#### Payload

```go
type ReadReceiptUpdatedEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    ActorID       AlkemioActorID `json:"actor_id"`
    EventID       MessageID      `json:"event_id"`             // Event ID that was marked as read
    ThreadID      *MessageID     `json:"thread_id,omitempty"`  // Thread root event ID (if thread-level)
    Timestamp     int64          `json:"timestamp"`            // Unix milliseconds
}
```

---

### 27b. Message Edited (Outgoing Event)

Published when a message is edited (via Matrix `m.replace` relation).

*   **Event Subject**: `communication.message.edited`

#### Payload

```go
type MessageEditedEvent struct {
    AlkemioRoomID     AlkemioRoomID  `json:"alkemio_room_id"`
    SenderActorID     AlkemioActorID `json:"sender_actor_id"`
    OriginalMessageID MessageID      `json:"original_message_id"` // Event ID of original message
    NewMessageID      MessageID      `json:"new_message_id"`      // Event ID of the edit event
    NewContent        string         `json:"new_content"`
    ThreadID          *MessageID     `json:"thread_id,omitempty"` // Thread root event ID (if in thread)
    Timestamp         int64          `json:"timestamp"`           // Unix milliseconds
}
```

---

### 27c. Message Redacted (Outgoing Event)

Published when a message is redacted (deleted).

*   **Event Subject**: `communication.message.redacted`

#### Payload

```go
type MessageRedactedEvent struct {
    AlkemioRoomID      AlkemioRoomID  `json:"alkemio_room_id"`
    RedactorActorID    AlkemioActorID `json:"redactor_actor_id"`
    RedactedMessageID  MessageID      `json:"redacted_message_id"`  // Event ID of the redacted message
    RedactionMessageID MessageID      `json:"redaction_message_id"` // Event ID of the redaction event
    Reason             string         `json:"reason,omitempty"`
    ThreadID           *MessageID     `json:"thread_id,omitempty"`  // Thread root event ID (if in thread)
    Timestamp          int64          `json:"timestamp"`            // Unix milliseconds
}
```

---

### 27d. Room Created (Outgoing Event)

Published when a room is created in Matrix.

*   **Event Subject**: `communication.room.created`

#### Payload

```go
type RoomCreatedEvent struct {
    AlkemioRoomID  AlkemioRoomID  `json:"alkemio_room_id"`
    CreatorActorID AlkemioActorID `json:"creator_actor_id"`
    RoomType       string         `json:"room_type"` // "room", "space"
    Name           string         `json:"name,omitempty"`
    Topic          string         `json:"topic,omitempty"`
    Timestamp      int64          `json:"timestamp"` // Unix milliseconds
}
```

---

### 27e. Room Member Updated (Outgoing Event)

Published when a user's membership status changes (join, invite, leave, ban, knock). This is the single source of truth for all membership transitions — `RoomMemberLeftEvent` (27) is deprecated.

*   **Event Subject**: `communication.room.member.updated`

#### Payload

```go
type RoomMemberUpdatedEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MemberActorID AlkemioActorID `json:"member_actor_id"` // Actor whose membership changed
    SenderActorID AlkemioActorID `json:"sender_actor_id"` // Actor who performed the action
    Membership    string         `json:"membership"`      // join, leave, invite, ban, knock
    Timestamp     int64          `json:"timestamp"`       // Unix milliseconds
}
```

---

### 27f. Room Updated (Outgoing Event)

Published when a room's properties change in Matrix (`m.room.name`, `m.room.avatar`, `m.room.topic` state events). Each state event produces one event with only the changed property populated.

*   **Event Subject**: `communication.room.updated`

#### Payload

```go
type RoomUpdatedEvent struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    DisplayName   *string        `json:"display_name,omitempty"`
    AvatarURL     *string        `json:"avatar_url,omitempty"`
    Topic         *string        `json:"topic,omitempty"`
    Timestamp     int64          `json:"timestamp"` // Unix milliseconds
}
```

**Note**: Only the changed property is populated (non-nil). Omitted fields indicate no change. This event fires for all state changes, including those triggered by the adapter itself (e.g., via `communication.room.update`).

---

### 28. Get Room Members

Retrieves the list of joined members in a room.

*   **Event Subject**: `communication.room.members.get`

#### Request Payload

```go
type GetRoomMembersRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}
```

#### Response Payload

```go
type GetRoomMembersResponse struct {
    BaseResponse
    AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
    MemberActorIDs []AlkemioActorID `json:"member_actor_ids"`
}
```

---

### 29. Get Thread Messages

Retrieves all messages in a thread, including the thread root message.

*   **Event Subject**: `communication.thread.messages.get`

#### Request Payload

```go
type GetThreadMessagesRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    ThreadID      MessageID     `json:"thread_id"`
}
```

#### Response Payload

```go
type GetThreadMessagesResponse struct {
    BaseResponse
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    ThreadID      MessageID     `json:"thread_id"`
    Messages      []MessageDto  `json:"messages"` // Root message first, then replies
}
```

---

### 30. Mark Message Read

Marks a message as read for a user. Supports both room-level and thread-level read receipts.

*   **Event Subject**: `communication.message.read`

#### Request Payload

```go
type MarkMessageReadRequest struct {
    ActorID       AlkemioActorID `json:"actor_id"`
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`            // Event ID to mark as read
    ThreadID      *MessageID     `json:"thread_id,omitempty"`   // Optional: for thread-specific receipts
}
```

#### Response Payload

Returns `BaseResponse` directly.

**Adapter Actions**:
1.  Resolve `ActorID` to Matrix User.
2.  Resolve `AlkemioRoomID` to Matrix Room ID.
3.  Send `m.read` receipt (room-level) or `m.read.thread` receipt (if `ThreadID` provided).
4.  Matrix homeserver handles multiple client synchronization.

---

### 31. Get Unread Counts

Retrieves unread message counts for a user in a room, optionally including thread-level counts.

*   **Event Subject**: `communication.room.unread_counts.get`

#### Request Payload

```go
type GetUnreadCountsRequest struct {
    ActorID       AlkemioActorID `json:"actor_id"`
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    ThreadIDs     []MessageID    `json:"thread_ids,omitempty"` // Optional: specific threads to query
}
```

#### Response Payload

```go
type GetUnreadCountsResponse struct {
    BaseResponse
    RoomUnreadCount    int            `json:"room_unread_count"`
    ThreadUnreadCounts map[string]int `json:"thread_unread_counts,omitempty"` // Map[ThreadID]Count
}
```

**Adapter Actions**:
1.  Resolve `ActorID` to Matrix User.
2.  Resolve `AlkemioRoomID` to Matrix Room ID.
3.  Query room state for unread counts.
4.  If `ThreadIDs` provided, also query thread-level unread counts.

---

### 32. Get Room As User

Retrieves room details from a specific user's perspective, including read state information for each message. This enables efficient UI rendering with read/unread indicators without additional API calls.

*   **Event Subject**: `communication.room.get.as_user`

#### Request Payload

```go
type GetRoomAsUserRequest struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    ActorID       AlkemioActorID `json:"actor_id"`
}
```

#### Response Payload

```go
// MessageWithReadStateDto extends MessageDto with read receipt info.
type MessageWithReadStateDto struct {
    MessageDto
    IsRead bool `json:"is_read"` // Has the requesting user read this message?
}

type GetRoomAsUserResponse struct {
    BaseResponse
    AlkemioRoomID   AlkemioRoomID             `json:"alkemio_room_id"`
    DisplayName     string                    `json:"display_name"`
    AvatarURL       string                    `json:"avatar_url,omitempty"`
    MemberActorIDs  []AlkemioActorID          `json:"member_actor_ids"`
    Messages        []MessageWithReadStateDto `json:"messages"`
    LastReadEventID *MessageID                `json:"last_read_event_id,omitempty"`
    UnreadCount     int                       `json:"unread_count"`
}
```

**Adapter Actions**:
1.  Resolve `AlkemioRoomID` to Matrix Room ID.
2.  Fetch room details (name, members, messages).
3.  Resolve `ActorID` to Matrix User ID.
4.  Query unread count for the user via Matrix `/sync` API.
5.  Mark messages as read/unread based on the unread count (last N messages are unread).
6.  Return enriched response with `is_read` flag per message.

**Note**: Read state uses the "high water mark" model - when a user reads message N, all messages before N are also considered read.

---

## HTTP Endpoints

### Health Check

*   **Method**: `GET`
*   **Path**: `/health`
*   **Response**: `{"status": "ok"}`

### DM Request Webhook

Receives DM creation requests from the Synapse spam checker module.

*   **Method**: `POST`
*   **Path**: `/_matrix/app/alkemio/dm-request`
*   **Authentication**: Bearer token (HS token from AppService registration)

#### Request Headers

```
Authorization: Bearer <hs_token>
Content-Type: application/json
```

#### Request Payload

```go
type DMWebhookPayload struct {
    Inviter string `json:"inviter"` // Matrix user ID: @uuid:domain
    Invitee string `json:"invitee"` // Matrix user ID: @uuid:domain
}
```

#### Response

Success (200 OK):
```json
{"status": "accepted"}
```

Errors:
- `401 Unauthorized`: Invalid or missing Bearer token
- `400 Bad Request`: Missing required fields or invalid user ID format
- `500 Internal Server Error`: Failed to publish event to RabbitMQ

**Adapter Actions**:
1. Validate Bearer token matches configured HS token
2. Extract Alkemio Actor UUIDs from Matrix user IDs
3. Publish `DMRequestedEvent` to `communication.room.dm.requested` topic
4. Return 200 OK (fire-and-forget)

---

## Error Handling Strategy

The Adapter must distinguish between:

1.  **Transient Errors** (Network issues, Matrix homeserver down):
    *   Adapter should retry internally if possible.
    *   If retries fail, return `ErrCodeMatrixError` or `ErrCodeInternalError`.
    *   Server may retry the RMQ message based on policy.

2.  **Permanent Errors** (Invalid ID, Malformed Data):
    *   Return `ErrCodeInvalidParam` or `ErrCodeRoomNotFound`.
    *   Server should **not** retry.

3.  **Business Logic Errors** (User not allowed):
    *   Return `ErrCodeNotAllowed`.
    *   Server handles as a domain exception.

## TS Library Export

The Go service should generate/export a TypeScript definition file matching these structures for the Node.js server to consume.

```typescript
// Example generated TS
export interface BaseResponse {
  success: boolean;
  error?: ErrorResponse;
}

export interface ErrorResponse {
  code: ErrorCode;
  message: string;
  details?: string;
}

// ... etc
```
