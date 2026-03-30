# Data Model: 013-space-room-params

**Updated**: 2026-03-30

## Domain Model Changes

### Room — added CustomState

```go
type Room struct {
    ID          id.RoomID
    AlkemioID   uuid.UUID
    Alias       string
    Name        string
    Topic       string
    AvatarURL   string
    CustomState map[string]map[string]interface{} // io.alkemio.* state events
    Type        string
    MemberIDs   []uuid.UUID
    Messages    []Message
}
```

### Space — added CustomState

```go
type Space struct {
    ID               id.RoomID
    AlkemioContextID uuid.UUID
    Name             string
    Topic            string
    AvatarURL        string
    Alias            string
    JoinRule         string
    CustomState      map[string]map[string]interface{} // io.alkemio.* state events
    MemberIDs        []uuid.UUID
    Children         []SpaceChild
    ParentContextID  *uuid.UUID
}
```

### SpaceUpdatedEvent — new

```go
type SpaceUpdatedEvent struct {
    AlkemioContextID uuid.UUID
    DisplayName      *string
    AvatarURL        *string
    Topic            *string
    Timestamp        time.Time
}
```

## Interface Changes

### MatrixPort

```
CreateRoomWithAlias(ctx, alkemioRoomID, roomType, name, topic, avatarURL, joinRule string,
                    customState map[string]map[string]interface{}, initialMembers []domain.Actor) (id.RoomID, error)

UpdateRoomState(ctx, roomID, actorID, name, topic, avatarURL, joinRule *string) error

UpdateSpaceState(ctx, roomID, name, topic, avatarURL, joinRule *string) error

SetRoomDirectoryVisibility(ctx, roomID, isPublic bool) error
SetCustomState(ctx, roomID, state map[string]map[string]interface{}) error
GetCustomState(ctx, roomID, eventTypes []string) (map[string]map[string]interface{}, error)
SetRoomSyncVisibility — REMOVED (replaced by generic SetCustomState)
```

### SynapseAdmin (new)

```
GetUser(ctx, userID) (*UserInfo, error)
SetUserAdmin(ctx, userID, admin bool) error
DeactivateUser(ctx, userID, erase bool) error
GetRegistrationNonce(ctx) (string, error)
RegisterWithMAC(ctx, nonce, username, password, mac string, admin bool) (string, error)
ListRooms(ctx, limit int) ([]AdminRoom, error)
GetRoomMembers(ctx, roomID) ([]string, error)
GetRoomMemberIDs(ctx, roomID) ([]id.UserID, error)
GetRoomState(ctx, roomID, eventType string) ([]json.RawMessage, error)
GetStateEventContent(ctx, roomID, eventType string) (map[string]interface{}, error)
GetCustomState(ctx, roomID, eventTypes []string) (map[string]map[string]interface{}, error)
GetRoomMessages(ctx, roomID, from, dir string, limit int) (*mautrix.RespMessages, error)
GetEvent(ctx, roomID, eventID) (*event.Event, error)
GetEventContext(ctx, roomID, eventID) (*mautrix.RespContext, error)
GetRelations(ctx, roomID, eventID, relType, eventType) ([]*event.Event, error)
GetTimestampToEvent(ctx, roomID, ts int64, dir string) (id.EventID, error)
JoinRoom(ctx, roomID, userID) error
```

## Startup Flow

```
1. waitForSynapse()          — retry until Synapse responds
2. ensureBotAdmin()          — check/bootstrap admin status
   ├─ already admin? → done
   ├─ has shared secret? → register bot as admin / temp admin bootstrap
   └─ no secret → warn with SQL instructions
3. Start appservice
4. redactCanonicalAliases()  — migration cleanup
5. leaveBotFromNonSpaceRooms() — bot exits non-space rooms
6. Set bot display name
7. Connect to RabbitMQ
```

## Bot Membership Rules

| Room Type | Bot Membership | State Reads | State Writes |
|-----------|---------------|-------------|-------------|
| Space | Stays as member | Admin API | BotIntent |
| Room (with members) | Leaves after creation | Admin API | Ghost user (highest PL) |
| Room (no members) | Stays until first member joins | Admin API | BotIntent |
| DM Room | Leaves after creation | Admin API | Ghost user |
