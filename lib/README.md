# @alkem-io/matrix-adapter-go-lib

TypeScript library providing type-safe DTOs and event types for interacting with the Alkemio Matrix Adapter service.

## Installation

```bash
# npm
npm install @alkem-io/matrix-adapter-go-lib

# pnpm
pnpm add @alkem-io/matrix-adapter-go-lib

# yarn
yarn add @alkem-io/matrix-adapter-go-lib
```

## Usage

### Event Types

Use `MatrixAdapterEventType` enum for RabbitMQ topic routing:

```typescript
import { MatrixAdapterEventType } from '@alkem-io/matrix-adapter-go-lib';

// Subscribe to commands
consumer.subscribe(MatrixAdapterEventType.COMMUNICATION_ROOM_CREATE);
consumer.subscribe(MatrixAdapterEventType.COMMUNICATION_SPACE_CREATE);

// Listen for events
consumer.subscribe(MatrixAdapterEventType.COMMUNICATION_MESSAGE_RECEIVED);
```

### Request/Response DTOs

All command payloads and responses are fully typed:

```typescript
import {
  CreateRoomRequest,
  CreateRoomResponse,
  RoomTypeCommunity,
  JoinRuleInvite,
  BaseResponse,
  ErrCodeRoomNotFound,
} from '@alkem-io/matrix-adapter-go-lib';

// Build a request
const request: CreateRoomRequest = {
  alkemio_room_id: 'uuid-v4-or-v7',
  type: RoomTypeCommunity,
  name: 'My Room',
  initial_members: ['actor-uuid-1', 'actor-uuid-2'],
  join_rule: JoinRuleInvite,
};

// Handle response
function handleResponse(response: CreateRoomResponse) {
  if (response.BaseResponse.success) {
    console.log('Room created');
  } else {
    const error = response.BaseResponse.error;
    if (error?.code === ErrCodeRoomNotFound) {
      console.error('Room not found:', error.message);
    }
  }
}
```

### Space Operations (Protocol V3)

```typescript
import {
  CreateSpaceRequest,
  GetSpaceResponse,
  SetParentRequest,
  BatchAddSpaceMemberRequest,
  JoinRuleRestricted,
} from '@alkem-io/matrix-adapter-go-lib';

// Create a space with hierarchy
const spaceRequest: CreateSpaceRequest = {
  alkemio_context_id: 'context-uuid',
  name: 'My Space',
  parent_context_id: 'parent-context-uuid',
  join_rule: JoinRuleRestricted,
  initial_members: ['actor-uuid'],
};

// Set parent-child relationship
const hierarchyRequest: SetParentRequest = {
  child_id: 'room-or-space-uuid',
  is_space: false, // true for subspaces, false for rooms
  parent_context_id: 'parent-space-uuid',
};

// Batch membership operations
const batchRequest: BatchAddSpaceMemberRequest = {
  actor_id: 'actor-uuid',
  alkemio_context_ids: ['space-1', 'space-2', 'space-3'],
};
```

## API Reference

### Event Types

