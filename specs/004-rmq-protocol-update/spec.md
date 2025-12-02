# Feature Specification: RMQ Protocol Update

**Feature Branch**: `004-rmq-protocol-update`  
**Created**: 2025-12-02  
**Status**: ✅ Implemented  
**Completed**: 2025-12-02

## Summary

Updated the Matrix Adapter RMQ protocol to use Alkemio-native UUIDs, structured responses, and new `communication.*` command patterns per `MatrixAdapterProtocol.md`.

## Problem Statement

The current Matrix Adapter RMQ protocol uses legacy naming conventions, mixed identifier types (some Matrix IDs exposed to the server), and inconsistent response structures. The new protocol specification (`MatrixAdapterProtocol.md`) introduces:

1. **Alkemio-Native Identifiers**: All exchanged identifiers are Alkemio UUIDs (v4 or v7). The Adapter maps these to internal Matrix IDs.
2. **Structured Error Handling**: Standardized error codes and response envelopes.
3. **New Command Set**: Updated event subjects and payload structures.
4. **Partial Success Support**: Batch operations return detailed per-item results.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Room Lifecycle Management (Priority: P1)

The Alkemio Server creates, updates, retrieves, and deletes communication rooms using Alkemio UUIDs. The server provides the room ID at creation time (no Matrix room IDs are exchanged).

**Why this priority**: Room operations are foundational - all other features depend on rooms existing.

**Independent Test**: Can be tested by sending room create/update/delete commands and verifying room state changes without needing message or membership operations.

**Acceptance Scenarios**:

1. **Given** a valid `CreateRoomRequest` with an `AlkemioRoomID`, **When** the adapter processes it, **Then** the adapter creates a Matrix room, stores the mapping, and returns success.
2. **Given** an `AlkemioRoomID` that already exists, **When** a `CreateRoomRequest` is sent, **Then** the adapter returns success (idempotency) without creating a duplicate.
3. **Given** a `CreateRoomRequest` with `Type=direct` and exactly 2 initial members, **When** processed, **Then** the adapter creates/reuses a DM room between those users.
4. **Given** a valid `GetRoomRequest`, **When** processed, **Then** the adapter returns room details including members and recent messages with Alkemio actor IDs.
5. **Given** a valid `UpdateRoomRequest` with name/topic changes, **When** processed, **Then** the adapter updates the Matrix room state.
6. **Given** a valid `DeleteRoomRequest`, **When** processed, **Then** the adapter kicks all members, leaves the room, and removes the room alias.

---

### User Story 2 - Message Operations (Priority: P1)

The Alkemio Server sends, retrieves, and deletes messages in rooms. Messages support threading/replies and markdown content.

**Why this priority**: Messaging is the core communication feature alongside rooms.

**Independent Test**: With at least one room created, send messages, retrieve message details, and delete messages.

**Acceptance Scenarios**:

1. **Given** a valid `SendMessageRequest` with room ID, sender, and content, **When** processed, **Then** the adapter sends the message via Matrix and returns the `MessageID` and timestamp.
2. **Given** a `SendMessageRequest` with a `ParentMessageID`, **When** processed, **Then** the adapter creates a threaded reply.
3. **Given** a valid `GetMessageRequest`, **When** processed, **Then** the adapter returns message details including sender (as Alkemio actor ID), content, and reactions.
4. **Given** a valid `DeleteMessageRequest`, **When** processed, **Then** the adapter redacts the message in Matrix.

---

### User Story 3 - Reaction Operations (Priority: P2)

Users add and remove emoji reactions to messages. The adapter tracks reaction IDs for removal.

**Why this priority**: Reactions enhance messaging experience but are not blocking for core communication.

**Independent Test**: With a message in a room, add a reaction, retrieve it, and remove it.

**Acceptance Scenarios**:

1. **Given** a valid `AddReactionRequest` with room, message, sender, and emoji, **When** processed, **Then** the adapter adds the reaction and returns a `ReactionID`.
2. **Given** a valid `RemoveReactionRequest` with the `ReactionID`, **When** processed, **Then** the adapter removes the reaction.
3. **Given** a valid `GetReactionRequest`, **When** processed, **Then** the adapter returns reaction details including sender.

---

### User Story 4 - Batch Membership Operations (Priority: P2)

The Alkemio Server adds or removes a single actor from multiple rooms in one operation (e.g., when a user joins a Space with sub-rooms). Operations support partial success.

**Why this priority**: Batch operations improve efficiency for complex membership changes but individual operations work without them.

**Independent Test**: Create multiple rooms, then use batch add/remove to manage an actor's membership across all of them.

**Acceptance Scenarios**:

