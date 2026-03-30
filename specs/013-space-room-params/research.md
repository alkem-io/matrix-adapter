# Research: Wire joinRule for Rooms & Remove isPublic

**Date**: 2026-03-25

## No NEEDS CLARIFICATION items

All technical context is resolved from codebase analysis. No external research required.

## Decision: Follow Space Pattern for Room joinRule

**Decision**: Wire `joinRule` through the room creation and update flows using the exact same pattern as spaces.

**Rationale**: The space operations (`CreateSpace`, `UpdateSpace`) already implement joinRule end-to-end via `m.room.join_rules` state events. Rooms should follow the identical pattern for consistency and correctness.

**Alternatives considered**:
- Using Matrix room presets instead of explicit state events: Rejected — presets only work at creation time and don't support updates. State events work for both.
- Keeping `isPublic` as an alias alongside `joinRule`: Rejected — `joinRule` is already the consistent pattern across spaces, and `isPublic` was never implemented.

## Decision: Remove isPublic field

**Decision**: Remove `IsPublic *bool` from `UpdateRoomRequest` DTO.

**Rationale**: The field was never implemented (service explicitly ignores it with `_ *bool`). `joinRule` already exists in the same DTO and is the correct, expressive mechanism. Keeping both creates confusion.

**Alternatives considered**:
- Deprecation period with `isPublic` mapping to `joinRule`: Rejected — field was never functional, so no callers depend on it.

## Decision: Direct-message room override

**Decision**: When `roomType == "direct"`, ignore `joinRule` and always use `trusted_private_chat` preset.

**Rationale**: Direct-message rooms must remain private by design. This matches the existing behavior where the preset is hardcoded for DM rooms.

## Key Implementation Pattern (from spaces)

CreateSpace sets joinRule via initial state event:
```go
if joinRule != "" {
    joinRuleContent := &event.JoinRulesEventContent{
        JoinRule: event.JoinRule(joinRule),
    }
    initialState = append(initialState, &event.Event{
        Type:    event.StateJoinRules,
        Content: event.Content{Parsed: joinRuleContent},
    })
}
```

UpdateSpaceState sets joinRule via SendStateEvent:
```go
if joinRule != "" {
    joinRuleContent := &event.JoinRulesEventContent{
        JoinRule: event.JoinRule(joinRule),
    }
    intent.SendStateEvent(ctx, roomID, event.StateJoinRules, "", joinRuleContent)
}
```

Both patterns will be replicated for rooms.
