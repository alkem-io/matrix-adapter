# Research: RMQ Events Extension

**Feature**: 007-rmq-events-extension  
**Date**: 2025-12-07  
**Status**: ✅ Complete - All decisions implemented

## Research Tasks

### 1. Matrix Relations API for Thread Messages

**Question**: How to retrieve all messages in a thread using mautrix-go?

**Findings**:
- mautrix-go provides `Client.GetRelations()` method in [client.go#L2292-2295](https://github.com/mautrix/go/blob/main/client.go#L2292-L2295)
- Uses `GET /_matrix/client/v1/rooms/{roomId}/relations/{eventId}/{relType}`
- For threads, use `RelationType = event.RelThread` ("m.thread")
- Request struct: `mautrix.ReqGetRelations` with fields:
  - `RelationType`: filter by relation type (m.thread)
  - `EventType`: optional event type filter  
  - `Dir`: pagination direction
  - `From/To`: pagination tokens
  - `Limit`: max results (server default if not specified)
- Response struct: `mautrix.RespGetRelations` with `Chunk []*event.Event`

**Decision**: Use `intent.Client.GetRelations()` with `RelationType: event.RelThread` to fetch thread messages. Since MVP doesn't need pagination, fetch with default limit and iterate all.

**Rationale**: This is the standard Matrix API for relations. mautrix-go has built-in support.

**Alternatives Considered**:
- Using `/messages` with filter - more complex and less efficient
- Room timeline with thread filter - requires sync state management

---

### 2. Joined Members API

**Question**: How to efficiently get only joined room members?

**Findings**:
- mautrix-go provides `intent.JoinedMembers()` in intent API
- Uses `GET /_matrix/client/v3/rooms/{roomId}/joined_members`
- Response: `mautrix.RespJoinedMembers` with `Joined map[id.UserID]JoinedMember`
- `JoinedMember` contains `DisplayName` and `AvatarURL`
- Already implemented in adapter: `MautrixAdapter.GetRoomMembers()` at [mautrix.go#L285-296](mautrix.go#L285-296)

**Decision**: Reuse existing `GetRoomMembers()` method, add new RMQ handler and DTO.

**Rationale**: Implementation already exists; just need RMQ command binding.

**Alternatives Considered**:
- Using room state with membership filter - returns all memberships (join, invite, leave, ban)
- None needed - existing pattern is optimal

---

### 3. Matrix Event Listener for Reactions

**Question**: How to capture reaction add/remove events from Matrix sync?

**Findings**:
- Reaction events are `m.reaction` type events (`event.EventReaction`)
- Reaction removal is a `m.room.redaction` event targeting the reaction event
- Current listener only handles `event.EventMessage` in `processEvent()`
- Need to extend listener to handle:
  1. `event.EventReaction` → ReactionAddedEvent
  2. `event.EventRedaction` where target is a reaction → ReactionRemovedEvent
- `event.ReactionEventContent` has `RelatesTo` with `EventID` (target message) and `Key` (emoji)

**Decision**: Extend `listener.go` to:
1. Handle `event.EventReaction` events
2. Handle `event.EventRedaction` events, checking if target was a reaction
3. Filter out bot's own events (already done for messages)
4. Publish to Watermill for RabbitMQ dispatch

**Rationale**: Follows existing event listener pattern; minimal code addition.

**Alternatives Considered**:
- Separate goroutine for reaction events - unnecessary complexity
- Polling relations API - inefficient compared to push

---

### 4. Matrix Event Listener for Membership Changes

**Question**: How to capture user leave events from Matrix sync?

**Findings**:
- Membership events are `m.room.member` state events (`event.StateMember`)
- Content type: `event.MemberEventContent` with `Membership` field
- Membership values: `event.MembershipJoin`, `event.MembershipLeave`, `event.MembershipBan`, etc.
- Leave includes:
  - Voluntary leave: user changes own membership to "leave"
  - Kick: another user changes target's membership to "leave"
  - Ban: membership becomes "ban" (implies leave)
- Need to detect transition TO "leave" or "ban" state

**Decision**: Extend listener to handle `event.StateMember` events where:
- `membership = "leave"` OR `membership = "ban"`
- State key (target user) is not the bot
- Publish `RoomMemberLeftEvent` with actor ID extracted from user MXID

**Rationale**: Standard Matrix membership tracking pattern.

**Alternatives Considered**:
- Tracking join→leave transitions explicitly - over-engineered for MVP
- Using `/sync` membership deltas - already handled by AppService events

---

### 5. Event Publishing Pattern

**Question**: How should outgoing events be published to RabbitMQ?

**Findings**:
- Current pattern: `MessageReceivedPayload` published via Watermill
- Topic names follow `communication.X.Y` convention
- Need to define new topics:
  - `communication.reaction.added`
  - `communication.reaction.removed`
  - `communication.room.member.left`
- Publishing done through `ports.QueuePort` interface

**Decision**: Create new DTO types in `pkg/dto/` and add topic constants. Use existing Watermill publisher pattern.

**Rationale**: Consistent with existing architecture.

---

### 6. Actor ID Extraction from Matrix User ID

**Question**: How to reliably extract Alkemio actor UUID from Matrix user ID?

**Findings**:
- Pattern confirmed: `@<uuid>:matrix.alkemio.org`
- Localpart IS the UUID directly (not prefixed)
- Two existing implementations available:
  1. `parseActorID()` in [listener.go#L67-76](listener.go#L67-76) - local helper
  2. `IDMapper.AlkemioActorID()` in [internal/core/domain/idmapper.go](internal/core/domain/idmapper.go) - centralized utility

**Decision**: Use `IDMapper.AlkemioActorID()` for all new event handlers (consistent with handler_room.go pattern). If it returns `uuid.Nil`, the event is from a non-ghost user and should be ignored.

**Rationale**: `IDMapper` is the canonical utility for ID conversions per hexagonal architecture. **Note**: There are no federated users on this server. Only Alkemio-controlled ghost users (with UUID localparts) can exist. Any user not matching the ghost pattern should be ignored.

---

## Summary of Technical Decisions

| Component | Decision | Risk |
|-----------|----------|------|
| Thread Messages | Use `GetRelations()` with `RelThread` | Low - standard API |
| Room Members | Reuse existing `GetRoomMembers()` | None - already works |
| Reaction Events | Extend listener for `EventReaction` | Low - simple addition |
| Redaction Events | Detect reaction redactions | Medium - need to track reaction event IDs |
| Membership Events | Listen for `StateMember` leave/ban | Low - standard pattern |
| Publishing | Watermill to existing topics | None - proven pattern |
