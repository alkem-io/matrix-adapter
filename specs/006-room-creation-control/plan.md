# Implementation Plan: Room Creation Control

**Branch**: `006-room-creation-control` | **Date**: 2025-12-06 | **Spec**: [spec.md](./spec.md)
**Status**: ✅ Implemented | **Completed**: 2025-12-07

## Summary

Implement room creation control where only the AppService bot can create Matrix rooms. Ghost users attempting to create DM rooms trigger a webhook to the adapter, which publishes a request event to RabbitMQ. The Alkemio Server decides on approval and commands the adapter to create the room.

**Technical Approach:**
1. **Synapse Module** (Python): Already exists - uses `user_may_create_room` callback to block ghost users and webhook DM requests to adapter
2. **Adapter Webhook Endpoint** (Go): New HTTP endpoint to receive DM request webhooks, authenticate with HS token, publish events
3. **New DTOs & Topics** (Go): DM request event and create command payloads
4. **New Handler** (Go): Process `communication.room.dm.create` command

## Technical Context

**Language/Version**: Go 1.25, Python 3.11+ (Synapse module)  
**Primary Dependencies**: mautrix-go, Watermill (RabbitMQ), aiohttp (Python), net/http (Go)  
**Storage**: N/A (stateless adapter)  
**Testing**: Go `testing` package, testify  
**Target Platform**: Linux containers (Docker)  
**Project Type**: Single Go service + Synapse Python module  
**Performance Goals**: DM flow end-to-end < 5 seconds (SC-002)  
**Constraints**: Fail-closed on webhook errors, deduplicate requests in Synapse module  
**Scale/Scope**: All users on homeserver are ghost users  

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Adapter-First Domain Isolation | ✅ PASS | All Matrix SDK calls in `internal/infrastructure/matrix` |
| 2. Event-Driven State Synchronization | ✅ PASS | DM flow uses Watermill events, async room creation |
| 3. Microservice Contract Stability | ✅ PASS | New topics documented in `pkg/dto`, TS lib generated |
| 4. Matrix Client Lifecycle | ✅ PASS | Uses existing intent management |
| 5. Observability with Matrix Context | ✅ PASS | Structured logging with userID, roomID, correlationID |
| 6. Pragmatic Testing | ✅ PASS | Unit tests mock MatrixPort; integration tests for DM flow |
| 7. Go Service as Source of Truth | ✅ PASS | DTOs in `pkg/dto`, `make generate` updates TS lib |
| 8. Secure Credential Management | ✅ PASS | HS token validated, never logged |
| 9. Container Determinism | ✅ PASS | No new runtime builds needed |
| 10. Simplicity | ✅ PASS | Feature justified by concrete user stories |

**Post-Design Re-Check**: Required after Phase 1 artifacts

## Project Structure

### Documentation (this feature)

```text
specs/006-room-creation-control/
├── plan.md              # This file
├── research.md          # Phase 0: Synapse spam checker API, webhook patterns
├── data-model.md        # Phase 1: DM request/response DTOs
├── quickstart.md        # Phase 1: Local testing steps
├── contracts/           # Phase 1: New RabbitMQ topics & HTTP endpoint
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Go Adapter (existing structure, new files marked with *)
internal/
├── core/
│   ├── domain/
│   │   └── model.go           # (no changes - IDMapper already has AlkemioActorID)
│   ├── ports/
│   │   └── queue.go           # (no changes - QueuePort.Publish already exists)
│   └── service/
│       └── dm_service.go      # * New DM request handling service
├── infrastructure/
│   ├── http/
│   │   ├── health.go          # (existing)
│   │   └── dm_webhook.go      # * New webhook endpoint handler
│   ├── matrix/
│   │   └── mautrix.go         # (no changes expected - uses existing CreateRoomWithAlias)
│   └── queue/
│       ├── handler_dm.go      # * New DM command handler
│       ├── router.go          # Add new topic subscription
│       └── topics.go          # Import new DM topics
├── app/
│   └── app.go                 # Wire up DM webhook server

pkg/
└── dto/
    ├── commands.go            # * Add DM topics to constants
    ├── dm.go                  # * New DM request/command DTOs

lib/
└── src/dto/generated.ts       # Auto-generated from Go DTOs

# Synapse Module (existing, may need updates)
synapse-modules/
├── alkemio_room_control.py    # (already implemented)
└── README.md                  # (existing)

# Tests
internal/
├── core/service/
│   └── dm_service_test.go     # * Unit tests
├── infrastructure/
│   ├── http/
│   │   └── dm_webhook_test.go # * Unit tests
│   └── queue/
│       └── handler_dm_test.go # * Unit tests
```

