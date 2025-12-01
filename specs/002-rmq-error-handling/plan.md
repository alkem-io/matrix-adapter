# Implementation Plan: RMQ Error Handling Separation

**Branch**: `002-rmq-error-handling` | **Spec**: [spec.md](spec.md)  
**Status**: ✅ Completed | **Completed**: 2025-12-01

## Summary

Separated business logic errors from infrastructure errors in RabbitMQ message handling. All handlers now return structured error responses with ACK semantics.

## Implementation Summary

| Phase | Description | Status |
|-------|-------------|--------|
| 1. Error Types | Created `pkg/dto/error.go` with ErrorCode, ErrorResponse, BaseResponse | ✅ |
| 2. Infrastructure | Updated `watermill.go` to always ACK, added error logging | ✅ |
| 3. DTO Updates | Embedded BaseResponse in all 21 response types | ✅ |
| 4. Handler Updates | Updated all 21 handlers to return error responses | ✅ |
| 5. Validation | `make generate`, `make lint`, `make build` all pass | ✅ |

## Files Changed

### Created
- `pkg/dto/error.go` - Error types, codes, helper functions
- `internal/infrastructure/queue/errors.go` - Error mapping utilities

### Modified
- `pkg/dto/room_manage.go` - 4 response types
- `pkg/dto/room_message.go` - 6 response types  
- `pkg/dto/actor_dm.go` - 3 response types
- `pkg/dto/actor_rooms.go` - 3 response types
- `pkg/dto/actor.go` - 1 response type
- `pkg/dto/room.go` - 2 response types
- `pkg/dto/admin.go` - 2 response types
- `internal/infrastructure/queue/watermill.go` - Always ACK + error logging
- `internal/infrastructure/queue/handler_actor.go` - 7 handlers
- `internal/infrastructure/queue/handler_room.go` - 12 handlers
- `internal/infrastructure/queue/handler_admin.go` - 2 handlers

### Regenerated
- `lib/src/dto/generated.ts` - TypeScript definitions

## Constitution Compliance

All principles verified:
- **Adapter Isolation**: Changes in `pkg/dto` and `internal/infrastructure/queue` only
- **Contract Stability**: Additive changes, TypeScript lib regenerated
- **Observability**: Structured error logging implemented
- **Source of Truth**: Go DTOs → TypeScript via `tygo`
