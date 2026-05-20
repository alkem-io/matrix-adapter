# Research: Element-Initiated Conversation Creation (Synchronous Check)

**Feature**: 050-element-room-check | **Date**: 2026-05-20

## R1: Adapter-Initiated RabbitMQ Request-Reply

**Decision**: Implement `PublishAndWait` on `QueuePort` using a temporary exclusive AMQP queue per request.

**Rationale**: The adapter currently only handles *incoming* RPC (Server publishes with `reply_to`, adapter replies via `sendReply`). For the check flow, the adapter must *initiate* RPC: publish a check request to the server and wait for a response with timeout.

The AMQP standard RPC pattern uses:
1. Create an exclusive, auto-delete reply queue (unique per request)
2. Publish message with `reply_to` set to the reply queue and a `correlation_id`
3. Subscribe to the reply queue, consume the response, match `correlation_id`
4. Close the reply queue

Watermill's `amqp.Publisher` supports setting native AMQP properties via the existing `RPCMarshaler`. For the reply consumer, we use raw `amqp091-go` channel operations (declare exclusive queue, consume, cancel) since Watermill's `Subscriber` creates durable queues which aren't suitable for ephemeral RPC reply queues.

**Alternatives considered**:
- *Reuse Watermill subscriber with temporary topic*: Watermill's `NewDurableQueueConfig` creates persistent queues, not suitable for ephemeral reply queues. Would need a different AMQP config.
- *Direct amqp091-go for both publish and consume*: Bypasses Watermill entirely. Rejected because the existing publisher infrastructure (marshaling, connection management) is already working well.
- *HTTP callback from server instead of RPC*: Would require the server to know the adapter's URL. Rejected because it inverts the dependency direction.

**Implementation approach**: Add `PublishAndWait(ctx context.Context, topic string, payload interface{}, timeout time.Duration) ([]byte, error)` to `QueuePort`. Implementation in `WatermillAdapter` uses the existing publisher for the outgoing message and raw AMQP for the exclusive reply queue. The `ctx` handles cancellation; timeout is enforced via `context.WithTimeout`.

## R2: Synapse Module HTTP Call in on_create_room

**Decision**: Use Synapse's `SimpleHttpClient.post_json_get_json` with a 3-second timeout.

**Rationale**: The module already uses `SimpleHttpClient` (lazy-initialized) for the DM webhook notification. The `on_create_room` callback is async (`async def`) and can await HTTP calls. `post_json_get_json` is the Synapse-standard way to make JSON POST requests from modules.

**Timeout**: 3 seconds. The RabbitMQ round-trip (adapter → server → adapter) should complete in <1 second under normal conditions. A 3-second timeout allows for transient latency while still failing fast enough for user experience.

**Error mapping**:
- HTTP timeout or connection error → `SynapseError(503, "Service temporarily unavailable", Codes.UNKNOWN)`
- HTTP 200 with `allow: false` → `SynapseError(403, reason_from_response, Codes.FORBIDDEN)`
- HTTP non-200 → `SynapseError(503, ...)`

**Alternatives considered**:
- *Longer timeout (5s)*: Rejected. Users wait synchronously; 5s feels broken. If the server can't respond in 3s, something is wrong.
- *Direct RabbitMQ from module*: Rejected. Would require amqp library in the Synapse module, adding complexity and a deployment dependency.

## R3: Reconciliation Trigger Mechanism

**Decision**: Detect `io.alkemio.pending` custom state event in `handleRoomCreateEvent` when `resolveAlkemioRoomID` returns `uuid.Nil`.

**Rationale**: When the Synapse module approves room creation, it injects `io.alkemio.pending` with `{alkemio_room_id: "<uuid>"}` into `initial_state`. The room has no alias yet (alias is set during reconciliation). When the appservice transaction delivers `m.room.create`, the listener calls `resolveAlkemioRoomID`, which returns `uuid.Nil` (no alias). Currently this silently drops the event. The new logic: if `uuid.Nil`, check for `io.alkemio.pending` state → if found, trigger reconciliation → after reconciliation sets alias, emit `RoomCreatedEvent`.

**Updated during implementation:** Reconciliation triggers via `resolveOrReconcile` in ALL event handlers (not just `handleRoomCreateEvent`), matching R9. Any event from a room without an alias will attempt reconciliation.

**Alternatives considered**:
- *Separate event type for pending rooms*: Would require a new mautrix event handler. Rejected because `io.alkemio.pending` is a state event, not a timeline event, and we already process `m.room.create` events.
- *Check `m.room.create` content for a custom field*: `on_create_room` modifies `request_content`, not the create event content itself. The create event content is controlled by Synapse.

## R4: io.alkemio.pending State Event Registration

**Decision**: Register `io.alkemio.pending` in mautrix-go's TypeMap via `custom_events.go`, same pattern as `io.alkemio.visibility`.

**Rationale**: Already have precedent with `StateAlkemioVisibility`. Add `StateAlkemioPending` with content type `AlkemioPendingContent{AlkemioRoomID string}`. This allows the SDK's StateStore to parse it and enables typed access in the listener.

