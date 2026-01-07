# Tasks: Temporary DB-Based Actor ID Mapper

**Input**: Design documents from `/specs/009-db-actor-id-mapper/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md

**Tests**: Tests will be implemented as part of user story delivery (Go testing patterns).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- This is an existing Go service using hexagonal architecture
- New code: `internal/infrastructure/alkemiodb/`, `internal/core/ports/alkemiodb.go`
- Modifications: `internal/config/config.go`, `internal/core/domain/idmapper.go`, `internal/app/app.go`

---

## Phase 1: Setup (Dependencies & SQLC Configuration)

**Purpose**: Add new dependencies and configure SQLC for database access

- [x] T001 Add pgx/v5 and pgxpool dependencies via `go get github.com/jackc/pgx/v5 github.com/jackc/pgx/v5/pgxpool`
- [x] T002 Create directory structure `internal/infrastructure/alkemiodb/` and `internal/infrastructure/alkemiodb/db/`
- [x] T003 [P] Create SQLC configuration in `internal/infrastructure/alkemiodb/sqlc.yaml`
- [x] T004 [P] Create SQL queries file in `internal/infrastructure/alkemiodb/queries.sql` with forward/reverse mapping queries (SELECT only - verify no INSERT/UPDATE/DELETE)
- [x] T005 [P] Create empty schema placeholder in `internal/infrastructure/alkemiodb/schema.sql` (read-only, no migrations)
- [x] T006 Run `sqlc generate` in `internal/infrastructure/alkemiodb/` to generate db/ package

---

## Phase 2: Foundational (Port Interface & Domain Errors)

**Purpose**: Define the port interface and domain errors that all user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T007 [P] Create ActorResolver port interface in `internal/core/ports/alkemiodb.go`
- [x] T008 [P] Add ErrActorNotFound and ErrEntityNotFound errors to `internal/core/domain/errors.go`
- [x] T009 Add ActorResolver configuration section to `internal/config/config.go` with environment variable loading

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Forward Actor ID Conversion (Priority: P1) 🎯 MVP

**Goal**: Convert Alkemio actor IDs to user/VC IDs for Matrix localpart when sending commands

**Independent Test**: Send a room join command with an actor ID, verify Matrix user created with user.id or virtual_contributor.id as localpart

### Implementation for User Story 1

- [x] T010 [US1] Implement AlkemioDBAdapter struct with NewAlkemioDBAdapter constructor in `internal/infrastructure/alkemiodb/adapter.go`
- [x] T011 [US1] Implement ResolveActorToEntityID method in `internal/infrastructure/alkemiodb/adapter.go` (query user table first, then virtual_contributor)
- [x] T012 [US1] Add SetActorResolver method to IDMapper in `internal/core/domain/idmapper.go`
- [x] T013 [US1] Modify IDMapper.UserID method in `internal/core/domain/idmapper.go` to use ActorResolver when set (add context.Context parameter)
- [x] T014 [US1] Add unit test for forward mapping in `internal/core/domain/idmapper_test.go` with mock ActorResolver

**Checkpoint**: Forward actor ID conversion working - can test with actual room join commands

---

## Phase 4: User Story 2 - Reverse Matrix User ID Conversion (Priority: P1)

**Goal**: Convert Matrix user localpart back to Alkemio actor ID for outbound events

**Independent Test**: Receive Matrix event with user ID, verify outbound event contains correct actor ID

### Implementation for User Story 2

- [x] T015 [US2] Implement ResolveEntityToActorID method in `internal/infrastructure/alkemiodb/adapter.go` (query user table first, then virtual_contributor)
- [x] T016 [US2] Modify IDMapper.AlkemioActorID method in `internal/core/domain/idmapper.go` to use ActorResolver when set (add context.Context parameter)
- [x] T017 [US2] Add unit test for reverse mapping in `internal/core/domain/idmapper_test.go` with mock ActorResolver

**Checkpoint**: Both forward and reverse mappings working - core feature complete

---

## Phase 5: User Story 3 - Feature Toggle Control (Priority: P2)

**Goal**: Enable/disable DB mapping via configuration; fail startup if enabled but DB unreachable

**Independent Test**: Toggle ACTOR_ID_MAPPER_ENABLED, verify behavior switches between DB-based and direct mapping

### Implementation for User Story 3

- [x] T018 [US3] Implement Close method for AlkemioDBAdapter in `internal/infrastructure/alkemiodb/adapter.go`
- [x] T019 [US3] Add connection validation at startup in NewAlkemioDBAdapter (fail if cannot connect)
- [x] T020 [US3] Modify `internal/app/app.go` to conditionally initialize AlkemioDBAdapter based on config
- [x] T021 [US3] Wire ActorResolver to IDMapper in `internal/app/app.go` when enabled
- [x] T022 [US3] Add shutdown cleanup for AlkemioDBAdapter in `internal/app/app.go` Stop method
- [x] T023 [US3] Verify disabled mode (ACTOR_ID_MAPPER_ENABLED=false) uses original direct mapping

**Checkpoint**: Feature toggle working - can enable/disable without code changes

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Error handling, logging, and validation

- [x] T024 [P] Add structured logging to AlkemioDBAdapter for all lookup operations in `internal/infrastructure/alkemiodb/adapter.go`
- [x] T025 [P] Add error logging for lookup failures (actor not found, DB errors) with actorID context
- [x] T026 Verify all error paths return typed errors (ErrActorNotFound, ErrEntityNotFound, wrapped DB errors)
- [x] T027 Run `make test` to verify all existing tests still pass
- [x] T028 Run `make lint` to verify code style compliance
- [ ] T029 Test full flow with real Alkemio database (manual validation per quickstart.md)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational - Forward mapping
- **User Story 2 (Phase 4)**: Depends on Foundational - Can start in parallel with US1
- **User Story 3 (Phase 5)**: Depends on US1 + US2 (needs both mapping directions for full toggle)
- **Polish (Phase 6)**: Depends on all user stories complete

### User Story Dependencies

- **User Story 1 (P1)**: Forward mapping - independent after Foundational
- **User Story 2 (P1)**: Reverse mapping - independent after Foundational, can parallelize with US1
- **User Story 3 (P2)**: Toggle control - requires US1 + US2 (needs both mappings to toggle)

### Within Each User Story

- Adapter implementation before IDMapper integration
- IDMapper modification before tests
- Tests verify complete story functionality

### Parallel Opportunities

**Phase 1 Setup:**
```bash
# After T001-T002, these can run in parallel:
Task: T003 "SQLC configuration"
Task: T004 "SQL queries"
Task: T005 "Schema placeholder"
```

**Phase 2 Foundational:**
```bash
# These can run in parallel:
Task: T007 "Port interface"
Task: T008 "Domain errors"
```

**User Stories 1 & 2 (after Foundational):**
```bash
# US1 and US2 can start in parallel (different methods in same files):
# Team Member A: T010-T014 (Forward mapping)
# Team Member B: T015-T017 (Reverse mapping)
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2)

