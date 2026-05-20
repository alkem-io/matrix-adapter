# Feature Specification: Element-Initiated Conversation Creation (Synchronous Check)

**Feature Branch**: `050-element-room-check`
**Created**: 2026-05-20
**Status**: Draft
**Input**: User description: "Synchronous check flow for Element-initiated DM and group conversation creation"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - DM Creation from Element (Priority: P1)

A user opens Element, selects another user, and starts a direct message conversation. The system checks whether the conversation is allowed (target has messaging enabled, no duplicate DM exists), creates the platform-side conversation entity, and sets up the room so both users can chat. The initiator can start typing immediately; the other member appears in the room within seconds.

**Why this priority**: DMs are the most common conversation type. This is the core value proposition — users can initiate conversations from Element without going through the Alkemio web UI.

**Independent Test**: Can be fully tested by having one ghost user create a DM to another in Element. Delivers value: a working DM conversation visible in both Element and the Alkemio platform.

**Acceptance Scenarios**:

1. **Given** two registered Alkemio users with messaging enabled, **When** user A starts a DM with user B in Element, **Then** the room is created, both users are joined, the conversation appears on the Alkemio platform, and both users can exchange messages.
2. **Given** user A starts a DM with user B, **When** user A sends the first message before the other member is joined, **Then** user B sees the message as unread when they are joined to the room.
3. **Given** a DM between user A and user B already exists on the platform, **When** user A attempts to create another DM with user B in Element, **Then** room creation is rejected and Element displays an error to the user.

---

### User Story 2 - Group Conversation from Element (Priority: P1)

A user opens Element and creates a group room with multiple participants. The system checks consent for each member, creates the platform-side conversation entity, and sets up the room with all consenting members joined.

**Why this priority**: Group conversations share 95% of the implementation with DMs. Implementing both together avoids rework.

**Independent Test**: Can be fully tested by having a ghost user create a room and invite 2+ other users. Delivers value: a working group conversation on both Element and Alkemio.

**Acceptance Scenarios**:

1. **Given** three registered users with messaging enabled, **When** user A creates a group room inviting B and C in Element, **Then** all three users are joined, the conversation appears on the platform, and all can exchange messages.
2. **Given** three users where one has messaging disabled, **When** user A creates a group room inviting B and C, **Then** the room is created with only consenting members (non-consenting members excluded from platform membership, not joined to room).
3. **Given** a group conversation already exists with users A, B, and C, **When** user A creates another group room with B and C, **Then** the new room is created (groups allow duplicates, unlike DMs).

---

### User Story 3 - Consent and Error Handling (Priority: P2)

When a user attempts to create a conversation that violates platform rules (target has messaging disabled, service unavailable), the system blocks room creation and shows a meaningful error in Element. No orphaned rooms are created.

**Why this priority**: Without proper error handling, users would see confusing failures or empty rooms. This is essential for production readiness but can be tested after the happy path works.

**Independent Test**: Can be tested by configuring a target user with messaging disabled and attempting a DM. Delivers value: users see clear feedback instead of broken state.

**Acceptance Scenarios**:

1. **Given** user B has messaging disabled, **When** user A attempts to create a DM with user B in Element, **Then** room creation is blocked and Element displays a "forbidden" error.
2. **Given** the adapter or server is temporarily unavailable, **When** a user attempts to create any room in Element, **Then** room creation is blocked and Element displays a "service unavailable" error. The user can retry.
3. **Given** all non-initiator members of a group have messaging disabled, **When** the initiator creates a group room, **Then** room creation is blocked (no viable conversation possible).

---

### User Story 4 - Space and Child Room Blocking (Priority: P2)

Ghost users remain blocked from creating spaces or rooms inside spaces from Element. Only standalone rooms (DMs and groups) are allowed through the check flow.

**Why this priority**: This is existing behavior that must not regress. Spaces are managed exclusively through the Alkemio platform.

**Independent Test**: Can be tested by attempting to create a space or a room with a space parent from Element. Delivers value: ensures platform governance over space structure.

**Acceptance Scenarios**:

1. **Given** a ghost user in Element, **When** they attempt to create a space, **Then** room creation is blocked with a 403 error.
2. **Given** a ghost user in Element, **When** they attempt to create a room inside an existing space, **Then** room creation is blocked with a 403 error.
3. **Given** the adapter bot, **When** it creates a space or room via the normal platform flow, **Then** creation succeeds (bot bypasses all restrictions).

---

### Edge Cases

