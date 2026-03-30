# Research: Fix Thread Reply Formatting and Message Ordering

**Date**: 2026-03-30
**Status**: Complete (retrofit — decisions already validated by implementation)

## R1: MSC3440 Thread Relation Format

**Decision**: Use dual-relation format: `m.thread` (with `rel_type`, `event_id`, `is_falling_back: true`) alongside `m.in_reply_to` in the same `m.relates_to` object.

**Rationale**: MSC3440 specifies that thread replies MUST include `rel_type: m.thread` with the thread root `event_id`. The `is_falling_back: true` flag combined with `m.in_reply_to` ensures backwards compatibility — clients that don't understand threads will render the message as a standard reply. This is the canonical format used by Element and other major Matrix clients.

**Alternatives considered**:
- Thread relation only (no `m.in_reply_to`): Breaks backwards compatibility with older clients.
- `m.in_reply_to` only (previous implementation): Thread-aware clients cannot identify the message as part of a thread; it appears as a standalone reply in the room timeline.

## R2: Relations API Response Ordering

**Decision**: Account for the Matrix relations API (`/relations`) returning events in reverse-chronological order (newest first), and append the root message last in the returned slice so the consumer's `.reverse()` places it at position 0.

**Rationale**: The Matrix spec's `/relations` endpoint returns events newest-first by default. The Alkemio Server reverses the message list it receives from the adapter. By appending the root message as the last element, after reversal it becomes the first element — giving the consumer a chronologically ordered thread (root → oldest reply → newest reply).

**Alternatives considered**:
- Sort messages by timestamp in the adapter: Adds complexity and allocations; the consumer already reverses, so leveraging that existing behavior is simpler.
- Prepend root before fetching replies (previous implementation): After the consumer's reverse, the root ended up at the end instead of the beginning.

## R3: Nil Root Message Handling

**Decision**: Guard all root message appends with a nil check. If the root event cannot be parsed, return only the parseable reply messages.

**Rationale**: The previous implementation unconditionally dereferenced the root message pointer, which could panic if `parseMessageEvent` returned nil. Defensive nil checks ensure graceful degradation — the consumer gets whatever messages could be parsed rather than an error.

**Alternatives considered**:
- Return an error if root can't be parsed: Overly strict; the replies are still valid and useful without the root.
- Skip the root fetch entirely: The root provides important context; it should be included when available.
