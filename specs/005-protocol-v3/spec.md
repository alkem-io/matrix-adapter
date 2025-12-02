# Feature Specification: Protocol V3 Implementation

**Feature Branch**: `005-protocol-v3`  
**Created**: 2025-12-02  
**Status**: ✅ Implemented  
**Completed**: 2025-12-03  
**Protocol**: See `MatrixAdapterProtocol_V3.md` for canonical reference

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Space Lifecycle Management (Priority: P1)

As a platform administrator, I need to create, update, and delete Matrix Spaces that correspond to Alkemio Spaces (Contexts), so that hierarchical organization of communication channels is possible.

**Why this priority**: Spaces are the foundational concept that enables hierarchical room organization. Without Space support, nested room structures and restricted join rules cannot function.

**Independent Test**: Can be fully tested by creating a Space via `communication.space.create`, verifying it exists with `communication.space.get`, updating its metadata, and then deleting it. Delivers the core infrastructure for hierarchical organization.

**Acceptance Scenarios**:

1. **Given** a valid AlkemioContextID and Space metadata, **When** `communication.space.create` is received, **Then** a Matrix Space is created with the specified name, topic, and avatar, and the mapping is stored.
2. **Given** an existing Space mapping, **When** `communication.space.create` is received for the same AlkemioContextID, **Then** the operation returns Success (idempotency) without creating a duplicate.
3. **Given** an existing Space, **When** `communication.space.update` is received with new name/topic/avatar, **Then** the Space metadata is updated in Matrix.
4. **Given** an existing Space, **When** `communication.space.delete` is received, **Then** the Space is archived/deleted and the mapping is removed.
5. **Given** an existing Space, **When** `communication.space.get` is received, **Then** the Space details including name, topic, avatar, members, and children are returned.

---

### User Story 2 - Room-Space Hierarchy Management (Priority: P1)

As a platform administrator, I need to organize Rooms and Spaces into a parent-child hierarchy, so that users can navigate and discover related communication channels through the Space structure.

**Why this priority**: Hierarchy is the core value proposition of Spaces. Without `set_parent` functionality, Spaces are just rooms with a different type and provide no organizational benefit.

**Independent Test**: Can be tested by creating a Space, creating a Room with `parent_context_id`, then using `communication.hierarchy.set_parent` to move the room to a different Space or orphan it.

**Acceptance Scenarios**:

1. **Given** a Room and a Space, **When** `communication.hierarchy.set_parent` is received with the Room as child and Space as parent, **Then** the `m.space.child` event is set in the parent Space and `m.space.parent` is set in the Room.
2. **Given** a Space (child) and another Space (parent), **When** `communication.hierarchy.set_parent` is received, **Then** the child Space is nested under the parent Space.
3. **Given** a Room/Space with an existing parent, **When** `communication.hierarchy.set_parent` is received with a different parent, **Then** the old parent relationship is removed and the new one is established.
4. **Given** a Room/Space with an existing parent, **When** `communication.hierarchy.set_parent` is received with `parent_context_id=null`, **Then** the room/space becomes orphaned (no parent).

---

### User Story 3 - Extended Room Creation (Priority: P2)

As a platform administrator, I need to create rooms with avatar URLs, parent context references, and join rules, so that rooms are properly integrated into the Space hierarchy from creation.

**Why this priority**: Extends existing room creation to support the new V3 fields. Important for proper room setup but the base room creation already works.

**Independent Test**: Can be tested by sending `communication.room.create` with the new fields (avatar_url, parent_context_id, join_rule) and verifying the room has the correct settings.

**Acceptance Scenarios**:

1. **Given** a CreateRoomRequest with `avatar_url`, **When** processed, **Then** the room is created with the specified avatar.
2. **Given** a CreateRoomRequest with `parent_context_id`, **When** processed, **Then** the room is added as a child of the specified Space.
3. **Given** a CreateRoomRequest with `join_rule=public`, **When** processed, **Then** the room allows anyone to join.
4. **Given** a CreateRoomRequest with `join_rule=restricted` and a `parent_context_id`, **When** processed, **Then** the room only allows members of the parent Space to join.
5. **Given** a CreateRoomRequest with `join_rule=invite`, **When** processed, **Then** the room requires explicit invitations.

---

### User Story 4 - Space Membership Batch Operations (Priority: P2)

As a platform administrator, I need to add or remove an actor from multiple Spaces in a single operation, so that onboarding and offboarding can be performed efficiently.

**Why this priority**: Mirrors the existing batch room membership operations but for Spaces. Important for efficient user management but not blocking core functionality.

**Independent Test**: Can be tested by creating multiple Spaces, then using `communication.space.member.batch.add` to add an actor to all of them, and verifying membership via `communication.space.get`.

**Acceptance Scenarios**:

1. **Given** a valid actor and list of AlkemioContextIDs, **When** `communication.space.member.batch.add` is received, **Then** the actor is invited/joined to each Space and per-Space results are returned.
2. **Given** an actor who is a member of multiple Spaces, **When** `communication.space.member.batch.remove` is received, **Then** the actor is removed from each Space and per-Space results are returned.
3. **Given** a batch operation where some Spaces don't exist, **When** processed, **Then** the operation returns Success with partial results indicating which Spaces failed.

---

### User Story 5 - Space Listing (Priority: P3)

As a platform administrator, I need to list all Spaces known to the adapter with pagination support, so that I can audit and manage Space mappings.

**Why this priority**: Administrative functionality for visibility into the adapter state. Lower priority as it doesn't affect core user workflows.

**Independent Test**: Can be tested by creating several Spaces, then calling `communication.space.list` and verifying all created Spaces appear in the response.

**Acceptance Scenarios**:

