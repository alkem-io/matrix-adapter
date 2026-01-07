# Quickstart: DB-Based Actor ID Mapper

**Feature**: 009-db-actor-id-mapper
**Date**: 2026-01-06

## Prerequisites

- Go 1.25+
- SQLC CLI (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`)
- Access to Alkemio PostgreSQL database
- Existing Matrix Adapter codebase

## Quick Enable

To enable the DB-based actor ID mapper, set these environment variables:

```bash
export ACTOR_ID_MAPPER_ENABLED=true
export DATABASE_HOST=localhost      # or 'postgres' in Docker
export DATABASE_PORT=5432
export DATABASE_USERNAME=synapse
export DATABASE_PASSWORD=synapse
export DATABASE_NAME=alkemio
```

Then restart the Matrix Adapter.

## Development Setup

### 1. Install SQLC

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

### 2. Generate Database Code

After modifying SQL queries:

```bash
cd internal/infrastructure/alkemiodb
sqlc generate
```

### 3. Add Dependencies

```bash
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
```

### 4. Run Tests

```bash
make test
```

## Configuration Reference

| Variable | Default | Required When Enabled | Description |
|----------|---------|----------------------|-------------|
| ACTOR_ID_MAPPER_ENABLED | false | - | Master toggle |
| DATABASE_HOST | postgres | Yes | PostgreSQL hostname |
| DATABASE_PORT | 5432 | No | PostgreSQL port |
| DATABASE_USERNAME | synapse | Yes | Database user |
| DATABASE_PASSWORD | synapse | Yes | Database password |
| DATABASE_NAME | alkemio | Yes | Database name |

## Behavior Summary

### When Disabled (Default)

- Matrix user IDs use actor ID directly as localpart
- Example: Actor `abc123...` → Matrix `@abc123...:homeserver`
- No database connection required

### When Enabled

- Matrix user IDs use user.id or virtual_contributor.id as localpart
- Example: Actor `abc123...` → Lookup → User `xyz789...` → Matrix `@xyz789...:homeserver`
- Database connection required at startup (fails if unavailable)
- All lookups must succeed (no fallback to direct mapping)

## Error Scenarios

| Scenario | Behavior |
|----------|----------|
| DB unreachable at startup | Adapter fails to start |
| DB unreachable during operation | Operation returns error |
| Actor ID not found in user/VC tables | Operation returns error |
| Entity ID not found (reverse lookup) | Operation returns error |

## File Locations

| Purpose | Path |
|---------|------|
| Port interface | `internal/core/ports/alkemiodb.go` |
| Domain errors | `internal/core/domain/errors.go` |
| DB adapter | `internal/infrastructure/alkemiodb/adapter.go` |
| SQLC queries | `internal/infrastructure/alkemiodb/queries.sql` |
| SQLC config | `internal/infrastructure/alkemiodb/sqlc.yaml` |
| Generated code | `internal/infrastructure/alkemiodb/db/` |

## Testing

### Unit Tests

Unit tests mock the `ActorResolver` interface:

```go
type mockActorResolver struct {
    resolveActorFn func(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error)
    resolveEntityFn func(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error)
}

func (m *mockActorResolver) ResolveActorToEntityID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error) {
    return m.resolveActorFn(ctx, actorID)
}
```

### Integration Tests

Integration tests require a running PostgreSQL with Alkemio schema:

```bash
# Start test database
docker-compose -f docker-compose.test.yml up -d postgres

# Run integration tests
go test -tags=integration ./internal/infrastructure/alkemiodb/...
```

## Removal Guide

When the migration period ends:

1. Set `ACTOR_ID_MAPPER_ENABLED=false` in production
2. Remove files:
   - `internal/core/ports/alkemiodb.go`
   - `internal/infrastructure/alkemiodb/` (entire directory)
3. Remove from `internal/app/app.go`:
   - ActorResolver initialization
   - `idMapper.SetActorResolver()` call
4. Remove from `internal/config/config.go`:
   - `ActorResolver` config section
   - Related env var loading
5. Remove dependencies:
   ```bash
   go mod tidy
   ```
