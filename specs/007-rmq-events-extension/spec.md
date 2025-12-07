# Feature Specification: RMQ Events Extension

**Feature Branch**: `007-rmq-events-extension`  
**Created**: 2025-12-07  
**Status**: ✅ Implemented

## Problem Statement

The Alkemio Server needs additional real-time notifications and query capabilities from the Matrix Adapter to support richer communication features:

1. **Reaction Notifications**: Server needs to know when users add or remove reactions to messages for activity feeds and notifications
2. **Room Members Query**: Server needs a lightweight way to get room membership without fetching full room state (messages, etc.)
3. **Thread Messages**: Server needs to retrieve messages within a specific thread for threaded conversation views
4. **User Leave Notifications**: Server needs to know when users leave rooms for membership tracking and cleanup

### Current State

The adapter currently supports:
- Commands to add/remove reactions (`communication.reaction.add`, `communication.reaction.remove`)
- Full room state retrieval (`communication.room.get`) which includes members but also all messages
- No thread-specific message retrieval
- No outgoing events for reaction changes or membership changes

## User Scenarios & Testing

### User Story 1 - Reaction Added Notification (Priority: P1)

As the Alkemio Server, I want to receive notifications when users add reactions to messages, so that I can update activity feeds and send notifications to message authors.

**Why this priority**: Core engagement feature - reactions are a primary way users interact with content, and the platform needs to track this for notifications and analytics.

**Independent Test**: Add a reaction via Element → Adapter publishes `communication.reaction.added` event with reaction details.

**Acceptance Scenarios**:

1. **Given** a user in a room with an existing message, **When** the user adds a reaction emoji to the message, **Then** the adapter publishes a `ReactionAddedEvent` containing the room ID, message ID, reaction emoji, and sender actor ID
2. **Given** the adapter receives a Matrix reaction event, **When** the reactor is a ghost user (not the bot), **Then** the event is published to RabbitMQ
3. **Given** the adapter receives a reaction event from the bot itself, **When** processing the event, **Then** no notification is published (avoid self-notifications)

---

### User Story 2 - Reaction Removed Notification (Priority: P1)

As the Alkemio Server, I want to receive notifications when users remove reactions from messages, so that I can update activity feeds and reaction counts.

**Why this priority**: Complements reaction added - both are needed for accurate reaction state tracking.

**Independent Test**: Remove a reaction via Element → Adapter publishes `communication.reaction.removed` event.

**Acceptance Scenarios**:

1. **Given** a user who has previously reacted to a message, **When** the user removes their reaction, **Then** the adapter publishes a `ReactionRemovedEvent` containing the room ID, message ID, reaction emoji, and sender actor ID
2. **Given** a redaction event for a reaction, **When** the adapter processes it, **Then** the original reaction details are included in the removal notification

---

### User Story 3 - Get Room Members (Priority: P1)

As the Alkemio Server, I want to query just the member list of a room without fetching all messages, so that I can efficiently check membership for authorization decisions.

**Why this priority**: Performance optimization - the existing `room.get` returns all messages which is expensive when only membership is needed.

**Independent Test**: Send `communication.room.members.get` command → Receive list of actor IDs without message data.

**Acceptance Scenarios**:

1. **Given** a valid room ID, **When** the server sends a `GetRoomMembersRequest`, **Then** the adapter returns a list of all current member actor IDs
2. **Given** a room with joined, invited, and left members, **When** querying members, **Then** only joined members are returned by default
3. **Given** a non-existent room ID, **When** querying members, **Then** the adapter returns `ROOM_NOT_FOUND` error

---

### User Story 4 - Get Thread Messages (Priority: P2)

As the Alkemio Server, I want to retrieve all messages in a specific thread, so that I can display threaded conversation views to users.

**Why this priority**: Important for conversation context but not blocking for core functionality.

**Independent Test**: Send `communication.thread.messages.get` command with thread root ID → Receive all reply messages in the thread.

**Acceptance Scenarios**:

1. **Given** a room with a threaded conversation, **When** the server sends a `GetThreadMessagesRequest` with the thread root message ID, **Then** the adapter returns all messages that are replies to that thread
2. **Given** a message ID that is not a thread root (has no replies), **When** querying thread messages, **Then** the adapter returns a list containing only that message itself (thread of one)

---

### User Story 5 - User Left Room Notification (Priority: P2)

As the Alkemio Server, I want to receive notifications when users leave rooms, so that I can update membership records and trigger cleanup workflows.

**Why this priority**: Important for membership tracking but less frequent than reaction events.

**Independent Test**: User leaves room via Element → Adapter publishes `communication.room.member.left` event.

