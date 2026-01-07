# Implementation Plan: Temporary DB-Based Actor ID Mapper

**Branch**: `009-db-actor-id-mapper` | **Date**: 2026-01-06 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/009-db-actor-id-mapper/spec.md`

## Summary

Implement a temporary, toggleable database-backed actor ID mapper that converts between Alkemio actor IDs (agent.id) and user/virtual_contributor IDs for Matrix localpart generation. This addresses a migration period requirement where Matrix user IDs must use user.id or virtual_contributor.id instead of agent.id. The feature uses SQLC for type-safe PostgreSQL queries and integrates with the existing `IDMapper` in the domain layer.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: mautrix-go, Watermill, SQLC (new), pgx/v5 (new PostgreSQL driver)
**Storage**: PostgreSQL (read-only access to Alkemio database)
**Testing**: Go testing package, testify
**Target Platform**: Linux container (Docker)
**Project Type**: Single service (Matrix Adapter)
**Constraints**: Read-only DB access, feature must be toggleable, code must be isolated for easy removal
**Scale/Scope**: Same scale as existing adapter; lookups are per-request

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | ✅ PASS | DB access will be encapsulated in `internal/infrastructure/alkemiodb` adapter |
| 2. Event-Driven State Synchronization | ✅ N/A | Feature is synchronous ID lookup, not event-driven |
| 3. Microservice Contract Stability | ✅ PASS | No changes to RabbitMQ contracts; internal change only |
| 4. Matrix Client Lifecycle Management | ✅ N/A | No Matrix client changes |
| 5. Observability with Matrix Context | ✅ PASS | Will log with actorID, userID context; errors logged |
| 6. Pragmatic Testing with SDK Boundaries | ✅ PASS | Unit tests will mock DB port interface |
| 7. Go Service as Source of Truth | ✅ N/A | No DTO changes |
| 8. Secure Matrix Credential Management | ✅ PASS | DB credentials from env vars only, never logged |
| 9. Container Determinism and Configuration | ✅ PASS | New deps locked in go.mod; config via env vars |
| 10. Simplicity and Matrix API Coverage | ✅ PASS | Minimal, targeted feature for specific migration need |

**No constitution violations. Proceeding to Phase 0.**

## Project Structure

### Documentation (this feature)

```text
specs/009-db-actor-id-mapper/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
# Existing structure with additions marked [NEW]

internal/
├── app/
│   └── app.go                    # [MODIFY] Wire up DB adapter conditionally
├── config/
│   └── config.go                 # [MODIFY] Add ActorResolver config section
├── core/
│   ├── domain/
│   │   └── idmapper.go           # [MODIFY] Add ActorResolver interface injection
│   └── ports/
│       └── alkemiodb.go          # [NEW] Port interface for Alkemio DB
└── infrastructure/
    └── alkemiodb/                # [NEW] Entire directory
        ├── adapter.go            # PostgreSQL adapter implementation
        ├── queries.sql           # SQLC queries
        ├── sqlc.yaml             # SQLC configuration
        └── db/                   # SQLC generated code
            ├── db.go
            ├── models.go
            └── queries.sql.go

tests/
└── unit/
    └── idmapper_test.go          # [NEW] Tests for DB-backed mapper
```

**Structure Decision**: Following existing hexagonal architecture. New `alkemiodb` infrastructure adapter isolated under `internal/infrastructure/` for easy removal. Port interface in `internal/core/ports/` maintains clean boundaries.

## Complexity Tracking

> No constitution violations to justify.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| N/A | - | - |
