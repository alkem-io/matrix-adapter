# Feature Specification: Fix Unreliable Unread Message Counts

**Feature Branch**: `012-fix-unread-counts`
**Created**: 2026-03-19
**Status**: Draft
**Input**: User description: "Fix unreliable unread message counts by replacing Synapse's notification_count with self-calculated counts based on timeline events and read receipts."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Accurate Unread Count After Reading Messages (Priority: P1)

A user opens a conversation in Alkemio, reads messages, and navigates away. When they return to the room list, the unread badge for that conversation shows the correct number of new messages received since they last read — reflecting only messages from other participants.

**Why this priority**: This is the core problem. Users currently see stale unread counts that don't reflect their read state, leading to confusion and repeated re-opening of conversations. Fixing this is the entire purpose of the feature.

**Independent Test**: Can be tested by sending a read receipt for a room, then requesting the unread count for that room and verifying it matches the number of messages after the receipt position (excluding the user's own messages).

**Acceptance Scenarios**:

1. **Given** a room with 10 messages and the user's read receipt is on message 7, **When** the system retrieves the unread count, **Then** it returns the count of messages 8-10 that were NOT sent by the user.
2. **Given** a room where the user just sent a read receipt for the latest message, **When** the system retrieves the unread count, **Then** it returns 0.
3. **Given** a room where the user sent message 9 and another user sent message 10, and the user's read receipt is on message 8, **When** the system retrieves the unread count, **Then** it returns 1 (only message 10, since message 9 is the user's own).

---

### User Story 2 - Batch Unread Counts Across Multiple Rooms (Priority: P1)

A user views their room list, which shows unread badges for multiple conversations. The system retrieves unread counts for all visible rooms efficiently in a single batch request, and each room's count is accurate.

**Why this priority**: The batch endpoint is the primary consumer of unread counts — room lists always fetch counts for multiple rooms at once. If only single-room counting works, the feature delivers no practical value.

**Independent Test**: Can be tested by setting up 3-5 rooms with known message counts and read receipt positions, requesting batch unread counts, and verifying each room's count matches the expected value.

**Acceptance Scenarios**:

1. **Given** 5 rooms with varying unread states, **When** batch unread counts are requested, **Then** each room returns the correct count based on read receipt position and message timeline.
2. **Given** a batch request including a room with no messages, **When** batch unread counts are requested, **Then** that room returns an unread count of 0.
3. **Given** a batch request including a room where the user has never sent a read receipt, **When** batch unread counts are requested, **Then** that room returns the total number of messages not sent by the user.

---

### User Story 3 - Unread Count for Room with No Read History (Priority: P2)

A user is added to a room they have never opened. The unread count shows the total number of messages from other participants, indicating the room has new content.

**Why this priority**: This is a common edge case (new room joins, invites accepted) but less frequent than the primary read-then-check flow.

**Independent Test**: Can be tested by creating a room with messages and a user who has no read receipt in that room, then verifying the unread count equals the total message count minus the user's own messages.

**Acceptance Scenarios**:

1. **Given** a room with 15 messages from various users and the requesting user has no read receipt, **When** the unread count is requested, **Then** it returns the count of all messages not sent by the requesting user.
2. **Given** a room with 5 messages all sent by the requesting user and no read receipt, **When** the unread count is requested, **Then** it returns 0.

---

### Edge Cases

- What happens when the read receipt references an event ID that no longer exists in the timeline (e.g., redacted)? The system falls back to the homeserver's notification_count for that room (the current approach), since there is no reliable positional anchor to calculate from.
- What happens when the room has only state events and no messages? The unread count should be 0.
- What happens when the room timeline is very long (thousands of messages since last read)? The system uses progressive batch fetching (5, 10, 20, 50, 200 events) backward from the latest event to find the read receipt position. If the receipt is not found within 200 events, the system falls back to the homeserver's notification_count.
- What happens when the user's read receipt is on the most recent message? The unread count should be 0.
- What happens when a room has messages but they are all from the requesting user? The unread count should be 0 regardless of read receipt position.
- What happens when the user has no read receipt and the room has more than 200 messages? This is a design limitation: the system falls back to the homeserver's notification_count, same as when a receipt is not found within 200 events.

### Out of Scope

- Thread-level unread counts (existing stubs remain unchanged; deferred until homeserver supports MSC3773).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST calculate unread counts by comparing the user's read receipt position against the room's message timeline, instead of relying on the homeserver's notification count.
- **FR-002**: System MUST retrieve the user's most recent read receipt position (event ID of last read message) for each requested room.
- **FR-003**: System MUST count only messages (not state events) that appear after the read receipt position in the timeline.
- **FR-004**: System MUST exclude messages sent by the requesting user from the unread count.
- **FR-005**: System MUST return a count equal to all non-self messages in the room when no read receipt exists for the user.
- **FR-006**: System MUST return 0 for rooms with no messages.
- **FR-007**: System MUST support both single-room and batch (multi-room) unread count requests using the same calculation logic.
- **FR-008**: System MUST fall back to the homeserver's notification_count when the read receipt event ID is not found within 200 fetched events (whether due to redaction, large gap, or missing data).
- **FR-009**: System MUST use progressive batch fetching (batch sizes: 5, 10, 20, 50, 200) backward from the latest event to locate the read receipt position, reusing the existing progressive fetch pattern.

### Key Entities

- **Read Receipt Position**: The event ID marking the last message a user has read in a specific room. One per user per room.
- **Room Timeline**: The ordered sequence of message events in a room, excluding state events.
- **Unread Count**: The number of messages in a room's timeline that appear after the user's read receipt position and were not sent by the user.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After a user marks a room as read, the next unread count request for that room returns 0 within 2 seconds (no stale counts).
- **SC-002**: Unread counts accurately reflect the number of messages from other participants received after the user's last read position, with 100% correctness for rooms with fewer than 200 unread messages.
- **SC-003**: Batch unread count requests for 10 rooms complete within 5 seconds under normal operating conditions.
- **SC-004**: Rooms with no messages or where all messages are from the requesting user consistently return an unread count of 0.

## Clarifications

### Session 2026-03-19

- Q: When the read receipt references a missing/redacted event, what should the system do? → A: Fall back to the homeserver's notification_count for that room (the current Synapse-based approach), since there is no reliable positional anchor to self-calculate from.
- Q: How should the system handle large timelines where the read receipt is far back? → A: Use progressive batch fetching (5, 10, 20, 50, 200 events) backward from latest, reusing the existing GetLastMessage pattern. Fall back to Synapse's notification_count if receipt not found within 200 events.
- Q: Should thread-level unread counts be included in this feature? → A: No, out of scope. Thread-level counts are unsupported by the homeserver and would multiply API calls. Keep existing stubs as-is.

## Assumptions

- The homeserver stores and serves read receipt data reliably via its client API, even though its notification_count derivation from that data is buggy.
- The adapter's appservice registration with `exclusive: false` and `receive_ephemeral: true` means read receipts sent by managed users are persisted by the homeserver and queryable.
- Room timelines of typical Alkemio conversations contain fewer than 1000 messages since the last read position, so fetching the timeline between receipt and latest message is practical.
- The existing message-fetching capabilities in the adapter (used for GetRoomMessages, GetLastMessage) can be reused or adapted for this calculation.
