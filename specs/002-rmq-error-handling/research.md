# Research: RMQ Error Handling Separation

**Feature**: 002-rmq-error-handling  
**Status**: ✅ Implemented (research archived)

## Key Decisions Made

| Topic | Decision |
|-------|----------|
| ACK/NACK semantics | Always ACK; business errors in response payload |
| Response structure | `BaseResponse` with `Success` + optional `Error` |
| Error codes | 6 codes: INVALID_PAYLOAD, VALIDATION_ERROR, NOT_FOUND, PERMISSION_DENIED, MATRIX_ERROR, INTERNAL_ERROR |
| Handler signature | Unchanged; returns `(errorResponse, nil)` for business errors |
| TypeScript | `omitempty` for optional Error field |

## Alternatives Rejected

| Alternative | Reason |
|-------------|--------|
| NACK for validation errors | Would cause duplicate operations on retry |
| Separate error type per handler | Inconsistent; hard to maintain |
| Error codes as integers | Less readable in logs |
