# Data Model: DB-Based Actor ID Mapper

**Feature**: 009-db-actor-id-mapper
**Date**: 2026-01-06

## External Entities (Read-Only from Alkemio DB)

These entities exist in the Alkemio PostgreSQL database. The Matrix Adapter reads but never writes to them.

### Agent

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, NOT NULL | Actor ID used in Alkemio ↔ Matrix communication |
| type | VARCHAR(128) | | Entity type: 'user', 'virtual-contributor', 'space', etc. |

**Notes**:
- `type` values relevant to this feature: `'user'`, `'virtual-contributor'`
- Other types ('space', 'account', 'organization') are not used for Matrix user mapping

### User

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, NOT NULL | User's own UUID - used as Matrix localpart |
| agentId | UUID | FK → agent(id), UNIQUE | Reference to the user's agent |

**Notes**:
- Table name requires quotes in SQL: `"user"`
- `agentId` has UNIQUE constraint - one-to-one with agent

### VirtualContributor

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, NOT NULL | VC's own UUID - used as Matrix localpart |
| agentId | UUID | FK → agent(id), UNIQUE | Reference to the VC's agent |

**Notes**:
- `agentId` has UNIQUE constraint - one-to-one with agent

## Domain Types (New)

### ActorResolver Interface

```go
// Package: internal/core/ports

// ActorResolver provides database-backed actor ID resolution.
// When enabled, it converts between Alkemio actor IDs (agent.id) and
// entity IDs (user.id or virtual_contributor.id) for Matrix user mapping.
type ActorResolver interface {
    // ResolveActorToEntityID converts an Alkemio actor ID to the corresponding
    // user or virtual_contributor ID. Returns ErrActorNotFound if no match.
    ResolveActorToEntityID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error)

    // ResolveEntityToActorID converts a user or virtual_contributor ID back to
    // the corresponding Alkemio actor ID. Returns ErrEntityNotFound if no match.
    ResolveEntityToActorID(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error)

    // Close releases database resources.
    Close() error
}
```

### Domain Errors (New)

```go
// Package: internal/core/domain

var (
    // ErrActorNotFound indicates no user or virtual_contributor exists for the actor ID.
    ErrActorNotFound = errors.New("actor not found in user or virtual_contributor tables")

    // ErrEntityNotFound indicates no actor ID found for the given entity ID.
    ErrEntityNotFound = errors.New("entity ID not found in user or virtual_contributor tables")
)
```

## Configuration (Extended)

### ActorResolver Config Section

```go
// Package: internal/config

type Config struct {
    // ... existing fields ...

    ActorResolver struct {
        Enabled  bool   `yaml:"enabled"`   // ACTOR_ID_MAPPER_ENABLED
        Database struct {
            Host     string `yaml:"host"`     // DATABASE_HOST
            Port     int    `yaml:"port"`     // DATABASE_PORT
            Username string `yaml:"username"` // DATABASE_USERNAME
            Password string `yaml:"password"` // DATABASE_PASSWORD
            Name     string `yaml:"name"`     // DATABASE_NAME
        } `yaml:"database"`
    } `yaml:"actor_resolver"`
}
```

### Environment Variable Mapping

| Env Variable | Config Field | Default | Description |
|--------------|--------------|---------|-------------|
| ACTOR_ID_MAPPER_ENABLED | ActorResolver.Enabled | false | Enable DB-based mapping |
| DATABASE_HOST | ActorResolver.Database.Host | postgres | PostgreSQL host |
| DATABASE_PORT | ActorResolver.Database.Port | 5432 | PostgreSQL port |
| DATABASE_USERNAME | ActorResolver.Database.Username | synapse | Database user |
| DATABASE_PASSWORD | ActorResolver.Database.Password | synapse | Database password |
| DATABASE_NAME | ActorResolver.Database.Name | alkemio | Database name |

## SQLC Generated Types

SQLC will generate the following types from queries:

```go
// Package: internal/infrastructure/alkemiodb/db

// GetUserIDByAgentIDRow represents the result of GetUserIDByAgentID query
type GetUserIDByAgentIDRow struct {
    ID pgtype.UUID
}

// GetVCIDByAgentIDRow represents the result of GetVCIDByAgentID query
type GetVCIDByAgentIDRow struct {
    ID pgtype.UUID
}

// GetAgentIDByUserIDRow represents the result of GetAgentIDByUserID query
type GetAgentIDByUserIDRow struct {
    AgentID pgtype.UUID
}

// GetAgentIDByVCIDRow represents the result of GetAgentIDByVCID query
type GetAgentIDByVCIDRow struct {
    AgentID pgtype.UUID
}
```

## Relationships

```
┌─────────────────────────────────────────────────────────────────┐
│                        Alkemio Database                         │
│                                                                 │
│   ┌─────────┐                                                   │
│   │  agent  │                                                   │
│   ├─────────┤                                                   │
│   │ id (PK) │◄─────────────────┐                                │
│   │ type    │                  │                                │
│   └─────────┘                  │                                │
│        ▲                       │                                │
│        │                       │                                │
│   ┌────┴────┐             ┌────┴──────────────┐                 │
│   │  "user" │             │ virtual_contributor │                │
│   ├─────────┤             ├─────────────────────┤                │
│   │ id (PK) │             │ id (PK)             │                │
│   │ agentId │             │ agentId             │                │
│   └─────────┘             └─────────────────────┘                │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

Forward Mapping (actorID → entityID):
  actorID → user.agentId → user.id (Matrix localpart)
  actorID → virtual_contributor.agentId → virtual_contributor.id (Matrix localpart)

Reverse Mapping (entityID → actorID):
  user.id → user.agentId → actorID
  virtual_contributor.id → virtual_contributor.agentId → actorID
```

## Validation Rules

1. **Actor ID Validity**: Must be a valid UUID
2. **Entity ID Validity**: Must be a valid UUID
3. **Mutual Exclusivity**: An actor ID maps to EITHER a user OR a virtual_contributor, never both (enforced by Alkemio schema)
4. **One-to-One**: Each user/VC has exactly one agent (enforced by UNIQUE constraint on agentId)

## State Transitions

Not applicable - this feature performs stateless lookups. No state is managed by the Matrix Adapter.