1. **Given** multiple existing Spaces, **When** `communication.space.list` is received, **Then** all AlkemioContextIDs are returned.
2. **Given** a paginated request with cursor, **When** processed, **Then** the next page of results is returned with an updated cursor.

---

### User Story 6 - Updated Room Operations (Priority: P2)

As a platform administrator, I need room update operations to support avatar and join rule changes, so that room settings can be managed consistently with creation options.

**Why this priority**: Extends UpdateRoomRequest to support the new V3 fields. Important for feature parity but existing update functionality continues to work.

**Independent Test**: Can be tested by updating a room's avatar_url and join_rule via `communication.room.update` and verifying the changes.

**Acceptance Scenarios**:

1. **Given** an existing room, **When** `communication.room.update` is received with `avatar_url`, **Then** the room avatar is updated.
2. **Given** an existing room, **When** `communication.room.update` is received with `join_rule`, **Then** the room join rule is updated.

---

### Edge Cases

- What happens when `join_rule=restricted` is specified without a `parent_context_id`? → Return error `INVALID_PARAM` with message explaining restricted requires parent.
- What happens when setting a parent to a non-Space room? → Return error `INVALID_PARAM` indicating parent must be a Space.
- What happens when creating circular hierarchy (Space A → Space B → Space A)? → Matrix should prevent this; adapter returns `MATRIX_ERROR` if detected.
- What happens when deleting a Space that has child rooms/spaces? → The children become orphaned; the delete succeeds.
- What happens when both `child_room_id` and `child_context_id` are provided in SetParentRequest? → Return error `INVALID_PARAM`; only one must be set.
- What happens when neither `child_room_id` nor `child_context_id` is provided? → Return error `INVALID_PARAM`.

## Requirements *(mandatory)*

### Functional Requirements

**Types & Constants**

- **FR-001**: System MUST support `AlkemioContextID` as a UUID v4/v7 type for Space/Context identification.
- **FR-002**: System MUST support `JoinRule` enum with values: `public`, `invite`, `restricted`.

**Space Lifecycle (Commands 15-17, 21-22)**

- **FR-010**: System MUST handle `communication.space.create` to create a Matrix Space with type `m.space`.
- **FR-011**: System MUST support optional `ParentContextID` on Space creation to establish hierarchy.
- **FR-012**: System MUST support `InitialMembers` on Space creation to invite/join actors.
- **FR-013**: System MUST handle `communication.space.update` to modify Space name, topic, avatar, visibility, and join rule.
- **FR-014**: System MUST handle `communication.space.delete` to archive/delete a Space.
- **FR-015**: System MUST handle `communication.space.get` to return Space details including children hierarchy.
- **FR-016**: System MUST handle `communication.space.list` to return all known Space mappings with pagination.

**Hierarchy Management (Command 18)**

- **FR-020**: System MUST handle `communication.hierarchy.set_parent` to establish parent-child relationships.
- **FR-021**: System MUST set `m.space.child` event in parent Space when hierarchy is established.
- **FR-022**: System MUST set `m.space.parent` event in child room/space when hierarchy is established.
- **FR-023**: System MUST remove old hierarchy events when parent changes or is removed.
- **FR-024**: System MUST validate that exactly one of `child_room_id` or `child_context_id` is provided.

**Space Membership (Commands 19-20)**

- **FR-030**: System MUST handle `communication.space.member.batch.add` to add an actor to multiple Spaces.
- **FR-031**: System MUST handle `communication.space.member.batch.remove` to remove an actor from multiple Spaces.
- **FR-032**: System MUST return per-Space results allowing partial success.

**Extended Room Operations (Commands 1, 8 updates)**

- **FR-040**: System MUST support `avatar_url` on room creation to set room avatar.
- **FR-041**: System MUST support `parent_context_id` on room creation to establish initial hierarchy.
- **FR-042**: System MUST support `join_rule` on room creation to set access policy.
- **FR-043**: System MUST support `avatar_url` on room update.
- **FR-044**: System MUST support `join_rule` on room update.

**Cleanup (Breaking Changes)**

- **FR-050**: System MUST remove `Limit` field from `ListRoomsRequest` (breaking change, no backward compatibility).

**DTOs**

- **FR-060**: System MUST define `SpaceChildDto` with optional `AlkemioRoomID`, optional `AlkemioContextID`, and optional `Order`.

### Key Entities

- **AlkemioContextID**: UUID identifying a Space/Context in Alkemio. Maps 1:1 to a Matrix Space (room with type=m.space). Currently corresponds to Authorization Policy ID in Alkemio.
- **JoinRule**: Enum controlling room/space access. `public` = anyone can join, `invite` = requires invitation, `restricted` = membership inherited from parent Space.
- **SpaceChildDto**: Represents an entry in a Space's hierarchy. Contains either a Room reference or nested Space reference, plus optional ordering.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 8 new Space-related commands respond successfully within 2 seconds for typical operations.
- **SC-002**: Room creation with hierarchy fields (`parent_context_id`, `join_rule`) works on first attempt for 95% of valid requests.
- **SC-003**: Batch Space membership operations correctly report per-Space success/failure in all cases.
- **SC-004**: Space hierarchy queries (`communication.space.get`) return accurate child lists reflecting current Matrix state.
- **SC-005**: All new DTOs are correctly generated in TypeScript library via `make generate`.
- **SC-006**: Existing V2 command functionality (excluding removed `Limit` field) continues to work without regression.

## Assumptions

- Matrix homeserver supports MSC1772 (Spaces) as implemented in Synapse.
- The adapter maintains persistent mappings between AlkemioContextID and Matrix Room IDs (similar to AlkemioRoomID mappings).
- `m.space.parent` and `m.space.child` state events follow the Matrix Spaces specification.
- The `restricted` join rule requires the Matrix homeserver to support MSC3083 (Restricted Rooms).
