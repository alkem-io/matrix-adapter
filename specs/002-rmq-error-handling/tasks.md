# Tasks: RMQ Error Handling Separation

**Status**: ✅ Completed (50/51 tasks, T051 manual testing pending)

## Summary

| Phase | Tasks | Status |
|-------|-------|--------|
| Setup | T001-T003 | ✅ Complete |
| Foundational | T004-T006 | ✅ Complete |
| DTO Updates | T007-T027 | ✅ Complete |
| Handler Updates | T028-T048 | ✅ Complete |
| Polish | T049-T050 | ✅ Complete |
| Manual Testing | T051 | ⏳ Pending |

## Completed Tasks

### Phase 1: Setup
- [X] T001 Create ErrorCode enumeration and ErrorResponse struct in pkg/dto/error.go
- [X] T002 Create BaseResponse struct in pkg/dto/error.go
- [X] T003 Add helper functions: NewErrorResponse(), NewSuccessResponse()

### Phase 2: Foundational
- [X] T004 Create error helpers in internal/infrastructure/queue/errors.go
- [X] T005 Simplify processMessage() to always ACK
- [X] T006 Add structured error logging in watermill.go

### Phase 3: DTO Updates (T007-T027)
- [X] All 21 response types updated to embed BaseResponse

### Phase 3: Handler Updates (T028-T048)
- [X] All 21 handlers updated to return error responses

### Phase 4: Polish
- [X] T049 Run `make generate` - TypeScript regenerated
- [X] T050 Run `make lint && make build` - Passed
- [ ] T051 Manual validation with running service:
  - Send `actor.register` with invalid UUID → expect `VALIDATION_ERROR`
  - Send `room.details` for non-existent room → expect `NOT_FOUND`
  - Verify structured logging (correlationId, errorCode, entityIds)
