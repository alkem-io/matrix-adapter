# Data Model: Protocol V3 Implementation

**Feature**: 005-protocol-v3  
**Status**: ✅ Implemented (reference archived)

## Glossary

| Term | Domain | Definition |
|------|--------|------------|
| **Context** | Alkemio | A hierarchical organizational unit in Alkemio (Space, Subspace, Challenge). Identified by `AlkemioContextID`. |
| **Space** | Matrix | A room with `type=m.space` that can contain child rooms/spaces. Same entity as Context when mapped. |
| **AlkemioContextID** | Alkemio | UUID identifying a Context. Maps 1:1 to a Matrix Space via room alias. |
| **AlkemioRoomID** | Alkemio | UUID identifying a communication Room (not a Space). Maps 1:1 to a Matrix Room via room alias. |

> **Note**: "Context" and "Space" refer to the same entity from different perspectives. Alkemio uses "Context" (authorization/hierarchy), Matrix uses "Space" (room type). The adapter bridges both terms.

---

## New Types

### AlkemioContextID

UUID v4/v7 identifying a Space/Context in Alkemio. Maps 1:1 to a Matrix Space (room with `type=m.space`).

```go
// pkg/dto/types.go
type AlkemioContextID uuid.UUID

// Methods: MarshalJSON, UnmarshalJSON, String, UUID (same pattern as AlkemioRoomID)
```

### JoinRule

Enum controlling room/space access policy.

```go
// pkg/dto/types.go
type JoinRule string

const (
    JoinRulePublic     JoinRule = "public"      // Anyone can join
    JoinRuleInvite     JoinRule = "invite"      // Requires explicit invitation
    JoinRuleRestricted JoinRule = "restricted"  // Parent Space members can join
)
```

### SpaceChildDto

Represents an entry in a Space's child hierarchy.

```go
// pkg/dto/space.go
type SpaceChildDto struct {
    AlkemioRoomID    *AlkemioRoomID    `json:"alkemio_room_id,omitempty"`
    AlkemioContextID *AlkemioContextID `json:"alkemio_context_id,omitempty"`
    Order            string            `json:"order,omitempty"`
}
```

---

## Extended Room DTOs

### CreateRoomRequest (Extended)

```go
// pkg/dto/room.go
type CreateRoomRequest struct {
    AlkemioRoomID   AlkemioRoomID     `json:"alkemio_room_id"`
    Type            RoomType          `json:"type"`
    Name            string            `json:"name,omitempty"`
    InitialMembers  []AlkemioActorID  `json:"initial_members,omitempty"`
    Topic           string            `json:"topic,omitempty"`
    // NEW V3 fields
    AvatarURL       string            `json:"avatar_url,omitempty"`
    ParentContextID *AlkemioContextID `json:"parent_context_id,omitempty"`
    JoinRule        JoinRule          `json:"join_rule,omitempty"` // Defaults to 'invite'
}
```

### UpdateRoomRequest (Extended)

```go
// pkg/dto/room.go
type UpdateRoomRequest struct {
    AlkemioRoomID AlkemioRoomID `json:"alkemio_room_id"`
    Name          *string       `json:"name,omitempty"`
    Topic         *string       `json:"topic,omitempty"`
    IsPublic      *bool         `json:"is_public,omitempty"`
    // NEW V3 fields
    AvatarURL     *string       `json:"avatar_url,omitempty"`
    JoinRule      *JoinRule     `json:"join_rule,omitempty"`
}
```

### ListRoomsRequest (Breaking Change)

```go
// pkg/dto/room.go
type ListRoomsRequest struct {
    // REMOVED: Limit field
    Cursor string `json:"cursor,omitempty"`
}
```

---

## New Space DTOs

### CreateSpaceRequest

```go
// pkg/dto/space.go
type CreateSpaceRequest struct {
    AlkemioContextID AlkemioContextID  `json:"alkemio_context_id"`
    Name             string            `json:"name"`
    Topic            string            `json:"topic,omitempty"`
    AvatarURL        string            `json:"avatar_url,omitempty"`
    ParentContextID  *AlkemioContextID `json:"parent_context_id,omitempty"`
    InitialMembers   []AlkemioActorID  `json:"initial_members,omitempty"`
    JoinRule         JoinRule          `json:"join_rule,omitempty"`
}

type CreateSpaceResponse struct {
    BaseResponse
}
```

