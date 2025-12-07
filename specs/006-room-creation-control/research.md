# Research: Room Creation Control

**Feature**: 006-room-creation-control | **Date**: 2025-12-06

## Overview

Research findings for implementing room creation control where only the AppService bot can create rooms, and DM requests from ghost users flow through a webhook → RabbitMQ → Alkemio Server → command pattern.

---

## 1. Synapse Spam Checker Module API

### Decision
Use Synapse's `user_may_create_room` spam checker callback to intercept all room creation requests from ghost users.

### Rationale
- Native Synapse extension point designed for this use case
- Async callback allows webhook communication without blocking indefinitely
- Returns `synapse.api.Codes` for proper Matrix error responses
- Module lifecycle managed by Synapse (singleton per worker)

### Alternatives Considered
1. **Matrix AppService filtering**: Rejected - no native "intercept room creation" hook
2. **Custom Synapse branch**: Rejected - maintenance burden, breaks on Synapse updates
3. **Post-creation room deletion**: Rejected - poor UX, race conditions with invites

### Implementation (Already Done)
Created `synapse-modules/alkemio_room_control.py`:
- `user_may_create_room(user_id, room_config, is_requester_admin)` callback
- Allows AppService bot (configurable `ALLOWED_BOT_USER_IDS`)
- Detects DM intent via `is_direct=True` or `invite` field + preset
- Webhooks to adapter on DM attempts
- Request deduplication with 30s cache (frozenset key)

---

## 2. Webhook Authentication Pattern

### Decision
Use Matrix Homeserver (HS) token authentication via `Authorization: Bearer <hs_token>` header.

### Rationale
- Reuses existing shared secret between Synapse and AppService
- Standard Matrix AppService authentication pattern
- Token already available in adapter config (`hs_token` from `registration.yaml`)
- Simple header-based validation, no additional key exchange

### Alternatives Considered
1. **mTLS**: Rejected - overkill for internal service communication, complex cert management
2. **Shared HMAC signature**: Rejected - requires key distribution, more complex than bearer token
3. **No auth (internal network only)**: Rejected - defense in depth, token provides caller verification

### Implementation Notes
- Adapter reads `hs_token` from config (already parsed from registration.yaml)
- HTTP handler validates `Authorization: Bearer {token}` header
- Return 401 if missing/invalid, 200 on success with optional JSON body
- Log authentication failures with source IP

---

## 3. HTTP Endpoint Design for Go

### Decision
Add new endpoint handler in `internal/infrastructure/http/dm_webhook.go` using stdlib `net/http`.

### Rationale
- Consistent with existing `health.go` pattern
- No additional dependencies (stdlib sufficient)
- Mux-based routing integrates with existing app startup
- Clean separation: HTTP concerns in infrastructure layer

### Alternatives Considered
1. **Echo/Gin framework**: Rejected - adds dependency for single endpoint, existing code uses stdlib
2. **gRPC**: Rejected - Synapse module uses HTTP, no benefit to different protocol
3. **Matrix AppService /transactions endpoint**: Rejected - designed for timeline events, not custom callbacks

### API Contract
```http
POST /_matrix/app/alkemio/dm-request HTTP/1.1
Authorization: Bearer {hs_token}
Content-Type: application/json

{
  "initiator_user_id": "@user1_alkemio-host:matrix.domain",
  "target_user_id": "@user2_alkemio-host:matrix.domain",
  "timestamp": "2024-01-15T10:30:00Z"
}

Response: 200 OK (accepted for processing)
Response: 401 Unauthorized (invalid token)
Response: 400 Bad Request (malformed payload)
```

---

## 4. RabbitMQ Topic Naming Convention

### Decision
Add two new topics following existing convention:
- `communication.room.dm.requested` (outgoing event to Alkemio Server)
- `communication.room.dm.create` (incoming command from Alkemio Server)

### Rationale
- Follows existing `communication.room.*` namespace
- Clear semantic distinction: `.requested` = event, `.create` = command
- Aligns with existing patterns (`TopicRoomCreate`, `TopicMemberJoin`, etc.)
- Enables clean subscription routing in Watermill

### Alternatives Considered
1. **Reuse `TopicRoomCreate`**: Rejected - DM creation has different authorization flow
2. **Generic `dm.request`/`dm.response`**: Rejected - loses semantic clarity of event vs command
3. **Nested topics `room.dm.request.received`**: Rejected - overly verbose, breaks existing flat pattern

### Registry Updates
```go
// pkg/dto/commands.go
const (
    TopicRoomDMRequested = "communication.room.dm.requested"  // Outgoing event
    TopicRoomDMCreate    = "communication.room.dm.create"     // Incoming command
)

// Add to OutgoingEventRegistry
func init() {
    OutgoingEventRegistry[TopicRoomDMRequested] = DMRequestedEvent{}
}

// Add to CommandRegistry
func init() {
    CommandRegistry[TopicRoomDMCreate] = CreateDMRoomCommand{}
}
```

---

## 5. DTO Structure for DM Flow

### Decision
Create new DTOs in `pkg/dto/dm.go` with fields matching existing patterns.

