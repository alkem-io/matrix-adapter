<!-- Implements constitution & agents.md. Does not introduce new governance. -->

<!--
Sync Impact Report
Version change: INITIAL → 2.0.0 (Go Implementation Rewrite)
Modified principles: All (adapted for Go/Mautrix)
Added sections: N/A
Removed sections: N/A
Templates requiring updates:
 - .specify/templates/plan-template.md (Constitution Check alignment)
 - .specify/templates/spec-template.md (no changes required)
 - .specify/templates/tasks-template.md (testing guidance note)
Deferred TODOs: None
-->

# Alkemio Matrix Adapter Engineering Constitution (Go Implementation)

See also [`agents.md`](../../agents.md) and [`copilot-instructions.md`](../../.github/copilot-instructions.md) for operational guidance derived from this document.

## Core Principles

### 1. Adapter-First Domain Isolation

All Matrix SDK interactions MUST be encapsulated within adapter domain modules (under `internal/infrastructure/matrix`). Adapters provide a clean abstraction boundary between the `mautrix-go` primitives and application-level concerns. No service, handler, or controller may directly import or instantiate `mautrix-go` clients. Adapter modules expose domain-specific methods, DTOs, and events—never raw Matrix SDK types. Any PR bypassing adapters to access SDK internals directly MUST refactor before merge.

**Rationale**: The Matrix protocol is complex and the `mautrix-go` library is an external dependency. Domain isolation protects the service from SDK API churn, enables independent testing without Matrix homeserver dependencies, and provides a stable internal contract for the rest of the application.

### 2. Event-Driven State Synchronization

All Matrix room state changes, timeline events, and membership updates MUST propagate through the internal event bus (Watermill). Synchronous state polling or blocking SDK calls in request handlers are forbidden. Event handlers subscribe to domain-specific topics (e.g., `room.message.received`) and react to state changes asynchronously. The service architecture assumes eventual consistency; write operations return acknowledgment of command acceptance, not confirmation of remote Matrix state.

**Rationale**: Matrix operates on an eventually consistent, federated protocol. Blocking request flows on remote homeserver sync creates cascading timeout failures and unpredictable latency. Go channels and Watermill provide robust message passing and backpressure control that align naturally with Matrix's event-driven architecture.

### 3. Microservice Contract Stability

The RabbitMQ microservice interface (command patterns and data payloads) is a published contract consumed by the Alkemio Server. Breaking changes to command structures, payload schemas, or response formats REQUIRE versioned message types and backward-compatible handlers during transition periods. All incoming commands MUST validate payloads at the boundary and return typed errors mapped to standardized error codes. New commands require documentation of: purpose, payload schema (via `pkg/dto`), expected responses, and error conditions.

**Rationale**: The Matrix Adapter is a critical dependency for the Alkemio platform's communication layer. Uncoordinated contract changes cause cascading failures across the ecosystem. The shared library approach enforces compile-time contract verification and enables coordinated rollout strategies.

### 4. Matrix Client Lifecycle Management

Matrix client instances (Intents) MUST be managed efficiently. While `mautrix-go` handles connection pooling, the application MUST ensure that user intents are initialized on-demand and resources are released when appropriate. Client creation failures, sync errors, and authentication rejections MUST emit structured errors. Silent client abandonment is forbidden.

**Rationale**: Matrix clients maintain state and connections. Unmanaged client accumulation leads to memory exhaustion and homeserver load spikes. Explicit lifecycle management ensures resource efficiency.

### 5. Observability with Matrix Context

Every adapter operation MUST log contextual identifiers: `userID`, `roomID`, `eventID`, and correlation IDs from inbound commands. Use structured logging (`zap`) for all operational logs. Metrics and tracing are currently DEFERRED for the MVP phase. Silent failure paths (swallowed exceptions, ignored sync errors) are forbidden.

**Rationale**: Matrix's federated and stateful nature creates complex failure modes that manifest as subtle inconsistencies rather than explicit errors. Rich contextual logging enables post-incident timeline reconstruction.

### 6. Pragmatic Testing with SDK Boundaries

Tests exist to defend adapter contract invariants and observable behaviors that matter. Unit tests MUST mock the Matrix SDK boundary (using interfaces in `internal/core/ports`) and verify adapter logic without homeserver dependencies. Integration tests against Synapse fixtures are REQUIRED only when adding new Matrix SDK interactions or complex event flows. 100% coverage is NOT required; tests MUST stay maintainable and purposeful.

