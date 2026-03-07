# Quickstart: Remove DB Actor ID Mapper

**Feature**: 011-remove-db-actor-mapper
**Date**: 2026-03-07

## Overview

Remove the temporary database-based Actor ID Mapper. The adapter should always use direct mode (actor UUID = Matrix localpart). This is a pure removal — no new functionality.

## Files to Delete

### Infrastructure Package
- `internal/infrastructure/alkemiodb/` — Entire directory (adapter.go, sqlc.yaml, schema.sql, queries.sql, db/*.go)

### Port Interface
- `internal/core/ports/alkemiodb.go` — ActorResolver port definition

## Files to Modify

### Domain Layer
- `internal/core/domain/idmapper.go` — Remove ActorResolver interface, SetActorResolver, HasActorResolver, simplify UserID() and AlkemioActorIDWithContext()
- `internal/core/domain/idmapper_test.go` — Remove resolver-related tests
- `internal/core/domain/errors.go` — Remove ErrActorNotFound, ErrEntityNotFound (verify unused elsewhere first)
- `internal/core/domain/errors_test.go` — Remove related test cases

### Configuration Layer
- `internal/config/config.go` — Remove ActorResolver struct, loadActorResolverEnv()

### App Wiring
- `internal/app/app.go` — Remove actorResolver field, conditional init block, cleanup in Stop()

### Infrastructure Layer
- `internal/infrastructure/matrix/mautrix.go` — Remove SetActorResolver()
- `internal/infrastructure/queue/errors.go` — Remove ErrActorNotFound mapping (if error removed)

### Dependencies
- `go.mod` / `go.sum` — Remove pgx/v5 dependency via `go mod tidy`

### Documentation
- `README.md` — Remove Actor ID Mapper section, DATABASE_* config table
- `CLAUDE.md` — Remove 009 references
- `MatrixAdapterProtocol_V3.md` — Remove ID Mapping Modes section
- `.claude/CLAUDE.md` — Remove ActorResolver references

## Build & Test

```bash
make build      # Verify compilation after removals
make test       # Run unit tests
make lint       # Check style
```

## Patterns to Follow

- **IDMapper simplification**: `UserID()` should become like `RoomAlias()` — pure string formatting, no error return needed
- **AlkemioActorIDWithContext simplification**: Should become like `AlkemioActorID()` — direct UUID parsing
- **Error removal**: Check all usages of ErrActorNotFound/ErrEntityNotFound before removing
- **go mod tidy**: Run after removing all pgx imports to clean dependencies