| Event | Topic | Description |
|-------|-------|-------------|
| `COMMUNICATION_ROOM_CREATE` | `communication.room.create` | Create a room |
| `COMMUNICATION_ROOM_GET` | `communication.room.get` | Get room details |
| `COMMUNICATION_ROOM_UPDATE` | `communication.room.update` | Update room metadata |
| `COMMUNICATION_ROOM_DELETE` | `communication.room.delete` | Delete a room |
| `COMMUNICATION_ROOM_LIST` | `communication.room.list` | List all rooms |
| `COMMUNICATION_ROOM_MEMBERS_GET` | `communication.room.members.get` | Get room members |
| `COMMUNICATION_ROOM_MEMBER_BATCH_ADD` | `communication.room.member.batch.add` | Add actor to rooms |
| `COMMUNICATION_ROOM_MEMBER_BATCH_REMOVE` | `communication.room.member.batch.remove` | Remove actor from rooms |
| `COMMUNICATION_SPACE_CREATE` | `communication.space.create` | Create a space |
| `COMMUNICATION_SPACE_GET` | `communication.space.get` | Get space details |
| `COMMUNICATION_SPACE_UPDATE` | `communication.space.update` | Update space metadata |
| `COMMUNICATION_SPACE_DELETE` | `communication.space.delete` | Delete a space |
| `COMMUNICATION_SPACE_LIST` | `communication.space.list` | List all spaces |
| `COMMUNICATION_SPACE_MEMBER_BATCH_ADD` | `communication.space.member.batch.add` | Add actor to spaces |
| `COMMUNICATION_SPACE_MEMBER_BATCH_REMOVE` | `communication.space.member.batch.remove` | Remove actor from spaces |
| `COMMUNICATION_HIERARCHY_SET_PARENT` | `communication.hierarchy.set_parent` | Set parent-child relationship |
| `COMMUNICATION_MESSAGE_SEND` | `communication.message.send` | Send a message |
| `COMMUNICATION_MESSAGE_GET` | `communication.message.get` | Get message details |
| `COMMUNICATION_MESSAGE_DELETE` | `communication.message.delete` | Delete a message |
| `COMMUNICATION_REACTION_ADD` | `communication.reaction.add` | Add reaction |
| `COMMUNICATION_REACTION_REMOVE` | `communication.reaction.remove` | Remove reaction |
| `COMMUNICATION_REACTION_GET` | `communication.reaction.get` | Get reaction details |
| `COMMUNICATION_THREAD_MESSAGES_GET` | `communication.thread.messages.get` | Get thread messages |
| `COMMUNICATION_ACTOR_SYNC` | `communication.actor.sync` | Sync actor profile |
| `COMMUNICATION_MESSAGE_RECEIVED` | `communication.message.received` | Message received event |
| `COMMUNICATION_ROOM_DM_REQUESTED` | `communication.room.dm.requested` | DM room creation requested (outbound) |
| `COMMUNICATION_REACTION_ADDED` | `communication.reaction.added` | Reaction added event (outbound) |
| `COMMUNICATION_REACTION_REMOVED` | `communication.reaction.removed` | Reaction removed event (outbound) |
| `COMMUNICATION_ROOM_MEMBER_LEFT` | `communication.room.member.left` | Room member left event (outbound) |

### Type Aliases

| Type | Description |
|------|-------------|
| `AlkemioRoomID` | UUID identifying a room in Alkemio |
| `AlkemioActorID` | UUID identifying an actor (user/bot) |
| `AlkemioContextID` | UUID identifying a context (Space) |
| `MessageID` | Opaque message identifier |
| `ReactionID` | Opaque reaction identifier |

### Constants

#### Room Types
- `RoomTypeCommunity` - General community room
- `RoomTypeDirect` - Direct message room (exactly 2 actors)

#### Join Rules
- `JoinRulePublic` - Anyone can join
- `JoinRuleInvite` - Requires invitation
- `JoinRuleRestricted` - Members of parent space can join (MSC3083)

#### Error Codes
- `ErrCodeInvalidParam` - Request validation failed
- `ErrCodeRoomNotFound` - Room does not exist
- `ErrCodeSpaceNotFound` - Space does not exist
- `ErrCodeActorNotFound` - Actor does not exist
- `ErrCodeMatrixError` - Matrix SDK/homeserver error
- `ErrCodeInternalError` - Unexpected system error
- `ErrCodeNotAllowed` - Operation not permitted

## Response Handling

All responses extend `BaseResponse`:

```typescript
interface BaseResponse {
  success: boolean;
  error?: ErrorResponse;
}

interface ErrorResponse {
  code: ErrorCode;
  message: string;
  details?: string;
}
```

Batch operations return per-item results using `BaseResponse`:

```typescript
interface BatchAddMemberResponse {
  success: boolean;
  error?: ErrorResponse;
  results?: { [roomId: string]: BaseResponse };
}
```

## Generation

This library is **auto-generated** from Go source code in `pkg/dto/`. Do not edit `src/dto/generated.ts` directly.

To regenerate after Go DTO changes:

```bash
# From repository root
make generate
```

## License

EUPL-1.2
