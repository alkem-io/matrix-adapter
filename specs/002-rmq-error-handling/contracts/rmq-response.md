# RabbitMQ Response Contract

**Feature**: 002-rmq-error-handling  
**Status**: ✅ Implemented  
**Version**: 1.0.0

## Base Response Structure

All responses include:

```json
{
  "success": true | false,
  "error": {                    // Only when success: false
    "code": "ERROR_CODE",
    "message": "Human-readable description"
  }
}
```

## Error Codes

| Code | Description |
|------|-------------|
| `INVALID_PAYLOAD` | Malformed JSON |
| `VALIDATION_ERROR` | Business validation failed |
| `NOT_FOUND` | Entity does not exist |
| `PERMISSION_DENIED` | Operation not allowed |
| `MATRIX_ERROR` | Matrix homeserver error |
| `INTERNAL_ERROR` | Unexpected failure |

## ACK Semantics

All handlers always ACK after execution. Infrastructure failures are handled by Watermill internally.

## TypeScript Usage

```typescript
import { BaseResponse, ErrorCodeNotFound } from '@alkem-io/matrix-adapter-go-lib';

if (!response.success && response.error?.code === ErrorCodeNotFound) {
  throw new NotFoundException(response.error.message);
}
```