## R5: Server-Side Room Info Retrieval During Reconciliation

**Decision**: New RabbitMQ topic `communication.room.info` (adapter → server request-reply).

**Rationale**: During reconciliation, the adapter needs the server's view of the room: members, type, and potentially other metadata. The existing `communication.room.get` returns Matrix-side room details (from the adapter), not server-side room info. The server is the source of truth during this flow.

The adapter publishes `{alkemio_room_id: "<uuid>"}` and the server responds with `{type: "conversation_direct", is_direct: true, members: [{actor_id, display_name}...]}`. The `type` field uses server-side room types (`conversation_direct`, `conversation_group`) which are extensible for future room types. The `is_direct` convenience flag simplifies adapter logic. The response is designed for future extension (avatar_url, name, topic, etc.) as the server-side room abstraction evolves.

The adapter then calls `EnsureUser` + `EnsureJoined` for each member.

**Alternatives considered**:
- *Include member list in the check response*: Would work, but requires storing state between the check (HTTP handler) and reconciliation (listener event). The adapter is stateless by design.
- *Extend existing `communication.room.get`*: This topic returns Matrix-side room details from the adapter. Overloading it for server-side queries would violate its contract.
- *`communication.conversation.members.get`*: Too narrow — naming it around "conversation" ties it to an abstraction layer that will eventually be removed. Returning only members misses the opportunity for a general-purpose server-side room info endpoint.

## R6: Power Level Management During Reconciliation

**Decision**: Use existing `SetCustomState` pattern or direct `SendStateEvent` for power level adjustment.

**Rationale**: During reconciliation, the bot needs to:
1. Admin-join to the room (via `SynapseAdmin.JoinRoom`)
2. Set power levels: bot=100, creator stays at their current level (100 as room creator), users_default=50
3. After alias + members: adjust creator to 50

The power level override in the room creation request sets `users_default: 50`. The creator gets PL 100 automatically as the room creator. The bot gets PL 100 via admin-join. After reconciliation, we lower the creator to 50 to match the standard Alkemio room configuration.

Power levels are set directly via bot intent `SendStateEvent` inside `ReconcileRoom` on the infrastructure layer — no MatrixPort method needed.

**Updated during implementation:** Power levels are set in a single step during reconciliation (bot=100, creator=50, users_default=50) rather than two phases. The creator has PL 100 as room creator until reconciliation adjusts it.

## R7: m.direct Account Data for DMs

**Decision**: Reuse existing `registerDirectRoomParticipants` logic from `CreateRoomWithAlias`.

**Rationale**: `mautrix.go:1760` calls `registerDirectRoomParticipants(ctx, roomID, isDirect, memberUserIDs)` after room creation. The same private method is called during reconciliation for DM rooms. It sets the `m.direct` account data on each participant so Element displays the room as a DM. Since reconciliation lives on `MautrixAdapter` (same struct), the private method is directly accessible.

## R8: Reconciliation Layer Placement

**Decision**: Reconciliation logic lives on `MautrixAdapter` (infrastructure layer), not as a separate service.

**Rationale**: `ReconcileRoom` orchestrates 7+ mautrix-go SDK operations: `SynapseAdmin.JoinRoom` (admin-join), member intents for `EnsureJoined`, `registerDirectRoomParticipants` (private), `SendStateEvent` (power levels), `SetRoomAlias`, `BotIntent().LeaveRoom`, and `getRoomCreator`. Placing this in the service layer would require either: (a) exposing 5+ new MatrixPort methods used by a single caller, or (b) violating Constitution Principle #1 by calling infrastructure directly from service. The infrastructure layer is the natural home.

**Alternatives considered**:
- *Service layer with new MatrixPort methods*: Would add `AdminJoinBot`, `BotLeaveRoom`, `SetDirectRoomFlags`, `SetRoomPowerLevels`, `GetRoomCreator` to MatrixPort. All used only by reconciliation. Rejected as unnecessary abstraction.
- *Service layer calling infrastructure directly*: Violates Constitution Principle #1. Rejected.

## R9: Reconciliation Retry on Subsequent Events

**Decision**: Replace all `resolveAlkemioRoomID` calls in event handlers with `resolveOrReconcile`, which checks for `io.alkemio.pending` when alias resolution returns nil.

**Rationale**: If reconciliation fails partway (e.g., server unreachable for member list), `io.alkemio.pending` remains set and the room has no alias. Any subsequent event from that room (message, reaction, receipt, membership, state change) triggers `resolveAlkemioRoomID` → `uuid.Nil` → pending check → reconciliation retry. Once reconciliation sets the alias, subsequent events resolve normally. A `sync.Map` of room IDs prevents concurrent reconciliation attempts for the same room.

**Alternatives considered**:
- *Only trigger on m.room.create*: Misses the retry case — `m.room.create` fires only once per room. If reconciliation fails, no retry until the adapter restarts (and even then, only if the event is redelivered).
- *Periodic background scan*: Adds complexity (timer, room enumeration). Rejected since event-driven retry is simpler and more responsive.
