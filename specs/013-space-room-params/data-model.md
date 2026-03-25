# Data Model: Wire joinRule for Rooms & Remove isPublic

**Date**: 2026-03-25

## DTO Changes

### UpdateRoomRequest — Remove isPublic

**Before**:
```
UpdateRoomRequest {
  AlkemioRoomID  AlkemioRoomID
  Name           *string
  Topic          *string
  IsPublic       *bool          ← REMOVE
  AvatarURL      *string
  JoinRule       *JoinRule      ← already exists, will be wired
}
```

**After**:
```
UpdateRoomRequest {
  AlkemioRoomID  AlkemioRoomID
  Name           *string
  Topic          *string
  AvatarURL      *string
  JoinRule       *JoinRule      ← now wired end-to-end
}
```

### CreateRoomRequest — No DTO changes

Already has `JoinRule JoinRule` field. Only handler/service/adapter wiring needed.

## Interface Changes

### MatrixPort (ports/matrix.go)

**CreateRoomWithAlias** — add `joinRule string` parameter:
```
Before: CreateRoomWithAlias(ctx, alkemioRoomID, roomType, name, topic, avatarURL, initialMembers)
After:  CreateRoomWithAlias(ctx, alkemioRoomID, roomType, name, topic, avatarURL, joinRule, initialMembers)
```

**UpdateRoomState** — add `joinRule string` parameter:
```
Before: UpdateRoomState(ctx, roomID, actorID, name, topic, avatarURL, alias)
After:  UpdateRoomState(ctx, roomID, actorID, name, topic, avatarURL, joinRule, alias)
```

## Service Signature Changes

### RoomService.CreateRoomWithAlkemioID — add `joinRule string`:
```
Before: (ctx, alkemioRoomID, roomType, name, topic, avatarURL, initialMembers)
After:  (ctx, alkemioRoomID, roomType, name, topic, avatarURL, joinRule, initialMembers)
```

### RoomService.UpdateRoomMetadata — replace `isPublic *bool` with `joinRule *string`:
```
Before: (ctx, alkemioRoomID, name, topic, avatarURL, isPublic)
After:  (ctx, alkemioRoomID, name, topic, avatarURL, joinRule)
```

## No Domain Model Changes

The `Room` domain model in `internal/core/domain/model.go` does not need a `JoinRule` field — joinRule is applied as a Matrix state event during creation/update and is not stored in the adapter's domain model.
