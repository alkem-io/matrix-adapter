# Implementation Plan: Element-Initiated Conversation Creation (Synchronous Check)

**Branch**: `050-element-room-check` | **Date**: 2026-05-20 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/050-element-room-check/spec.md`

## Summary

Enable ghost users to create DM and group conversations directly from Element by implementing a synchronous check flow: the Synapse module intercepts `createRoom`, calls the adapter's HTTP check endpoint, the adapter does RabbitMQ request-reply to the server for consent/dedup/entity creation, and on approval injects room state (`io.alkemio.pending` marker, power levels, visibility). After Synapse creates the room, the adapter detects the pending marker on the `m.room.create` event, reconciles the room (bot admin-join, get members from server, EnsureJoined, m.direct, power levels, alias, bot leave), and emits the standard `RoomCreatedEvent`.

## Technical Context

**Language/Version**: Go 1.25 + Python 3.11 (Synapse module)
**Primary Dependencies**: mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging), Synapse ModuleApi
**Storage**: N/A (no new persistence; adapter is stateless)
**Testing**: Go `testing` + `testify`; manual integration tests for Synapse module
**Target Platform**: Linux containers (Docker)
**Project Type**: Web service (Matrix appservice adapter)
**Performance Goals**: Room creation + reconciliation < 5 seconds end-to-end (SC-001/SC-002); check endpoint response < 3 seconds (before Synapse times out)
**Constraints**: Synapse `on_create_room` is synchronous; adapter must respond within timeout. All users are ghost users managed by the appservice. No new persistence layer.
**Scale/Scope**: Single homeserver, ~1000 concurrent users, up to 20 members per group conversation

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Status | Notes |
|---|-----------|--------|-------|
| 1 | Adapter-First Domain Isolation | PASS | New HTTP endpoint uses MatrixPort interface. Reconciliation logic (`ReconcileRoom`) lives on MautrixAdapter (infrastructure layer) because it orchestrates 7+ SDK operations; service layer only handles the check flow via `RoomCheckService` |
| 2 | Event-Driven State Synchronization | PASS | Reconciliation is triggered by appservice transaction event (m.room.create). RoomCreatedEvent published via Watermill after reconciliation |
| 3 | Microservice Contract Stability | PASS | New topics (`communication.room.check`, `communication.room.info`) are additive. Existing topics unchanged. DTOs added to `pkg/dto` |
| 4 | Matrix Client Lifecycle Management | PASS | Bot intent used for admin-join during reconciliation; member intents used for EnsureJoined; bot leaves after reconciliation |
| 5 | Observability with Matrix Context | PASS | All reconciliation steps log roomID, userID, alkemioRoomID. Structured logging via Zap |
| 6 | Pragmatic Testing | PASS | Unit tests for RoomCheckService mock QueuePort. ReconcileRoom tested via integration tests (Element). Synapse module tested manually via Element |
| 7 | Go Service as Source of Truth | PASS | New DTOs in `pkg/dto`; `make generate` updates TypeScript lib |
| 8 | Secure Matrix Credential Management | PASS | hs_token used for check endpoint auth; constant-time comparison; no secrets in logs |
| 9 | Container Determinism | PASS | No new runtime dependencies; configuration via environment variables |
| 10 | Simplicity and Matrix API Coverage | PASS | Only implements APIs required by the spec; no speculative wrapping |

All gates pass. No violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/050-element-room-check/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── check-room-http.md
│   ├── room-check-rabbitmq.md
│   └── room-info-rabbitmq.md
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
# Existing structure — changes marked with (NEW) or (MOD)

synapse-modules/
└── alkemio_room_control.py            # (MOD) Add synchronous check in on_create_room

cmd/adapter/
└── main.go                            # No change

internal/
├── app/
│   └── app.go                         # (MOD) Wire RoomCheckService + CheckRoomHandler; pass queueAdapter to MautrixAdapter for ReconcileRoom
├── core/
│   ├── domain/
│   │   └── room_check.go              # (NEW) Domain types for check request/response
│   ├── ports/
│   │   ├── matrix.go                  # No change (reconciliation uses MautrixAdapter directly, not MatrixPort)
│   │   └── queue.go                   # (MOD) Add PublishAndWait for adapter-initiated RPC
│   └── service/
│       └── room_check_service.go      # (NEW) Check endpoint logic: parse request, RMQ request-reply, return response
├── infrastructure/
│   ├── http/
│   │   ├── check_room_handler.go      # (NEW) HTTP handler for POST /_matrix/app/alkemio/check-room
│   │   └── dm_webhook.go              # (MOD) Deprecate or remove old DM webhook
│   ├── matrix/
│   │   ├── mautrix.go                 # (MOD) Add ReconcileRoom, getRoomCreator (private helpers for reconciliation)
│   │   ├── listener.go                # (MOD) Add resolveOrReconcile; replace resolveAlkemioRoomID in all event handlers
│   │   └── custom_events.go           # (MOD) Register io.alkemio.pending event type
│   └── queue/
│       └── topics.go                  # (MOD) Add re-export aliases for TopicRoomCheck, TopicRoomInfo

pkg/dto/
├── commands.go                        # (MOD) Add TopicRoomCheck, TopicRoomInfo
├── room_check.go                      # (NEW) CheckRoomRequest, CheckRoomResponse DTOs
└── dm.go                              # (MOD) Deprecate DMWebhookPayload
```

**Structure Decision**: Feature fits within the existing hexagonal architecture. `RoomCheckService` handles the synchronous check (service layer). Reconciliation logic lives on `MautrixAdapter` (infrastructure layer) as `ReconcileRoom` because it orchestrates 7+ mautrix-go SDK operations (admin-join, EnsureJoined, power levels, alias, m.direct, bot leave) — extracting these into MatrixPort methods would add unnecessary abstraction for a single caller. HTTP handler follows the existing `dm_webhook.go` pattern on the appservice router (port 8280).

## Complexity Tracking

No constitution violations. No complexity justifications needed.
