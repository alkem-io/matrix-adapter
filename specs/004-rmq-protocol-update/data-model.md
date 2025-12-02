# Data Model: RMQ Protocol Update

**Spec**: [spec.md](spec.md)  
**Status**: ✅ Implemented

## Overview

This document defines the data structures for the new RMQ protocol. All structures are defined in Go with JSON tags for serialization. TypeScript definitions are generated automatically.

---

## Core Types

### Identifier Types

```go
// pkg/dto/types.go

import "github.com/google/uuid"

// AlkemioRoomID is a UUID v4 or v7 identifying a room in Alkemio.
// The adapter maps this to a Matrix room ID via room alias.
type AlkemioRoomID uuid.UUID

// AlkemioActorID is a UUID v4 or v7 identifying an actor (user or bot) in Alkemio.
// The adapter maps this to a Matrix user ID.
type AlkemioActorID uuid.UUID

// MessageID is an opaque string identifier for a message.
// Internally maps to Matrix Event ID.
type MessageID string

// ReactionID is an opaque string identifier for a reaction.
// Internally maps to Matrix Event ID.
type ReactionID string

// RoomType defines the type of room to create.
type RoomType string

const (
    RoomTypeCommunity RoomType = "community"
    RoomTypeDirect    RoomType = "direct"
)
```

### Error Types

```go
// pkg/dto/error.go

// ErrorCode represents categorized error types for programmatic handling.
type ErrorCode string

const (
    ErrCodeInvalidParam   ErrorCode = "INVALID_PARAM"
    ErrCodeRoomNotFound   ErrorCode = "ROOM_NOT_FOUND"
    ErrCodeActorNotFound  ErrorCode = "ACTOR_NOT_FOUND"
    ErrCodeMatrixError    ErrorCode = "MATRIX_ERROR"
    ErrCodeInternalError  ErrorCode = "INTERNAL_ERROR"
    ErrCodeNotAllowed     ErrorCode = "NOT_ALLOWED"
)

// ErrorResponse contains structured error information.
type ErrorResponse struct {
    Code    ErrorCode `json:"code"`
    Message string    `json:"message"`
    Details string    `json:"details,omitempty"` // Optional technical details
}

// BaseResponse is the standard response structure for all command responses.
type BaseResponse struct {
    Success bool           `json:"success"`
    Error   *ErrorResponse `json:"error,omitempty"`
}
```

---

## Room Commands

### CreateRoom

**Topic**: `communication.room.create`

```go
// CreateRoomRequest creates a new communication channel.
type CreateRoomRequest struct {
    AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
    Type           RoomType         `json:"type"`
    Name           string           `json:"name,omitempty"`           // Ignored for 'direct'
    InitialMembers []AlkemioActorID `json:"initial_members,omitempty"`
    Topic          string           `json:"topic,omitempty"`
}

// CreateRoomResponse confirms room creation.
type CreateRoomResponse struct {
    BaseResponse
    // No additional data - server already knows the ID
}
```

### GetRoom

**Topic**: `communication.room.get`

```go
// GetRoomRequest retrieves current state of a room.
type GetRoomRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
}

// GetRoomResponse returns room details with members and messages.
type GetRoomResponse struct {
    BaseResponse
    AlkemioRoomID  AlkemioRoomID    `json:"alkemio_room_id"`
    DisplayName    string           `json:"display_name"`
    MemberActorIDs []AlkemioActorID `json:"member_actor_ids"`
    Messages       []MessageDto     `json:"messages"`
}
```

### UpdateRoom

**Topic**: `communication.room.update`

```go
// UpdateRoomRequest updates room metadata.
type UpdateRoomRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    Name          *string       `json:"name,omitempty"`
    Topic         *string       `json:"topic,omitempty"`
    IsPublic      *bool         `json:"is_public,omitempty"`
}

// UpdateRoomResponse confirms the update.
type UpdateRoomResponse struct {
    BaseResponse
}
```

