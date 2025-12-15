# Read Receipts Feature Research

**Created**: 2025-12-12  
**Feature**: Message Events and Read Receipts (`008-read-receipts`)  
**SDK**: mautrix-go v0.26.0

---

## Executive Summary

This research documents the findings for implementing read receipt functionality in the Matrix adapter. The feature requires:
- Sending read receipts (room-level and thread-level)
- Querying unread message counts
- Listening for receipt events from other users
- Handling thread-level receipts independently from room-level receipts

**Key Finding**: mautrix-go v0.26.0 provides native support for:
- Sending read receipts via `Client.MarkRead()` and `Client.SendReceipt()`
- Thread-specific receipts via `ReqSendReceipt` struct
- Querying messages via `Client.Messages()` and `Client.GetThreadMessages()`
- Event listeners for `m.receipt` events via the sync handler pattern

---

## Decision 1: Read Receipt Sending

### Chosen Approach

Use **`Client.SendReceipt()`** for maximum flexibility with thread support.

**Method Signature** (from mautrix-go v0.26.0):
```go
func (cli *Client) SendReceipt(
    ctx context.Context,
    roomID id.RoomID,
    eventID id.EventID,
    receiptType event.ReceiptType,
    content interface{},
) (*RespSendEvent, error)
```

**Thread-Specific Receipt Content** (mautrix-go):
```go
type ReqSendReceipt struct {
    ThreadID string `json:"thread_id,omitempty"`
}
```

### Implementation Pattern

```go
// Room-level read receipt
_, err := intent.SendReceipt(ctx, roomID, messageID, event.ReceiptTypeRead, nil)

// Thread-level read receipt
threadContent := &mautrix.ReqSendReceipt{
    ThreadID: threadRootID.String(),
}
_, err := intent.SendReceipt(ctx, roomID, messageID, event.ReceiptTypeRead, threadContent)
```

### Rationale

1. **Thread Support**: `ReqSendReceipt` explicitly supports thread-specific receipts via `thread_id` field
2. **Type Safety**: Uses `event.ReceiptTypeRead` constant for standard compliance
3. **Nil Content Safe**: Library handles nil content gracefully for room-level receipts
4. **Intent Support**: Works with appservice Intent API for ghost user operations

### Alternatives Considered

- **`Client.MarkRead()`**: Simpler but deprecated for custom content (replaced by `SendReceipt`)
- **`Client.SetReadMarkers()`**: Intended for fully-read/read markers, not standard receipts
- **Manual HTTP Request**: Unnecessary complexity when SDK provides typed methods

### Code References

- Implementation: [internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go) (lines 2294-2315)
- Request Type: mautrix-go `ReqSendReceipt` struct (added in v0.12.4)
- Matrix Spec: `PUT /_matrix/client/v3/rooms/{roomId}/receipt/{receiptType}/{eventId}`

---

## Decision 2: Unread Count Calculation

### Chosen Approach

**Calculate on-demand** by querying messages since last receipt using `Client.Messages()` and comparing timestamps.

### Implementation Strategy

1. **Query Last Receipt**: Use `Client.StateEvent()` to get `m.receipt` state for user
2. **Query Messages**: Use `Client.Messages()` with `DirectionBackward` and filter by timestamp
3. **Calculate Count**: Count messages after the last receipt timestamp

```go
// Pseudocode for unread count calculation
func CalculateUnreadCount(ctx context.Context, roomID id.RoomID, userID id.UserID) (int, error) {
    // 1. Get last read receipt timestamp from m.receipt state event
    var receiptContent event.ReceiptEventContent
    err := intent.StateEvent(ctx, roomID, event.TypeReceipt, "", &receiptContent)
    
    lastReadTimestamp := parseLastReceiptTimestamp(receiptContent, userID)
    
    // 2. Query messages since that timestamp
    resp, err := intent.Messages(ctx, roomID, "", "", mautrix.DirectionBackward, nil, 1000)
    
    // 3. Count messages after last receipt
    count := 0
    for _, msg := range resp.Chunk {
        if msg.Timestamp > lastReadTimestamp && msg.Type == event.EventMessage {
            count++
        }
    }
    
    return count, nil
}
```

