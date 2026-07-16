# Alkemio Matrix Adapter (Go)

A high-performance, stateless Matrix Adapter service written in Go, implementing the Alkemio Matrix Adapter specification.

[![Build Status](https://app.travis-ci.com/alkem-io/matrix-adapter.svg?branch=develop)](https://app.travis-ci.com/alkem-io/matrix-adapter.svg?branch=develop)
[![Coverage Status](https://coveralls.io/repos/github/alkem-io/matrix-adapter/badge.svg?branch=develop)](https://coveralls.io/github/alkem-io/matrix-adapter?branch=develop)
[![Deploy to DockerHub](https://github.com/alkem-io/matrix-adapter/actions/workflows/build-release-docker-hub.yml/badge.svg)](https://github.com/alkem-io/matrix-adapter/actions/workflows/build-release-docker-hub.yml)


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
| `MATRIX_BOT_DISPLAY_NAME` | Display name for the bot user in Matrix | `Alkemio` |
| `SYNAPSE_SERVER_SHARED_SECRET` | Synapse `registration_shared_secret` for auto-promoting bot to server admin | - |
| `FILE_SERVICE_URL` | Optional internal file-service base URL; required only when sending outbound media attachments (`GET {url}/internal/file/{id}/content`) | - |
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
| Pull Request | **pkg.pr.new** | `@alkemio/matrix-adapter-lib` | Commit-based preview URL | — |
| Manual dispatch | **pkg.pr.new** | `@alkemio/matrix-adapter-lib` | Commit-based preview URL | — |

### Release Process

1. **Development**: PRs that modify `lib/` or `pkg/dto/` automatically publish preview versions via [pkg.pr.new](https://pkg.pr.new) for testing.

2. **Production Release**: Create a git tag to publish to npmjs:
   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```

3. **Install from npmjs** (production):
   ```bash
   npm install @alkemio/matrix-adapter-lib
   ```

4. **Install preview** (development/testing):
   ```bash
   npm install https://pkg.pr.new/alkem-io/matrix-adapter/@alkemio/matrix-adapter-lib@{commit-sha}
   ```

### Authentication

- **npmjs**: Uses OIDC trusted publishing (no token required) with provenance attestation.
- **pkg.pr.new**: No authentication required — uses the [pkg.pr.new GitHub App](https://github.com/apps/pkg-pr-new).

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

The Matrix Adapter uses Actor IDs (UUIDs) to identify users in external APIs.

- **Actor ID**: The Alkemio agent ID (`agent.id`) - used in all RabbitMQ commands and events
- **Matrix ID**: The Matrix user identifier (e.g., `@uuid:server`)

Actor IDs are used directly as Matrix localparts (`@actor-uuid:server`).

All external APIs (RabbitMQ commands and events) always use Actor IDs.

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
| **Custom State** | Set Room State | `communication.room.state.set` |
| | Get Room State | `communication.room.state.get` |
| | Set Space State | `communication.space.state.set` |
| | Get Space State | `communication.space.state.get` |

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
| Room Updated | `communication.room.updated` | Emitted when a room's name, avatar, or topic changes |
| Space Updated | `communication.space.updated` | Emitted when a space's name, avatar, or topic changes |

### Media Attachments

`communication.message.send` accepts an optional `attachments` array
(`AttachmentRef`: `document_id`, `display_name`, `mime_type`, `size`,
`width?`, `height?`). For each ref the adapter fetches the document bytes from
file-service (`GET {FILE_SERVICE_URL}/internal/file/{document_id}/content`),
uploads them to the homeserver, and sends one Matrix media event
(`m.image`/`m.video`/`m.audio`/`m.file`, chosen by MIME) carrying the resulting
`url` (mxc), `info` (mime/size/dimensions), and a custom
`io.alkemio.document_id` field. Text plus N attachments becomes one `m.text`
event (when text is present) plus N media events. `content` may be empty when
the message is attachment-only.

Deleting a message redacts its primary event; the attachment media events are
not cascade-redacted (a stateless adapter cannot reliably rediscover them) and
are reclaimed as unreferenced media by Synapse media retention.

Inbound media events are translated onto `communication.message.received`:
each carries a `ReceivedAttachment` (`media_id` parsed from the mxc URL,
`mime_type`/`size`/dimensions from `info`, and `document_id` when the event
echoes our own `io.alkemio.document_id`). The adapter is **stateless** — it only
surfaces these raw refs; the server re-homes media and resolves URLs.

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

## Bot Architecture

The adapter's bot user (`@00000000-...:domain`) operates as a **Synapse server admin** but is **not a member of rooms** (only spaces). This architecture minimizes the bot's footprint in user-visible room lists.

### Bot Behavior

- **Spaces**: Bot stays as a member (required for hierarchy operations)
- **Rooms**: Bot leaves after creation; re-joins temporarily only if no ghost users are available for writes
- **Reads**: All room reads (state, messages, members) use the Synapse Admin API — no membership required
- **Writes**: State events are sent via ghost users (the member with the highest power level)
- **Invites**: Ghost users are joined directly via `EnsureJoined` — no invite events sent to clients

### Server Admin Bootstrap

On startup, the adapter ensures the bot is a Synapse server admin:

1. **Already admin** → proceeds normally
2. **`SYNAPSE_SERVER_SHARED_SECRET` set** → registers bot as admin via shared secret, or creates a temporary admin to promote the bot
3. **No secret** → logs a warning with SQL instructions

### Custom State Events (`io.alkemio.*`)

The adapter supports setting/reading custom state events with the `io.alkemio.` prefix on any room or space. These are used for:

- **`io.alkemio.visibility`**: Controls whether a room appears in Element via the Synapse module
- Custom metadata for rooms/spaces managed by the Alkemio platform

Custom state can be set at room creation (via `custom_state` field) or independently via the `state.set/get` API endpoints.

## Synapse Configuration

The following Synapse `homeserver.yaml` settings are required:

```yaml
# Allow bot to publish rooms to the public directory
room_list_publication_rules:
  - user_id: "@00000000-0000-0000-0000-000000000000:your.domain"
    action: allow
  - action: deny

# Optional: Enable admin bootstrap (alternative to manual DB setup)
registration_shared_secret: "your-secret"
```

### Synapse Module

The adapter includes a companion Synapse module (`alkemio_room_control.py`) that:

1. **Controls room creation** — only the AppService bot can create rooms
2. **Filters `/sync` responses** — hides rooms with `io.alkemio.visibility: {visible: false}` from Element
3. **DM webhook** — notifies the adapter when users attempt to create DMs from Element

## Synapse AppService Registration

The adapter requires an AppService registration file on Synapse. The bot uses `MATRIX_BOT_ACTOR_ID` as its Matrix localpart, following the same UUID pattern as all other actors.

### Example Registration (registration.yaml)

```yaml
id: alkemio-matrix-adapter
url: "http://matrix-adapter:8280"
as_token: <your-as-token>
hs_token: <your-hs-token>
sender_localpart: "00000000-0000-0000-0000-000000000000"  # Must match MATRIX_BOT_ACTOR_ID
namespaces:
  users:
    # Bot user - exclusive (security: prevents impersonation)
    - exclusive: true
      regex: "@00000000-0000-0000-0000-000000000000:.*"
    # Regular UUID users - NOT exclusive so they can login via OIDC/Element
    - exclusive: false
      regex: "@[0-9a-fA-F-]{36}:.*"
  aliases:
    - exclusive: true
      regex: "#[0-9a-fA-F-]{36}:.*"  # Room aliases
rate_limited: false
```

**Important**:
- The `sender_localpart` must match the `MATRIX_BOT_ACTOR_ID` environment variable
- The bot user namespace should be `exclusive: true` to prevent impersonation attacks
