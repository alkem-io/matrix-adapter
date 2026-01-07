# Feature Specification: Temporary DB-Based Actor ID Mapper

**Feature Branch**: `009-db-actor-id-mapper`
**Created**: 2026-01-06
**Status**: Draft
**Input**: User description: "Add temporary mapper for actor ID conversion via Alkemio DB (PostgreSQL). Convert actorId (agent.id) to user.id or virtual_contributor.id for Matrix localpart, with toggle to enable/disable."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Forward Actor ID Conversion (Priority: P1)

When Alkemio Server sends a command with an actor ID (agent UUID), the Matrix Adapter must look up the corresponding user or virtual contributor ID in the Alkemio database and use that ID as the Matrix user localpart instead of the agent ID.

**Why this priority**: This is the core functionality required for the migration period. Without forward conversion, users cannot be correctly identified in Matrix.

**Independent Test**: Can be fully tested by sending a room join command with an actor ID and verifying the Matrix user is created with the correct user/VC ID as localpart.

**Acceptance Scenarios**:

1. **Given** an actor ID that maps to a user in the Alkemio database, **When** the adapter receives a command with this actor ID, **Then** the system looks up the agent record, finds the user record with matching agentId, and uses the user.id as the Matrix localpart.

2. **Given** an actor ID that maps to a virtual contributor in the Alkemio database, **When** the adapter receives a command with this actor ID, **Then** the system looks up the agent record, finds the virtual_contributor record with matching agentId, and uses the virtual_contributor.id as the Matrix localpart.

3. **Given** the DB mapper feature is disabled via configuration, **When** the adapter receives a command with an actor ID, **Then** the system uses the original behavior (actor ID directly as localpart).

---

### User Story 2 - Reverse Matrix User ID Conversion (Priority: P1)

When the Matrix Adapter receives events from Matrix containing user IDs, it must reverse-lookup the Matrix localpart to find the original Alkemio actor ID (agent ID) to include in outbound events.

**Why this priority**: Equally critical as forward conversion - without this, Matrix events cannot be correlated back to Alkemio actors.

**Independent Test**: Can be tested by receiving a Matrix event with a user ID and verifying the outbound Alkemio event contains the correct actor ID.

**Acceptance Scenarios**:

1. **Given** a Matrix user ID with localpart matching a user.id in the Alkemio database, **When** the adapter processes an event from this user, **Then** the system looks up the user record, retrieves the agentId, and uses it as the actor ID in the outbound event.

2. **Given** a Matrix user ID with localpart matching a virtual_contributor.id in the Alkemio database, **When** the adapter processes an event from this user, **Then** the system looks up the virtual_contributor record, retrieves the agentId, and uses it as the actor ID in the outbound event.

3. **Given** the DB mapper feature is disabled via configuration, **When** the adapter processes a Matrix event, **Then** the system uses the original behavior (localpart directly as actor ID).

---

### User Story 3 - Feature Toggle Control (Priority: P2)

Operators must be able to enable or disable the DB-based actor ID mapping via configuration, allowing for gradual rollout and easy rollback.

**Why this priority**: Important for operational control but not strictly required for the core conversion functionality.

**Independent Test**: Can be tested by toggling the configuration and verifying the system switches between DB-based and direct ID mapping.

**Acceptance Scenarios**:

1. **Given** the DB mapper is configured as enabled, **When** the adapter starts, **Then** it establishes a database connection and uses DB-based lookups for all actor ID conversions.

2. **Given** the DB mapper is configured as disabled (default), **When** the adapter starts, **Then** it uses the original direct ID mapping without database access.

3. **Given** the DB mapper is configured as enabled but database is unreachable, **When** the adapter attempts to start, **Then** startup fails entirely with a clear error message.

---

### Edge Cases

