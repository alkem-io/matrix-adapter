# Research: Protocol V3 Implementation

**Feature**: 005-protocol-v3  
**Status**: ✅ Implemented (research archived)

## Research Topics

### 1. Matrix Spaces Implementation (MSC1772)

**Question**: How to create and manage Matrix Spaces using `mautrix-go`?

**Decision**: Use `mautrix.ReqCreateRoom` with `CreationContent` setting `type: m.space`

**Rationale**:
- Matrix Spaces are rooms with `type: m.space` in creation content
- Existing `CreateRoom` pattern in `mautrix.go` can be extended
- No special Space-specific SDK methods needed

**Implementation**:
```go
req := &mautrix.ReqCreateRoom{
    Name:   name,
    Topic:  topic,
    Preset: "private_chat", // or "public_chat" based on JoinRule
    CreationContent: map[string]interface{}{
        "type": "m.space",
    },
}
```

**Alternatives Considered**:
- Dedicated Space SDK methods: Not available in `mautrix-go`; standard room creation with type works

---

### 2. Space Hierarchy State Events

**Question**: How to establish parent-child relationships between Spaces/Rooms?

**Decision**: Use `m.space.child` and `m.space.parent` state events via `SendStateEvent`

**Rationale**:
- `m.space.child` in parent Space declares the child relationship
- `m.space.parent` in child room/space declares canonical parent
- Standard Matrix Spaces specification pattern

**Implementation**:
```go
// In parent space
childContent := map[string]interface{}{
    "via": []string{homeserverDomain},
    "suggested": false,
}
intent.SendStateEvent(ctx, parentSpaceID, event.Type{Type: "m.space.child"}, childRoomID.String(), &childContent)

// In child room
parentContent := map[string]interface{}{
    "via": []string{homeserverDomain},
    "canonical": true,
}
intent.SendStateEvent(ctx, childRoomID, event.Type{Type: "m.space.parent"}, parentSpaceID.String(), &parentContent)
```

**Alternatives Considered**:
- Only set `m.space.child`: Insufficient—both events recommended for proper hierarchy display

---

### 3. Restricted Join Rules (MSC3083)

**Question**: How to implement `restricted` join rule requiring parent Space membership?

**Decision**: Set `join_rule: restricted` with `allow` array referencing parent Space

**Rationale**:
- Synapse supports MSC3083 for restricted rooms
- Join rule state event specifies which spaces grant access
- Aligns with Alkemio's hierarchical access model

**Implementation**:
```go
joinRuleContent := map[string]interface{}{
    "join_rule": "restricted",
    "allow": []map[string]interface{}{
        {
            "type": "m.room_membership",
            "room_id": parentSpaceID.String(),
        },
    },
}
intent.SendStateEvent(ctx, roomID, event.StateJoinRules, "", &joinRuleContent)
```

**Alternatives Considered**:
- Implement access control in adapter: Violates Matrix-native principle; homeserver handles access

---

### 4. Room Avatar Support

**Question**: How to set room/space avatars in `mautrix-go`?

**Decision**: Use `m.room.avatar` state event after room creation

**Rationale**:
- `ReqCreateRoom` doesn't have direct avatar field
- Avatar is set via state event with `url` field (mxc:// URI)
- Must upload to Matrix media if URL is http/https (out of scope for V3)

**Implementation**:
```go
// Assumes avatar_url is already an mxc:// URI
if avatarURL != "" {
    avatarContent := event.RoomAvatarEventContent{
        URL: id.MustParseContentURI(avatarURL),
    }
    intent.SendStateEvent(ctx, roomID, event.StateRoomAvatar, "", &avatarContent)
}
```

**Alternatives Considered**:
- Automatic upload from http URLs: Adds complexity; defer to future version

---

### 5. Alias Pattern for Spaces

**Question**: Should Spaces use the same alias pattern as Rooms?

**Decision**: Yes, use `#<AlkemioContextID>:<homeserver>` pattern for Spaces

**Rationale**:
- Consistent with existing room alias pattern
- AlkemioContextID is UUID, same as AlkemioRoomID
- Enables idempotent Space creation via alias lookup
- No collision risk: Context IDs and Room IDs are disjoint in Alkemio

**Implementation**:
```go
func (s *SpaceService) buildSpaceAlias(contextID uuid.UUID) string {
    return fmt.Sprintf("#%s:%s", contextID.String(), s.matrix.HomeserverDomain())
}
```

**Alternatives Considered**:
- Prefix-based aliases (`#space-<id>`): Unnecessary complexity; UUIDs are sufficient

---

### 6. Breaking Change Strategy

**Question**: How to handle removal of `Limit` field from `ListRoomsRequest`?

**Decision**: Remove field entirely; update `ListRooms` service method signature

**Rationale**:
- User explicitly requested no backward compatibility
- Simpler code preferred over legacy support
- Coordinated release with Alkemio Server ensures no in-flight issues

**Implementation**:
```go
// Before
type ListRoomsRequest struct {
    Limit  int    `json:"limit,omitempty"`
    Cursor string `json:"cursor,omitempty"`
}

// After
type ListRoomsRequest struct {
    Cursor string `json:"cursor,omitempty"`
}
```

**Alternatives Considered**:
- Deprecation period: Rejected per user requirements

---

## SDK Method Mapping

| V3 Operation | Matrix API | mautrix-go Method |
|--------------|------------|-------------------|
| Create Space | POST /_matrix/client/v3/createRoom (with type=m.space) | `Intent.CreateRoom` |
| Update Space | PUT /_matrix/client/v3/rooms/{roomId}/state/{eventType} | `Intent.SendStateEvent` |
| Delete Space | Same as room deletion | Existing patterns |
| Set Hierarchy | PUT state events m.space.child/m.space.parent | `Intent.SendStateEvent` |
| Set Join Rule | PUT /_matrix/client/v3/rooms/{roomId}/state/m.room.join_rules | `Intent.SendStateEvent` |
| Set Avatar | PUT /_matrix/client/v3/rooms/{roomId}/state/m.room.avatar | `Intent.SendStateEvent` |
| Get Space Children | GET /_matrix/client/v1/rooms/{roomId}/hierarchy | Manual HTTP request |

---

## Open Questions (Resolved)

1. ~~How to detect if a room is a Space?~~ → Check `m.room.create` event for `type: m.space`
2. ~~Can we reuse RoomService for Space operations?~~ → No, create separate SpaceService for clarity
3. ~~How to handle Space hierarchy depth limits?~~ → Matrix handles; we just set relationships

---

## References

- [MSC1772: Matrix Spaces](https://github.com/matrix-org/matrix-spec-proposals/blob/main/proposals/1772-groups-as-rooms.md)
- [MSC3083: Restricted Rooms](https://github.com/matrix-org/matrix-spec-proposals/blob/main/proposals/3083-restricted-rooms.md)
- [mautrix-go Documentation](https://pkg.go.dev/maunium.net/go/mautrix)
- [Matrix Spec: Room Creation](https://spec.matrix.org/v1.6/client-server-api/#post_matrixclientv3createroom)
