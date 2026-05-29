# Feature Specification: RMQ Error Handling Separation

**Feature Branch**: `002-rmq-error-handling`  
**Created**: 2025-11-28  
**Status**: ✅ Implemented  
**Completed**: 2025-12-01

## Summary

Separates business logic errors from infrastructure errors in RabbitMQ message handling. All handlers now return structured error responses with ACK semantics, enabling consumers to distinguish retryable failures from business errors.

## User Stories

### US-1: Distinguish Business Logic Failures from Infrastructure Errors (P1)

Consumers can now distinguish between:
- **Business errors**: ACKed messages with `success: false` (do not retry)
- **Infrastructure errors**: Connection failures handled by Watermill (retry automatically)

### US-2: Receive Structured Error Information (P2)

All error responses include:
- `code`: Categorized error code
- `message`: Human-readable description

## Implemented Requirements

| ID | Requirement | Status |
|----|-------------|--------|
| FR-001 | ACK all messages after handler execution | ✅ |
| FR-003 | All responses include `success` boolean | ✅ |
| FR-004 | `BaseResponse` type with `success` and optional `error` | ✅ |
| FR-005 | Error responses include `code` and `message` | ✅ |
| FR-006 | Success responses omit `error` field | ✅ |
| FR-007 | Status-only operations use `BaseResponse` directly | ✅ |
| FR-008 | 6 standardized error codes defined | ✅ |
| FR-009 | Structured error logging with context | ✅ |
| FR-010 | No sensitive info in error messages | ✅ |

## Error Codes

| Code | Description |
|------|-------------|
| `INVALID_PAYLOAD` | JSON parsing failed |
| `VALIDATION_ERROR` | UUID or field validation failed |
| `NOT_FOUND` | Entity does not exist |
| `PERMISSION_DENIED` | Operation not permitted |
| `MATRIX_ERROR` | Matrix SDK/homeserver error |
| `INTERNAL_ERROR` | Unexpected system error |

## Consumer Usage

```typescript
import { BaseResponse, ErrorCodeNotFound } from '@alkem-io/matrix-adapter-lib';

if (!response.success) {
  if (response.error?.code === ErrorCodeNotFound) {
    throw new NotFoundException(response.error.message);
  }
}
```

## Files Changed

| File | Change |
|------|--------|
| `pkg/dto/error.go` | NEW: Error types and helpers |
| `pkg/dto/*.go` | All responses embed `BaseResponse` |
| `internal/infrastructure/queue/errors.go` | NEW: Error mapping utilities |
| `internal/infrastructure/queue/watermill.go` | Always ACK + error logging |
| `internal/infrastructure/queue/handler_*.go` | Return error responses |
| `lib/src/dto/generated.ts` | TypeScript definitions regenerated |