**Rationale**: Testing against a live Matrix homeserver is slow, brittle, and introduces non-deterministic federation timing. Focused unit tests at the adapter boundary provide fast feedback on business logic correctness. Integration tests guard against SDK API misuse.

### 7. Go Service as Source of Truth

The `pkg/dto` package in the Go service defines the canonical command payloads, event types, and DTO structures. The shared TypeScript library (`@alkem-io/matrix-adapter-go-lib`) MUST be generated from these Go definitions (e.g., using `cmd/gen-events` or `tygo`). Manual edits to the TypeScript definitions are forbidden.

**Rationale**: Centralizing the source of truth in the Go service ensures that the implementation and the contract remain in sync. Automated generation prevents drift between the producer (Go) and consumer (Node.js/TypeScript).

### 8. Secure Matrix Credential Management

Matrix access tokens, registration shared secrets, and user credentials MUST never appear in logs, error messages, or exported metrics. All secrets are sourced from environment configuration or secure vaults—no hardcoded defaults.

**Rationale**: Matrix access tokens grant full user impersonation capabilities. Credential leakage enables room hijacking, message spoofing, and unauthorized user enumeration across the federation.

### 9. Container Determinism and Configuration

Container images MUST be reproducible: explicit base image tags, locked dependency versions (`go.mod`/`go.sum`), no runtime builds. Matrix homeserver URLs, sync endpoints, and authentication methods are deployment-time configuration—not hardcoded.

**Rationale**: The Matrix Adapter's reliability depends on predictable behavior and homeserver compatibility. Non-deterministic builds or dynamic configuration changes introduce subtle protocol incompatibilities.

### 10. Simplicity and Matrix API Coverage

Implement only Matrix features explicitly required by Alkemio platform use cases. Do not speculatively wrap Matrix SDK capabilities "for future use." New Matrix API surface area requires written justification referencing concrete user stories or operational requirements.

**Rationale**: The Matrix specification is vast and evolving. Indiscriminate API surface expansion creates maintenance burden and testing debt without delivering value. Incremental hardening based on observed needs maintains service focus.

## Architecture Standards

1. **Directory Layout**:
   - `internal/core/domain`: Domain models and business entities.
   - `internal/core/ports`: Interfaces (ports) for driving and driven adapters.
   - `internal/core/service`: Application business logic (use cases).
   - `internal/infrastructure/matrix`: Matrix adapter implementation (`mautrix-go`).
   - `internal/infrastructure/queue`: RabbitMQ adapter implementation (`watermill`).
   - `pkg/dto`: Public Data Transfer Objects and contracts.
   - `cmd/`: Application entry points.

2. **Matrix SDK Encapsulation**: Direct `mautrix-go` imports are ONLY permitted in `internal/infrastructure/matrix`. All other code must use domain ports.

3. **Error Handling**: Matrix SDK errors MUST map to application error codes defined in `pkg/dto` or domain errors. Generic `error` propagation across the microservice boundary is forbidden without context.

4. **Configuration Validation**: All Matrix homeserver URLs and credentials MUST validate at service startup. Invalid configuration prevents application start.

## Engineering Workflow

1. **PRs MUST state**: New commands or events added, adapter contract changes, integration test coverage.

2. **Matrix SDK Upgrades**: Require regression testing of core flows and CHANGELOG review for breaking changes.

3. **New Adapter Features**: Provide domain rationale, SDK method mapping, and error scenarios.

4. **Contract Changes**: Run `make generate` to update TypeScript definitions and commit the changes.

5. **Incident Learnings**: Create or refine a principle or Architecture Standard within 5 business days.

## Governance

Amendments require: proposal PR referencing impacted principles, rationale, and version bump classification. Semantic versioning of this constitution:

- **MAJOR**: Removal or redefinition of a principle.
- **MINOR**: Addition of a new principle or architecture standard.
- **PATCH**: Clarifications without behavioral change.

**Compliance Review**:

- Constitution Check section in planning MUST reference any intentional deviations.
- Unjustified violations block merge.

**Enforcement**:

- Automated lint / CI may enforce adapter isolation and credential masking.
- Manual review ensures event stream discipline and testing adequacy.

**Version**: 2.0.0 | **Ratified**: 2025-11-25 | **Last Amended**: 2025-11-25