1. **Given** a `BatchAddMemberRequest` with an actor and multiple room IDs, **When** processed, **Then** the adapter returns a `Results` map with success/failure per room.
2. **Given** a `BatchAddMemberRequest` where some rooms don't exist, **When** processed, **Then** the top-level `Success` is true but failed rooms have error details in `Results`.
3. **Given** a `BatchAddMemberRequest` where the actor doesn't exist, **When** processed, **Then** the top-level `Success` is false with an error.
4. **Given** a `BatchRemoveMemberRequest`, **When** processed, **Then** the adapter removes the actor from all specified rooms with per-room results.

---

### User Story 5 - Actor Profile Synchronization (Priority: P2)

The Alkemio Server syncs actor profiles (display name, avatar) to Matrix. This replaces the legacy `tryRegisterNewUser` pattern.

**Why this priority**: Profile sync is needed for proper user representation but actors can function without updated profiles.

**Independent Test**: Send a `SyncActorRequest` and verify the Matrix user's profile is updated.

**Acceptance Scenarios**:

1. **Given** a `SyncActorRequest` for a new actor, **When** processed, **Then** the adapter auto-provisions the Matrix user via Intent API and sets their profile.
2. **Given** a `SyncActorRequest` for an existing actor with updated display name, **When** processed, **Then** the adapter updates the Matrix user's profile.

---

### User Story 6 - Admin Room Listing (Priority: P3)

Administrators list all rooms known to the adapter for auditing and finding orphaned rooms.

**Why this priority**: Admin operations are operational tooling, not core communication.

**Independent Test**: Send a `ListRoomsRequest` and verify it returns room IDs with pagination support.

**Acceptance Scenarios**:

1. **Given** a `ListRoomsRequest`, **When** processed, **Then** the adapter returns a paginated list of Alkemio room IDs.
2. **Given** a `ListRoomsRequest` with a cursor, **When** processed, **Then** the adapter returns the next page of results.

---

### Edge Cases

- What happens when a room mapping exists in the adapter but the Matrix room was deleted externally? Return `ErrCodeRoomNotFound`.
- What happens when an actor ID cannot be mapped to a Matrix user? Return `ErrCodeActorNotFound`.
- What happens when Matrix homeserver is temporarily unavailable? Return `ErrCodeMatrixError` immediately (no internal retries; RMQ/server handles retry policy).
- What happens when a batch operation is requested with an empty room list? Return success with empty `Results` map (top-level `Success: true`).
- What happens when a room has thousands of messages? Return all messages (v2 has no pagination); log warning if count exceeds 1000 for future optimization tracking.

## Requirements *(mandatory)*

### Functional Requirements

#### Data Types

- **FR-001**: System MUST define `AlkemioRoomID` as a UUID type (v4 or v7) for room identification.
- **FR-002**: System MUST define `AlkemioActorID` as a UUID type (v4 or v7) for actor identification.
- **FR-003**: System MUST define `MessageID` as an opaque string type (maps to Matrix Event ID internally).
- **FR-004**: System MUST define `ReactionID` as an opaque string type for reaction identification.

#### Error Handling

- **FR-005**: System MUST define standardized error codes: `INVALID_PARAM`, `ROOM_NOT_FOUND`, `ACTOR_NOT_FOUND`, `MATRIX_ERROR`, `INTERNAL_ERROR`, `NOT_ALLOWED`.
- **FR-006**: All response payloads MUST embed `BaseResponse` with `Success` boolean and optional `Error` containing code, message, and optional details.

#### Room Commands

- **FR-007**: System MUST support `communication.room.create` command with `AlkemioRoomID`, `Type` (community/direct), `Name`, `InitialMembers`, and `Topic`.
- **FR-008**: System MUST support `communication.room.get` command returning room details, member IDs (as Alkemio actor IDs), and all messages. Note: No pagination in v2; future versions may add `limit`/`cursor` for rooms with large message histories.
- **FR-009**: System MUST support `communication.room.update` command for modifying name, topic, and visibility.
- **FR-010**: System MUST support `communication.room.delete` command which kicks all members, leaves the room, and removes the room alias.
- **FR-011**: System MUST support `communication.room.list` command with pagination (limit/cursor).

#### Message Commands

- **FR-012**: System MUST support `communication.message.send` command with room, sender, content (markdown), and optional parent message (for threads).
- **FR-013**: System MUST support `communication.message.get` command returning message details with sender as Alkemio actor ID.
- **FR-014**: System MUST support `communication.message.delete` command for message redaction.

#### Reaction Commands