### DeleteRoom

**Topic**: `communication.room.delete`

```go
// DeleteRoomRequest archives or deletes a room.
type DeleteRoomRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    Reason        string        `json:"reason,omitempty"`
}

// DeleteRoomResponse confirms deletion.
type DeleteRoomResponse struct {
    BaseResponse
}
```

### ListRooms

**Topic**: `communication.room.list`

```go
// ListRoomsRequest retrieves all rooms (admin operation).
type ListRoomsRequest struct {
    Limit  int    `json:"limit,omitempty"`
    Cursor string `json:"cursor,omitempty"`
}

// ListRoomsResponse returns paginated room list.
type ListRoomsResponse struct {
    BaseResponse
    AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
    NextCursor     string          `json:"next_cursor,omitempty"`
}
```

---

## Message Commands

### SendMessage

**Topic**: `communication.message.send`

```go
// SendMessageRequest sends a text message to a room.
type SendMessageRequest struct {
    AlkemioRoomID   AlkemioRoomID  `json:"alkemio_room_id"`
    SenderActorID   AlkemioActorID `json:"sender_actor_id"`
    Content         string         `json:"content"` // Markdown supported
    ParentMessageID *MessageID     `json:"parent_message_id,omitempty"` // For threads
}

// SendMessageResponse returns the message ID and timestamp.
type SendMessageResponse struct {
    BaseResponse
    MessageID MessageID `json:"message_id"`
    Timestamp time.Time `json:"timestamp"` // UTC time from Matrix
}
```

### GetMessage

**Topic**: `communication.message.get`

```go
// GetMessageRequest retrieves details of a specific message.
type GetMessageRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    MessageID     MessageID     `json:"message_id"`
}

// GetMessageResponse returns message details.
type GetMessageResponse struct {
    BaseResponse
    Message MessageDto `json:"message"`
}
```

### DeleteMessage

**Topic**: `communication.message.delete`

```go
// DeleteMessageRequest redacts/deletes a message.
type DeleteMessageRequest struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Reason        string         `json:"reason,omitempty"`
}

// DeleteMessageResponse confirms deletion.
type DeleteMessageResponse struct {
    BaseResponse
}
```

---

## Reaction Commands

### AddReaction

**Topic**: `communication.reaction.add`

```go
// AddReactionRequest adds an emoji reaction to a message.
type AddReactionRequest struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    MessageID     MessageID      `json:"message_id"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Emoji         string         `json:"emoji"`
}