**Acceptance Scenarios**:

1. **Given** a user who is a member of a room, **When** the user leaves the room (voluntarily or kicked), **Then** the adapter publishes a `RoomMemberLeftEvent` containing the room ID and actor ID
2. **Given** the bot itself leaves or is removed from a room, **When** processing the leave event, **Then** no notification is published for the bot
3. **Given** a user is banned from a room, **When** processing the ban, **Then** a leave notification is published (ban implies leave)

---

### Edge Cases

- **Rapid reaction toggle**: User adds then immediately removes a reaction - both events should be published in order
- **Reaction on deleted message**: If the target message is redacted, reaction events may still be received - include message ID regardless
- **Room query during state changes**: Membership query during join/leave operations should return consistent snapshot
- **Thread in deleted room**: Thread message query for a deleted room should return `ROOM_NOT_FOUND`
- **Self-reactions**: Bot's own reactions should not trigger notifications (prevent feedback loops)

## Requirements

### Functional Requirements

#### Outgoing Events (Adapter → Server)

- **FR-001**: Adapter MUST publish `ReactionAddedEvent` when a non-bot user adds a reaction to a message
- **FR-002**: Adapter MUST publish `ReactionRemovedEvent` when a non-bot user removes a reaction from a message
- **FR-003**: Adapter MUST publish `RoomMemberLeftEvent` when a non-bot user leaves a room
- **FR-004**: All outgoing events MUST include the `alkemio_room_id` (not Matrix room ID)
- **FR-005**: All outgoing events involving users MUST include `actor_id` (Alkemio UUID, not Matrix user ID)

#### Commands (Server → Adapter)

- **FR-006**: Adapter MUST support `GetRoomMembersRequest` to retrieve only member list for a room
- **FR-007**: Adapter MUST support `GetThreadMessagesRequest` to retrieve messages in a thread
- **FR-008**: `GetRoomMembersResponse` MUST return only joined members (no filter parameter, joined-only for MVP)
- **FR-009**: `GetThreadMessagesResponse` MUST return all messages in thread (pagination deferred to future iteration, similar to room.get pattern)

#### Event Filtering

- **FR-010**: Adapter MUST NOT publish events for actions performed by the AppService bot itself
- **FR-011**: Adapter MUST ignore events from non-ghost users (if Matrix user ID localpart is not a valid UUID, skip the event). Note: There are no federated users on this server - only Alkemio-controlled ghost users can exist.

### Key Entities

- **ReactionAddedEvent**: Room ID, message ID, reaction emoji, sender actor ID, timestamp
- **ReactionRemovedEvent**: Room ID, message ID, reaction emoji (empty string if unavailable), sender actor ID, timestamp
- **RoomMemberLeftEvent**: Room ID, actor ID, reason (optional), timestamp
- **GetRoomMembersRequest/Response**: Room ID → List of actor IDs
- **GetThreadMessagesRequest/Response**: Room ID, thread root message ID → List of all messages in thread (pagination deferred)

## Success Criteria

### Measurable Outcomes

- **SC-001**: Reaction added/removed events are published within 500ms of the Matrix event
- **SC-002**: Room members query returns results in under 100ms for rooms with up to 1000 members
- **SC-003**: Thread messages query returns all results in under 500ms for threads up to 100 messages
- **SC-004**: Leave notifications are published for 100% of user departures (excluding bot)
- **SC-005**: Zero duplicate events published for the same action
- **SC-006**: All new events follow existing DTO patterns and are included in TypeScript library generation

## Clarifications

### Session 2025-12-07

- Q: Should GetRoomMembersRequest support filtering by membership state? → A: No, return joined members only (no filter parameter) for MVP simplicity
- Q: Thread message pagination default/max page size? → A: Return all messages (no pagination for MVP); add pagination later similar to room.get
- Q: How to resolve actor_id from Matrix user ID? → A: Extract UUID from Matrix localpart (`@<uuid>:...` → `<uuid>`) - the localpart IS the actor UUID directly
- Q: Should RoomMemberLeftEvent include kicks? → A: Yes, publish for both voluntary leave and kicks (both result in user no longer in room)
- Q: Does GetThreadMessagesResponse include the thread root message? → A: Yes, the thread root message is ALWAYS included in the response (first message in the list)

## Assumptions

- Matrix SDK (`mautrix-go`) provides access to reaction and membership events via the sync listener
- Thread messages can be retrieved using Matrix's thread relations API (MSC3440)
- The existing event listener infrastructure can be extended for new event types
- Room alias to Alkemio ID mapping is already available (from existing room creation flow)
