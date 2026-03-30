# Contract Changes: 013-space-room-params

**Updated**: 2026-03-30

## New Fields on Existing DTOs

### CreateRoomRequest

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `JoinRule` | `JoinRule` | `join_rule` | **NOW FUNCTIONAL** — was present but not wired |
| `IsPublic` | `*bool` | `is_public` | **NEW** — room directory visibility |
| `CustomState` | `map[string]map[string]interface{}` | `custom_state` | **NEW** — io.alkemio.* state events set as InitialState |

### UpdateRoomRequest

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `JoinRule` | `*JoinRule` | `join_rule` | **NOW FUNCTIONAL** — was present but not wired |
| `IsPublic` | `*bool` | `is_public` | **NEW** — room directory visibility |
| `CustomState` | `map[string]map[string]interface{}` | `custom_state` | **NEW** — io.alkemio.* state events |

### GetRoomResponse

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `CustomState` | `map[string]map[string]interface{}` | `custom_state` | **NEW** — io.alkemio.* state events |

### CreateSpaceRequest

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `IsPublic` | `*bool` | `is_public` | **NEW** — room directory visibility |
| `CustomState` | `map[string]map[string]interface{}` | `custom_state` | **NEW** — io.alkemio.* state events |

### UpdateSpaceRequest

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `IsPublic` | `*bool` | `is_public` | **NEW** — room directory visibility |
| `CustomState` | `map[string]map[string]interface{}` | `custom_state` | **NEW** — io.alkemio.* state events |

### GetSpaceResponse

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `CustomState` | `map[string]map[string]interface{}` | `custom_state` | **NEW** — io.alkemio.* state events |

## New DTOs (pkg/dto/state.go)

### SetRoomStateRequest / SetSpaceStateRequest

```json
{
  "alkemio_room_id": "uuid",
  "state": {
    "io.alkemio.visibility": {"visible": true},
    "io.alkemio.category": {"type": "forum"}
  }
}
```

### GetRoomStateRequest / GetSpaceStateRequest

```json
{
  "alkemio_room_id": "uuid",
  "event_types": ["io.alkemio.visibility"]
}
```

### GetRoomStateResponse / GetSpaceStateResponse

```json
{
  "success": true,
  "alkemio_room_id": "uuid",
  "state": {
    "io.alkemio.visibility": {"visible": true}
  }
}
```

## New Event: SpaceUpdatedEvent

```json
{
  "alkemio_context_id": "uuid",
  "display_name": "New Name",
  "timestamp": 1234567890
}
```

Topic: `communication.space.updated`

## Update Semantics (Pointer Fields)

For update operations, `nil` / omitted = no change, empty string = clear the value:

| Value | Meaning |
|-------|---------|
| Field omitted / `null` | Don't change this field |
| `""` (empty string) | Clear/erase this field |
| `"value"` | Set to this value |

Exception: `join_rule` requires a non-empty value (can't be cleared).

## Migration Guide

1. `is_public` is now available on all create/update operations — use alongside `join_rule`
2. `custom_state` allows setting `io.alkemio.*` state events at creation time
3. Room name/topic/avatar can be erased by sending empty string
4. Use standalone state API (`state.set/get`) for managing custom state independently
5. Ghost users are joined directly (no invite) — no invite notification in Element
