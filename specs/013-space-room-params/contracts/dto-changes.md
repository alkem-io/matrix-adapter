# Contract Changes: Wire joinRule for Rooms & Remove isPublic

**Date**: 2026-03-25

## Breaking Change: UpdateRoomRequest

### Removed field

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `IsPublic` | `*bool` | `is_public` | **REMOVED** — was never implemented (silently ignored) |

### Wired field (existing, no schema change)

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `JoinRule` | `*JoinRule` | `join_rule` | **NOW FUNCTIONAL** — was present but not wired |

## No Schema Change: CreateRoomRequest

| Field | Type | JSON | Status |
|-------|------|------|--------|
| `JoinRule` | `JoinRule` | `join_rule` | **NOW FUNCTIONAL** — was present but not wired |

## TypeScript Library Impact

After `make generate`, the `UpdateRoomRequest` interface in `lib/src/dto/generated.ts` will:
- Remove `is_public?: boolean`
- Retain `join_rule?: JoinRule` (already present, now functional)

The `CreateRoomRequest` interface is unchanged (already has `join_rule?: JoinRule`).

## Migration Guide for Callers

1. Replace any `is_public: true` usage with `join_rule: "public"`
2. Replace any `is_public: false` usage with `join_rule: "invite"`
3. No changes needed for callers already using `join_rule`
