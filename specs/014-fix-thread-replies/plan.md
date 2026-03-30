# Implementation Plan: Fix Thread Reply Formatting and Message Ordering

**Branch**: `014-fix-thread-replies` | **Date**: 2026-03-30 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/014-fix-thread-replies/spec.md`
**Status**: Retrofit — implementation already complete in unstaged changes

## Summary

Fix two bugs in the Matrix adapter's thread handling: (1) `SendReply` now emits proper MSC3440 thread relations (`m.thread` with `rel_type`, `event_id`, `is_falling_back`) alongside the existing `m.in_reply_to`, ensuring thread replies display correctly in all Matrix clients; (2) `GetThreadMessages` fixes message ordering by accounting for the relations API returning newest-first and appending the root message last so the consumer's `.reverse()` places it first.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)
**Storage**: N/A (no persistence changes)
**Testing**: Go `testing` package, `testify`
**Target Platform**: Linux server (Docker container)
**Project Type**: Microservice (Matrix adapter)
**Performance Goals**: N/A (bug fix, no performance-sensitive changes)
**Constraints**: Must comply with MSC3440 threading spec; must maintain backwards compatibility with non-thread-aware clients
**Scale/Scope**: 2 methods modified in 1 file (`internal/infrastructure/matrix/mautrix.go`)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | PASS | Changes are entirely within `internal/infrastructure/matrix/mautrix.go` — the adapter boundary |
| 2. Event-Driven State Synchronization | PASS | No changes to event flow or state sync patterns |
| 3. Microservice Contract Stability | PASS | No changes to RabbitMQ command/response payloads or `pkg/dto` |
| 4. Matrix Client Lifecycle Management | PASS | No changes to client/intent lifecycle |
| 5. Observability with Matrix Context | PASS | Existing debug logging preserved |
| 6. Pragmatic Testing with SDK Boundaries | PASS | Changes are in the adapter layer; unit tests should mock the MatrixPort interface |
| 7. Go Service as Source of Truth | PASS | No DTO or contract changes |
| 8. Secure Matrix Credential Management | PASS | No credential handling changes |
| 9. Container Determinism and Configuration | PASS | No build or config changes |
| 10. Simplicity and Matrix API Coverage | PASS | Fixes existing thread functionality, no new Matrix API surface |

**Gate result**: ALL PASS — no violations.

## Project Structure

### Documentation (this feature)

```text
specs/014-fix-thread-replies/
├── plan.md              # This file
├── spec.md              # Feature specification (retrofit)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── checklists/
    └── requirements.md  # Spec quality checklist
```

### Source Code (repository root)

```text
internal/infrastructure/matrix/
└── mautrix.go           # SendReply (~line 498) and GetThreadMessages (~line 1160)
```

**Structure Decision**: No new files or directories. All changes are contained within the existing `mautrix.go` adapter file.

## Complexity Tracking

> No constitution violations — this section is empty.