- **FR-015**: System MUST support `communication.reaction.add` command returning a reaction ID.
- **FR-016**: System MUST support `communication.reaction.remove` command using the reaction ID.
- **FR-017**: System MUST support `communication.reaction.get` command returning reaction details.

#### Membership Commands

- **FR-018**: System MUST support `communication.room.member.batch.add` command for adding an actor to multiple rooms with per-room results.
- **FR-019**: System MUST support `communication.room.member.batch.remove` command for removing an actor from multiple rooms with per-room results.

#### Actor Commands

- **FR-020**: System MUST support `communication.actor.sync` command for creating/updating actor profiles in Matrix using the Intent API (Application Service auto-provisioning).

#### Mapping & Idempotency

- **FR-021**: System MUST map Alkemio room UUIDs to Matrix rooms using room aliases in the format `#<AlkemioRoomID>:<homeserver>`. The homeserver domain is sourced from `m.as.HomeserverDomain` (Application Service configuration).
- **FR-022**: System MUST map Alkemio actor UUIDs to Matrix users using the format `@<AlkemioActorID>:<homeserver>`. The UUID string becomes the localpart directly (e.g., `@550e8400-e29b-41d4-a716-446655440000:matrix.alkemio.io`).
- **FR-023**: Room creation MUST be idempotent - resolve alias first; if exists, return success without creating duplicate.
- **FR-024**: All responses MUST use Alkemio actor IDs, never exposing Matrix user IDs to the server.

### Key Entities

- **AlkemioRoomID**: UUID identifying a room in Alkemio. The adapter maps this to a Matrix room ID internally.
- **AlkemioActorID**: UUID identifying an actor (user or bot) in Alkemio. The adapter maps this to a Matrix user ID.
- **MessageID**: Opaque string identifier for a message, returned from send operations and used for replies/reactions.
- **ReactionID**: Opaque string identifier for a reaction, returned from add reaction and used for removal.
- **MessageDto**: Represents a message with ID, content, sender (as actor ID), timestamp, reactions, and optional thread ID.
- **ReactionDto**: Represents a reaction with ID, emoji, sender (as actor ID), and timestamp.
- **RoomOperationResult**: Per-item result for batch operations with success flag and optional error.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 14 command types defined in the protocol specification are implemented and respond within 2 seconds under normal load.
- **SC-002**: 100% of responses conform to the `BaseResponse` structure with proper success/error handling.
- **SC-003**: All exchanged identifiers are Alkemio UUIDs - no Matrix internal IDs are exposed to the Alkemio Server.
- **SC-004**: Batch operations return per-item results allowing the server to handle partial failures.
- **SC-005**: Room creation is idempotent - duplicate requests with the same Alkemio room ID return success without creating duplicate Matrix rooms.
- **SC-006**: Error responses include appropriate error codes allowing the server to distinguish between transient and permanent failures.
- **SC-007**: TypeScript definitions are generated from Go structs and match the protocol specification.

## Assumptions

- Room mappings use Matrix room aliases (`#<UUID>:<homeserver>`) - no external mapping store required for rooms.
- Actor mappings follow the existing pattern (Matrix user ID derived from Alkemio actor UUID).
- The Matrix homeserver supports all required operations (room creation, messaging, reactions).
- The Alkemio Server will be updated concurrently to use the new protocol - no backward compatibility is needed.
- Actor sync operations will be called before actors are used in other operations.
- Direct message rooms require exactly 2 initial members.

## Dependencies

- `MatrixAdapterProtocol.md` - The source of truth for the protocol specification.
- `mautrix-go` library for Matrix SDK operations.
- Watermill for RabbitMQ message handling.
- `tygo` or similar tool for Go-to-TypeScript generation.

## Clarifications

### Session 2025-12-02

- Q: How should the Alkemio-to-Matrix room ID mappings be persisted? → A: Use Synapse room aliases in the form `#<UUID>:<homeserver>`, consistent with actor mapping pattern.
- Q: What retry behavior should the adapter use for transient Matrix errors? → A: No internal retries; fail fast with structured error and let RMQ/server handle retry policy.
- Q: How many recent messages should `communication.room.get` return? → A: Return all messages (no limit).
- Q: What should happen when a room is deleted via `communication.room.delete`? → A: Kick all members, leave, and remove alias (aggressive cleanup).
- Q: How should new actors be provisioned in Matrix when `communication.actor.sync` is called? → A: Use mautrix-go Intent API (auto-provision via Application Service).

## Out of Scope

- Backward compatibility with the legacy protocol.
- Migration of existing data/mappings (assumed to be handled separately if needed).
- Real-time event streaming from Matrix to Alkemio (only command-response pattern covered).
- End-to-end encryption handling.