### Rationale
- Separate file maintains single-responsibility (room.go is already large)
- Field naming matches existing DTOs (`AlkemioActorID`, snake_case JSON)
- Response wrapping follows `CreateRoomResponse` pattern

### Design
```go
// DMRequestedEvent - published when ghost user attempts DM creation
type DMRequestedEvent struct {
    InitiatorActorID AlkemioActorID `json:"initiator_actor_id"`
    TargetActorID    AlkemioActorID `json:"target_actor_id"`
    Timestamp        time.Time      `json:"timestamp"`
    CorrelationID    string         `json:"correlation_id,omitempty"`
}

// CreateDMRoomCommand - received from Alkemio Server to create DM room
type CreateDMRoomCommand struct {
    InitiatorActorID AlkemioActorID `json:"initiator_actor_id"`
    TargetActorID    AlkemioActorID `json:"target_actor_id"`
    CorrelationID    string         `json:"correlation_id,omitempty"`
}

// CreateDMRoomResponse - returned after DM room creation
type CreateDMRoomResponse struct {
    RoomID        MatrixRoomID `json:"room_id"`
    CorrelationID string       `json:"correlation_id,omitempty"`
}
```

### Alternatives Considered
1. **Extend CreateRoomRequest**: Rejected - DM has simpler payload, authorization differs
2. **Generic map[string]interface{}**: Rejected - loses type safety, breaks TS generation

---

## 6. Handler Pattern Analysis

### Decision
Follow existing `handler_room.go` pattern for new DM handler.

### Rationale
- Proven pattern in codebase (CreateRoom, GetRoom, DeleteRoom handlers)
- Clean separation: unmarshal → validate → service call → response
- Structured error handling with `NewInvalidPayloadError`
- Consistent logging with operation context

### Key Pattern Elements (from handler_room.go)
```go
func (h *DMHandler) HandleCreateDMRoom(ctx context.Context, payload []byte) (interface{}, error) {
    var req dto.CreateDMRoomCommand
    if err := json.Unmarshal(payload, &req); err != nil {
        h.logger.Error("failed to unmarshal CreateDMRoomCommand", zap.Error(err))
        return NewInvalidPayloadError(err), nil
    }
    
    // Validation
    if req.InitiatorActorID == "" || req.TargetActorID == "" {
        return NewMissingFieldError("initiator_actor_id or target_actor_id"), nil
    }
    
    // Service call
    roomID, err := h.dmService.CreateDMRoom(ctx, req.InitiatorActorID, req.TargetActorID)
    if err != nil {
        // ... error handling
    }
    
    return dto.CreateDMRoomResponse{RoomID: roomID, CorrelationID: req.CorrelationID}, nil
}
```

---

## 7. Error Handling & Retry Strategy

### Decision
Implement fail-closed retry strategy with exponential backoff.

### Rationale
- Synapse module: 3 retries with 100ms/200ms/400ms backoff, then FORBIDDEN
- Adapter webhook: Immediate response, async processing
- Alkemio Server timeout: Adapter retries event 3 times over 60s

### Error Response Mapping
| Scenario | Synapse Module Behavior | Adapter Behavior |
|----------|------------------------|------------------|
| Webhook unreachable | Retry 3x, then FORBIDDEN | N/A |
| Webhook timeout (>5s) | Retry 3x, then FORBIDDEN | N/A |
| Invalid auth | N/A | Return 401, log warning |
| Malformed payload | N/A | Return 400, log error |
| RabbitMQ publish fail | N/A | Return 500, retry internally |
| Room creation fails | N/A | Return error in response payload |

---

## 8. Testing Strategy

### Decision
Unit tests for service/handler logic; integration test for full DM flow.

### Rationale
- Constitution §6: Mock Matrix SDK boundary, integration tests for complex flows
- DM flow spans multiple components (HTTP → Queue → Matrix)
- Unit tests verify business logic isolation
- Integration test ensures end-to-end contract

### Test Scope
1. **Unit Tests**:
   - `dm_webhook_test.go`: Auth validation, payload parsing, error responses
   - `handler_dm_test.go`: Command handling, service calls, response mapping
   - `dm_service_test.go`: Business logic with mocked MatrixPort

2. **Integration Test** (deferred to Phase 2):
   - Full flow: Mock Synapse → Webhook → RabbitMQ → Handler → Matrix mock
   - Verify correlation ID propagation

---

## Open Questions (All Resolved)

| Question | Resolution | Source |
|----------|------------|--------|
| Duplicate DM requests | Synapse module deduplicates with 30s cache | Spec clarification |
| Webhook timeout handling | Retry 3x with backoff, then FORBIDDEN | Spec clarification |
| Server non-response | Adapter retries 3x over 60s | Spec clarification |
| Invalid target user | Server validates, doesn't send command | Spec clarification |

---

## References

- [Synapse Module Development](https://matrix-org.github.io/synapse/latest/modules/index.html)
- [Spam Checker Callbacks](https://matrix-org.github.io/synapse/latest/modules/spam_checker_callbacks.html)
- [mautrix-go Documentation](https://pkg.go.dev/maunium.net/go/mautrix)
- [Watermill RabbitMQ](https://watermill.io/pubsubs/amqp/)
