# Feature Specification: Wire joinRule for Rooms & Remove isPublic

**Feature Branch**: `013-space-room-params`
**Created**: 2026-03-25
**Status**: Draft
**Input**: User description: "Wire joinRule end-to-end for createRoom and updateRoom (matching spaces), and remove the redundant isPublic field from UpdateRoomRequest."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create a Room with Controlled Visibility (Priority: P1)

An Alkemio user creates a new room and specifies its visibility via the `joinRule` parameter (e.g., `public` or `invite`). The system creates the corresponding Matrix room with the correct join rule.

**Why this priority**: The `joinRule` field already exists in the CreateRoomRequest DTO but is silently ignored — wiring it through is the highest-value fix.

**Independent Test**: Can be fully tested by sending a createRoom command with `join_rule: "public"` and verifying the resulting Matrix room has a public join rule; repeat with `join_rule: "invite"` and verify invite-only.

**Acceptance Scenarios**:

1. **Given** a createRoom request with `joinRule` set to `public`, **When** the room is created, **Then** the Matrix room has join rule `public`.
2. **Given** a createRoom request with `joinRule` set to `invite`, **When** the room is created, **Then** the Matrix room has join rule `invite`.
3. **Given** a createRoom request with `joinRule` omitted, **When** the room is created, **Then** the system uses the existing default behavior (preset-based).
4. **Given** a createRoom request for a direct-message room with `joinRule` set to `public`, **When** the room is created, **Then** the `joinRule` is ignored and the room remains private.

---

### User Story 2 - Update a Room's Visibility (Priority: P1)

An Alkemio administrator updates an existing room's visibility by changing its `joinRule`.

**Why this priority**: Equally important — rooms need to change visibility over time, and the current updateRoom handler does not pass `joinRule` through to Matrix.

**Independent Test**: Can be tested by sending an updateRoom command with `join_rule: "public"` on a private room and verifying the Matrix room join rule changes.

**Acceptance Scenarios**:

1. **Given** an existing private room and an updateRoom request with `joinRule` set to `public`, **When** the update is processed, **Then** the Matrix room join rule changes to `public`.
2. **Given** an existing public room and an updateRoom request with `joinRule` set to `invite`, **When** the update is processed, **Then** the Matrix room join rule changes to `invite`.
3. **Given** an updateRoom request with `joinRule` omitted, **When** the update is processed, **Then** the room's join rule remains unchanged.

---

### User Story 3 - Remove isPublic from UpdateRoomRequest (Priority: P2)

The `isPublic` field is removed from the UpdateRoomRequest DTO since `joinRule` is the consistent, more expressive mechanism used across all space and room operations.

**Why this priority**: Cleanup that removes a redundant, never-implemented field. Lower priority than wiring joinRule, but important for API consistency.

**Independent Test**: Can be verified by confirming the `is_public` field no longer appears in the Go DTO or generated TypeScript interface, and that existing callers use `join_rule` instead.

**Acceptance Scenarios**:

1. **Given** the UpdateRoomRequest DTO, **When** inspected, **Then** the `isPublic` / `is_public` field no longer exists.
2. **Given** the generated TypeScript library, **When** regenerated, **Then** `UpdateRoomRequest` no longer includes `is_public`.

---

### Edge Cases

- What happens when `joinRule` is set on a direct-message room at creation? Direct-message rooms ignore the `joinRule` and always use `trusted_private_chat` preset.
- What happens when an unsupported `joinRule` value is provided? The system passes it to Matrix, which will return an error. The adapter wraps the Matrix error with context (room ID, attempted join rule) and returns it to the caller via the standard error response format. The error is logged with structured context per existing adapter patterns.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The createRoom handler MUST pass the `joinRule` field from the DTO through the service layer to the Matrix adapter.
- **FR-002**: The updateRoom handler MUST pass the `joinRule` field from the DTO through the service layer to the Matrix adapter.
- **FR-003**: The Matrix adapter MUST apply the `joinRule` as a state event when creating a room (if provided).
- **FR-004**: The Matrix adapter MUST update the join rule state event when updating a room (if provided).
- **FR-005**: When `joinRule` is omitted, the system MUST preserve existing default behavior.
- **FR-006**: The `joinRule` MUST be ignored for direct-message rooms, which always remain private.
- **FR-007**: The `isPublic` field MUST be removed from UpdateRoomRequest DTO.
- **FR-008**: The generated TypeScript library MUST be regenerated to reflect the DTO change.
- **FR-009**: Existing space operations (createSpace, updateSpace) MUST remain unaffected — they already handle `joinRule` correctly.

### Key Entities

- **Room**: A Matrix room representing an Alkemio communication channel. Its `joinRule` parameter is now wired end-to-end for both creation and update.
- **JoinRule**: The Matrix concept (`public`, `invite`, `restricted`) controlling room/space visibility. Used consistently across all operations.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: createRoom correctly applies `joinRule` end-to-end, from request through to Matrix room state.
- **SC-002**: updateRoom correctly applies `joinRule` end-to-end, from request through to Matrix room state.
- **SC-003**: The `isPublic` field no longer exists in the UpdateRoomRequest DTO or generated TypeScript.
- **SC-004**: Unit tests cover joinRule handling for createRoom and updateRoom (present, absent, direct-message override).
- **SC-005**: Existing space joinRule handling remains functional and unmodified.

## Assumptions

- The `joinRule` field already exists in both CreateRoomRequest and UpdateRoomRequest DTOs — no DTO additions needed, only wiring through handler/service/adapter layers.
- Removing `isPublic` from UpdateRoomRequest is a breaking change for any callers currently sending that field. Since the field was never implemented (silently ignored), this is considered safe.
- Space operations (createSpace, updateSpace) already handle `joinRule` correctly end-to-end and require no changes.
