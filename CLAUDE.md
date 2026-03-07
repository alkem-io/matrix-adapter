# matrix-adapter-go Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-03-07

## Active Technologies
- Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging) (010-room-state-events)
- N/A (no new persistence) (010-room-state-events)

- Go 1.25 + mautrix-go, Watermill, SQLC (new), pgx/v5 (new PostgreSQL driver) (009-db-actor-id-mapper)

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
- 010-room-state-events: Added Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)

- 009-db-actor-id-mapper: Added Go 1.25 + mautrix-go, Watermill, SQLC (new), pgx/v5 (new PostgreSQL driver)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
