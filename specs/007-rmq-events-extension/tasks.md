# Tasks: RMQ Events Extension

**Feature**: 007-rmq-events-extension  
**Input**: spec.md (5 user stories), plan.md, data-model.md, contracts/rmq-events.md, research.md

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1-US5)
- Exact file paths included

---

## Phase 1: Setup (DTOs & Contracts)

**Purpose**: Define new data structures and topic constants

- [X] T001 [P] Add outgoing event DTOs (ReactionAddedEvent, ReactionRemovedEvent, RoomMemberLeftEvent) to pkg/dto/event.go
- [X] T002 [P] Add thread message DTOs (GetThreadMessagesRequest, GetThreadMessagesResponse) to pkg/dto/message.go
- [X] T003 [P] Add GetRoomMembersRequest and GetRoomMembersResponse to pkg/dto/room.go
- [X] T004 [P] Add topic constants and registry entries to pkg/dto/commands.go
- [X] T005 Add topic aliases to internal/infrastructure/queue/topics.go

**Checkpoint**: `make lint` passes, all DTOs compile ✓

---

## Phase 2: Foundational (Interface & Adapter)

**Purpose**: Add GetThreadMessages to matrix port

- [X] T006 Add GetThreadMessages(ctx, roomID, threadRootID) method to internal/core/ports/matrix.go
- [X] T007 Implement GetThreadMessages in internal/infrastructure/matrix/mautrix.go using Client.GetRelations with RelThread

**Checkpoint**: Interface and implementation ready ✓

---

## Phase 3: User Story 1 - Reaction Added (P1)

**Goal**: Publish ReactionAddedEvent when users add reactions

- [X] T008 [US1] Extend listener.go: add case for event.EventReaction in event processing loop
- [X] T009 [US1] In reaction handler: filter bot events, validate ghost user via IDMapper.AlkemioActorID(), extract emoji from RelatesTo.Key, resolve room alias, publish ReactionAddedEvent

**Checkpoint**: Reaction added events published ✓

---

## Phase 4: User Story 2 - Reaction Removed (P1)

**Goal**: Publish ReactionRemovedEvent when reactions are redacted

- [X] T010 [US2] Extend listener.go: add case for event.EventRedaction
- [X] T011 [US2] In redaction handler: check if redacted event was a reaction (lookup original), filter bot, extract emoji (empty if unavailable), publish ReactionRemovedEvent

**Checkpoint**: Reaction removed events published ✓

---

## Phase 5: User Story 3 - Get Room Members (P1)

**Goal**: Return joined member UUIDs for a room

- [X] T012 [US3] Add HandleGetRoomMembers to handler_room.go: resolve alias, call GetRoomMembers, convert IDs via IDMapper.AlkemioActorID (skip uuid.Nil), return response
- [X] T013 [US3] Register TopicRoomMembersGet route in router.go

**Checkpoint**: Room members query works ✓

---

## Phase 6: User Story 4 - Get Thread Messages (P2)

**Goal**: Return all messages in a thread

- [X] T014 [US4] Add HandleGetThreadMessages to handler_room.go: resolve alias, call GetThreadMessages from T007, build response with messages
- [X] T015 [US4] Register TopicThreadMessagesGet route in router.go

**Checkpoint**: Thread messages query works ✓

---

## Phase 7: User Story 5 - Member Left (P2)

**Goal**: Publish RoomMemberLeftEvent when users leave

- [X] T016 [US5] Extend listener.go: add case for event.StateMember with membership "leave" or "ban"
- [X] T017 [US5] In membership handler: filter bot (state_key), validate ghost user, resolve room alias, extract reason, publish RoomMemberLeftEvent

**Checkpoint**: Member left events published ✓

---

## Phase 8: Integration

**Purpose**: Generate TypeScript, validate

- [X] T018 Run `make generate` to update TypeScript library
- [X] T019 Run `make lint && make test && make build`
- [X] T020 Manual integration tests (all 5 features)

**Checkpoint**: All features working ✓

---

## Dependencies

```
Phase 1 (T001-T005) → Phase 2 (T006-T007) → User Stories (T008-T017) → Phase 8 (T018-T020)
                                            ↳ US1-US5 can run in parallel
```

---

## Task Summary

| Phase | Tasks | Story |
|-------|-------|-------|
| Phase 1: Setup | T001-T005 | - |
| Phase 2: Foundation | T006-T007 | - |
| Phase 3: US1 | T008-T009 | Reaction Added |
| Phase 4: US2 | T010-T011 | Reaction Removed |
| Phase 5: US3 | T012-T013 | Room Members |
| Phase 6: US4 | T014-T015 | Thread Messages |
| Phase 7: US5 | T016-T017 | Member Left |
| Phase 8: Integration | T018-T020 | - |
| **Total** | **20 tasks** | |

---

## Existing Code to Reuse

| Functionality | Location | Usage |
|--------------|----------|-------|
| `IDMapper.AlkemioActorID()` | internal/core/domain/idmapper.go | Convert Matrix user ID → Alkemio UUID |
| `IDMapper.AlkemioRoomID()` | internal/core/domain/idmapper.go | Extract UUID from room alias |
| `resolveRoomAlias()` | handler_room.go:34-42 | Resolve Alkemio room ID → Matrix room ID |
| `parseActorID()` | listener.go:67-76 | Parse UUID from Matrix user ID |
| `m.as.BotMXID()` | listener.go:33 | Bot filtering pattern |
| `GetRoomMembers()` | ports/matrix.go | Already exists - returns []id.UserID |

## Notes

- **Reduced from 57 → 20 tasks** by eliminating redundancy
- No new files created - extend existing event.go, message.go, handler_room.go
- Topics go in commands.go (single source of truth), aliases in topics.go
- Logging is part of implementation, not separate tasks
- ID resolution uses existing IDMapper methods throughout
- Bot filtering uses existing pattern from listener.go
