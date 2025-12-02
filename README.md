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
| `MATRIX_BOT_ACTOR_ID` | Bot Actor ID (UUID) | `00000000-0000-0000-0000-000000000000` |
| `RABBITMQ_URL` | Full AMQP Connection URL | - |
| `RABBITMQ_HOST` | RabbitMQ Host (if URL not set) | - |
| `RABBITMQ_PORT` | RabbitMQ Port (if URL not set) | - |
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
| **Actor** | Sync Actor Profile | `communication.actor.sync` |

### Outgoing Events

| Event | Topic | Description |
|-------|-------|-------------|
| Message Received | `communication.message.received` | Emitted when a message is received in a room |

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
