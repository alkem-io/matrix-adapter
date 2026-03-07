# Feature Specification: Remove DB Actor ID Mapper

**Feature Branch**: `011-remove-db-actor-mapper`
**Created**: 2026-03-07
**Status**: Draft
**Input**: Remove the temporary DB-based Actor ID Mapper introduced in 009-db-actor-id-mapper. The code should always behave as if ACTOR_ID_MAPPER_ENABLED=false (direct mode).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Remove Database Dependency (Priority: P1)

The temporary database-based Actor ID Mapper was introduced during a migration period to resolve Alkemio actor IDs (agent.id) to entity IDs (user.id / virtual_contributor.id) via the Alkemio PostgreSQL database. This migration period is over. The adapter should no longer depend on or connect to the Alkemio database. All ID mapping should use direct mode: actor IDs are used directly as Matrix localparts without any database lookup.

**Why this priority**: Removing the database dependency is the core objective. It eliminates an external system dependency, simplifies deployment, reduces configuration surface, and removes code that is no longer needed.

**Independent Test**: Can be fully tested by deploying the adapter without any DATABASE_* environment variables and without ACTOR_ID_MAPPER_ENABLED, and verifying all operations (room creation, member management, event handling) work correctly using direct actor ID mapping.

**Acceptance Scenarios**:

1. **Given** the adapter is deployed without any database configuration, **When** a command uses an actor ID (e.g., creating a room with initial members), **Then** the actor ID is used directly as the Matrix user localpart without any database lookup.
2. **Given** the adapter receives a Matrix event from a user, **When** it extracts the Alkemio actor ID from the Matrix user ID, **Then** it parses the localpart directly as the actor UUID without any database lookup.
3. **Given** the adapter is started without ACTOR_ID_MAPPER_ENABLED or DATABASE_* environment variables, **Then** it starts successfully without errors or warnings about missing database configuration.

---

### User Story 2 - Clean Configuration Surface (Priority: P2)

Operators and deployment manifests currently include configuration options for a feature that is no longer used. The removed configuration options (ACTOR_ID_MAPPER_ENABLED, DATABASE_HOST, DATABASE_PORT, DATABASE_NAME, DATABASE_USERNAME, DATABASE_PASSWORD) should no longer be recognized or documented, reducing confusion and deployment complexity.

**Why this priority**: Configuration cleanup follows naturally from the code removal and ensures operators are not confused by obsolete options.

**Independent Test**: Can be verified by reviewing the configuration loading code and confirming no database-related environment variables are read, and by checking that documentation no longer references these options.

**Acceptance Scenarios**:

1. **Given** the adapter's configuration documentation, **When** an operator reviews it, **Then** there are no references to ACTOR_ID_MAPPER_ENABLED or DATABASE_* variables.
2. **Given** the adapter's internal configuration struct, **When** it loads environment variables, **Then** it does not attempt to read any database-related variables.

---

### User Story 3 - Simplify ID Mapping Interface (Priority: P3)

The IDMapper currently has two code paths: one with ActorResolver (database lookup) and one without (direct mapping). With the database mapper removed, the IDMapper should be simplified to always use direct mapping. Methods that accepted a context parameter solely for database access should be simplified where the context is no longer needed for the mapping logic itself.

**Why this priority**: Simplifying the IDMapper is a code quality improvement that makes the codebase easier to maintain. It depends on the database code being removed first.

**Independent Test**: Can be verified by reviewing the IDMapper code and confirming it has no optional resolver, no conditional branching for resolution mode, and all ID conversions use direct UUID-to-localpart mapping.

**Acceptance Scenarios**:

1. **Given** the IDMapper, **When** converting an actor ID to a Matrix user ID, **Then** it always uses the actor UUID directly as the localpart without any resolver lookup.
2. **Given** the IDMapper, **When** extracting an Alkemio actor ID from a Matrix user ID, **Then** it always parses the localpart directly as a UUID without any resolver lookup.
3. **Given** the IDMapper code, **When** reviewed, **Then** there is no ActorResolver interface, no SetActorResolver method, no HasActorResolver method, and no conditional resolver logic.

---

### Edge Cases

- What happens if an operator sets ACTOR_ID_MAPPER_ENABLED=true or DATABASE_* variables in their deployment? These variables should be silently ignored — the adapter no longer reads them.
- What happens to reserved UUID handling (nil UUID, bot UUID)? The `isReservedUUID` check exists only to bypass the ActorResolver lookup. Under direct mapping, all UUIDs (including reserved ones) map directly — no special handling needed. Remove `isReservedUUID` along with the resolver code.
- What happens to error types (ErrActorNotFound, ErrEntityNotFound) used by the database mapper? These should be removed if no other code uses them, or retained if they serve other purposes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The adapter MUST use direct ID mapping for all actor ID conversions: actor UUID is used directly as the Matrix user localpart.
- **FR-002**: The adapter MUST NOT connect to or depend on any external database for ID mapping.
- **FR-003**: The adapter MUST NOT read ACTOR_ID_MAPPER_ENABLED, DATABASE_HOST, DATABASE_PORT, DATABASE_NAME, DATABASE_USERNAME, or DATABASE_PASSWORD from environment variables or configuration.
- **FR-004**: The adapter MUST remove all database-related code including the alkemiodb package, generated database code, SQL schema, and SQL queries.
- **FR-005**: The IDMapper MUST be simplified to always use direct mapping, removing the optional ActorResolver, SetActorResolver, and HasActorResolver.
- **FR-006**: The adapter MUST remove the database driver dependency from its module.
- **FR-007**: The adapter documentation (README, protocol doc) MUST be updated to remove all references to the Actor ID Mapper, database configuration, and DB-mapped mode.
- **FR-008**: All existing functionality (room operations, space operations, messaging, events) MUST continue to work identically to the current ACTOR_ID_MAPPER_ENABLED=false behavior.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The adapter builds and all tests pass without any database-related code or dependencies.
- **SC-002**: The adapter starts and operates correctly without any DATABASE_* environment variables.
- **SC-003**: Zero database-related imports remain in the codebase (no database driver references in application code).
- **SC-004**: The IDMapper has a single code path for all ID conversions (no conditional resolver logic).
- **SC-005**: Documentation contains zero references to ACTOR_ID_MAPPER_ENABLED or database configuration.

## Assumptions

- The migration period that required the DB-based mapper is fully complete — no deployments still rely on ACTOR_ID_MAPPER_ENABLED=true.
- All Alkemio deployments now use actor IDs directly as Matrix localparts (direct mode).
- Reserved UUID handling (`isReservedUUID`) exists solely to bypass the ActorResolver and is removed along with it. Under direct mapping, reserved UUIDs work like any other UUID.
- The ErrActorNotFound and ErrEntityNotFound error types were introduced solely for the DB mapper — if unused elsewhere, they should be removed.
- Deployment manifests in this repository (if any) should also be cleaned of database configuration references.
