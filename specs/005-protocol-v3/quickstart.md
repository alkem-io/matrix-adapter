# Developer Quickstart: Protocol V3 Implementation

**Feature**: 005-protocol-v3  
**Status**: ✅ Implemented (reference archived)

## Overview

This guide helps developers implement Protocol V3 changes. The work is organized into clear phases that can be tackled independently.

## Prerequisites

- Go 1.25+ installed
- Access to Matrix Homeserver with MSC1772 (Spaces) support
- RabbitMQ running (for integration testing)
- Familiarity with existing codebase patterns

## Quick Reference

### Files to Create
- `pkg/dto/space.go` - Space DTOs
- `pkg/dto/hierarchy.go` - Hierarchy DTOs
- `internal/core/service/space_service.go` - Space business logic
- `internal/infrastructure/queue/handler_space.go` - Space command handlers

### Files to Modify
- `pkg/dto/types.go` - Add `AlkemioContextID`, `JoinRule`
- `pkg/dto/room.go` - Extend `CreateRoomRequest`, `UpdateRoomRequest`; remove `Limit` from `ListRoomsRequest`
- `pkg/dto/batch.go` - Add Space batch DTOs
- `internal/core/domain/model.go` - Add `Space` domain model
- `internal/core/ports/matrix.go` - Add Space-related interface methods
- `internal/infrastructure/matrix/mautrix.go` - Implement Space SDK operations
- `internal/infrastructure/queue/router.go` - Register 8 new topics
- `internal/infrastructure/queue/handler_room.go` - Handle new room fields
- `lib/src/matrix.adapter.event.type.ts` - Add new event types

---

## Implementation Order

### Phase 1: DTOs (Foundation)
Start here. All other work depends on these types.

```bash
# 1. Add new types to pkg/dto/types.go
# - AlkemioContextID (copy AlkemioRoomID pattern)
# - JoinRule enum

# 2. Create pkg/dto/space.go
# - All Space request/response DTOs

# 3. Create pkg/dto/hierarchy.go
# - SetParentRequest/Response

# 4. Update pkg/dto/room.go
# - Add fields to CreateRoomRequest
# - Add fields to UpdateRoomRequest
# - Remove Limit from ListRoomsRequest

# 5. Update pkg/dto/batch.go
# - Add BatchAddSpaceMemberRequest/Response
# - Add BatchRemoveSpaceMemberRequest/Response

# Verify compilation
go build ./...
```

### Phase 2: Matrix Port Interface
Define the contract before implementation.

```bash
# Update internal/core/ports/matrix.go
# Add these methods:

# CreateSpace(ctx, alkemioContextID, name, topic, avatarURL, parentContextID, initialMembers, joinRule) (id.RoomID, error)
# UpdateSpace(ctx, roomID, name, topic, avatarURL, isPublic, joinRule) error
# DeleteSpace(ctx, roomID, reason) error
# GetSpaceDetails(ctx, roomID) (*domain.Space, error)
# GetSpaceChildren(ctx, roomID) ([]domain.SpaceChild, error)
# SetSpaceChild(ctx, parentSpaceID, childID, order) error
# RemoveSpaceChild(ctx, parentSpaceID, childID) error
# SetSpaceParent(ctx, childID, parentSpaceID) error
# RemoveSpaceParent(ctx, childID) error
# SetRoomJoinRule(ctx, roomID, joinRule, parentSpaceID) error
# SetRoomAvatar(ctx, roomID, avatarURL) error

# Verify compilation
go build ./...
```

### Phase 3: Matrix Adapter Implementation
Implement the interface methods.

```bash
# Update internal/infrastructure/matrix/mautrix.go

# Key patterns:
# - Spaces are rooms with CreationContent["type"] = "m.space"
# - Use SendStateEvent for m.space.child, m.space.parent
# - Use SendStateEvent for m.room.join_rules, m.room.avatar
```

### Phase 4: Domain Model & Service
Add domain entity and business logic.

```bash
# 1. Update internal/core/domain/model.go
# - Add Space struct
# - Add SpaceChild struct

# 2. Create internal/core/service/space_service.go
# - Copy pattern from room_service.go
# - buildSpaceAlias() using AlkemioContextID
# - CreateSpaceWithAlkemioID()
# - UpdateSpaceMetadata()
# - DeleteSpaceFully()
# - GetSpaceWithChildren()
# - ListSpaces()
# - SetParent()
# - BatchAddSpaceMembers()
# - BatchRemoveSpaceMembers()

# 3. Update internal/core/service/room_service.go
# - UpdateRoomMetadata() - handle avatar, join_rule
# - CreateRoomWithAlkemioID() - handle parent_context_id, avatar, join_rule
```

### Phase 5: Queue Handlers
Wire up the RabbitMQ handlers.

