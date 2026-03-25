# Implementation Plan: Wire joinRule for Rooms & Remove isPublic

**Branch**: `013-space-room-params` | **Date**: 2026-03-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/013-space-room-params/spec.md`

## Summary

Wire the existing `joinRule` DTO field end-to-end for `createRoom` and `updateRoom` operations (matching the already-working space pattern), and remove the redundant, never-implemented `isPublic` field from `UpdateRoomRequest`. The implementation follows the exact pattern used by space operations — adding `joinRule` as a parameter through handler → service → port → adapter layers, and applying it as a Matrix `m.room.join_rules` state event.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)
**Storage**: N/A (no persistence changes)
**Testing**: Go `testing` package, `testify`
**Target Platform**: Linux server (Docker container)
**Project Type**: web-service (message-driven adapter)
**Performance Goals**: N/A (no performance-sensitive changes)
**Constraints**: Must not break existing space joinRule handling; isPublic removal is a breaking DTO change
**Scale/Scope**: 5 files modified, ~50 lines changed

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | PASS | All Matrix SDK interactions remain in `internal/infrastructure/matrix`. joinRule is passed as a plain string through service/port layers. |
| 2. Event-Driven State Synchronization | PASS | No changes to event flow. joinRule is set during room creation/update via state events. |
| 3. Microservice Contract Stability | PASS with note | Removing `isPublic` is a breaking change, but the field was never implemented (silently ignored). No callers depend on it. `joinRule` already exists in the DTO. |
| 4. Matrix Client Lifecycle Management | PASS | No client lifecycle changes. |
| 5. Observability with Matrix Context | PASS | Existing logging patterns preserved. |
| 6. Pragmatic Testing with SDK Boundaries | PASS | Unit tests will mock MatrixPort interface. |
| 7. Go Service as Source of Truth | PASS | DTO changes in Go, TS lib regenerated via `make generate`. |
| 8. Secure Matrix Credential Management | PASS | No credential handling changes. |
| 9. Container Determinism | PASS | No build/config changes. |
| 10. Simplicity and Matrix API Coverage | PASS | Wiring an existing, unused DTO field — not adding new Matrix API surface. |

## Project Structure

### Documentation (this feature)

```text
specs/013-space-room-params/
├── spec.md
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── dto-changes.md
└── tasks.md             # Phase 2 output (via /speckit.tasks)
```

### Source Code (files to modify)

```text
pkg/dto/
└── room.go                          # Remove isPublic from UpdateRoomRequest

internal/core/
├── ports/
│   └── matrix.go                    # Add joinRule param to CreateRoomWithAlias, UpdateRoomState
└── service/
    └── room_service.go              # Add joinRule to CreateRoomWithAlkemioID, UpdateRoomMetadata

internal/infrastructure/
├── matrix/
│   └── mautrix.go                   # Add joinRule handling to CreateRoomWithAlias, UpdateRoomState
└── queue/
    └── handler_room.go              # Pass joinRule from DTO to service calls

lib/src/dto/
└── generated.ts                     # Regenerated via make generate
```

## Complexity Tracking

No constitution violations. No complexity justification needed.
