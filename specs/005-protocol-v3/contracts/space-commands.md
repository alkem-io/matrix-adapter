# RabbitMQ Command Contracts: Protocol V3

**Feature**: 005-protocol-v3  
**Status**: ✅ Implemented (reference archived)

## New Event Topics

### Space Commands

| Topic | Request DTO | Response DTO |
|-------|-------------|--------------|
| `communication.space.create` | `CreateSpaceRequest` | `CreateSpaceResponse` |
| `communication.space.update` | `UpdateSpaceRequest` | `UpdateSpaceResponse` |
| `communication.space.delete` | `DeleteSpaceRequest` | `DeleteSpaceResponse` |
| `communication.space.get` | `GetSpaceRequest` | `GetSpaceResponse` |
| `communication.space.list` | `ListSpacesRequest` | `ListSpacesResponse` |
| `communication.space.member.batch.add` | `BatchAddSpaceMemberRequest` | `BatchAddSpaceMemberResponse` |
| `communication.space.member.batch.remove` | `BatchRemoveSpaceMemberRequest` | `BatchRemoveSpaceMemberResponse` |

### Hierarchy Commands

| Topic | Request DTO | Response DTO |
|-------|-------------|--------------|
| `communication.hierarchy.set_parent` | `SetParentRequest` | `SetParentResponse` |

---

## Command Specifications

### 1. communication.space.create

Creates a new Matrix Space (Context).

**Request**:
```json
{
  "alkemio_context_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "My Space",
  "topic": "A collaborative space",
  "avatar_url": "mxc://matrix.org/abc123",
  "parent_context_id": "660e8400-e29b-41d4-a716-446655440000",
  "initial_members": ["770e8400-e29b-41d4-a716-446655440000"],
  "join_rule": "restricted"
}
```

**Response (Success)**:
```json
{
  "success": true
}
```

**Response (Error - Space exists)**:
```json
{
  "success": true
}
```
Note: Idempotent - returns success if Space already exists.

**Response (Error - Invalid parent)**:
```json
{
  "success": false,
  "error": {
    "code": "INVALID_PARAM",
    "message": "parent_context_id not found"
  }
}
```

---

### 2. communication.space.update

Updates Space metadata.

**Request**:
```json
{
  "alkemio_context_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Updated Space Name",
  "topic": "Updated topic",
  "avatar_url": "mxc://matrix.org/xyz789",
  "is_public": true,
  "join_rule": "public"
}
```

**Response (Success)**:
```json
{
  "success": true
}
```

**Response (Error - Not found)**:
```json
{
  "success": false,
  "error": {
    "code": "ROOM_NOT_FOUND",
    "message": "Space 550e8400-e29b-41d4-a716-446655440000 not found"
  }
}
```

---

### 3. communication.space.delete

Deletes/archives a Space.

**Request**:
```json
{
  "alkemio_context_id": "550e8400-e29b-41d4-a716-446655440000",
  "reason": "Space deprecated"
}
```

**Response (Success)**:
```json
{
  "success": true
}
```

Note: Idempotent - returns success if Space doesn't exist.

---

### 4. communication.space.get

Retrieves Space details including hierarchy.

