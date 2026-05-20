# matrix-adapter-go Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-03-07

## Active Technologies
- Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging) (010-room-state-events)
- N/A (no new persistence) (010-room-state-events)
- Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging) — removing pgx/v5 and SQLC (011-remove-db-actor-mapper)
- N/A (removing PostgreSQL dependency) (011-remove-db-actor-mapper)
- Go 1.25 + mautrix-go v0.26.0 → v0.26.4 (update), Watermill (RabbitMQ), Zap (logging) (012-fix-unread-counts)
- N/A (no persistence changes) (012-fix-unread-counts)
- N/A (no persistence changes) (013-space-room-params)
- Go 1.25 + Python 3.11 (Synapse module) + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging), Synapse ModuleApi (050-element-room-check)
- N/A (no new persistence; adapter is stateless) (050-element-room-check)

## Project Structure

```text
cmd/       # Application entry points (adapter, gen-events)
internal/  # Private application code (core, infrastructure)
pkg/       # Public packages / DTOs
lib/       # TypeScript library (generated from pkg/dto)
```

## Commands

```bash
make build      # Build the adapter binary
make test       # Run unit tests
make lint       # Run go vet and golangci-lint
make generate   # Generate TypeScript library from Go DTOs
make run        # Run the service locally
```

## Code Style

Go 1.25: Follow standard conventions

## Recent Changes
- 050-element-room-check: Added Go 1.25 + Python 3.11 (Synapse module) + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging), Synapse ModuleApi
- 014-fix-thread-replies: Added Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)
- 013-space-room-params: Added Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
