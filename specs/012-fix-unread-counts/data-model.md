# Data Model: Fix Unreliable Unread Message Counts

## Entities

### No new entities required

This feature modifies the internal calculation logic of existing operations. The domain model, ports, services, handlers, and DTOs remain unchanged.

### Existing entities used (no modifications)

| Entity | Location | Role in this feature |
|--------|----------|---------------------|
| `domain.UnreadCountSummary` | `internal/core/domain/read_receipt.go` | Return type for `GetUnreadCounts` (unchanged) |
| `domain.Actor` | `internal/core/domain/model.go` | Identifies the requesting user (unchanged) |
| `domain.ReadReceiptEvent` | `internal/core/domain/read_receipt.go` | Used by listener for incoming receipts (unchanged) |

### Internal data flow (new, within mautrix.go only)

The following intermediate data is extracted during calculation but NOT persisted or exposed:

| Data | Source | Purpose |
|------|--------|---------|
| User's read receipt event ID | `/sync` ephemeral `m.receipt` | Anchor point for counting unread |
| Synapse notification_count | `/sync` `UnreadNotifications` | Fallback value |
| Timeline events | `intent.Messages()` backward | Scanned to count messages after receipt |

## State Transitions

None. This feature does not introduce new state or lifecycle changes. It changes how an existing value (unread count) is calculated.

## Validation Rules

- Receipt event ID must be a valid Matrix event ID format (`$` prefix)
- Room IDs must be valid Matrix room IDs
- Actor must be a registered appservice user (enforced by existing `EnsureRegistered` call)