**Request**:
```json
{
  "alkemio_context_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Response (Success)**:
```json
{
  "success": true,
  "alkemio_context_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "My Space",
  "topic": "A collaborative space",
  "avatar_url": "mxc://matrix.org/abc123",
  "member_actor_ids": [
    "770e8400-e29b-41d4-a716-446655440000",
    "880e8400-e29b-41d4-a716-446655440000"
  ],
  "children": [
    {
      "alkemio_room_id": "990e8400-e29b-41d4-a716-446655440000",
      "order": "aaa"
    },
    {
      "alkemio_context_id": "aa0e8400-e29b-41d4-a716-446655440000",
      "order": "bbb"
    }
  ]
}
```

---

### 5. communication.space.list

Lists all Spaces with pagination.

**Request**:
```json
{
  "cursor": ""
}
```

**Response (Success)**:
```json
{
  "success": true,
  "alkemio_context_ids": [
    "550e8400-e29b-41d4-a716-446655440000",
    "660e8400-e29b-41d4-a716-446655440000"
  ],
  "next_cursor": ""
}
```

---

### 6. communication.hierarchy.set_parent

Sets or removes the parent of a Room or Space.

**Request (Set parent for a Room)**:
```json
{
  "child_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "parent_context_id": "660e8400-e29b-41d4-a716-446655440000"
}
```

**Request (Set parent for a Space)**:
```json
{
  "child_context_id": "550e8400-e29b-41d4-a716-446655440000",
  "parent_context_id": "660e8400-e29b-41d4-a716-446655440000"
}
```

**Request (Remove parent - orphan)**:
```json
{
  "child_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "parent_context_id": null
}
```

**Response (Success)**:
```json
{
  "success": true
}
```

**Response (Error - Both IDs provided)**:
```json
{
  "success": false,
  "error": {
    "code": "INVALID_PARAM",
    "message": "exactly one of child_room_id or child_context_id must be set"
  }
}
```

**Response (Error - Neither ID provided)**:
```json
{
  "success": false,
  "error": {
    "code": "INVALID_PARAM",
    "message": "exactly one of child_room_id or child_context_id must be set"
  }
}
```

**Response (Error - Parent is not a Space)**:
```json
{
  "success": false,
  "error": {
    "code": "INVALID_PARAM",
    "message": "parent_context_id must reference a Space (type=m.space)"
  }
}
```

**Response (Error - Circular hierarchy detected)**:
```json
{
  "success": false,
  "error": {
    "code": "MATRIX_ERROR",
    "message": "circular hierarchy detected: Space A cannot be parent of Space B which is already an ancestor of A"
  }
}
```
Note: Circular hierarchy detection is performed by the Matrix homeserver. The adapter returns `MATRIX_ERROR` with the homeserver's error message.

---

### 7. communication.space.member.batch.add

Adds an actor to multiple Spaces.

**Request**:
```json
{
  "actor_id": "550e8400-e29b-41d4-a716-446655440000",
  "alkemio_context_ids": [
    "660e8400-e29b-41d4-a716-446655440000",
    "770e8400-e29b-41d4-a716-446655440000"
  ]
}
```

**Response (Success with partial failure)**:
```json
{
  "success": true,
  "results": {
    "660e8400-e29b-41d4-a716-446655440000": {
      "success": true
    },
    "770e8400-e29b-41d4-a716-446655440000": {
      "success": false,
      "error": {
        "code": "ROOM_NOT_FOUND",
        "message": "Space not found"
      }
    }
  }
}
```

---

### 8. communication.space.member.batch.remove

Removes an actor from multiple Spaces.

**Request**:
```json
{
  "actor_id": "550e8400-e29b-41d4-a716-446655440000",
  "alkemio_context_ids": [
    "660e8400-e29b-41d4-a716-446655440000",
    "770e8400-e29b-41d4-a716-446655440000"
  ],
  "reason": "User offboarding"
}
```

**Response (Success)**:
```json
{
  "success": true,
  "results": {
    "660e8400-e29b-41d4-a716-446655440000": {
      "success": true
    },
    "770e8400-e29b-41d4-a716-446655440000": {
      "success": true
    }
  }
}
```

---

## Extended Room Commands

### communication.room.create (Updated)

**Request (with new fields)**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "community",
  "name": "My Room",
  "topic": "Discussion room",
  "initial_members": ["660e8400-e29b-41d4-a716-446655440000"],
  "avatar_url": "mxc://matrix.org/abc123",
  "parent_context_id": "770e8400-e29b-41d4-a716-446655440000",
  "join_rule": "restricted"
}
```

### communication.room.update (Updated)

**Request (with new fields)**:
```json
{
  "alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Updated Room",
  "topic": "Updated topic",
  "is_public": false,
  "avatar_url": "mxc://matrix.org/xyz789",
  "join_rule": "invite"
}
```

### communication.room.list (Breaking Change)

**Request (Limit field REMOVED)**:
```json
{
  "cursor": ""
}
```

---

## Error Codes Reference

| Code | When Used |
|------|-----------|
| `INVALID_PARAM` | Missing required field, invalid UUID, validation failure |
| `ROOM_NOT_FOUND` | Room or Space alias doesn't resolve |
| `ACTOR_NOT_FOUND` | Actor cannot be registered in Matrix |
| `MATRIX_ERROR` | Matrix SDK or homeserver error |
| `INTERNAL_ERROR` | Unexpected system error |
| `NOT_ALLOWED` | Permission denied by Matrix |

---

## TypeScript Event Types

Add to `lib/src/matrix.adapter.event.type.ts`:

```typescript
export enum MatrixAdapterEventType {
  // Existing...
  
  // NEW V3 Space Commands
  COMMUNICATION_SPACE_CREATE = 'communication.space.create',
  COMMUNICATION_SPACE_UPDATE = 'communication.space.update',
  COMMUNICATION_SPACE_DELETE = 'communication.space.delete',
  COMMUNICATION_SPACE_GET = 'communication.space.get',
  COMMUNICATION_SPACE_LIST = 'communication.space.list',
  COMMUNICATION_SPACE_MEMBER_BATCH_ADD = 'communication.space.member.batch.add',
  COMMUNICATION_SPACE_MEMBER_BATCH_REMOVE = 'communication.space.member.batch.remove',
  
  // NEW V3 Hierarchy Command
  COMMUNICATION_HIERARCHY_SET_PARENT = 'communication.hierarchy.set_parent',
}
```
