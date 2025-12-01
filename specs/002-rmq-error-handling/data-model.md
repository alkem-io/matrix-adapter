# Data Model: RMQ Error Handling Separation

**Feature**: 002-rmq-error-handling  
**Status**: ✅ Implemented

## Entities

### ErrorCode

6 standardized error codes in `pkg/dto/error.go`:

| Value | Description |
|-------|-------------|
| `INVALID_PAYLOAD` | JSON parsing failed |
| `VALIDATION_ERROR` | UUID/field validation failed |
| `NOT_FOUND` | Entity not found |
| `PERMISSION_DENIED` | Operation not permitted |
| `MATRIX_ERROR` | Matrix SDK error |
| `INTERNAL_ERROR` | Unexpected error |

### ErrorResponse

```go
type ErrorResponse struct {
    Code    ErrorCode `json:"code"`
    Message string    `json:"message"`
}
```

### BaseResponse

```go
type BaseResponse struct {
    Success bool           `json:"success"`
    Error   *ErrorResponse `json:"error,omitempty"`
}
```

## Response Types Updated

All 21 response types now embed `BaseResponse`:
- Room: Create, Delete, Details, Members, UpdateState, Invite
- Message: Send, Reply, Delete, Reaction, RemoveReaction, Details
- Actor: Register, AddToRooms, RemoveFromRooms, GetRooms, StartDM, StopDM, DirectRooms
- Admin: AllRooms, ReplicateMembership

## Validation Rules

1. `error` present only when `success: false`
2. Error messages never contain tokens/credentials/stack traces
3. Partial failures use `FailedRooms` array (not `error` field)
