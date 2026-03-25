# Tasks: Wire joinRule for Rooms & Remove isPublic

**Input**: Design documents from `/specs/013-space-room-params/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Unit tests are specified in SC-004 of the spec. Dedicated test-writing tasks are included in Phase 5; existing tests are validated there as well.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Foundational (Port Interface Changes)

**Purpose**: Update the MatrixPort interface — all user stories depend on this.

**CRITICAL**: These changes will temporarily break compilation. All callers must be updated in the user story phases.

- [x] T001 Add `joinRule string` parameter to `CreateRoomWithAlias` in `internal/core/ports/matrix.go` (insert after `avatarURL` param, before `initialMembers`)
- [x] T002 Add `joinRule string` parameter to `UpdateRoomState` in `internal/core/ports/matrix.go` (insert after `avatarURL` param, before `alias`)

**Checkpoint**: Port interface updated. Compilation will be broken until adapter and service are updated.

---

## Phase 2: User Story 1 - Create a Room with Controlled Visibility (Priority: P1) MVP

**Goal**: Wire `joinRule` from CreateRoomRequest DTO through handler → service → port → adapter so rooms are created with the correct Matrix join rule.

**Independent Test**: Send a createRoom command with `join_rule: "public"` and verify the Matrix room has a public join rule.

### Implementation for User Story 1

- [x] T003 [US1] Add `joinRule string` parameter to `CreateRoomWithAlkemioID` in `internal/core/service/room_service.go` (after `avatarURL`, before `initialMembers`). Pass it through to `s.matrix.CreateRoomWithAlias()` call. For direct-message rooms (`roomType == "direct"`), ignore joinRule (pass empty string). When joinRule is empty (omitted by caller), pass empty string so the adapter preserves existing default preset behavior (FR-005).
- [x] T004 [US1] Add `joinRule string` parameter to `CreateRoomWithAlias` implementation in `internal/infrastructure/matrix/mautrix.go`. When `joinRule != ""` and `roomType != "direct"`, add a `m.room.join_rules` state event to `req.InitialState` (follow the exact pattern from `CreateSpace` at ~line 1453-1463). When joinRule is provided for non-direct rooms, also switch preset from `"public_chat"` to `"private_chat"` to let the explicit state event control visibility. Note: when joinRule is empty (omitted), the existing preset logic remains unchanged — `"public_chat"` for community rooms, `"trusted_private_chat"` for DMs (FR-005).
- [x] T005 [US1] Update `HandleCreateRoom` in `internal/infrastructure/queue/handler_room.go` to pass `string(req.JoinRule)` to the service call `CreateRoomWithAlkemioID`.

**Checkpoint**: createRoom now respects `joinRule`. Verify with `make build`.

---

## Phase 3: User Story 2 - Update a Room's Visibility (Priority: P1)

**Goal**: Wire `joinRule` from UpdateRoomRequest DTO through handler → service → port → adapter so room visibility can be changed after creation.

**Independent Test**: Send an updateRoom command with `join_rule: "public"` on a private room and verify the Matrix room join rule changes.

### Implementation for User Story 2

- [x] T006 [US2] Replace `_ *bool` (isPublic) parameter with `joinRule *string` in `UpdateRoomMetadata` in `internal/core/service/room_service.go`. Dereference joinRule to a string value and pass it to `s.matrix.UpdateRoomState()` call (follow the pattern from `UpdateSpace` at ~line 186). When joinRule is nil (omitted by caller), pass empty string so the adapter skips the join rule state event (FR-005).
- [x] T007 [US2] Add `joinRule string` parameter to `UpdateRoomState` implementation in `internal/infrastructure/matrix/mautrix.go`. When `joinRule != ""`, send a `m.room.join_rules` state event using `intent.SendStateEvent()` (follow the exact pattern from `UpdateSpaceState` at ~line 1600-1607).
- [x] T008 [US2] Update `HandleUpdateRoom` in `internal/infrastructure/queue/handler_room.go` to convert `req.JoinRule` to `*string` and pass it to `UpdateRoomMetadata` instead of `req.IsPublic`. Follow the pattern from `HandleUpdateSpace` at ~line 145-151.

**Checkpoint**: updateRoom now respects `joinRule`. Verify with `make build`.

---

## Phase 4: User Story 3 - Remove isPublic from UpdateRoomRequest (Priority: P2)

**Goal**: Remove the redundant, never-implemented `isPublic` field from the DTO and regenerate the TypeScript library.

**Independent Test**: Confirm `is_public` no longer appears in Go DTO or generated TypeScript.

### Implementation for User Story 3

- [x] T009 [US3] Remove `IsPublic *bool` field (and its JSON tag) from `UpdateRoomRequest` struct in `pkg/dto/room.go`
- [x] T010 [US3] Regenerate TypeScript library by running `make generate`. Verify `is_public` no longer appears in `lib/src/dto/generated.ts` for `UpdateRoomRequest`.

**Checkpoint**: `isPublic` fully removed. Verify with `make build && make generate`.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Validation, tests, and final checks.

- [x] T011 [P] Write or update unit tests for `CreateRoomWithAlkemioID` in the appropriate test file under `internal/core/service/` to cover: joinRule provided (`public`, `invite`), joinRule omitted (empty string), and joinRule on direct-message room (ignored). Mock `MatrixPort.CreateRoomWithAlias` and assert the joinRule value passed. (SC-004)
- [x] T012 [P] Write or update unit tests for `UpdateRoomMetadata` in the appropriate test file under `internal/core/service/` to cover: joinRule provided, joinRule omitted (nil), and verify the correct value is passed to `MatrixPort.UpdateRoomState`. (SC-004)
- [x] T013 Run `make lint` to verify no lint issues across all modified files
- [x] T014 Run `make test` to verify all tests pass (existing + new)
- [x] T015 Verify space operations (createSpace, updateSpace) remain unaffected — no changes to `internal/core/service/space_service.go` or `internal/infrastructure/queue/handler_space.go`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Foundational (Phase 1)**: No dependencies — start immediately. BLOCKS all user stories.
- **User Story 1 (Phase 2)**: Depends on Phase 1 completion.
- **User Story 2 (Phase 3)**: Depends on Phase 1 completion. Can run in parallel with US1 (different methods/code paths), but sequencing after US1 is simpler since both touch the same files.
- **User Story 3 (Phase 4)**: Depends on US2 completion (handler must stop referencing `req.IsPublic` before field removal).
- **Polish (Phase 5)**: Depends on all user stories being complete.

### Execution Order

```
T001, T002 (foundational, sequential - same file)
  → T003, T004, T005 (US1 - sequential, cross-file dependencies)
    → T006, T007, T008 (US2 - sequential, cross-file dependencies)
      → T009, T010 (US3 - sequential)
        → T011, T012 (unit tests - parallel, different test files)
          → T013, T014, T015 (validation - T013/T015 parallel, then T014)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Port interface changes
2. Complete Phase 2: US1 — createRoom joinRule wiring
3. **STOP and VALIDATE**: `make build` passes, createRoom applies joinRule

### Incremental Delivery

1. Phase 1 + Phase 2 → createRoom works with joinRule (MVP)
2. Add Phase 3 → updateRoom works with joinRule
3. Add Phase 4 → isPublic removed, TS lib clean
4. Phase 5 → All validation passes

---

## Notes

- All changes follow the existing space pattern — no new patterns introduced
- Total: 15 tasks across 5 files + TS regeneration + unit tests
- Commit after each phase for clean git history
