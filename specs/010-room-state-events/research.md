# Research: Room State Events

**Feature**: 010-room-state-events
**Date**: 2026-03-06

## Research Findings

### R1: Avatar Fetching Pattern (GetSpaceDetails)

**Decision**: Reuse the exact same pattern from `GetSpaceDetails` to fetch room avatar in `GetRoomDetails`.

**Rationale**: `GetSpaceDetails` already successfully fetches avatar using `intent.StateEvent(ctx, roomID, event.StateRoomAvatar, "", &avatarContent)` and extracts `string(avatarContent.URL)`. Rooms and spaces are both Matrix rooms — the same state event API works identically.

**Alternatives considered**:
- Custom HTTP request to `/_matrix/client/v3/rooms/{roomId}/state/m.room.avatar` — rejected: unnecessary when `intent.StateEvent` abstracts this.

### R2: Event Publishing Flow

**Decision**: Follow the established listener → domain event → EventService → QueuePort.Publish pattern.

**Rationale**: All 10 existing outbound events use this exact flow:
1. `listener.go` handles Matrix event in `processEvent()` switch
2. Resolves Alkemio IDs asynchronously in goroutine
3. Calls `EventHandlers.OnXxx(domainEvent)` callback
4. `EventService.HandleXxx()` converts domain → DTO and calls `queue.Publish(topic, payload)`
5. Wired in `app.go` via `SetEventHandlers()`

**Alternatives considered**:
- Direct publishing from listener — rejected: violates hexagonal architecture, bypasses EventService.

### R3: Self-Event Filtering

**Decision**: Rely on existing `processEvent()` filter: `if evt.Sender == m.as.BotMXID() { return }`.

**Rationale**: The top-level filter in `processEvent()` (listener.go:50) already discards all events from the appservice bot before the type switch. New state event handlers added to the switch will automatically benefit from this filter. No additional filtering needed.

**Alternatives considered**:
- Per-handler sender check — rejected: redundant with existing top-level filter.
- Filter by appservice-managed users (not just bot) — rejected: room property changes by user intents represent legitimate user actions that should be propagated.

### R4: Room ID Resolution for State Events

**Decision**: Use existing `resolveAlkemioRoomID()` helper in goroutine, same as message/reaction handlers.

**Rationale**: State events arrive with `evt.RoomID` (Matrix room ID). The `resolveAlkemioRoomID()` method fetches room details and extracts the Alkemio UUID from the room alias. This is already used by all other event handlers and includes logging for unmapped rooms.

**Alternatives considered**:
- None — this is the established pattern.

### R5: State Event Types in mautrix-go

**Decision**: Use `event.StateRoomName`, `event.StateRoomAvatar`, `event.StateTopic` constants from mautrix-go.

**Rationale**: These are the standard mautrix-go constants for `m.room.name`, `m.room.avatar`, and `m.room.topic` state events. Already used in `GetSpaceDetails` (avatar) and `getRoomNameAndTopic` (name, topic).

**Content types**:
- `event.RoomNameEventContent` → `.Name` field
- `event.RoomAvatarEventContent` → `.URL` field (type `id.ContentURI`)
- `event.TopicEventContent` → `.Topic` field

### R6: GetRoomAsUser Avatar Parity

**Decision**: `GetRoomAsUser` inherits avatar support automatically because it calls `GetRoomWithMessages()` which calls `GetRoomDetails()`.

**Rationale**: The `GetRoomAsUserResponse` DTO will also need the `AvatarURL` field added, but the service layer change flows through automatically since `GetRoomAsUser` delegates to `GetRoomWithMessages`.

### R7: Outgoing Event Registration

**Decision**: Add the new topic and DTO to `OutgoingEventRegistry` in `commands.go` for TypeScript code generation.

**Rationale**: All outgoing events are registered in the `OutgoingEventRegistry` slice. The `gen-events` tool reads this to generate TypeScript types and event constants. Missing registration means the TypeScript lib won't include the new event type.