1. Complete Phase 1: Setup (dependencies, SQLC)
2. Complete Phase 2: Foundational (port, errors, config)
3. Complete Phase 3: User Story 1 (forward mapping)
4. Complete Phase 4: User Story 2 (reverse mapping)
5. **STOP and VALIDATE**: Test both mapping directions
6. Core feature complete - can deploy with hardcoded enabled state

### Full Feature (Add User Story 3)

7. Complete Phase 5: User Story 3 (toggle control)
8. Complete Phase 6: Polish
9. Feature complete with operational toggle

### Files Changed Summary

| File | Action | Phase |
|------|--------|-------|
| go.mod | MODIFY | Phase 1 |
| internal/infrastructure/alkemiodb/sqlc.yaml | NEW | Phase 1 |
| internal/infrastructure/alkemiodb/queries.sql | NEW | Phase 1 |
| internal/infrastructure/alkemiodb/schema.sql | NEW | Phase 1 |
| internal/infrastructure/alkemiodb/db/* | GENERATED | Phase 1 |
| internal/core/ports/alkemiodb.go | NEW | Phase 2 |
| internal/core/domain/errors.go | MODIFY | Phase 2 |
| internal/config/config.go | MODIFY | Phase 2 |
| internal/infrastructure/alkemiodb/adapter.go | NEW | Phase 3-5 |
| internal/core/domain/idmapper.go | MODIFY | Phase 3-4 |
| internal/core/domain/idmapper_test.go | NEW | Phase 3-4 |
| internal/app/app.go | MODIFY | Phase 5 |

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- US1 and US2 are both P1 priority but can be parallelized
- US3 depends on both US1 and US2 being complete
- SQLC generates code - run `sqlc generate` after modifying queries.sql
- All DB queries are read-only (SELECT only)
- No fallback to direct mapping when enabled - errors are explicit
