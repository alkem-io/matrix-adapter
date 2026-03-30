# Quickstart: Fix Thread Reply Formatting and Message Ordering

**Date**: 2026-03-30
**Status**: Already implemented (retrofit)

## What Changed

Two methods in `internal/infrastructure/matrix/mautrix.go`:

### 1. `SendReply` (~line 498)

The `RelatesTo` field now includes proper MSC3440 thread relation fields:
- `Type: event.RelThread` — marks this as a thread reply
- `EventID: threadID` — references the thread root
- `IsFallingBack: true` — enables backwards compatibility

### 2. `GetThreadMessages` (~line 1160)

- Root message is parsed before fetching replies but appended **last** (not first)
- Nil guard on root message in both the error path and the append path
- Comment documents that the relations API returns newest-first

## How to Verify

```bash
# Build
make build

# Run tests
make test

# Lint
make lint
```

### Manual Verification

1. Send a reply to a threaded message via the adapter
2. Check in Element that the reply appears within the thread (not as a standalone message)
3. Fetch thread messages and verify root is first, replies in chronological order