### Thread-Level Unread Count

For thread-level counts, use `Client.GetThreadMessages()` which leverages the relations API:

```go
// Thread unread count uses relations API
messages, err := matrix.GetThreadMessages(ctx, roomID, threadRootID)
// Filter messages by timestamp after last thread-specific receipt
```

**Existing Implementation Reference**: [internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go#L808-L850)

### Rationale

1. **No Adapter State**: Maintains stateless architecture - homeserver is source of truth
2. **Consistency**: Read receipts persist on homeserver, survives adapter restarts
3. **Matrix Native**: Follows Matrix spec for receipt storage (`m.receipt` event)
4. **Thread Independence**: Separate queries for room vs. thread receipts

### Alternatives Considered

- **Pre-computed Cache**: Would require maintaining state in adapter (violates constitution)
- **Alkemio Server Calculation**: Requires event streaming, duplicates work, increases latency
- **Matrix Homeserver Extension**: Would require custom Synapse module (out of scope)

### Performance Considerations

- **Acceptable for MVP**: Users typically have <1000 messages per room
- **Pagination**: `Messages()` supports pagination tokens for larger rooms
- **Optimization Path**: Future enhancement could use timestamp-based filtering at HTTP level

### Code References

- Messages Query: [internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go#L670-L713) (`GetRoomMessages`)
- Thread Messages: [internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go#L808-L850) (`GetThreadMessages`)
- mautrix-go API: `Client.Messages()` - pagination with direction and limit

---

## Decision 3: Event Handler Registration

### Chosen Approach

Follow existing **`EventHandlers` callback pattern** used for message/reaction events.

### Implementation Pattern

Based on existing code in [internal/infrastructure/matrix/listener.go](../../internal/infrastructure/matrix/listener.go):

```go
// Extend EventHandlers struct
type EventHandlers struct {
    OnMessage           func(msg domain.Message) error
    OnReactionAdded     func(reaction domain.ReactionEvent) error
    OnReactionRemoved   func(reaction domain.ReactionRemovedEvent) error
    OnMemberLeft        func(membership domain.MembershipEvent) error
    OnReadReceiptUpdate func(receipt domain.ReadReceiptEvent) error  // NEW
}

// Add case to processEvent switch
func (m *MautrixAdapter) processEvent(evt *event.Event) {
    // Ignore own events
    if evt.Sender == m.as.BotMXID() {
        return
    }

    switch evt.Type {
    case event.EventMessage:
        m.handleMessageEvent(evt)
    case event.EventReaction:
        m.handleReactionEvent(evt)
    case event.EventRedaction:
        m.handleRedactionEvent(evt)
    case event.StateMember:
        m.handleMembershipEvent(evt)
    case event.TypeReceipt:  // NEW
        m.handleReceiptEvent(evt)
    }
}

// Handler implementation
func (m *MautrixAdapter) handleReceiptEvent(evt *event.Event) {
    if m.eventHandlers.OnReadReceiptUpdate == nil {
        return
    }
    
    // Parse receipt content
    content, ok := parseEventContent[event.ReceiptEventContent](evt)
    if !ok {
        return
    }
    
    // Extract receipt data and invoke callback
    // (async for HTTP calls, following existing pattern)
}
```

### Rationale

1. **Consistency**: Matches existing event handler architecture
2. **Asynchronous**: HTTP calls (room ID resolution) happen in goroutines
3. **Type Safety**: Uses mautrix-go's typed event content structs
4. **Tested Pattern**: Proven architecture from message/reaction features

### Event Loop Pattern

**Existing Implementation**: [internal/infrastructure/matrix/listener.go](../../internal/infrastructure/matrix/listener.go#L27-L56)

- Events arrive via `m.as.Events` channel from appservice transaction push
- `startEventLoop()` processes events in dedicated goroutine
- Each handler spawns goroutine for blocking operations (HTTP calls)
- IDMapper resolves Matrix IDs ↔ Alkemio UUIDs

### Code References

- Event Handlers: [internal/infrastructure/matrix/listener.go](../../internal/infrastructure/matrix/listener.go#L13-L21)
- Event Processing: [internal/infrastructure/matrix/listener.go](../../internal/infrastructure/matrix/listener.go#L42-56)
- Service Registration: [internal/app/app.go](../../internal/app/app.go#L85-88)

---

## Decision 4: Thread Support in Matrix

### Matrix Thread Specification

Threads use the **`m.thread` relation type** defined in MSC3440:
- Thread root: First message in conversation
- Thread replies: Messages with `m.relates_to.rel_type = "m.thread"` pointing to root
- Thread receipts: Separate from room-level receipts using `thread_id` parameter

### mautrix-go Thread Support

**Built-in Thread Support**: YES ✅

1. **Thread Relations API** ([internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go#L808-850)):
```go
func (m *MautrixAdapter) GetThreadMessages(
    ctx context.Context, roomID id.RoomID, threadRootID id.EventID,
) ([]domain.Message, error)
```

Implementation uses:
```go
url := buildRelationsURL(intent, roomID, threadRootID, event.RelThread, event.EventMessage)
```

2. **Thread Reply Sending** ([internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go#L396-419)):
```go
msgContent := event.MessageEventContent{
    MsgType: event.MsgText,
    Body:    content,
    RelatesTo: &event.RelatesTo{
        InReplyTo: &event.InReplyTo{
            EventID: threadID,
        },
    },
}
```

### Thread Receipt Strategy

**Room vs. Thread Receipts**:
- **Room-level**: `SendReceipt(ctx, roomID, messageID, event.ReceiptTypeRead, nil)`
- **Thread-level**: `SendReceipt(ctx, roomID, messageID, event.ReceiptTypeRead, &ReqSendReceipt{ThreadID: threadRootID})`

### Rationale

1. **Native SDK Support**: mautrix-go has `event.RelThread` constant and relations API
2. **Independent Tracking**: Thread receipts use separate `thread_id` field
3. **Existing Patterns**: Adapter already uses `GetThreadMessages()` for thread queries
4. **Spec Compliance**: Follows MSC3440 for thread relations

### Code References

- Thread Messages: [internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go#L808-850)
- Relations API: mautrix-go `buildRelationsURL()` with `event.RelThread`
- DTO Support: [pkg/dto/message.go](../../pkg/dto/message.go#L16) (`ThreadID *MessageID`)

---

## Decision 5: Error Handling Patterns

### Error Response Construction

**Existing Pattern** from [internal/infrastructure/queue/errors.go](../../internal/infrastructure/queue/errors.go):

```go
// Typed error creators
func NewInvalidParamError(msg string) dto.BaseResponse {
    return dto.NewErrorResponse(dto.ErrCodeInvalidParam, msg)
}

func NewMessageNotFoundError(msg string) dto.BaseResponse {
    return dto.NewErrorResponse(dto.ErrCodeMessageNotFound, msg)
}

// Service error mapping
func MapServiceError(err error) dto.BaseResponse {
    if err == nil {
        return dto.NewSuccessResponse()
    }

    // Check typed domain errors first (preferred)
    switch {
    case errors.Is(err, domain.ErrRoomNotFound):
        return dto.NewErrorResponse(dto.ErrCodeRoomNotFound, "Room not found")
    case errors.Is(err, domain.ErrInvalidParam):
        return dto.NewErrorResponse(dto.ErrCodeInvalidParam, err.Error())
    // ... more cases
    }
    
    // Fallback to string matching for Matrix SDK errors
    return dto.NewErrorResponse(dto.ErrCodeMatrixError, msg)
}
```

### Read Receipt Error Handling

**Specific Error Cases**:

1. **Invalid Message ID**:
```go
msg, err := h.service.GetMessage(ctx, roomID, id.EventID(req.MessageID))
if err != nil {
    // Returns specific "message not found" error
    return NewMessageNotFoundError(string(req.MessageID)), nil
}
```

2. **Invalid Thread Root ID**:
```go
// Will be handled by GetThreadMessages returning error
messages, err := h.matrix.GetThreadMessages(ctx, roomID, threadRootID)
if err != nil {
    return MapServiceError(err), nil  // Maps to appropriate error code
}
```

3. **User Not in Room**:
```go
// Matrix SDK will return M_FORBIDDEN
// MapServiceError detects this pattern:
case domain.IsForbiddenError(err):
    return dto.NewErrorResponse(dto.ErrCodeNotAllowed, msg)
```

### Domain Error Types

From [internal/core/domain/errors.go](../../internal/core/domain/errors.go):

```go
// Sentinel errors for typed error checking
var ErrRoomNotFound = fmt.Errorf("room %w", ErrNotFound)
var ErrActorNotFound = fmt.Errorf("actor %w", ErrNotFound)
var ErrForbidden = errors.New("forbidden")
var ErrInvalidParam = errors.New("invalid parameter")

// Detection helpers
func IsNotFoundError(err error) bool {
    return errors.Is(err, ErrNotFound) || strings.Contains(msg, "not found")
}

func IsForbiddenError(err error) bool {
    return errors.Is(err, ErrForbidden) || strings.Contains(msg, "forbidden")
}
```

### Validation Patterns

From [internal/infrastructure/queue/handler_room.go](../../internal/infrastructure/queue/handler_room.go):

```go
// UUID validation
if errResp := RequireUUID(req.SenderActorID, "sender_actor_id"); errResp != nil {
    return *errResp, nil
}

// Non-empty string validation
if errResp := RequireNonEmpty(req.Content, "content"); errResp != nil {
    return *errResp, nil
}

// Room resolution (returns typed error response)
roomID, errResp := h.resolveRoomAlias(ctx, req.AlkemioRoomID)
if errResp != nil {
    return *errResp, nil  // Already formatted as BaseResponse
}
```

### Read Receipt Command Validation

**New Validation for Read Receipts**:

```go
func (h *RoomHandler) HandleMarkMessageRead(ctx context.Context, payload []byte) (interface{}, error) {
    var req dto.MarkMessageReadRequest
    if err := json.Unmarshal(payload, &req); err != nil {
        return NewInvalidPayloadError(err), nil
    }

    // Validate required fields
    if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
        return *errResp, nil
    }
    if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
        return *errResp, nil
    }
    if errResp := RequireNonEmpty(string(req.MessageID), "message_id"); errResp != nil {
        return *errResp, nil
    }
    
    // Optional thread_id validation (only if provided)
    if req.ThreadID != nil && *req.ThreadID == "" {
        return NewInvalidParamError("thread_id cannot be empty if provided"), nil
    }
    
    // Resolve room alias
    roomID, errResp := h.resolveRoomAlias(ctx, req.AlkemioRoomID)
    if errResp != nil {
        return *errResp, nil
    }
    
    // Verify message exists before sending receipt
    _, err := h.matrix.GetMessage(ctx, roomID, id.EventID(req.MessageID))
    if err != nil {
        // Return error but preserve existing receipt state
        return NewMessageNotFoundError(string(req.MessageID)), nil
    }
    
    // Send receipt...
}
```

### Logging Patterns

From [internal/infrastructure/matrix/listener.go](../../internal/infrastructure/matrix/listener.go):

```go
// Info level for successful operations
m.logger.Info("Room created", "room_id", roomID, "alias", alias)

// Warn level for recoverable errors
m.logger.Warn("Failed to set display name", "user_id", userID, "error", err)

// Debug level for non-critical info
m.logger.Debug("Ignoring message from non-UUID user", "sender", evt.Sender)

// Error level for operation failures
m.logger.Error("Error handling message", "error", err)
```

### Rationale

1. **Consistency**: Follows established error handling patterns across the adapter
2. **Type Safety**: Uses sentinel errors for typed error checking via `errors.Is()`
3. **User Friendly**: Validation errors provide clear field-specific messages
4. **Preservation**: Invalid operations don't corrupt existing read receipt state
5. **Matrix SDK Integration**: Gracefully handles SDK errors via string pattern matching

### Code References

- Error Definitions: [internal/core/domain/errors.go](../../internal/core/domain/errors.go)
- Error Mapping: [internal/infrastructure/queue/errors.go](../../internal/infrastructure/queue/errors.go)
- Validation Examples: [internal/infrastructure/queue/handler_room.go](../../internal/infrastructure/queue/handler_room.go)

---

## Implementation Checklist

### Phase 1: Core Read Receipt Support
- [ ] Extend `EventHandlers` with `OnReadReceiptUpdate` callback
- [ ] Implement `handleReceiptEvent()` in `listener.go`
- [ ] Add domain model `ReadReceiptEvent` with room/thread context
- [ ] Create `MarkMessageRead` command handler in `handler_room.go`
- [ ] Implement `SendReceipt()` wrapper in service layer with thread support
- [ ] Add validation for message ID existence before sending receipt

### Phase 2: Unread Count Calculation
- [ ] Create `GetUnreadCounts` command handler
- [ ] Implement `CalculateRoomUnreadCount()` using `Messages()` API
- [ ] Implement `CalculateThreadUnreadCount()` using `GetThreadMessages()`
- [ ] Add DTO types for unread count response (room-level + thread-level)
- [ ] Handle edge cases (no receipts, user not in room, empty rooms)

### Phase 3: Event Publishing
- [ ] Add `ReadReceiptUpdatedEvent` DTO type
- [ ] Implement RMQ publishing in event service
- [ ] Define topic: `communication.receipt.updated`
- [ ] Include room ID, thread ID (optional), user ID, message ID, timestamp

### Phase 4: Testing & Validation
- [ ] Unit tests for receipt sending (room-level and thread-level)
- [ ] Unit tests for unread count calculation
- [ ] Integration tests with mock Matrix SDK boundary
- [ ] Verify thread independence (room receipt doesn't affect threads)
- [ ] Test error handling (invalid message IDs, non-existent threads)

### Phase 5: Documentation
- [ ] Update Protocol V3 docs with receipt commands
- [ ] Document DTO schemas in `pkg/dto/`
- [ ] Add examples to README for receipt operations
- [ ] Update TypeScript lib with new command types

---

## Open Questions

1. **Receipt Persistence**: Should the adapter cache read receipts locally for performance, or always query the homeserver?
   - **Recommendation**: Always query homeserver (stateless architecture per constitution)

2. **Bulk Receipt Operations**: Should we support marking multiple rooms as read in a single command?
   - **Recommendation**: Out of scope for MVP, single-room operations sufficient

3. **Receipt Event Filtering**: Should the adapter forward all receipt events or only for tracked users?
   - **Recommendation**: Only forward receipts for UUID ghost users (filter non-UUID senders)

4. **Pagination Strategy**: How to handle rooms with >1000 messages for unread count calculation?
   - **Recommendation**: Acceptable for MVP; optimization can use pagination tokens if needed

5. **Cross-Device Sync**: How are receipts synchronized when users have multiple active clients?
   - **Answer**: Matrix homeserver handles this automatically; last receipt wins per spec

---

## References

### Matrix Specification
- **Read Receipts**: [Section 11.2.1.4](https://spec.matrix.org/v1.2/client-server-api/#receipts) (Client-Server API v1.2)
- **Threads**: MSC3440 - Threading (not yet in stable spec, supported by mautrix-go)
- **Receipt Types**: `m.read`, `m.read.thread` (thread-specific receipts)
- **Receipt Endpoint**: `PUT /_matrix/client/v3/rooms/{roomId}/receipt/{receiptType}/{eventId}`

### mautrix-go Documentation
- **Package Docs**: [mautrix-go v0.26.0](https://pkg.go.dev/maunium.net/go/mautrix@v0.26.0)
- **SendReceipt**: Added in v0.12.4, supports thread_id parameter
- **ReqSendReceipt**: Thread-aware receipt request struct
- **Messages API**: `Client.Messages()` for pagination with direction and limit

### Existing Codebase Patterns
- **Event Handlers**: [internal/infrastructure/matrix/listener.go](../../internal/infrastructure/matrix/listener.go)
- **Command Handlers**: [internal/infrastructure/queue/handler_room.go](../../internal/infrastructure/queue/handler_room.go)
- **Error Handling**: [internal/infrastructure/queue/errors.go](../../internal/infrastructure/queue/errors.go)
- **Thread Support**: [internal/infrastructure/matrix/mautrix.go](../../internal/infrastructure/matrix/mautrix.go#L808-850)

### SDK Version
- **mautrix-go**: v0.26.0 (confirmed in [go.mod](../../go.mod#L15))
- **Go Version**: 1.25+