**Structure Decision**: Standard Go hexagonal layout following existing patterns. New HTTP endpoint handler in `internal/infrastructure/http`, new queue handler in `internal/infrastructure/queue`, DTOs in `pkg/dto`.

## Complexity Tracking

> No constitution violations to justify

---

## Post-Design Constitution Re-Check

*Re-evaluated after Phase 1 design artifacts completed.*

| Principle | Status | Design Artifact Validation |
|-----------|--------|---------------------------|
| 1. Adapter-First Domain Isolation | ✅ PASS | `dm_webhook.go` in infrastructure; no SDK imports in service |
| 2. Event-Driven State Synchronization | ✅ PASS | Async flow: webhook → event → command → room |
| 3. Microservice Contract Stability | ✅ PASS | [contracts/rmq-dm-topics.md](contracts/rmq-dm-topics.md) defines schemas |
| 4. Matrix Client Lifecycle | ✅ PASS | Reuses existing intent management in mautrix.go |
| 5. Observability with Matrix Context | ✅ PASS | correlation_id in all DTOs; structured logging planned |
| 6. Pragmatic Testing | ✅ PASS | Unit tests mock ports; integration for full flow |
| 7. Go Service as Source of Truth | ✅ PASS | [data-model.md](data-model.md) defines Go DTOs first |
| 8. Secure Credential Management | ✅ PASS | [contracts/http-dm-webhook.md](contracts/http-dm-webhook.md) specifies token never logged |
| 9. Container Determinism | ✅ PASS | No new dependencies; uses stdlib net/http |
| 10. Simplicity | ✅ PASS | Minimal new surface: 1 endpoint, 2 topics, 3 DTOs |

**Result**: All principles satisfied. Proceeding to implementation.

---

## Phase Summary

| Phase | Status | Artifacts |
|-------|--------|-----------|
| 0: Research | ✅ Complete | [research.md](research.md) |
| 1: Design | ✅ Complete | [data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md) |
| 2: Tasks | ✅ Complete | [tasks.md](tasks.md) |
| 3: Implementation | ✅ Complete | All code merged |

---

## Implementation Order (for /speckit.tasks)

The following task order is recommended based on dependencies:

1. **DTOs & Topics** (`pkg/dto/dm.go`, `pkg/dto/commands.go`)
   - Foundation for all other work
   - Enables TypeScript lib generation

2. **HTTP Webhook Handler** (`internal/infrastructure/http/dm_webhook.go`)
   - Receives Synapse module webhooks
   - Publishes `DMRequestedEvent`

3. **Queue Handler** (`internal/infrastructure/queue/handler_dm.go`)
   - Handles `CreateDMRoomCommand`
   - Creates DM room via Matrix service

4. **Router & Wiring** (`internal/infrastructure/queue/router.go`, `internal/app/app.go`)
   - Register new topic subscription
   - Wire webhook handler into HTTP server

5. **Tests** (`*_test.go`)
   - Unit tests for each component
   - Integration test for full flow

6. **Documentation & TS Lib** (`make generate`, update README if needed)
   - Generate TypeScript definitions
   - Verify lib exports

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Synapse module fails silently | Low | High | Health check endpoint, structured logging |
| Webhook timeout causes poor UX | Medium | Medium | Fast 200 response, async processing |
| RabbitMQ unavailable | Low | High | Retry logic, fail-closed behavior |
| Actor ID extraction fails | Low | Medium | Validation at webhook boundary |

---

## Exit Criteria

See [spec.md#success-criteria](./spec.md#success-criteria) for authoritative success criteria (SC-001 through SC-004).

---

## Next Steps

Run `/speckit.tasks` to generate the implementation task list (`tasks.md`) based on this plan.
