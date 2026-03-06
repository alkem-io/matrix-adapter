# Data Model: Room State Events

**Feature**: 010-room-state-events
**Date**: 2026-03-06

## Entity Changes

### Room (enhanced)

**Location**: `internal/core/domain/model.go`

| Field     | Type        | Change  | Notes                          |
|-----------|-------------|---------|--------------------------------|
| ID        | id.RoomID   | Exists  |                                |
| AlkemioID | uuid.UUID   | Exists  |                                |
| Alias     | string      | Exists  |                                |
| Name      | string      | Exists  |                                |
| Topic     | string      | Exists  |                                |
| AvatarURL | string      | **NEW** | Matrix content URI (mxc://)    |
| Type      | string      | Exists  |                                |
| MemberIDs | []uuid.UUID | Exists  |                                |
| Messages  | []Message   | Exists  |                                |

### RoomUpdatedEvent (new)

**Location**: `internal/core/domain/model.go` (or `read_receipt.go` following existing pattern)

| Field         | Type      | Required | Notes                                     |
|---------------|-----------|----------|-------------------------------------------|
| AlkemioRoomID | uuid.UUID | Yes      | Alkemio room UUID                         |
| DisplayName   | *string   | No       | New room name (nil if unchanged)          |
| AvatarURL     | *string   | No       | New avatar URL (nil if unchanged)         |
| Topic         | *string   | No       | New topic (nil if unchanged)              |
| Timestamp     | time.Time | Yes      | When the state event occurred             |

Uses pointer types for optional fields to distinguish "not changed" (nil) from "changed to empty" (pointer to empty string).

## DTO Changes

### GetRoomResponse (enhanced)

**Location**: `pkg/dto/room.go`

| Field          | Type             | Change  | JSON Tag                       |
|----------------|------------------|---------|--------------------------------|
| BaseResponse   | BaseResponse     | Exists  | embedded                       |
| AlkemioRoomID  | AlkemioRoomID    | Exists  | `alkemio_room_id`              |
| DisplayName    | string           | Exists  | `display_name`                 |
| AvatarURL      | string           | **NEW** | `avatar_url,omitempty`         |
| MemberActorIDs | []AlkemioActorID | Exists  | `member_actor_ids`             |
| Messages       | []MessageDto     | Exists  | `messages`                     |

### GetRoomAsUserResponse (enhanced)

**Location**: `pkg/dto/room.go`

| Field           | Type                      | Change  | JSON Tag                       |
|-----------------|---------------------------|---------|--------------------------------|
| AvatarURL       | string                    | **NEW** | `avatar_url,omitempty`         |

### RoomUpdatedEvent (new DTO)

**Location**: `pkg/dto/event.go`

| Field         | Type          | Required | JSON Tag                       |
|---------------|---------------|----------|--------------------------------|
| AlkemioRoomID | AlkemioRoomID | Yes      | `alkemio_room_id`              |
| DisplayName   | *string       | No       | `display_name,omitempty`       |
| AvatarURL     | *string       | No       | `avatar_url,omitempty`         |
| Topic         | *string       | No       | `topic,omitempty`              |
| Timestamp     | int64         | Yes      | `timestamp`                    |

## Topic Constants

### New Constant

| Name                 | Value                          | Direction         |
|----------------------|--------------------------------|-------------------|
| TopicRoomUpdated     | `communication.room.updated`   | Adapter → Server  |

**Location**: `pkg/dto/commands.go` — add to outgoing event topics section.

## Registry

### OutgoingEventRegistry Addition

```
{Topic: TopicRoomUpdated, RequestType: "", ResponseType: "RoomUpdatedEvent"}
```