### UpdateSpaceRequest

```go
// pkg/dto/space.go
type UpdateSpaceRequest struct {
    AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
    Name             *string          `json:"name,omitempty"`
    Topic            *string          `json:"topic,omitempty"`
    AvatarURL        *string          `json:"avatar_url,omitempty"`
    IsPublic         *bool            `json:"is_public,omitempty"`
    JoinRule         *JoinRule        `json:"join_rule,omitempty"`
}

type UpdateSpaceResponse struct {
    BaseResponse
}
```

### DeleteSpaceRequest

```go
// pkg/dto/space.go
type DeleteSpaceRequest struct {
    AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
    Reason           string           `json:"reason,omitempty"`
}

type DeleteSpaceResponse struct {
    BaseResponse
}
```

### GetSpaceRequest

```go
// pkg/dto/space.go
type GetSpaceRequest struct {
    AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
}

type GetSpaceResponse struct {
    BaseResponse
    AlkemioContextID AlkemioContextID `json:"alkemio_context_id"`
    Name             string           `json:"name"`
    Topic            string           `json:"topic"`
    AvatarURL        string           `json:"avatar_url"`
    MemberActorIDs   []AlkemioActorID `json:"member_actor_ids"`
    Children         []SpaceChildDto  `json:"children"`
}
```

### ListSpacesRequest

```go
// pkg/dto/space.go
type ListSpacesRequest struct {
    Cursor string `json:"cursor,omitempty"`
}

type ListSpacesResponse struct {
    BaseResponse
    AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
    NextCursor        string             `json:"next_cursor,omitempty"`
}
```

---

## New Hierarchy DTOs

### SetParentRequest

```go
// pkg/dto/hierarchy.go
type SetParentRequest struct {
    // One of ChildRoomID or ChildContextID must be set
    ChildRoomID     *AlkemioRoomID    `json:"child_room_id,omitempty"`
    ChildContextID  *AlkemioContextID `json:"child_context_id,omitempty"`
    // The new parent. If nil, the existing parent is removed (orphaned).
    ParentContextID *AlkemioContextID `json:"parent_context_id"`
}

type SetParentResponse struct {
    BaseResponse
}
```

---

## Matrix Port Interface Extensions

New methods to add to `internal/core/ports/matrix.go`:

```go
// Space Lifecycle
CreateSpace(ctx context.Context, alias string, name string, topic string, avatarURL string, joinRule string) (id.RoomID, error)
UpdateSpace(ctx context.Context, roomID id.RoomID, name *string, topic *string, avatarURL *string, joinRule *string) error
DeleteSpace(ctx context.Context, roomID id.RoomID, alias string) error
GetSpaceDetails(ctx context.Context, roomID id.RoomID) (name string, topic string, avatarURL string, err error)
GetSpaceChildren(ctx context.Context, roomID id.RoomID) ([]SpaceChildInfo, error)

// Hierarchy Management
SetSpaceChild(ctx context.Context, parentRoomID id.RoomID, childRoomID id.RoomID, order string) error
RemoveSpaceChild(ctx context.Context, parentRoomID id.RoomID, childRoomID id.RoomID) error
SetSpaceParent(ctx context.Context, childRoomID id.RoomID, parentRoomID id.RoomID, canonical bool) error
RemoveSpaceParent(ctx context.Context, childRoomID id.RoomID, parentRoomID id.RoomID) error

// Room Extensions
SetRoomJoinRule(ctx context.Context, roomID id.RoomID, joinRule string, allowRoomIDs []id.RoomID) error
SetRoomAvatar(ctx context.Context, roomID id.RoomID, avatarURL string) error
```

**Note**: `id.RoomID` refers to `mautrix.go/id.RoomID`. Domain model abstracts these; actual imports are in `internal/infrastructure/matrix/`.

---

## New Space Batch DTOs

### BatchAddSpaceMemberRequest

```go
// pkg/dto/space.go
type BatchAddSpaceMemberRequest struct {
    ActorID           AlkemioActorID     `json:"actor_id"`
    AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
}

type BatchAddSpaceMemberResponse struct {
    BaseResponse
    Results map[string]RoomOperationResult `json:"results,omitempty"`
}
```

### BatchRemoveSpaceMemberRequest