```bash
# 1. Create internal/infrastructure/queue/handler_space.go
# - SpaceHandler struct
# - NewSpaceHandler()
# - HandleCreateSpace()
# - HandleUpdateSpace()
# - HandleDeleteSpace()
# - HandleGetSpace()
# - HandleListSpaces()
# - HandleSetParent()
# - HandleBatchAddSpaceMember()
# - HandleBatchRemoveSpaceMember()

# 2. Update internal/infrastructure/queue/router.go
# - Import SpaceHandler
# - Add 8 new topic subscriptions

# 3. Update internal/infrastructure/queue/handler_room.go
# - HandleCreateRoom() - handle new fields
# - HandleUpdateRoom() - handle new fields
# - HandleListRooms() - remove limit handling
```

### Phase 6: TypeScript Generation

```bash
# 1. Update lib/src/matrix.adapter.event.type.ts
# - Add 8 new event type entries

# 2. Generate TypeScript DTOs
make generate

# 3. Verify generated output
cat lib/src/dto/generated.ts | grep -A5 "Space"
```

### Phase 7: Testing

```bash
# Run unit tests
make test

# Run linting
make lint

# Build
make build
```

---

## Code Snippets

### AlkemioContextID Type

```go
// pkg/dto/types.go
type AlkemioContextID uuid.UUID

func (c AlkemioContextID) MarshalJSON() ([]byte, error) {
    return uuid.UUID(c).MarshalText()
}

func (c *AlkemioContextID) UnmarshalJSON(data []byte) error {
    var u uuid.UUID
    if err := u.UnmarshalText(data[1 : len(data)-1]); err != nil {
        return err
    }
    *c = AlkemioContextID(u)
    return nil
}

func (c AlkemioContextID) String() string {
    return uuid.UUID(c).String()
}

func (c AlkemioContextID) UUID() uuid.UUID {
    return uuid.UUID(c)
}
```

### JoinRule Enum

```go
// pkg/dto/types.go
type JoinRule string

const (
    JoinRulePublic     JoinRule = "public"
    JoinRuleInvite     JoinRule = "invite"
    JoinRuleRestricted JoinRule = "restricted"
)
```

### Create Space with mautrix-go

```go
// internal/infrastructure/matrix/mautrix.go
func (m *MautrixAdapter) CreateSpace(
    ctx context.Context,
    alkemioContextID uuid.UUID,
    name, topic, avatarURL string,
    initialMembers []domain.Actor,
    joinRule string,
) (id.RoomID, error) {
    aliasLocalpart := alkemioContextID.String()
    intent := m.as.BotIntent()
    
    // Prepare invites
    invites := make([]id.UserID, 0, len(initialMembers))
    for _, member := range initialMembers {
        userID, err := m.EnsureUser(ctx, member)
        if err != nil {
            return "", err
        }
        invites = append(invites, userID)
    }
    
    // Determine preset
    preset := "private_chat"
    if joinRule == "public" {
        preset = "public_chat"
    }
    
    req := &mautrix.ReqCreateRoom{
        Name:          name,
        Topic:         topic,
        Preset:        preset,
        RoomAliasName: aliasLocalpart,
        Invite:        invites,
        CreationContent: map[string]interface{}{
            "type": "m.space",
        },
    }
    
    resp, err := intent.CreateRoom(ctx, req)
    if err != nil {
        return "", fmt.Errorf("failed to create space: %w", err)
    }
    
    // Set avatar if provided
    if avatarURL != "" {
        // ... set m.room.avatar state event
    }
    
    return resp.RoomID, nil
}
```

### Set Hierarchy

```go
// internal/infrastructure/matrix/mautrix.go
func (m *MautrixAdapter) SetSpaceChild(
    ctx context.Context,
    parentSpaceID id.RoomID,
    childID id.RoomID,
    order string,
) error {
    intent := m.as.BotIntent()
    
    content := map[string]interface{}{
        "via":       []string{m.as.HomeserverDomain},
        "suggested": false,
    }
    if order != "" {
        content["order"] = order
    }
    
    _, err := intent.SendStateEvent(
        ctx,
        parentSpaceID,
        event.Type{Type: "m.space.child", Class: event.StateEventType},
        childID.String(),
        content,
    )
    return err
}
```

---

## Testing Checklist

- [ ] Unit tests for new DTOs (JSON marshal/unmarshal)
- [ ] Unit tests for SpaceService with mocked MatrixPort
- [ ] Unit tests for SpaceHandler with mocked SpaceService
- [ ] Verify `make generate` produces valid TypeScript
- [ ] Verify `make lint` passes
- [ ] Verify `make build` succeeds

---

## Common Issues

### Issue: "m.space.child" state key format
**Solution**: State key is the child room ID as string, not a JSON key.

### Issue: Restricted join rule requires parent
**Solution**: When `join_rule=restricted`, the `allow` array in the join rule event MUST reference the parent Space.

### Issue: Alias collision between Rooms and Spaces
**Solution**: No collision—AlkemioRoomID and AlkemioContextID are disjoint UUID namespaces in Alkemio.

---

## References

- [Existing Room Handler](../../internal/infrastructure/queue/handler_room.go)
- [Existing Room Service](../../internal/core/service/room_service.go)
- [Existing Matrix Adapter](../../internal/infrastructure/matrix/mautrix.go)
- [Protocol V3 Spec](../../MatrixAdapterProtocol_V3.md)
