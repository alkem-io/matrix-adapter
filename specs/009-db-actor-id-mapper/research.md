# Research: DB-Based Actor ID Mapper

**Feature**: 009-db-actor-id-mapper
**Date**: 2026-01-06

## Database Schema Analysis

### Agent Table

```sql
Table "public.agent"
     Column      |            Type             | Nullable |      Default
-----------------+-----------------------------+----------+--------------------
 id              | uuid                        | not null | uuid_generate_v4()
 type            | character varying(128)      |          |

-- Agent types (from DISTINCT query):
-- 'user', 'virtual-contributor', 'space', 'account', 'organization'
```

**Key Finding**: The `agent.type` column indicates what kind of entity owns this agent. For our use case, we care about `'user'` and `'virtual-contributor'` types.

### User Table

```sql
Table "public.user"
       Column        |    Type      | Nullable
---------------------+--------------+----------
 id                  | uuid         | not null    -- User's own UUID
 agentId             | uuid         |             -- FK to agent(id)
```

**Key Finding**: The `user` table has a quoted name (`"user"`) in PostgreSQL because "user" is a reserved word. SQLC queries must use `"user"` with quotes.

### Virtual Contributor Table

```sql
Table "public.virtual_contributor"
   Column    |  Type   | Nullable
-------------+---------+----------
 id          | uuid    | not null    -- VC's own UUID
 agentId     | uuid    |             -- FK to agent(id)
```

## Query Design

### Forward Mapping: actorID → user/VC ID

Two-step approach (checking agent type first is unnecessary - just try both tables):

```sql
-- Query 1: Try user table first
SELECT id FROM "user" WHERE "agentId" = $1;

-- Query 2: If not found, try virtual_contributor
SELECT id FROM virtual_contributor WHERE "agentId" = $1;
```

**Decision**: Single-query approach using COALESCE is possible but less readable. Using two sequential queries is clearer and easier to debug/log.

**Alternative Considered**: JOIN agent table to check type first, then branch. Rejected because:
- Adds complexity without benefit
- Two simple queries are more maintainable
- Agent type is informational only; we need the actual user/VC ID regardless

### Reverse Mapping: user/VC ID → actorID

```sql
-- Query 1: Try user table first
SELECT "agentId" FROM "user" WHERE id = $1;

-- Query 2: If not found, try virtual_contributor
SELECT "agentId" FROM virtual_contributor WHERE id = $1;
```

## SQLC Configuration

**Decision**: Use SQLC with pgx/v5 driver for type-safe, generated Go code.

**Rationale**:
- Type-safe queries prevent SQL injection
- Generated code eliminates boilerplate
- pgx/v5 is the modern PostgreSQL driver for Go
- SQLC generates models that match our domain types (uuid.UUID)

**Configuration**:
```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries.sql"
    schema: "schema.sql"  # Will be empty - read-only access
    gen:
      go:
        package: "db"
        out: "db"
        sql_package: "pgx/v5"
        emit_json_tags: false
        emit_interface: true
```

## Connection Management

**Decision**: Use connection pool (`pgxpool`) for efficient connection reuse.

**Rationale**:
- Multiple concurrent lookups during message processing
- Pool handles connection lifecycle automatically
- Standard pattern for Go + PostgreSQL

**Configuration**: Pool settings via DATABASE_* environment variables plus standard pgx defaults.

## Error Handling Strategy

**Decision**: Return typed errors that distinguish between:
1. `ErrActorNotFound` - No matching user/VC for the actorID
2. `ErrEntityNotFound` - No matching actorID for the user/VC ID (reverse lookup)
3. Database connection/query errors - Wrapped with context

**Rationale**: Per spec clarification, errors must be explicit (no fallback). Typed errors allow callers to distinguish "not found" from "database unavailable".

## Integration Pattern

**Decision**: Inject `ActorResolver` interface into `IDMapper` via optional setter.

```go
type ActorResolver interface {
    ResolveActorToEntityID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error)
    ResolveEntityToActorID(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error)
}
```

**Rationale**:
- `IDMapper` remains usable without DB dependency (disabled mode)
- Clean separation: domain logic in IDMapper, DB access in infrastructure
- Easy to remove: delete adapter, remove setter call, done

## Performance Considerations

**Decision**: No caching initially.

**Rationale**:
- Actor/Entity ID mappings are immutable during adapter runtime
- Database queries are indexed (PK lookups) - sub-millisecond
- Adding cache adds complexity; measure first, optimize if needed

**Future optimization** (if needed): Add in-memory LRU cache with 5-minute TTL.

## Summary of Decisions

| Topic | Decision | Alternatives Rejected |
|-------|----------|----------------------|
| Query approach | Two sequential queries (user, then VC) | JOIN with agent type check - more complex |
| PostgreSQL driver | pgx/v5 | database/sql - less performant, no native UUID |
| Query generation | SQLC | Raw SQL - no type safety; ORM - too heavy |
| Caching | None initially | LRU cache - premature optimization |
| Error handling | Typed domain errors | Generic errors - insufficient for caller decisions |
| Integration | Interface injection into IDMapper | Wrapping IDMapper - breaks existing callers |
