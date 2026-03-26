# Claude Code Onboarding Guide

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

- **Constitution**: Read `.specify/memory/constitution.md` first. It defines strict rules for Adapter isolation, Event-driven state, and Client lifecycle.
- **Agents**: Follow `agents.md` for operational roles and workflow phases (`/spec` → `/plan` → `/implement` → `/done`).
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

## Synapse Admin API

All Synapse Admin API calls MUST use `SynapseAdmin` (`internal/infrastructure/matrix/synapse_admin.go`). Bare HTTP requests to `/_synapse/admin/` are forbidden outside this package. Add new admin operations as methods on `SynapseAdmin`, not as inline HTTP calls.

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

- **Matrix SDK Isolation**: Services MUST NOT make direct mautrix-go SDK calls (e.g., `client.SendMessage()`). Always use `internal/core/ports.MatrixPort` interface methods. Note: Using mautrix *types* (`id.RoomID`, `id.EventID`, `id.UserID`) in services is acceptable since ports expose these types - see "Architectural Decision: mautrix Types" below.
- **Event Streams**: Use Watermill for all event publishing/subscribing. Ensure topics match `pkg/dto` definitions.
- **RabbitMQ**: The service listens for commands defined in `pkg/dto`.
- **Debugging**:
  - Use `dlv debug` or VS Code launch configurations for Go.

## ID Mapping (CRITICAL)

**All Alkemio <-> Matrix ID conversions MUST use `internal/core/domain.IDMapper`.**

DO NOT create duplicate ID conversion utilities. The IDMapper is the **single source of truth** for:

| Conversion | Method |
|------------|--------|
| Alkemio Room UUID -> Matrix Alias | `IDMapper.RoomAlias(uuid)` |
| Alkemio Context UUID -> Matrix Space Alias | `IDMapper.SpaceAlias(uuid)` |
| Alkemio Actor UUID -> Matrix User ID | `IDMapper.UserID(uuid)` |
| Matrix Room Alias -> Alkemio UUID | `IDMapper.AlkemioRoomID(alias)` |
| Matrix Space Alias -> Alkemio UUID | `IDMapper.AlkemioContextID(alias)` |
| Matrix User ID -> Alkemio Actor UUID | `IDMapper.AlkemioActorID(userID)` |

**Usage patterns:**
- Handlers/Services: Create `idMapper := domain.NewIDMapper(matrix.HomeserverDomain())`
- Infrastructure layer (mautrix.go, listener.go): Has its own `m.idMapper` field

**Infrastructure helper (listener.go only):**

| Helper | Purpose | Uses |
|--------|---------|------|
| `m.resolveAlkemioRoomID(ctx, roomID)` | Get Alkemio UUID from Matrix room ID | `GetRoomDetails()` + `IDMapper.AlkemioRoomID()` |

This helper exists ONLY in `internal/infrastructure/matrix/listener.go` because it needs access to the Matrix adapter's `GetRoomDetails()` method for HTTP lookup. DO NOT duplicate it elsewhere.

**NEVER duplicate these conversions inline or create new utility functions.**

## Future Improvement: User-Scoped Read Operations

Currently, read operations (`GetRoomMessages`, `GetMessage`, `GetThreadMessages`, `GetRoomDetails`, `GetRoomMembers`, `GetReaction`, `GetSpaceDetails`, `GetSpaceChildren`) use `as.BotIntent()` which bypasses Matrix's permission model.

**Future change**: These operations should use `as.Intent(userID)` to ensure users only read what Matrix authorizes them to see. This requires:
1. Adding `actor_id` parameter to all read request DTOs
2. Changing implementations to use user-specific intents
3. Handling Matrix permission errors appropriately

This ensures the adapter respects Matrix's access control rather than relying solely on Alkemio Server authorization.

## Architectural Decision: mautrix Types

The domain, ports, and service layers use mautrix-go types directly (`id.RoomID`, `id.EventID`, `id.UserID`). This is an **intentional design decision**, not technical debt.

**Rationale**:
- This adapter's sole purpose is Matrix integration - abstracting Matrix types adds complexity without practical benefit
- The ports interface (`MatrixPort`) exposes 35+ methods using mautrix types; services must use these types to call port methods
- Consistent throughout: domain models, ports interfaces, and services all use mautrix types

**What this means**:
- Services MAY import `maunium.net/go/mautrix/id` for type definitions
- Services MUST NOT import other mautrix packages or make direct SDK calls
- All Matrix operations MUST go through `ports.MatrixPort` interface

**Current usage** (as of 2025-12):
| Layer | Files with mautrix imports | Purpose |
|-------|---------------------------|---------|
| `domain/` | 3 files | Value objects storing Matrix IDs |
| `ports/` | 2 files | Interface definitions |
| `service/` | 3 files | Type usage for port method calls |

**Alternative considered**: Creating domain type aliases (`domain.MatrixRoomID` as string) and converting at boundaries. This would affect ~20+ files and provide cleaner hexagonal architecture, but was deemed unnecessary given the adapter's focused purpose. This can be revisited if SDK swapping becomes a requirement.

## Active Technologies

- **Runtime**: Go 1.25.
- **Matrix**: `mautrix-go`.
- **Messaging**: Watermill (RabbitMQ).
- **Logging**: Zap.
- **Testing**: Go `testing` package, `testify`.
