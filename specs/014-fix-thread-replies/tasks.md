# Tasks: Fix Thread Reply Formatting and Message Ordering

**Input**: Design documents from `/specs/014-fix-thread-replies/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md
**Status**: Retrofit — all implementation tasks are already complete

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No setup needed — existing project, no new dependencies or files

N/A — all changes are within the existing codebase.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: No foundational work needed — no new entities, no schema changes, no new dependencies

N/A — this is a contained bug fix in existing adapter methods.

**Checkpoint**: No prerequisites — user story implementation can proceed directly.

---

## Phase 3: User Story 1 - Thread replies display correctly in Matrix clients (Priority: P1) 🎯 MVP

**Goal**: Thread replies include proper MSC3440 `m.thread` relation alongside `m.in_reply_to` so replies appear within threads in all Matrix clients.

**Independent Test**: Send a reply to a threaded message and verify it appears within the thread in Element, not as a standalone room message.

### Implementation for User Story 1

- [ ] T001 [US1] Add `Type: event.RelThread`, `EventID: threadID`, and `IsFallingBack: true` fields to the `RelatesTo` struct in `SendReply` method in `internal/infrastructure/matrix/mautrix.go` (~line 498)

**Checkpoint**: Thread replies sent via `SendReply` now include proper MSC3440 thread relations with backwards-compatible fallback.

---

## Phase 4: User Story 2 - Thread messages are returned in chronological order (Priority: P1)

**Goal**: `GetThreadMessages` returns messages in chronological order (root first, replies ascending) accounting for the relations API's newest-first ordering.

**Independent Test**: Fetch messages for a thread with 3+ replies and verify root is first, replies follow in chronological order.

### Implementation for User Story 2

- [ ] T002 [US2] Refactor root message parsing to use a deferred `rootMsg` variable instead of immediately appending to `messages` slice in `GetThreadMessages` in `internal/infrastructure/matrix/mautrix.go` (~line 1163)
- [ ] T003 [US2] Add nil guard for `rootMsg` on the error/no-relations path in `GetThreadMessages` in `internal/infrastructure/matrix/mautrix.go` (~line 1180)
- [ ] T004 [US2] Move root message append to after the reply loop so root is last in the slice (consumer's `.reverse()` puts it first) in `GetThreadMessages` in `internal/infrastructure/matrix/mautrix.go` (~line 1200)

**Checkpoint**: Thread message retrieval returns correctly ordered messages with graceful nil-root handling.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Validation and cleanup

- [ ] T005 Run `make build` to verify compilation
- [ ] T006 Run `make lint` to verify linting passes
- [ ] T007 Run `make test` to verify existing tests pass
- [ ] T008 Run quickstart.md manual verification steps

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: N/A
- **Foundational (Phase 2)**: N/A
- **User Story 1 (Phase 3)**: No dependencies — can start immediately
- **User Story 2 (Phase 4)**: No dependencies — can start immediately (different method than US1)
- **Polish (Phase 5)**: Depends on both user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Independent — modifies `SendReply` only
- **User Story 2 (P1)**: Independent — modifies `GetThreadMessages` only

### Parallel Opportunities

- US1 (T001) and US2 (T002-T004) modify different methods in the same file but non-overlapping regions — they can be implemented in parallel
- T005, T006, T007 can run in parallel after implementation

---

## Parallel Example: Both User Stories

```bash
# Both stories touch different methods in the same file — can be done in parallel:
Task T001: "Add thread relation fields to SendReply in internal/infrastructure/matrix/mautrix.go"
Task T002-T004: "Fix message ordering in GetThreadMessages in internal/infrastructure/matrix/mautrix.go"

# Validation tasks can run in parallel:
Task T005: "make build"
Task T006: "make lint"
Task T007: "make test"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete T001 — thread replies display correctly
2. **STOP and VALIDATE**: Verify in Element that replies appear in threads
3. Deploy if ready — this alone fixes the most visible user-facing bug

### Incremental Delivery

1. T001 → Thread reply formatting fixed → Validate (MVP)
2. T002-T004 → Thread message ordering fixed → Validate
3. T005-T008 → Full validation pass → Ready for merge

---

## Notes

- All implementation tasks (T001-T004) are already complete in unstaged changes
- Only validation tasks (T005-T008) remain to be executed
- Total: 8 tasks (4 implementation + 4 validation)
- Both user stories are independent and touch different methods
