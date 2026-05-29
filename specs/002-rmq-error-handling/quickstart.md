# Quickstart: RMQ Error Handling

**Feature**: 002-rmq-error-handling  
**Status**: ✅ Implemented

## Handler Pattern

Handlers return error responses (ACK) instead of errors (NACK):

```go
func (h *ActorHandler) HandleRegister(payload []byte) (interface{}, error) {
    var req dto.ActorRegisterPayload
    if err := json.Unmarshal(payload, &req); err != nil {
        return NewInvalidPayloadError(err), nil  // ACK with error
    }
    
    actorID, err := uuid.Parse(req.ActorID)
    if err != nil {
        return NewValidationError("Invalid actor ID"), nil  // ACK with error
    }
    
    matrixID, err := h.service.RegisterActor(ctx, actor)
    if err != nil {
        return MapServiceError(err), nil  // ACK with mapped error
    }
    
    return dto.ActorRegisterResponsePayload{
        BaseResponse: dto.NewSuccessResponse(),
        MatrixID:     string(matrixID),
    }, nil
}
```

## Consumer Usage (TypeScript)

```typescript
import { BaseResponse, ErrorCodeNotFound } from '@alkem-io/matrix-adapter-lib';

if (!response.success) {
  switch (response.error?.code) {
    case 'NOT_FOUND':
      throw new NotFoundException(response.error.message);
    case 'VALIDATION_ERROR':
      throw new BadRequestException(response.error.message);
    default:
      throw new InternalServerErrorException(response.error?.message);
  }
}
```

## Error Helper Functions

Located in `internal/infrastructure/queue/errors.go`:

- `NewInvalidPayloadError(err)` → `INVALID_PAYLOAD`
- `NewValidationError(msg)` → `VALIDATION_ERROR`  
- `MapServiceError(err)` → Maps by error message patterns
