# Quickstart: Element-Initiated Conversation Creation (Synchronous Check)

**Feature**: 050-element-room-check | **Date**: 2026-05-20

## Prerequisites

- Docker Compose environment running: Synapse, RabbitMQ, Alkemio Server, Matrix Adapter
- At least 3 ghost users provisioned (users A, B, C with messaging enabled)
- One user with messaging disabled (user D) for rejection testing
- Updated Synapse module (`alkemio_room_control.py`) deployed

## Test Scenario 1: DM Creation (Happy Path)

1. Log into Element as User A
2. Start a new DM with User B
3. Type and send a message

**Expected**:
- Room is created within 5 seconds
- Both User A and User B are members
- User B sees the message as unread
- Conversation appears on the Alkemio platform
- No invite events in the room timeline

## Test Scenario 2: Group Conversation (Happy Path)

1. Log into Element as User A
2. Create a new room, invite User B and User C
3. Send a message

**Expected**:
- Room is created within 5 seconds
- All three users are members
- Conversation appears on the Alkemio platform

## Test Scenario 3: DM Rejection (Duplicate)

1. Ensure a DM already exists between User A and User B on the platform
2. Log into Element as User A
3. Attempt to create another DM with User B

**Expected**:
- Element shows a 403 error (room creation blocked)
- No orphaned room is created

## Test Scenario 4: DM Rejection (Consent Disabled)

1. Log into Element as User A
2. Attempt to create a DM with User D (messaging disabled)

**Expected**:
- Element shows a 403 error with "messaging disabled" reason
- No orphaned room is created

## Test Scenario 5: Service Unavailable

1. Stop the Matrix Adapter service
2. Log into Element as User A
3. Attempt to create a DM with User B

**Expected**:
- Element shows a 503 error within 5 seconds
- No orphaned room is created
- After restarting the adapter, retry succeeds

## Test Scenario 6: Space Blocking (Regression)

1. Log into Element as User A (ghost user)
2. Attempt to create a Space

**Expected**:
- Element shows a 403 error
- Space creation remains blocked for ghost users

## Verification Checklist

- [ ] Room timeline has no invite events (only join events from EnsureJoined)
- [ ] Room alias is set (visible in room settings)
- [ ] io.alkemio.visibility is set to `{visible: true}`
- [ ] Bot is NOT a member after reconciliation (bot leaves)
- [ ] Unread counts: first message is unread for non-initiator members
- [ ] m.direct account data is set for DM rooms
- [ ] Power levels: users_default=50, no user above 50 after reconciliation
- [ ] RoomCreatedEvent received by server (check server logs / GraphQL subscription)
