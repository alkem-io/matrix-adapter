# matrix-adapter Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-11-28

## Active Technologies
- **Runtime**: Go 1.25
- **Matrix SDK**: mautrix-go v0.26.0
- **Messaging**: Watermill (RabbitMQ)
- **Logging**: Zap
- **TS Generation**: tygo
- **Storage**: Stateless - uses Matrix room/space aliases for ID mapping (no external DB)

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
- 008-read-receipts: Added Go 1.25 + mautrix-go v0.26.0, watermill (RabbitMQ), zap (logging)
- 007-rmq-events-extension: Added Go 1.25 + mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)
- 006-room-creation-control: Added Go 1.25, Python 3.11+ (Synapse module) + mautrix-go, Watermill (RabbitMQ), aiohttp (Python), net/http (Go)


<!-- MANUAL ADDITIONS START -->

## Architectural Decision: mautrix Types

Services use mautrix-go types (`id.RoomID`, `id.EventID`, `id.UserID`) directly - this is intentional, not technical debt.

**Rules**:
- Services MAY import `maunium.net/go/mautrix/id` for type definitions
- Services MUST NOT import other mautrix packages or make direct SDK calls
- All Matrix operations MUST go through `ports.MatrixPort` interface

<!-- MANUAL ADDITIONS END -->
