# Data Model: Room Creation Control

**Feature**: 006-room-creation-control | **Date**: 2025-12-06

## Overview

This document defines the data transfer objects (DTOs) and domain entities for the DM room creation control feature.

---

## 1. New DTOs

### 1.1 DMRequestedEvent

**Purpose**: Published to RabbitMQ when a ghost user attempts to create a DM room.

**Location**: `pkg/dto/dm.go`

```go
// DMRequestedEvent is published when a ghost user attempts to create a DM room.
// The Alkemio Server receives this event and decides whether to authorize creation.
type DMRequestedEvent struct {
    // InitiatorActorID is the Alkemio actor ID of the user requesting the DM
    InitiatorActorID AlkemioActorID `json:"initiator_actor_id"`
    
    // TargetActorID is the Alkemio actor ID of the intended DM recipient
    TargetActorID AlkemioActorID `json:"target_actor_id"`
    
    // Timestamp when the DM request was received by the adapter
    Timestamp time.Time `json:"timestamp"`
    
    // CorrelationID for request tracing (optional)
    CorrelationID string `json:"correlation_id,omitempty"`
}
```

**TypeScript Generated** (lib/src/dto/generated.ts):
```typescript
export interface DMRequestedEvent {
  initiator_actor_id: string;
  target_actor_id: string;
  timestamp: string; // ISO 8601
  correlation_id?: string;
}
```

**Validation Rules**:
- `InitiatorActorID`: Required, non-empty
- `TargetActorID`: Required, non-empty
- `Timestamp`: Auto-populated by adapter

---

### 1.2 DMWebhookPayload

**Purpose**: Payload received from Synapse spam checker module via webhook.

**Location**: `pkg/dto/dm.go` (internal use, not exported to TS lib)

```go
// DMWebhookPayload is the JSON body received from the Synapse spam checker webhook.
// This matches the payload sent by alkemio_room_control.py.
type DMWebhookPayload struct {
    // InitiatorUserID is the full Matrix user ID (e.g., @uuid:matrix.domain)
    InitiatorUserID string `json:"initiator_user_id"`
    
    // TargetUserID is the full Matrix user ID of the DM target
    TargetUserID string `json:"target_user_id"`
    
    // Timestamp when the request was made (ISO 8601)
    Timestamp string `json:"timestamp"`
}
```

**Note**: This DTO is adapter-internal. The webhook handler transforms Matrix user IDs to Alkemio actor IDs before publishing.

---

## 2. Room Creation via Existing DTO

### CreateRoomRequest (EXISTING)

**Purpose**: Server uses the existing `CreateRoomRequest` to create DM rooms.

**Location**: `pkg/dto/room.go` (existing - no changes needed)

```go
type CreateRoomRequest struct {
    AlkemioRoomID   AlkemioRoomID     `json:"alkemio_room_id"`
    Type            RoomType          `json:"type"`           // Use "direct" for DMs
    Name            string            `json:"name,omitempty"` // Ignored for direct rooms
    InitialMembers  []AlkemioActorID  `json:"initial_members,omitempty"` // Both actors
    Topic           string            `json:"topic,omitempty"`
    // ... other fields
}
```

**DM Room Creation Example**:
```json
{
  "alkemio_room_id": "dm-550e8400-6ba7b810-generated-uuid",
  "type": "direct",
  "initial_members": [
    "550e8400-e29b-41d4-a716-446655440000",
    "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
  ]
}
```

**Server Responsibility**: Generate `alkemio_room_id` (deterministic uuid v5 from sorted actor pair recommended for idempotency).

---

## 3. Existing Types (Referenced)

### AlkemioActorID

**Location**: `pkg/dto/types.go` (existing)

```go
// AlkemioActorID represents a unique identifier for an Alkemio user/actor
type AlkemioActorID string
```

Used for: `InitiatorActorID`, `TargetActorID` in DM DTOs.

### MatrixRoomID

**Location**: `pkg/dto/types.go` (existing)

```go
// MatrixRoomID represents a Matrix room identifier (e.g., !roomid:server.org)
type MatrixRoomID string
```

Used for: `RoomID` in `CreateDMRoomResponse`.