- What happens if the same two users race to create DMs to each other simultaneously? The server's dedup check handles this: one succeeds, the other gets a "duplicate" rejection.
- What happens if the adapter's check endpoint is slow (>3 seconds)? The Synapse module times out and returns 503 to Element. No room is created.
- What happens if Synapse crashes after the check succeeds but before the room is created? The server has an orphaned Conversation entity. A periodic server-side cleanup job handles this (separate TODO).
- What happens if reconciliation fails partway (e.g., server unreachable for member list)? The room has `io.alkemio.pending` state but no alias. On any subsequent event in that room, the adapter can detect the pending state and retry reconciliation.
- What happens if a user sends messages before reconciliation completes? Messages are delivered to the room. The initiator can chat immediately. Other members see unread messages once they are joined during reconciliation.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST intercept standalone room creation requests from ghost users in Synapse and perform a synchronous check with the adapter before allowing creation.
- **FR-002**: System MUST block room creation if the check returns a rejection (consent disabled, duplicate DM) and present the rejection reason to the user.
- **FR-003**: System MUST block room creation if the check times out or fails, returning a "service unavailable" error to the user.
- **FR-004**: System MUST strip all invite entries from the room creation request to prevent invite events from appearing in the room timeline.
- **FR-005**: System MUST inject power level overrides, visibility state, and a reconciliation marker (`io.alkemio.pending`) into approved room creation requests.
- **FR-006**: System MUST detect rooms with the `io.alkemio.pending` state event when processing room creation events and trigger post-creation reconciliation.
- **FR-007**: During reconciliation, the system MUST join the bot, retrieve the member list from the server, directly join each member (without creating invite events), configure DM account data for direct conversations, adjust power levels, and set the room alias.
- **FR-008**: After reconciliation, the system MUST emit the standard room-created notification so the server can fire platform subscriptions.
- **FR-009**: System MUST continue to block space creation and room-inside-space creation for ghost users.
- **FR-010**: System MUST allow the adapter bot and server admins to bypass all room creation restrictions.
- **FR-011**: System MUST support both DM (2 members) and group (2+ members) conversation types through the same check flow, distinguished by an `is_direct` flag.
- **FR-012**: DM dedup MUST be the server's responsibility. The adapter passes all requests through; the server checks for existing conversations between the same pair.
- **FR-013**: If reconciliation fails partway, the `io.alkemio.pending` marker MUST remain so reconciliation can be retried on subsequent events.

### Key Entities

- **Check Request**: Creator identity, invited member identities, conversation type (direct/group). Sent from adapter to server during the synchronous check.
- **Check Response**: Allow/deny decision, assigned room UUID (if approved), rejection reason (if denied). Returned from server to adapter.
- **Reconciliation Marker (`io.alkemio.pending`)**: Custom Matrix state event carrying the assigned UUID. Injected at room creation, consumed during reconciliation, superseded by the room alias.
- **Conversation Entity**: Platform-side record created by the server during the check. Contains member list, conversation type, and the assigned UUID that maps to the Matrix room.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can create a DM from Element and exchange messages within 5 seconds of initiating the conversation.
- **SC-002**: Users can create a group conversation from Element with up to 20 members within 5 seconds.
- **SC-003**: When a conversation is rejected (consent, duplicate), the user sees an error in Element within 3 seconds and no orphaned room exists.
- **SC-004**: When the adapter or server is unavailable, the user sees a "service unavailable" error within 5 seconds and can retry successfully once the service recovers.
- **SC-005**: Space creation and room-inside-space creation remain blocked for ghost users with zero regressions.
- **SC-006**: No invite events appear in the room timeline for any conversation created through this flow.
- **SC-007**: The first message sent by the conversation initiator is visible as unread to other members after they are joined.
- **SC-008**: Conversations created from Element appear on the Alkemio platform and trigger real-time UI updates for all members.

## Assumptions

- All users in the system are ghost users managed by the appservice. No federated or real Matrix users participate.
- The Synapse module's `on_create_room` callback is synchronous and can make HTTP calls before returning.
- The adapter's existing appservice HTTP server (port 8280) can serve additional endpoints alongside Matrix appservice routes.
- RabbitMQ request-reply round-trip (adapter → server → adapter) completes within 3 seconds under normal conditions.
- Synapse's default push rules do not generate notifications for state events (`m.room.member`), so join events during reconciliation do not affect unread counts.
- The adapter's self-calculated unread counting only considers `m.room.message` events, not state events.
- Setting `room_alias_name` in the room creation request auto-sets canonical alias, which breaks DM member-name display in Element. Alias must be set separately after creation.

## Open Questions

1. **What timeout should the Synapse module use for the adapter check call?**
   **Resolved:** 3 seconds (adapter default via SimpleHttpClient).

2. **What happens if reconciliation fails partway — does the system retry?**
   **Resolved:** Yes. `resolveOrReconcile` replaces `resolveAlkemioRoomID` in ALL event handlers, so any subsequent event from the room triggers reconciliation retry. A `sync.Map` per-room dedup prevents concurrent attempts.

3. **How does group room creation differ from DM creation in the check flow?**
   **Resolved:** Same flow. `is_direct` is false for groups; the server approves without dedup (no dedup for groups).
