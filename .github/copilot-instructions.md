<!-- Implements constitution & agents.md. Does not introduce new governance. -->

# Copilot Onboarding Guide

## Repository Snapshot

- **Purpose**: Matrix Adapter service for the Alkemio platform; bridges the Alkemio Server and Matrix Homeserver using `mautrix-go` and RabbitMQ (Watermill).
- **Stack**: Go 1.25, `mautrix-go`, Watermill (RabbitMQ), Zap (Logging), TypeScript (for shared lib generation).
- **Structure**: Standard Go layout:
  - `cmd/`: Application entry points (`adapter`, `gen-events`).
  - `internal/`: Private application code.
    - `core/`: Domain logic and ports.
    - `infrastructure/`: Adapters (Matrix, Queue, Logger).
  - `pkg/`: Public libraries (DTOs).
  - `lib/`: Shared TypeScript library (generated from `pkg/dto`).
- **Docs**: `README.md`, `agents.md` (Governance).

## Governance & Workflow

- **Constitution**: Read `[.specify/memory/constitution.md](../.specify/memory/constitution.md)` first. It defines strict rules for Adapter isolation, Event-driven state, and Client lifecycle.
- **Agents**: Follow `[agents.md](../agents.md)` for operational roles and workflow phases (`/spec` → `/plan` → `/implement` → `/done`).
- **Specs**: Feature work should be defined in `specs/<NNN-slug>/` (create if missing) following the standard lifecycle.
- **Shared Contract**: The `pkg/dto` package is the source of truth. TypeScript definitions in `lib/` are GENERATED from Go structs.

## Environment & Toolchain

- **Prerequisites**: Go 1.25+, Node.js (for lib generation), Docker + Compose.
- **Installation**:
  - `go mod download`: Install Go dependencies.
  - `pnpm install` (in `lib/`): Install TS dependencies.
- **Service Setup**:
  - `make build`: Builds the Go binary.
  - `make run`: Runs the service locally.
  - `make test`: Runs unit tests.
- **Lib Setup**:
  - `make generate`: Generates TypeScript definitions from Go code.

## Quality Gates & Validation

- **Lint**: `make lint` (runs `go vet`, `golangci-lint`).
- **Tests**:
  - `make test`: Runs all tests.
  - `make test-coverage`: Runs tests with coverage report.
  - **Constitution Rule**: Unit tests MUST mock the Matrix SDK boundary (`internal/core/ports`). Integration tests against live Synapse are reserved for complex SDK interactions.
- **Build**: `make build` must pass.

## Layout & Key Paths

- **Core (`internal/core/`)**:
  - `domain/`: **Core Logic**. Domain models (`Actor`, `Room`, `Message`).
  - `ports/`: Interfaces for driving (Service) and driven (Matrix, Queue) adapters.
  - `service/`: Application services implementing business logic.
- **Infrastructure (`internal/infrastructure/`)**:
  - `matrix/`: **Mautrix-Go Adapter**. Encapsulates all Matrix SDK interactions.
  - `queue/`: **Watermill Adapter**. Handles RabbitMQ routing and event dispatching.
- **Public (`pkg/`)**:
  - `dto/`: Data Transfer Objects defining the external contract.

## CI & Release Signals

- **GitHub Actions**:
  - `build-release-docker-hub.yml`: Builds and pushes the `alkemio/matrix-adapter-go` Docker image.
- **Versioning**:
  - `VERSION` file or git tags for Go service.
  - `lib/package.json`: Library version (published to npm/GitHub Packages).

## Operational Tips

- **Matrix SDK Isolation**: NEVER import `mautrix-go` directly in `internal/core/service`. Always use `internal/core/ports` interfaces.
- **Event Streams**: Use Watermill for all event publishing/subscribing. Ensure topics match `pkg/dto` definitions.
- **RabbitMQ**: The service listens for commands defined in `pkg/dto`.
- **Debugging**:
  - Use `dlv debug` or VS Code launch configurations for Go.
- **MCP Usage**:
  - Use **GitHub MCP** for repo context.
  - Use **Context7** or **Tavily** for Matrix Spec or Mautrix documentation queries.

## ID Mapping (CRITICAL)

**All Alkemio ↔ Matrix ID conversions MUST use `internal/core/domain.IDMapper`.**

DO NOT create duplicate ID conversion utilities. The IDMapper is the **single source of truth** for:

| Conversion | Method |
|------------|--------|
| Alkemio Room UUID → Matrix Alias | `IDMapper.RoomAlias(uuid)` |
| Alkemio Context UUID → Matrix Space Alias | `IDMapper.SpaceAlias(uuid)` |
| Alkemio Actor UUID → Matrix User ID | `IDMapper.UserID(uuid)` |
| Matrix Room Alias → Alkemio UUID | `IDMapper.AlkemioRoomID(alias)` |
| Matrix Space Alias → Alkemio UUID | `IDMapper.AlkemioContextID(alias)` |
| Matrix User ID → Alkemio Actor UUID | `IDMapper.AlkemioActorID(userID)` |

**Usage patterns:**
- Handlers/Services: Create `idMapper := domain.NewIDMapper(matrix.HomeserverDomain())`
- Infrastructure layer (mautrix.go, listener.go): Has its own `m.idMapper` field

**Infrastructure helper (listener.go only):**

| Helper | Purpose | Uses |
|--------|---------|------|
| `m.resolveAlkemioRoomID(ctx, roomID)` | Get Alkemio UUID from Matrix room ID | `GetRoomDetails()` + `IDMapper.AlkemioRoomID()` |

This helper exists ONLY in `internal/infrastructure/matrix/listener.go` because it needs access to the Matrix adapter's `GetRoomDetails()` method for HTTP lookup. DO NOT duplicate it elsewhere.

**NEVER duplicate these conversions inline or create new utility functions.**

## Active Technologies

See **Stack** in Repository Snapshot. Testing uses the standard Go `testing` package.