- What happens when actor ID lookup fails (no matching agent record)? System returns an error for the operation and logs error-level message.
- What happens when agent exists but has no matching user or virtual_contributor? System returns an error for the operation and logs error-level message.
- What happens when database connection fails during operation? System returns an error for the affected operation and logs error-level message.
- What happens when the same localpart could theoretically match both a user and virtual_contributor? The system checks user table first; if found, uses that. Otherwise checks virtual_contributor. (UUIDs are disjoint in Alkemio, so this should not occur in practice.)
- What happens when DB mapper is enabled but database is unreachable at startup? Adapter fails to start entirely with a clear error message.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a configuration option to enable/disable the DB-based actor ID mapper (disabled by default).
- **FR-002**: When enabled, system MUST connect to the Alkemio PostgreSQL database using configurable connection parameters.
- **FR-003**: For forward mapping (Alkemio → Matrix), system MUST query user table by agentId first, then virtual_contributor table if not found, to retrieve the corresponding entity ID.
- **FR-004**: For reverse mapping (Matrix → Alkemio), system MUST lookup the user or virtual_contributor by their ID (from Matrix localpart), then retrieve the agentId as the actor ID.
- **FR-005**: When DB mapper is enabled, system MUST return an error for the operation if DB lookup fails (no silent fallback to direct mapping).
- **FR-006**: System MUST log all DB lookup failures with appropriate error level for operational visibility.
- **FR-007**: System MUST isolate all database access logic to allow easy removal when the temporary mapping is no longer needed.
- **FR-008**: System MUST use read-only database access (no writes to Alkemio database).
- **FR-009**: When DB mapper is enabled, system MUST fail startup if database connection cannot be established.

### Configuration

| Environment Variable   | Default Value | Description                              |
|------------------------|---------------|------------------------------------------|
| ACTOR_ID_MAPPER_ENABLED | false         | Enable/disable DB-based actor ID mapping |
| DATABASE_HOST          | postgres      | Alkemio database host                    |
| DATABASE_PORT          | 5432          | Alkemio database port                    |
| DATABASE_USERNAME      | synapse       | Database username                        |
| DATABASE_PASSWORD      | synapse       | Database password                        |
| DATABASE_NAME          | alkemio       | Database name                            |

### Key Entities

- **Agent**: Represents an actor in Alkemio. Contains `id` (UUID) which is the actor ID used in communications.
- **User**: A human user in Alkemio. Contains `id` (UUID) and `agentId` (foreign key to Agent).
- **VirtualContributor**: An AI-based contributor in Alkemio. Contains `id` (UUID) and `agentId` (foreign key to Agent).

### Database Tables Involved

| Table               | Relevant Columns   | Purpose              |
|---------------------|--------------------|----------------------|
| agent               | id (PK)            | Source of actor ID   |
| user                | id (PK), agentId (FK) | User entity lookup   |
| virtual_contributor | id (PK), agentId (FK) | VC entity lookup     |

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: System correctly maps 100% of valid actor IDs to their corresponding user/VC IDs when feature is enabled.
- **SC-003**: System returns clear error responses for all database failures when DB mapper is enabled (no silent fallback).
- **SC-004**: Feature can be enabled/disabled via environment variable (service restart required).
- **SC-005**: All DB mapping code is isolated in dedicated modules, allowing removal with changes to no more than 3 files outside the mapping module itself.

## Clarifications

### Session 2026-01-06

- Q: What should happen when DB lookup fails (connection error, no matching record)? → A: No fallback to direct mapping when DB mapper is enabled; failures must be explicit errors.
- Q: Startup behavior when DB mapper enabled but DB unreachable? → A: Fail startup entirely - adapter won't start without DB connection.

## Assumptions

- The Alkemio database schema has the `agent`, `user`, and `virtual_contributor` tables with the described structure.
- UUIDs are globally unique across user and virtual_contributor tables (no collision possible).
- The Matrix Adapter has network access to the Alkemio PostgreSQL database.
- This feature is temporary and will be removed after the migration period.
- Read-only database access is sufficient; no schema migrations are required.
