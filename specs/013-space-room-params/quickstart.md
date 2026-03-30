# Quickstart: Wire joinRule for Rooms & Remove isPublic

## What this feature does

Wires the existing `joinRule` DTO field end-to-end for room operations (`createRoom`, `updateRoom`), matching how spaces already work. Removes the redundant `isPublic` field from `UpdateRoomRequest`.

## Changes at a glance

1. **DTO**: Remove `IsPublic` from `UpdateRoomRequest` (pkg/dto/room.go)
2. **Port**: Add `joinRule` parameter to `CreateRoomWithAlias` and `UpdateRoomState` (internal/core/ports/matrix.go)
3. **Service**: Add `joinRule` to `CreateRoomWithAlkemioID` and replace `isPublic` with `joinRule` in `UpdateRoomMetadata` (internal/core/service/room_service.go)
4. **Adapter**: Add joinRule state event handling to room create and update (internal/infrastructure/matrix/mautrix.go)
5. **Handler**: Pass `joinRule` from DTO to service calls (internal/infrastructure/queue/handler_room.go)
6. **TS lib**: Regenerate via `make generate`

## How to verify

```bash
make build      # Compilation succeeds
make test       # All tests pass (new + existing)
make generate   # TS lib reflects changes
make lint       # No lint issues
```

## Pattern reference

Follow the space implementation for joinRule handling:
- Creation: Add as `m.room.join_rules` state event in `req.InitialState`
- Update: Use `intent.SendStateEvent()` with `event.StateJoinRules`
