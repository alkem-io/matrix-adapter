# Feature Specification: Room Creation Control

**Feature Branch**: `006-room-creation-control`  
**Created**: 2025-12-06  
**Status**: ✅ Implemented  
**Completed**: 2025-12-07

## Problem Statement

All Matrix users in Alkemio's homeserver are ghost users provisioned by the AppService. Room creation must be controlled to ensure Alkemio Server maintains authority over all communication channels:

1. **Community Rooms**: Created ONLY via Alkemio Server commands through the adapter
2. **DM Rooms**: Users can initiate from Element, but actual creation is authorized and commanded by Alkemio Server

### User Model
- **All users are ghost users** (`@uuid:domain`) provisioned by the AppService
- Users authenticate via OIDC directly to Synapse
- No "regular" Matrix users exist on this homeserver

## User Scenarios & Testing

### User Story 1 - Block Unauthorized Community Room Creation (Priority: P1)

As an Alkemio platform administrator, I want to prevent users from creating community rooms directly in Element, so that all community rooms are created through Alkemio's authorization flow.

**Why this priority**: Core security requirement - ensures Alkemio Server maintains control over community structure.

**Independent Test**: Attempt to create a named room in Element as a ghost user → should be rejected with FORBIDDEN error.

**Acceptance Scenarios**:

1. **Given** a ghost user logged into Element, **When** they attempt to create a new room with a name, **Then** the request is rejected with M_FORBIDDEN error
2. **Given** a ghost user logged into Element, **When** they attempt to create a group room with multiple invitees, **Then** the request is rejected with M_FORBIDDEN error
3. **Given** the AppService bot, **When** it creates a room via the adapter, **Then** the room is created successfully

---

### User Story 2 - DM Room Request Flow (Priority: P1)

As an Alkemio user using Element, I want to initiate a direct message conversation with another user, so that I can communicate privately while Alkemio tracks and authorizes the conversation.

**Why this priority**: Core user functionality - users need to communicate via DMs.

**Independent Test**: Click "New DM" in Element → see room appear after short delay (server processing).

**Acceptance Scenarios**:

1. **Given** a ghost user in Element, **When** they attempt to create a DM with another user (is_direct=true, 1 invitee), **Then** the Synapse module sends a webhook to the adapter
2. **Given** the adapter receives a DM request webhook, **When** it processes the request, **Then** it publishes a `communication.room.dm.requested` event to RabbitMQ
3. **Given** Alkemio Server receives the DM request event, **When** it approves the request, **Then** it sends a `communication.room.create` command with `type: "direct"` and both actors in `initial_members`
4. **Given** the adapter receives the room.create command with `type: "direct"`, **When** it processes the command, **Then** it creates a DM room with both users as members (using existing handler)

---

### User Story 3 - AppService Bot Room Creation (Priority: P1)

As the Alkemio Server, I want to create rooms through the adapter's AppService bot, so that I maintain full control over room provisioning.

**Why this priority**: Foundation for all room creation - adapter must be able to create rooms.

**Independent Test**: Send room.create command via RabbitMQ → room created successfully.

**Acceptance Scenarios**:

1. **Given** the AppService bot user, **When** the Synapse module checks room creation permission, **Then** the bot is allowed to create rooms
2. **Given** a room.create command from Alkemio Server, **When** the adapter processes it, **Then** the room is created via the AppService bot

---

### Edge Cases

- **Duplicate DM requests**: Synapse module deduplicates requests for the same user pair within a 30-second cache window; only the first request is forwarded to the adapter
- **Webhook timeout/error**: Synapse module retries webhook 2-3 times with exponential backoff; if all retries fail, returns M_FORBIDDEN to user (fail-closed behavior)
- **Server non-response**: Adapter retries publishing DM request event to RabbitMQ 2-3 times over 60 seconds; if no `dm.create` command received, request is dropped and user can retry via UI
- **Invalid target user**: Alkemio Server validates target user existence; if invalid, Server simply doesn't send `dm.create` command (no room created, no explicit error to user)

## Requirements

### Functional Requirements

- **FR-001**: System MUST reject all room creation requests from ghost users at the Synapse level
- **FR-002**: System MUST allow room creation requests from the AppService bot user only
- **FR-003**: System MUST detect DM creation attempts (is_direct=true with single invitee)
- **FR-004**: System MUST notify the adapter via webhook when a DM creation is attempted
- **FR-005**: Adapter MUST publish `communication.room.dm.requested` event when receiving DM webhook
- **FR-006**: Adapter handles DM room creation via existing `communication.room.create` with `type: "direct"` (no new command needed)
- **FR-007**: DM rooms MUST be created with is_direct flag and both users as members
- **FR-008**: Synapse module MUST authenticate webhook calls using AppService HS token

### Key Entities

- **DM Request**: Represents a user's intent to start a DM (initiator, target, timestamp)
- **DM Room**: A Matrix room with is_direct=true and exactly two members

## Success Criteria

### Measurable Outcomes

- **SC-001**: 100% of unauthorized room creation attempts are blocked at Synapse level
- **SC-002**: DM requests are processed end-to-end in under 5 seconds (webhook → event → command → room)
- **SC-003**: AppService bot can create rooms without any restrictions
- **SC-004**: Users see the new DM room appear in Element after initiating (async creation)

## Architecture

### Room Creation Flow

#### Community Rooms
```
Alkemio Server → RabbitMQ → Adapter → AppService Bot creates room
```

#### DM Rooms (User-Initiated)
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. User clicks "New DM" in Element                                          │
│    ↓                                                                        │
│ 2. Synapse receives createRoom request (is_direct=true)                     │
│    ↓                                                                        │
│ 3. Synapse Module intercepts, extracts initiator + target                   │
│    ↓                                                                        │
│ 4. Module calls webhook to Adapter (POST /_matrix/app/alkemio/dm-request)   │
│    ↓                                                                        │
│ 5. Adapter publishes "dm.request" event to RabbitMQ                         │
│    ↓                                                                        │
│ 6. Alkemio Server receives request, validates, decides                      │
│    ↓                                                                        │
│ 7. Server sends "room.dm.create" command via RabbitMQ                       │
│    ↓                                                                        │
│ 8. Adapter receives command, creates room via AppService Bot                │
│    ↓                                                                        │
│ 9. User sees new DM room in Element                                         │
│    ↓                                                                        │
│ 10. Module returns FORBIDDEN to original request (room already created)     │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Note**: The original createRoom request is always rejected. The room is created asynchronously by the AppService bot after server approval.

## Assumptions

- Synapse supports the `user_may_create_room` spam checker callback (Synapse 1.37+)
- The adapter's AppService HTTP server is accessible from Synapse for webhook calls
- Alkemio Server will implement the corresponding event handler and command sender
- Users will tolerate a brief delay (1-5 seconds) for DM room creation

## Clarifications

### Session 2025-12-06

- Q: How should duplicate DM requests for the same user pair be handled? → A: Synapse module deduplicates with short window cache (30s) before forwarding to adapter
- Q: What should Synapse module do when adapter webhook times out or errors? → A: Retry 2-3 times with backoff, then return FORBIDDEN to user (fail-closed)
- Q: What if Alkemio Server doesn't respond to DM request event? → A: Adapter retries event 2-3 times over 60 seconds; if no response, request dropped (user can retry)
- Q: What if target user doesn't exist in Alkemio? → A: Alkemio Server validates and simply doesn't send create command; no room created
