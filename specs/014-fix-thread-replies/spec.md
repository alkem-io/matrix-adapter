# Feature Specification: Fix Thread Reply Formatting and Message Ordering

**Feature Branch**: `014-fix-thread-replies`
**Created**: 2026-03-30
**Status**: Draft (Retrofit — already implemented)
**Input**: User description: "Fix thread reply formatting to use proper Matrix thread relations, and fix thread message ordering so the root message appears first."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Thread replies display correctly in Matrix clients (Priority: P1)

When a user sends a reply within a thread, the reply must appear as part of that thread in all Matrix clients. Previously, replies used only `m.in_reply_to` without the `m.thread` relation, causing some clients to treat them as standalone messages rather than thread replies.

**Why this priority**: Thread replies not appearing in the correct thread breaks the core conversation experience for users.

**Independent Test**: Send a reply to a threaded message and verify it appears within the thread in Element or another Matrix client, not as a standalone message in the room timeline.

**Acceptance Scenarios**:

1. **Given** a user is replying to a message in a thread, **When** the reply is sent, **Then** the reply appears within that thread in all compliant Matrix clients.
2. **Given** a user is replying to a message in a thread, **When** the reply is sent, **Then** clients that do not support threads still display the reply as a regular in-reply-to message (backwards compatibility via falling back).

---

### User Story 2 - Thread messages are returned in chronological order (Priority: P1)

When a user retrieves messages for a thread, the root message must appear first, followed by replies in chronological order. Previously, the root message was prepended before fetching replies, and the relations API returns newest-first, resulting in incorrect ordering after the consumer reverses the list.

**Why this priority**: Incorrect message ordering makes thread conversations unreadable.

**Independent Test**: Fetch messages for a thread with 3+ replies and verify the root message is first and replies follow in chronological (oldest-first) order.

**Acceptance Scenarios**:

1. **Given** a thread with a root message and multiple replies, **When** the thread messages are retrieved, **Then** the root message is the first message and replies follow in chronological order.
2. **Given** a thread root message that fails to parse, **When** thread messages are retrieved, **Then** only the parseable reply messages are returned without errors.
3. **Given** a thread with no replies (relations API returns an error), **When** thread messages are retrieved, **Then** only the root message is returned.

---

### Edge Cases

- What happens when the thread root message cannot be parsed? The system returns only the reply messages without the root.
- What happens when the relations API returns no results or errors? The system returns just the root message (if parseable), or an empty list.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Thread replies MUST include both `m.thread` relation (with relation type, event ID, and falling-back flag set to true) and `m.in_reply_to` to comply with the Matrix threading specification (MSC3440).
- **FR-002**: The thread relation MUST reference the thread root event ID so clients can group the reply within the correct thread.
- **FR-003**: The falling-back flag MUST be set to true so that clients without thread support fall back to displaying the message as a standard reply.
- **FR-004**: When retrieving thread messages, the system MUST return messages in chronological order with the root message first.
- **FR-005**: When retrieving thread messages, if the root message cannot be parsed, the system MUST still return all parseable reply messages.
- **FR-006**: When retrieving thread messages, if the relations API returns no results or errors, the system MUST return only the root message (if parseable).

### Key Entities

- **Thread Root Event**: The original message that started the thread; identified by its Matrix event ID.
- **Thread Reply**: A message sent in response within a thread; carries both thread relation and in-reply-to relation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of thread replies sent through the adapter appear within the correct thread when viewed in spec-compliant Matrix clients.
- **SC-002**: Thread message retrieval returns messages in chronological order (root first, replies ascending by timestamp) in all cases.
- **SC-003**: Thread retrieval gracefully handles unparseable root messages and empty relation results without returning errors to the consumer.

## Assumptions

- The consuming server reverses the message list returned by the adapter (hence appending root last so it ends up first after reversal).
- The Matrix homeserver supports the relations API for thread lookups.
- MSC3440 (threading) is the target specification for thread relation formatting.
