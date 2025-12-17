# Alkemio Matrix Adapter (Go)

A high-performance, stateless Matrix Adapter service written in Go, implementing the Alkemio Matrix Adapter specification.

[![Build Status](https://app.travis-ci.com/alkem-io/matrix-adapter-go.svg?branch=develop)](https://app.travis-ci.com/alkem-io/matrix-adapter-go.svg?branch=develop)
[![Coverage Status](https://coveralls.io/repos/github/alkem-io/matrix-adapter-go/badge.svg?branch=develop)](https://coveralls.io/github/alkem-io/matrix-adapter-go?branch=develop)
[![Deploy to DockerHub](https://github.com/alkem-io/matrix-adapter-go/actions/workflows/build-release-docker-hub.yml/badge.svg)](https://github.com/alkem-io/matrix-adapter-go/actions/workflows/build-release-docker-hub.yml)


This repository contains two core elements:
* **Matrix Adapter Service**: The Go application that provides the runtime service.
* **Matrix Adapter Library**: A TypeScript library (`lib/`) providing shared payloads and event types for clients of the service.

## Features

- **Hexagonal Architecture**: Clean separation of core domain, ports, and adapters.
- **Stateless Design**: Designed for horizontal scalability.
- **Matrix AppService**: Connects to Matrix Homeserver as an Application Service.
- **RabbitMQ Integration**: Consumes commands and publishes events via AMQP.
- **Type-Safe DTOs**: Shared contract definitions with TypeScript generation.
- **Actor ID Support**: Native support for Alkemio Actor IDs (UUIDs) mapped to Matrix IDs.

## Prerequisites

- Go 1.25+
- Docker & Docker Compose
- Matrix Homeserver (Synapse/Dendrite)
- RabbitMQ
- Node.js 22+ / pnpm 9+ (for Library)

## Configuration

The service is configured via environment variables or a `config.yaml` file. The environment variables are compatible with the legacy TypeScript service.

| Variable | Description | Default |
|----------|-------------|---------|
| `ENVIRONMENT` | Environment (development, production) | `development` |
| `LOGGING_LEVEL_CONSOLE` | Logging level (debug, info, warn, error) | `info` |
| `SYNAPSE_SERVER_URL` | URL of the Matrix Homeserver | - |
| `SYNAPSE_HOMESERVER_NAME` | Matrix Homeserver Name | - |
| `MATRIX_AS_TOKEN` | AppService Token (as_token) | - |
| `MATRIX_HS_TOKEN` | Homeserver Token (hs_token) | - |
| `MATRIX_BOT_ACTOR_ID` | Bot Actor ID (UUID), also used as Matrix localpart | `00000000-0000-0000-0000-000000000000` |
| `RABBITMQ_URL` | Full AMQP Connection URL | - |
| `RABBITMQ_HOST` | RabbitMQ Host (if URL not set) | - |
| `RABBITMQ_PORT` | RabbitMQ Port (if URL not set) | `5672` |
| `RABBITMQ_USER` | RabbitMQ User (if URL not set) | - |
| `RABBITMQ_PASSWORD` | RabbitMQ Password (if URL not set) | - |

## Running Locally

1. **Install Dependencies**:
   ```bash
   make deps
   ```

2. **Run the Service**:
   ```bash
   make run
   ```

## Building

### Service (Go)
```bash
make build
```

### Library (TypeScript)
```bash
cd lib
pnpm install
pnpm build
```

## Docker

### Build Image
```bash
make docker-build
```

### Run with Docker Compose
```bash
docker-compose up --build
```

## Development

- **Linting**: `make lint`
- **Testing**: `make test`
- **Test Coverage**: `make test-coverage`
- **Format**: `make fmt`
- **Generate DTOs**: `make generate` (Updates Go DTOs and generates TypeScript definitions in `lib/`)

## TypeScript Library Publishing

The shared TypeScript library (`lib/`) is automatically published via GitHub Actions workflows.

### Publishing Targets

| Trigger | Registry | Package Name | Version | Tag |
|---------|----------|--------------|---------|-----|
| Git tag (`v*`) | **npmjs** | `@alkemio/matrix-adapter-lib` | From tag (e.g., `v1.2.3` → `1.2.3`) | `latest` |
| Pull Request | GitHub Packages | `@alkem-io/matrix-adapter-go-lib` | `0.0.0-pr-{number}-{sha}` | `canary` |
| Manual dispatch | GitHub Packages | `@alkem-io/matrix-adapter-go-lib` | `0.0.0-manual-{sha}` | `canary` |

### Release Process

1. **Development**: PRs that modify `lib/` or `pkg/dto/` automatically publish canary versions to GitHub Packages for testing.

2. **Production Release**: Create a git tag to publish to npmjs:
   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```

3. **Install from npmjs** (production):
   ```bash
   npm install @alkemio/matrix-adapter-lib
   ```

4. **Install from GitHub Packages** (development/testing):
   ```bash
   npm install @alkem-io/matrix-adapter-go-lib@canary --registry=https://npm.pkg.github.com
   ```

### Authentication

- **npmjs**: Uses OIDC trusted publishing (no token required) with provenance attestation.
- **GitHub Packages**: Uses `GITHUB_TOKEN` automatically provided by Actions.

## Architecture

The project follows a Hexagonal (Ports & Adapters) architecture:

- `cmd/`: Entrypoints (main application wiring).
- `internal/app/`: Application lifecycle, dependency injection, and startup logic.
- `internal/config/`: Configuration loading and validation.
- `internal/core/domain`: Pure business logic and entities.
- `internal/core/ports`: Interfaces defining interactions with the outside world.
- `internal/core/service`: Implementation of business use cases.
- `internal/infrastructure`: Adapters for external systems (Matrix, RabbitMQ, HTTP).
- `pkg/dto`: Data Transfer Objects shared with external consumers (source for `tygo` generation).
- `lib/`: Shared TypeScript library containing generated DTOs and event types.

## Actor ID Usage

The Matrix Adapter uses Actor IDs (UUIDs) to identify users.
- **Actor ID**: The unique identifier of the user in the Alkemio platform (UUID).
- **Matrix ID**: The identifier of the user in the Matrix homeserver (e.g., `@uuid:server`).

The adapter automatically handles the mapping between Actor IDs and Matrix IDs.
All external APIs (RabbitMQ commands and events) use Actor IDs.

## Protocol

The Matrix Adapter implements a structured RabbitMQ protocol for communication with the Alkemio Server. See [MatrixAdapterProtocol_V3.md](MatrixAdapterProtocol_V3.md) for the full specification.

### Supported Commands

| Category | Command | Topic |
|----------|---------|-------|
| **Room** | Create Room | `communication.room.create` |
| | Get Room | `communication.room.get` |
| | Update Room | `communication.room.update` |
| | Delete Room | `communication.room.delete` |
| | List Rooms | `communication.room.list` |
| | Get Room Members | `communication.room.members.get` |
| | Batch Add Member | `communication.room.member.batch.add` |
| | Batch Remove Member | `communication.room.member.batch.remove` |
| **Space** | Create Space | `communication.space.create` |
| | Get Space | `communication.space.get` |
| | Update Space | `communication.space.update` |
| | Delete Space | `communication.space.delete` |
| | List Spaces | `communication.space.list` |
| | Batch Add Member | `communication.space.member.batch.add` |
| | Batch Remove Member | `communication.space.member.batch.remove` |
| **Hierarchy** | Set Parent | `communication.hierarchy.set_parent` |
| **Message** | Send Message | `communication.message.send` |
| | Get Message | `communication.message.get` |
| | Delete Message | `communication.message.delete` |
| **Reaction** | Add Reaction | `communication.reaction.add` |
| | Remove Reaction | `communication.reaction.remove` |
| | Get Reaction | `communication.reaction.get` |
| **Thread** | Get Thread Messages | `communication.thread.messages.get` |
| **Actor** | Sync Actor Profile | `communication.actor.sync` |
| **Read Receipts** | Mark Message Read | `communication.message.read` |
| | Get Unread Counts | `communication.room.unread_counts.get` |

### Outgoing Events

| Event | Topic | Description |
|-------|-------|-------------|
| Message Received | `communication.message.received` | Emitted when a message is received in a room |
| DM Requested | `communication.room.dm.requested` | Emitted when a DM room creation is requested via webhook |
| Reaction Added | `communication.reaction.added` | Emitted when a user adds a reaction to a message |
| Reaction Removed | `communication.reaction.removed` | Emitted when a user removes a reaction from a message |
| Room Member Left | `communication.room.member.left` | Emitted when a user leaves or is kicked from a room |
| Read Receipt Updated | `communication.room.receipt.updated` | Emitted when a user's read position is updated |
| Message Edited | `communication.message.edited` | Emitted when a message is edited |
| Message Redacted | `communication.message.redacted` | Emitted when a message is deleted/redacted |
| Room Created | `communication.room.created` | Emitted when a room is created in Matrix |
| Room Member Updated | `communication.room.member.updated` | Emitted when a user's membership status changes (join, invite, etc.) |

## DM Room Creation Flow

The adapter supports controlled DM (Direct Message) room creation through a webhook-based flow. This allows Synapse to request approval from the Alkemio Server before creating DM rooms.

### Flow Overview

```
┌─────────┐        ┌──────────────┐        ┌─────────────────┐        ┌───────────┐
│ Synapse │  1.    │   Adapter    │   2.   │  Alkemio Server │   3.   │  Adapter  │
│  Spam   │───────▶│   Webhook    │───────▶│   (via RabbitMQ)│───────▶│  (via     │
│ Checker │ POST   │   Handler    │ Publish│                 │ Command│  RabbitMQ)│
└─────────┘        └──────────────┘        └─────────────────┘        └───────────┘
                                                                            │
                                                    4. Create DM Room       │
                                                    (type: "direct")        ▼
                                                                     ┌───────────┐
                                                                     │   Matrix  │
                                                                     │Homeserver │
                                                                     └───────────┘
```

1. **Synapse Spam Checker** calls the adapter's webhook when a user attempts to create a DM
2. **Adapter** publishes `DMRequestedEvent` to `communication.room.dm.requested` topic
3. **Alkemio Server** processes the request and sends `communication.room.create` with `type: "direct"`
4. **Adapter** creates the DM room in Matrix using existing room creation logic

### Webhook Endpoint

| Method | Path | Auth |
|--------|------|------|
| POST | `/_matrix/app/alkemio/dm-request` | Bearer token (HS token) |

### Request Payload

```json
{
  "inviter": "@550e8400-e29b-41d4-a716-446655440001:matrix.alkemio.org",
  "invitee": "@660e8400-e29b-41d4-a716-446655440002:matrix.alkemio.org"
}
```

### Response

Success (202 Accepted):
```json
{"status": "accepted"}
```

Error responses use standard HTTP error codes with JSON error messages:
- `401 Unauthorized`: Invalid or missing Bearer token
- `400 Bad Request`: Invalid payload or missing fields
- `500 Internal Server Error`: Failed to publish event

### DM Room Creation Command

The Alkemio Server creates DM rooms using the existing `communication.room.create` command with `type: "direct"`:

```json
{
  "alkemio_room_id": "new-uuid-for-dm-room",
  "type": "direct",
  "name": "DM: User A - User B",
  "initial_members": ["actor-uuid-1", "actor-uuid-2"],
  "join_rule": "invite"
}
```

### Response Structure

All responses follow a standard envelope:

```json
{
  "success": true,
  "error": null
}
```

Error responses include structured error information:

```json
{
  "success": false,
  "error": {
    "code": "ROOM_NOT_FOUND",
    "message": "Room with ID xyz does not exist",
    "details": "Optional technical details"
  }
}
```

### Error Codes

| Code | Description |
|------|-------------|
| `INVALID_PARAM` | Request validation failed (invalid payload or parameters) |
| `ROOM_NOT_FOUND` | Referenced room does not exist |
| `SPACE_NOT_FOUND` | Referenced space does not exist |
| `ACTOR_NOT_FOUND` | Referenced actor does not exist |
| `MATRIX_ERROR` | Matrix SDK/homeserver error |
| `INTERNAL_ERROR` | Unexpected system error |
| `NOT_ALLOWED` | Operation not permitted |