// AddReactionResponse returns the reaction ID.
type AddReactionResponse struct {
    BaseResponse
    ReactionID ReactionID `json:"reaction_id"`
}
```

### RemoveReaction

**Topic**: `communication.reaction.remove`

```go
// RemoveReactionRequest removes a previously added reaction.
type RemoveReactionRequest struct {
    AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
    ReactionID    ReactionID     `json:"reaction_id"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
}

// RemoveReactionResponse confirms removal.
type RemoveReactionResponse struct {
    BaseResponse
}
```

### GetReaction

**Topic**: `communication.reaction.get`

```go
// GetReactionRequest retrieves details of a specific reaction.
type GetReactionRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    ReactionID    ReactionID    `json:"reaction_id"`
}

// GetReactionResponse returns reaction details.
type GetReactionResponse struct {
    BaseResponse
    Reaction ReactionDto `json:"reaction"`
}
```

---

## Membership Commands

### BatchAddMember

**Topic**: `communication.room.member.batch.add`

```go
// BatchAddMemberRequest adds a single actor to multiple rooms.
type BatchAddMemberRequest struct {
    ActorID        AlkemioActorID  `json:"actor_id"`
    AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
}

// RoomOperationResult represents per-room operation outcome.
type RoomOperationResult struct {
    Success bool           `json:"success"`
    Error   *ErrorResponse `json:"error,omitempty"`
}

// BatchAddMemberResponse returns per-room results.
type BatchAddMemberResponse struct {
    BaseResponse
    // Results maps AlkemioRoomID (string) to operation result.
    // Only populated if BaseResponse.Success is true (batch was processed).
    Results map[string]RoomOperationResult `json:"results,omitempty"`
}
```

### BatchRemoveMember

**Topic**: `communication.room.member.batch.remove`

```go
// BatchRemoveMemberRequest removes a single actor from multiple rooms.
type BatchRemoveMemberRequest struct {
    ActorID        AlkemioActorID  `json:"actor_id"`
    AlkemioRoomIDs []AlkemioRoomID `json:"alkemio_room_ids"`
    Reason         string          `json:"reason,omitempty"`
}

// BatchRemoveMemberResponse returns per-room results.
type BatchRemoveMemberResponse struct {
    BaseResponse
    Results map[string]RoomOperationResult `json:"results,omitempty"`
}
```

---

## Actor Commands

### SyncActor

**Topic**: `communication.actor.sync`

```go
// SyncActorRequest ensures an actor exists and updates their profile.
type SyncActorRequest struct {
    ActorID     AlkemioActorID `json:"actor_id"`
    DisplayName string         `json:"display_name"`
    AvatarURL   string         `json:"avatar_url,omitempty"`
}

// SyncActorResponse confirms the sync.
type SyncActorResponse struct {
    BaseResponse
}
```

---

## Data Transfer Objects

### MessageDto

```go
// MessageDto represents a message in a room.
type MessageDto struct {
    ID            MessageID      `json:"id"`
    Content       string         `json:"content"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     time.Time      `json:"timestamp"`
    Reactions     []ReactionDto  `json:"reactions"`
    ThreadID      *MessageID     `json:"thread_id,omitempty"`
}
```

### ReactionDto

```go
// ReactionDto represents a reaction to a message.
type ReactionDto struct {
    ID            ReactionID     `json:"id"`
    Emoji         string         `json:"emoji"`
    SenderActorID AlkemioActorID `json:"sender_actor_id"`
    Timestamp     time.Time      `json:"timestamp"`
}
```

---

## Domain Entities (internal/core/domain)

### Reaction Entity (New)

```go
// Reaction represents a reaction event.
type Reaction struct {
    ID        id.EventID
    RoomID    id.RoomID
    MessageID id.EventID
    Emoji     string
    SenderID  uuid.UUID
    Timestamp time.Time
}
```

### Updated Room Entity

```go
// Room represents a Matrix room.
type Room struct {
    ID           id.RoomID
    AlkemioID    uuid.UUID     // Alkemio room UUID
    Alias        string        // #<UUID>:<homeserver>
    Name         string
    Topic        string
    Type         string        // "community" or "direct"
    MemberIDs    []uuid.UUID   // Alkemio actor IDs
    Messages     []Message     // For room.get
}
```

---

## Validation Rules

| Field | Validation |
|-------|-----------|
| `AlkemioRoomID` | Must be valid UUID v4/v7 |
| `AlkemioActorID` | Must be valid UUID v4/v7 |
| `MessageID` | Non-empty string |
| `ReactionID` | Non-empty string |
| `RoomType` | Must be "community" or "direct" |
| `Content` | Non-empty for send operations |
| `Emoji` | Valid Unicode emoji |
| `InitialMembers` | Exactly 2 for direct rooms |

---

## State Transitions

### Room Lifecycle

```
[Not Exists] → CreateRoom → [Active] → DeleteRoom → [Deleted]
                               ↓
                          UpdateRoom (name, topic, visibility)
```

### Message Lifecycle

```
[Not Exists] → SendMessage → [Active] → DeleteMessage → [Redacted]
                                ↓
                           AddReaction / RemoveReaction
```

### Actor Lifecycle

```
[Not Exists] → SyncActor → [Registered] → SyncActor → [Updated]
                               ↓
                    BatchAddMember / BatchRemoveMember
```
