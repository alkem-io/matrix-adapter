# Tasks: Remove DB Actor ID Mapper

**Input**: Design documents from `/specs/011-remove-db-actor-mapper/`
**Prerequisites**: plan.md (required), spec.md (required), quickstart.md

**Organization**: Tasks are grouped by user story. This is a removal feature — tasks progress from infrastructure deletion through code simplification to documentation cleanup.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: No setup needed — this is a removal feature working on existing codebase.

(No tasks)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Verify current state before making changes

- [x] T001 Verify all tests pass before starting removal by running `make test`
- [x] T002 Verify build succeeds before starting removal by running `make build`

**Checkpoint**: Baseline verified — removal can begin

---

## Phase 3: User Story 1 - Remove Database Dependency (Priority: P1) 🎯 MVP

**Goal**: Remove the alkemiodb infrastructure package, database adapter, SQLC config, and all database connection code.

**Independent Test**: `make build && make test` pass with zero database-related imports.

### Implementation for User Story 1

- [x] T003 [US1] Delete the entire `internal/infrastructure/alkemiodb/` directory (adapter.go, sqlc.yaml, schema.sql, queries.sql, db/*.go)
- [x] T004 [US1] Delete the ActorResolver port file `internal/core/ports/alkemiodb.go`
- [x] T005 [US1] Remove the `actorResolver` field and all conditional DB init logic from `internal/app/app.go` (lines ~27-28 field, ~62-86 init block, ~176-178 cleanup)
- [x] T006 [US1] Remove `SetActorResolver()` method from `internal/infrastructure/matrix/mautrix.go`
- [x] T007 [US1] Remove `ErrActorNotFound` and `ErrEntityNotFound` sentinels and their constructors from `internal/core/domain/errors.go`
- [x] T008 [US1] Remove `ErrActorNotFound` mapping case from `internal/infrastructure/queue/errors.go` (line ~94-95)
- [x] T009 [US1] Remove `ErrActorNotFound`/`ErrEntityNotFound` test cases from `internal/core/domain/errors_test.go` and `internal/infrastructure/queue/errors_test.go`
- [x] T010 [US1] Run `go mod tidy` to remove pgx/v5 and other unused dependencies from go.mod/go.sum
- [x] T011 [US1] Verify `make build && make test` pass after all removals

**Checkpoint**: Database dependency fully removed. Adapter has zero DB-related code.

---

## Phase 4: User Story 2 - Clean Configuration Surface (Priority: P2)

**Goal**: Remove all database-related configuration options.

**Independent Test**: Configuration loads without reading any DATABASE_* or ACTOR_ID_MAPPER_ENABLED env vars.

### Implementation for User Story 2

- [x] T012 [US2] Remove `ActorResolver` struct, `ActorResolver` field from Config, and `loadActorResolverEnv()` function from `internal/config/config.go`
- [x] T013 [US2] Verify `make build && make test` pass after config cleanup

**Checkpoint**: Configuration surface cleaned — no database-related options remain.

---

## Phase 5: User Story 3 - Simplify ID Mapping Interface (Priority: P3)

**Goal**: Simplify IDMapper to always use direct mapping, removing ActorResolver interface and conditional logic.

**Independent Test**: IDMapper has single code path for all conversions, no resolver branching.

### Implementation for User Story 3

- [x] T015 [US3] Remove `ActorResolver` interface, `SetActorResolver()`, `HasActorResolver()`, `isReservedUUID()`, reserved UUID variables, and `AlkemioActorIDWithContext()` from `internal/core/domain/idmapper.go`
- [x] T016 [US3] Simplify `UserID()` in `internal/core/domain/idmapper.go`: remove context parameter and error return, make it a pure string formatter like `RoomAlias()` (returns `id.UserID` directly)
- [x] T017 [US3] Update all callers of `UserID(ctx, actorID)` to use the simplified signature (no context, no error): `internal/infrastructure/matrix/mautrix.go`, `internal/core/service/room_service.go`, `internal/core/service/space_service.go`
- [x] T018 [US3] Update all callers of `AlkemioActorIDWithContext(ctx, userIDStr)` to use the existing `AlkemioActorID(userID)`: `internal/infrastructure/matrix/listener.go`, `internal/infrastructure/http/dm_webhook.go`, `internal/infrastructure/queue/handler_room.go`, `internal/core/service/room_service.go`, `internal/core/service/space_service.go`
- [x] T019 [US3] Update `internal/core/domain/idmapper_test.go`: remove all resolver-related tests, simplify remaining tests to match new signatures
- [x] T020 [US3] Verify `make build && make test && make lint` pass after IDMapper simplification

**Checkpoint**: IDMapper fully simplified — single direct-mapping code path.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation updates and final cleanup

- [x] T021 [P] Remove Actor ID Mapper section, DATABASE_* config table, and DB-mapped mode from `README.md`
- [x] T022 [P] Remove ID Mapping Modes section and DB-mapped references from `MatrixAdapterProtocol_V3.md`
- [x] T023 [P] Remove 009-db-actor-id-mapper references from `CLAUDE.md`
- [x] T024 [P] Remove ActorResolver and DB mapper references from `.claude/CLAUDE.md`
- [x] T025 Run `make build && make test && make lint` as final verification

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: N/A
- **Foundational (Phase 2)**: Baseline verification — BLOCKS all removals
- **US1 (Phase 3)**: Depends on Phase 2 — removes DB code
- **US2 (Phase 4)**: Depends on Phase 3 — removes config that references removed code
- **US3 (Phase 5)**: Depends on Phase 4 — simplifies IDMapper after resolver is gone
- **Polish (Phase 6)**: Depends on Phase 5 — documentation reflects final state

### User Story Dependencies

- **US1 → US2 → US3**: Sequential dependency chain (each builds on previous removal)
- US2 depends on US1 because config references the alkemiodb adapter type
- US3 depends on US1+US2 because IDMapper simplification requires resolver code to be gone first

### Parallel Opportunities

- T003 and T004 can run in parallel (different directories)
- T007, T008, T009 can run in parallel (different files) but depend on T003-T006
- T021, T022, T023, T024 can all run in parallel (different documentation files)
- T017 and T018 could be parallelized across different files but touch overlapping concerns

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 2: Verify baseline
2. Complete Phase 3: Remove all DB code
3. **STOP and VALIDATE**: `make build && make test` must pass
4. The adapter now works in direct mode only

### Incremental Delivery

1. US1: Remove DB code → build/test pass → functional adapter without DB
2. US2: Remove config → build/test pass → clean config surface
3. US3: Simplify IDMapper → build/test/lint pass → clean codebase
4. Polish: Update docs → final verification

---

## Notes

- This is a removal feature — most tasks delete code rather than write it
- Each phase MUST end with a passing build+test to catch breakage early
- The IDMapper simplification (US3) is the most complex part — it changes method signatures affecting ~8 caller files
- Run `go mod tidy` only after ALL pgx-importing code is removed
- `isReservedUUID` and reserved UUID variables exist solely for the ActorResolver bypass — they are removed along with it
