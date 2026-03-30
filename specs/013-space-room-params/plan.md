# Implementation Plan: Space & Room Parameters, Bot Architecture, Custom State

**Branch**: `013-space-room-params` | **Date**: 2026-03-25 | **Updated**: 2026-03-30 | **Spec**: [spec.md](spec.md)

## Summary

Originally scoped to wire `joinRule` for rooms, this feature grew to include directory visibility, bot architecture changes, custom state API, sync filtering, and a dedicated Synapse admin client. The implementation touched most of the adapter's infrastructure layer.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: mautrix-go (Matrix SDK), Watermill (RabbitMQ), Zap (logging)
**Storage**: N/A (no persistence changes)
**Testing**: Go `testing` package, `testify`
**Target Platform**: Linux server (Docker container)
**Project Type**: web-service (message-driven adapter)
**Scale/Scope**: ~20 files modified, ~2000 lines changed

## Source Code Structure

```text
pkg/dto/
├── room.go                          # joinRule wiring, isPublic, customState on create/update/get
├── space.go                         # isPublic, customState on create/update/get
├── state.go                         # NEW: SetRoomState/GetRoomState, SetSpaceState/GetSpaceState DTOs
├── event.go                         # SpaceUpdatedEvent
└── commands.go                      # New topics: state.set/get, space.updated

internal/core/
├── domain/model.go                  # CustomState on Room/Space, SpaceUpdatedEvent
├── ports/matrix.go                  # Updated signatures, SetCustomState/GetCustomState, SetRoomDirectoryVisibility
└── service/
    ├── room_service.go              # joinRule, isPublic, customState, pointer semantics
    ├── space_service.go             # isPublic, customState
    └── event_service.go             # HandleSpaceUpdated

internal/infrastructure/
├── matrix/
│   ├── mautrix.go                   # Bot architecture, ghost intents, admin API migration
│   ├── synapse_admin.go             # NEW: Encapsulated Synapse Admin API client
│   └── listener.go                  # parseStateChange, isSpaceRoom via admin, space event routing
└── queue/
    ├── handler_room.go              # State handlers, AliasResolver
    ├── handler_space.go             # State handlers, AliasResolver
    ├── resolver.go                  # NEW: Shared AliasResolver
    ├── router.go                    # New state routes
    ├── topics.go                    # New topic constants
    └── errors.go                    # NewSpaceNotFoundError

internal/config/config.go            # BotDisplayName, RegistrationSecret
```

## Key Architectural Decisions

| Decision | Rationale |
|----------|-----------|
| Bot leaves rooms, stays in spaces | Avoids bot appearing in DM member lists; spaces need bot for hierarchy ops |
| All reads via Synapse Admin API | Bot not in rooms; admin API doesn't require membership |
| Writes via ghost user (highest PL) | Can't write state without membership; ghost with highest PL has best chance |
| Admin bootstrap via shared secret | Zero-touch deployment; bot self-promotes at startup |
| Custom state as InitialState | Visibility filtering active before members join (prevents /sync leak) |
| No invite events | Ghost users join directly; prevents Element notification for hidden rooms |
| SynapseAdmin encapsulation | All admin API calls in one place; forbidden to use bare HTTP elsewhere |
| Shared AliasResolver | Eliminates DRY violation across handlers |
