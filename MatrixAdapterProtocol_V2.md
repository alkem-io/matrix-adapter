# Matrix Adapter Protocol Specification (V2)

> **Status**: ⚠️ **DEPRECATED** - See [MatrixAdapterProtocol_V3.md](MatrixAdapterProtocol_V3.md) for current specification  
> **Superseded By**: Protocol V3 (Spaces, Hierarchy, Extended Room/Space operations)  
> **Last Updated**: 2025-12-02

This document defines the communication protocol between the Alkemio Server and the Matrix Adapter service. The protocol is designed to be transport-agnostic but is currently implemented over RabbitMQ.

## Implementation Reference

- **Go DTOs**: `pkg/dto/` - Source of truth for all data structures
- **TypeScript Library**: `lib/src/dto/generated.ts` - Auto-generated from Go
- **Event Types**: `lib/src/matrix.adapter.event.type.ts` - Enum of all topics

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
    ErrCodeInvalidParam    ErrorCode = "INVALID_PARAM"
    ErrCodeRoomNotFound    ErrorCode = "ROOM_NOT_FOUND"
    ErrCodeActorNotFound   ErrorCode = "ACTOR_NOT_FOUND"
    ErrCodeMatrixError     ErrorCode = "MATRIX_ERROR"
    ErrCodeInternalError   ErrorCode = "INTERNAL_ERROR"
    ErrCodeNotAllowed      ErrorCode = "NOT_ALLOWED"
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
    AlkemioRoomID   AlkemioRoomID    `json:"alkemio_room_id"`
    Type            RoomType         `json:"type"`
    Name            string           `json:"name,omitempty"` // Ignored for 'direct'
    InitialMembers  []AlkemioActorID `json:"initial_members,omitempty"`
    Topic           string           `json:"topic,omitempty"`
}
```

#### Response Payload

```go
type CreateRoomResponse struct {
    BaseResponse
    // No additional data needed on success, as Server already knows the ID.
}
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

```go
type RemoveReactionResponse struct {
    BaseResponse
}
```

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

```go
type DeleteMessageResponse struct {
    BaseResponse
}
```

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
type RoomOperationResult struct {
    Success bool           `json:"success"`
    Error   *ErrorResponse `json:"error,omitempty"`
}

type BatchAddMemberResponse struct {
    BaseResponse
    // Results is a map of AlkemioRoomID (string representation) to the operation result.
    // Only populated if BaseResponse.Success is true (meaning the batch was processed).
    // If BaseResponse.Success is false, the entire batch failed (e.g. Actor not found).
    Results map[string]RoomOperationResult `json:"results,omitempty"`
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
    IsPublic      *bool         `json:"is_public,omitempty"` // Maps to WorldReadable/GuestAccess
}
```

#### Response Payload

```go
type UpdateRoomResponse struct {
    BaseResponse
}
```

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

```go
type DeleteRoomResponse struct {
    BaseResponse
}
```

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
    Results map[string]RoomOperationResult `json:"results,omitempty"`
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

```go
type SyncActorResponse struct {
    BaseResponse
}
```

---

### 12. List All Rooms (Admin)

Retrieves a list of all rooms known to the Adapter. Used for auditing and finding orphaned rooms.

*   **Event Subject**: `communication.room.list`

#### Request Payload

```go
type ListRoomsRequest struct {
    Limit  int    `json:"limit,omitempty"`
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
