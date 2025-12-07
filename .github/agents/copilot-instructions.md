# matrix-adapter-go Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-11-28

## Active Technologies
- Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging) (004-rmq-protocol-update)
- Matrix room aliases (no external DB) (004-rmq-protocol-update)
- Go 1.25 + `mautrix-go` (Matrix SDK), `Watermill` (RabbitMQ), `zap` (Logging), `tygo` (TS generation) (005-protocol-v3)
- Room/Space aliases for ID mapping (no external DB) (005-protocol-v3)
- Go 1.25, Python 3.11+ (Synapse module) + mautrix-go, Watermill (RabbitMQ), aiohttp (Python), net/http (Go) (006-room-creation-control)
- N/A (stateless adapter) (006-room-creation-control)
- Matrix room state (no external DB) (007-rmq-events-extension)

- Go 1.25 + Watermill (RabbitMQ), mautrix-go, zap (logging) (002-rmq-error-handling)

## Project Structure

```text
src/
tests/
```

## Commands

# Add commands for Go 1.25

## Code Style

Go 1.25: Follow standard conventions

## Recent Changes
- 007-rmq-events-extension: Added Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)
- 007-rmq-events-extension: Added [if applicable, e.g., PostgreSQL, CoreData, files or N/A]
- 006-room-creation-control: Added Go 1.25, Python 3.11+ (Synapse module) + mautrix-go, Watermill (RabbitMQ), aiohttp (Python), net/http (Go)


<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
