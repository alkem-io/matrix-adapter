# Quickstart: Fix Unreliable Unread Message Counts

## What changed

The `GetUnreadCounts` and `GetBatchUnreadCounts` methods in `internal/infrastructure/matrix/mautrix.go` now self-calculate unread counts instead of trusting Synapse's `notification_count`.

## How it works

1. **Get receipt position**: `/sync` with ephemeral events enabled extracts the user's `m.read` receipt event ID per room
2. **Count messages**: Progressive batch fetch backward from latest event, counting `m.room.message` events after the receipt position (excluding self-sent)
3. **Fallback**: If receipt not found in 200 events or no receipt in ephemeral → uses Synapse's `notification_count`

## Testing

```bash
make test          # Unit tests with mocked MatrixPort
make build         # Verify compilation
```

## Key files

| File | Change |
|------|--------|
| `internal/infrastructure/matrix/mautrix.go` | Rewritten `GetUnreadCounts` and `GetBatchUnreadCounts` |
| `go.mod` | Updated mautrix-go to v0.26.4 |

## No interface changes

- `MatrixPort` interface unchanged
- DTOs unchanged
- Queue handlers unchanged
- Service layer unchanged
