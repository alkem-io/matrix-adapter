# Quickstart: Room State Events

**Feature**: 010-room-state-events
**Date**: 2026-03-06

## Overview

Two changes to the Matrix Adapter:
1. **Room avatar in queries** — Add `avatar_url` to `GetRoomResponse` and `GetRoomAsUserResponse`
2. **Room updated events** — Emit `communication.room.updated` events when room name/avatar/topic change in Matrix

## Files to Modify

### Domain Layer
- `internal/core/domain/model.go` — Add `AvatarURL` field to `Room` struct
- `internal/core/domain/model.go` (or `read_receipt.go`) — Add `RoomUpdatedEvent` domain type

### Ports Layer
- `internal/infrastructure/matrix/listener.go` — Add `OnRoomUpdated` to `EventHandlers`, handle state events in `processEvent()`

### Service Layer
- `internal/core/service/event_service.go` — Add `HandleRoomUpdated()` method

### Infrastructure Layer
- `internal/infrastructure/matrix/mautrix.go` — Add avatar fetch to `GetRoomDetails()`
- `internal/infrastructure/matrix/listener.go` — Add handlers for `StateRoomName`, `StateRoomAvatar`, `StateTopic`
- `internal/infrastructure/queue/handler_room.go` — Populate `AvatarURL` in responses

### DTO Layer
- `pkg/dto/room.go` — Add `AvatarURL` to `GetRoomResponse` and `GetRoomAsUserResponse`
- `pkg/dto/event.go` — Add `RoomUpdatedEvent` DTO struct
- `pkg/dto/commands.go` — Add `TopicRoomUpdated` constant and registry entry

### App Wiring
- `internal/app/app.go` — Wire `OnRoomUpdated` handler

### Generated
- `lib/` — Regenerated via `make generate`

## Build & Test

```bash
make build      # Verify compilation
make test       # Run unit tests
make lint       # Check style
make generate   # Regenerate TypeScript library
```

## Patterns to Follow

- **Avatar fetch**: Same as `GetSpaceDetails` in `mautrix.go:1511-1550`
- **Event handler**: Same as `handleRoomCreateEvent` in `listener.go:565-613`
- **Event service**: Same as `HandleRoomCreated` in `event_service.go:232-255`
- **Self-event filter**: Already handled by `processEvent()` at `listener.go:50`