### RoomType

**Location**: `pkg/dto/types.go` (existing)

```go
type RoomType string

const (
    RoomTypeCommunity RoomType = "community"
    RoomTypeDirect    RoomType = "direct"
)
```

Used internally to specify room type during creation.

---

## 4. Topic Constants

**Location**: `pkg/dto/commands.go` (additions)

```go
const (
    // TopicRoomDMRequested is published when a ghost user attempts to create a DM.
    // Direction: Adapter → Alkemio Server
    TopicRoomDMRequested = "communication.room.dm.requested"
)
```

**Registry Updates**:
```go
func init() {
    // Outgoing event (adapter publishes)
    OutgoingEventRegistry[TopicRoomDMRequested] = DMRequestedEvent{}
    
    // No new incoming command - Server uses existing TopicRoomCreate with type="direct"
}
```

**Note**: Server responds via existing `communication.room.create` topic with `type: "direct"` - no new topic needed.

---

## 5. Entity Relationships

```
┌─────────────────────────────────────────────────────────────────┐
│                        DM Flow Data Model                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Synapse Module                    Adapter                       │
│  ┌─────────────────┐              ┌─────────────────┐           │
│  │DMWebhookPayload │  ──HTTP──►   │DMRequestedEvent │           │
│  │ - initiator_id  │              │ - initiator_id  │           │
│  │ - target_id     │              │ - target_id     │           │
│  │ - timestamp     │              │ - timestamp     │           │
│  └─────────────────┘              └────────┬────────┘           │
│                                            │                     │
│                                            │ RabbitMQ            │
│                                            ▼                     │
│                                   ┌─────────────────┐           │
│                                   │  Alkemio Server │           │
│                                   │  (Authorization)│           │
│                                   └────────┬────────┘           │
│                                            │                     │
│                                            │ RabbitMQ (existing) │
│                                            ▼                     │
│  Matrix Room                      ┌─────────────────┐           │
│  ┌─────────────────┐  ◄──────────│CreateRoomReq    │           │
│  │ Created DM Room │              │ - room_id       │           │
│  │ - room_id       │              │ - type="direct" │           │
│  │ - is_direct=true│              │ - members[2]    │           │
│  └─────────────────┘              └─────────────────┘           │
│           │                                                      │
│           │ Response                                             │
│           ▼                                                      │
│  ┌─────────────────┐                                            │
│  │BaseResponse     │                                            │
│  │ - success: true │                                            │
│  └─────────────────┘                                            │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 6. ID Transformation

The adapter transforms between Matrix IDs and Alkemio IDs using `IDMapper` (existing in codebase):

| Source | Matrix Format | Alkemio Format |
|--------|---------------|----------------|
| User ID | `@{uuid}:{domain}` | `{uuid}` |
| Room Alias | `#{uuid}:{domain}` | `{uuid}` |

**Example**:
- Matrix: `@550e8400-e29b-41d4-a716-446655440000:matrix.alkemio.org`
- Alkemio: `550e8400-e29b-41d4-a716-446655440000`

**Transformation Logic** (existing in `internal/core/domain/idmapper.go`):
```go
// AlkemioActorID extracts the Alkemio UUID from a Matrix user ID.
// Input:  @550e8400-e29b-41d4-a716-446655440000:matrix.alkemio.org
// Output: 550e8400-e29b-41d4-a716-446655440000
func (m *IDMapper) AlkemioActorID(userID id.UserID) uuid.UUID
```

---

## 7. Validation Summary

| Field | Required | Format | Validation |
|-------|----------|--------|------------|
| `initiator_actor_id` | Yes | String | Non-empty, valid Alkemio actor |
| `target_actor_id` | Yes | String | Non-empty, valid Alkemio actor |
| `timestamp` | Auto | ISO 8601 | Set by adapter |
| `correlation_id` | No | String | Passthrough for tracing |
| `alkemio_room_id` | Request | UUID | Generated by Server (uuid v5 recommended) |

---

## 8. Generated TypeScript Index

**File**: `lib/src/dto/index.ts`

> **Note**: No manual updates needed. File re-exports all from `generated.ts`. Running `make generate` handles all TypeScript generation automatically.