```go
// pkg/dto/space.go
type BatchRemoveSpaceMemberRequest struct {
    ActorID           AlkemioActorID     `json:"actor_id"`
    AlkemioContextIDs []AlkemioContextID `json:"alkemio_context_ids"`
    Reason            string             `json:"reason,omitempty"`
}

type BatchRemoveSpaceMemberResponse struct {
    BaseResponse
    Results map[string]RoomOperationResult `json:"results,omitempty"`
}
```

---

## Domain Model Extension

### Space (Domain Entity)

```go
// internal/core/domain/model.go
type Space struct {
    ID          id.RoomID       // Matrix Room ID (Space is a room)
    AlkemioID   uuid.UUID       // Alkemio Context ID
    Alias       string          // #uuid:domain
    Name        string
    Topic       string
    AvatarURL   string
    MemberIDs   []uuid.UUID     // Alkemio Actor IDs
    Children    []SpaceChild    // Child rooms/spaces
}

type SpaceChild struct {
    RoomID    *uuid.UUID // Alkemio Room ID (if child is a room)
    ContextID *uuid.UUID // Alkemio Context ID (if child is a space)
    Order     string
}
```

---

## Relationships

```
┌─────────────────────────────────────────────────────────────┐
│                      Alkemio Server                          │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐    │
│  │ RoomID (v4) │   │ContextID   │   │  ActorID (v4)   │    │
│  │   (UUID)    │   │   (v4/v7)  │   │    (UUID)       │    │
│  └──────┬──────┘   └──────┬──────┘   └────────┬────────┘    │
└─────────┼─────────────────┼──────────────────┼──────────────┘
          │                 │                  │
          │ Maps via alias  │ Maps via alias   │ Maps via localpart
          ▼                 ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│                    Matrix Adapter                            │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐    │
│  │Matrix RoomID│   │Matrix SpaceID│  │ Matrix UserID   │    │
│  │ !abc:server │   │ !xyz:server │   │ @uuid:server    │    │
│  │  (Room)     │   │(Room+m.space)│  │                 │    │
│  └──────┬──────┘   └──────┬──────┘   └────────┬────────┘    │
│         │                 │                   │             │
│         │   m.space.child │                   │             │
│         └────────────────►│                   │             │
│                           │                   │             │
└───────────────────────────┼───────────────────┼─────────────┘
                            │                   │
                            ▼                   ▼
                    ┌─────────────────────────────┐
                    │     Matrix Homeserver       │
                    └─────────────────────────────┘
```

---

## Validation Rules

| Field | Rule |
|-------|------|
| `AlkemioContextID` | Required, valid UUID (accepts any version per `uuid.Parse()`) |
| `AlkemioRoomID` | Required, valid UUID (accepts any version per `uuid.Parse()`) |
| `JoinRule` | Optional; if `restricted`, `parent_context_id` MUST be set |
| `SetParentRequest` | Exactly one of `child_room_id` or `child_context_id` must be set |
| `SetParentRequest.parent_context_id` | If provided and not nil, MUST resolve to a Space (type=m.space); return `INVALID_PARAM` if target is a regular room |
| `AvatarURL` | Optional; accepts any string (no format validation at adapter boundary). Matrix homeserver validates `mxc://` URI on upload. Pass-through policy. |

> **Note on UUID versions**: The adapter accepts any valid UUID format. "v4/v7" in spec indicates Alkemio's generation policy, not adapter validation.

---

## State Transitions

### Space Lifecycle

```
┌─────────┐   create   ┌─────────┐   update   ┌─────────┐
│ (none)  │ ─────────► │ Active  │ ─────────► │ Active  │
└─────────┘            └────┬────┘            └────┬────┘
                            │                      │
                            │ delete               │ delete
                            ▼                      ▼
                       ┌─────────┐            ┌─────────┐
                       │ Deleted │            │ Deleted │
                       └─────────┘            └─────────┘
```

### Hierarchy State

```
┌──────────┐  set_parent(A)   ┌──────────┐  set_parent(B)  ┌──────────┐
│ Orphaned │ ───────────────► │ Child(A) │ ──────────────► │ Child(B) │
└──────────┘                  └──────────┘                 └──────────┘
     ▲                                                           │
     │                     set_parent(nil)                       │
     └───────────────────────────────────────────────────────────┘
```
