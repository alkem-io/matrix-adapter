# Feature Specification: Room State Events

**Feature Branch**: `010-room-state-events`
**Created**: 2026-03-06
**Status**: Draft
**Input**: Server request for room avatar in queries and inbound room update events

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Room Avatar in Queries (Priority: P1)

When the Alkemio Server queries room details, it needs the room's avatar URL to display the room visually in the UI. Currently, space queries already return avatar URLs, but room queries do not. The server requests room details and expects to receive the avatar alongside other room properties (display name, members, messages).

**Why this priority**: This is the simpler change with immediate value — the server already consumes room details responses and simply needs the additional field to display room avatars. No new event infrastructure is needed.

**Independent Test**: Can be fully tested by sending a `communication.room.get` or `communication.room.get.as_user` command for a room that has an avatar set, and verifying the response includes the `avatar_url` field.

**Acceptance Scenarios**:

1. **Given** a Matrix room with an avatar set, **When** the server requests room details via the `communication.room.get` command, **Then** the response includes the `avatar_url` field with the Matrix content URI of the avatar.
2. **Given** a Matrix room without an avatar set, **When** the server requests room details, **Then** the response includes the `avatar_url` field as an empty string (or omitted).
3. **Given** a Matrix room where the avatar was recently changed, **When** the server requests room details, **Then** the response reflects the current avatar URL.

---

### User Story 2 - Inbound Room Update Events (Priority: P2)

When room properties (display name, avatar, topic) change in Matrix — whether through the Matrix client directly or via other integrations — the Alkemio Server currently has no way to learn about these changes. The adapter must detect Matrix state changes for rooms and notify the server by publishing events to the message queue, enabling the server to keep its own room data in sync.

**Why this priority**: This is a more complex change requiring new event infrastructure (new event type, new listener handlers, new publishing logic) but is essential for bidirectional synchronization. Without it, rooms modified in Matrix become stale in the Alkemio platform.

**Independent Test**: Can be fully tested by changing a room's name, avatar, or topic in Matrix and verifying that a `RoomUpdatedEvent` is published to the message queue with the correct Alkemio room ID and updated property values.

**Acceptance Scenarios**:

1. **Given** an existing Matrix room mapped to an Alkemio room, **When** the room's display name is changed in Matrix, **Then** the adapter publishes a room updated event containing the Alkemio room ID and the new display name.
2. **Given** an existing Matrix room mapped to an Alkemio room, **When** the room's avatar is changed in Matrix, **Then** the adapter publishes a room updated event containing the Alkemio room ID and the new avatar URL.
3. **Given** an existing Matrix room mapped to an Alkemio room, **When** the room's topic is changed in Matrix, **Then** the adapter publishes a room updated event containing the Alkemio room ID and the new topic.
4. **Given** a state event for a room that cannot be resolved to an Alkemio room ID, **When** the adapter receives the event, **Then** it logs a warning and does not publish any event.
5. **Given** multiple room properties change simultaneously, **When** the adapter receives each state event, **Then** it publishes a separate room updated event for each state change, each containing only the changed property.

---

### Edge Cases

- What happens when a room state event is received for a room the adapter doesn't manage (no Alkemio mapping)? The event should be silently ignored with a debug/warning log.
- What happens when the avatar state event contains an empty URL (avatar removed)? The event should still be published with an empty `avatar_url` so the server can clear it.
- What happens when the adapter's own actions (e.g., creating a room) trigger state events? These should be filtered out to avoid circular event loops — only process events from external sources (not the appservice bot).
- What happens when the room name/topic state event arrives before the room is fully created in Alkemio? The adapter should attempt resolution and log a warning if the room cannot be mapped, without crashing.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The room details query response (`communication.room.get` and `communication.room.get.as_user`) MUST include an `avatar_url` field containing the room's current avatar content URI from Matrix.
- **FR-002**: When a room has no avatar set, the `avatar_url` field MUST be omitted or empty in the response.
- **FR-003**: The adapter MUST listen for `m.room.name` state events on managed rooms and publish a room updated event when the display name changes.
- **FR-004**: The adapter MUST listen for `m.room.avatar` state events on managed rooms and publish a room updated event when the avatar changes.
- **FR-005**: The adapter MUST listen for `m.room.topic` state events on managed rooms and publish a room updated event when the topic changes.
- **FR-006**: Each room updated event MUST contain the Alkemio room ID (UUID) of the affected room.
- **FR-007**: Each room updated event MUST contain only the properties that changed (display name, avatar URL, and/or topic), with unchanged properties omitted.
- **FR-008**: The adapter MUST define a new event type constant for room updated events (e.g., `communication.room.updated`).
- **FR-009**: The adapter MUST NOT publish room updated events for state changes triggered by its own appservice bot, to prevent circular event loops.
- **FR-010**: The adapter MUST gracefully handle cases where a room's Matrix ID cannot be resolved to an Alkemio room ID (log and skip).

### Key Entities

- **RoomUpdatedEvent**: An outbound event published when room properties change in Matrix. Contains the Alkemio room ID and optional fields for display name, avatar URL, and topic — only populated fields represent changes.
- **Room (enhanced)**: The existing room model, extended with an avatar URL property to support the enriched query response.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of room detail query responses include the avatar URL when the room has an avatar set in Matrix.
- **SC-002**: Room property changes (name, avatar, topic) in Matrix result in a corresponding event being published to the message queue within the adapter's normal event processing latency.
- **SC-003**: The server can receive and parse room updated events for all three property types (display name, avatar, topic) without errors.
- **SC-004**: No circular events are generated when the adapter itself changes room properties.

## Assumptions

- The adapter already receives Matrix state events for rooms it manages via the appservice registration (no additional Synapse configuration needed).
- The `m.room.avatar` state event follows the standard Matrix spec format with a `url` field containing a `mxc://` content URI.
- The existing pattern used by `GetSpaceDetails` for fetching avatar state (using `intent.StateEvent` with `event.StateRoomAvatar`) is the correct approach for rooms as well.
- The server (consumer) will handle the new event type and DTO — this spec only covers the adapter side.
- Filtering out self-triggered events can be done by checking the event sender against the appservice bot's user ID.
