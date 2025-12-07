# Quickstart: RMQ Events Extension

**Feature**: 007-rmq-events-extension  
**Status**: ✅ Implemented

## Prerequisites

- Go 1.25+
- RabbitMQ running (via Docker Compose)
- Matrix homeserver accessible

## Setup

```bash
# Clone and checkout feature branch
git checkout 007-rmq-events-extension

# Install dependencies
go mod download

# Build
make build

# Run tests
make test
```

## Key Files Modified

### DTOs (pkg/dto/)

- **`pkg/dto/event.go`**: Added `ReactionAddedEvent`, `ReactionRemovedEvent`, `RoomMemberLeftEvent`
- **`pkg/dto/message.go`**: Added `GetThreadMessagesRequest`, `GetThreadMessagesResponse`
- **`pkg/dto/room.go`**: Added `GetRoomMembersRequest`, `GetRoomMembersResponse`
- **`pkg/dto/commands.go`**: Added 5 topic constants and registry entries

### Matrix Adapter (internal/infrastructure/matrix/)

- **`mautrix.go`**: Added `GetThreadMessages()` method using `Client.GetRelations()` with `RelThread`
- **`listener.go`**: Added handlers for `EventReaction`, `EventRedaction`, `StateMember`

### Queue Handlers (internal/infrastructure/queue/)

- **`handler_room.go`**: Added `HandleGetRoomMembers`, `HandleGetThreadMessages`
- **`router.go`**: Registered new command handlers
- **`topics.go`**: Added topic aliases

## Testing Approach

### Unit Tests

```go
// Test reaction event parsing
func TestParseReactionEvent(t *testing.T) {
    // Mock Matrix event
    // Verify ReactionAddedEvent fields
}

// Test thread messages retrieval
func TestGetThreadMessages(t *testing.T) {
    // Mock MatrixClient interface
    // Verify response structure
}
```

### Manual Testing

1. Start local Matrix homeserver (Synapse)
2. Create test room with bot
3. Send message → Add reaction → Verify `reaction.added` event
4. Remove reaction → Verify `reaction.removed` event
5. Leave room → Verify `room.member.left` event
6. Call `room.members.get` → Verify member list
7. Create thread → Call `thread.messages.get` → Verify response

## TypeScript Library Update

After implementing DTOs, regenerate the TypeScript library:

```bash
make generate
cd lib && pnpm build
```

## Implementation Checklist

- [x] DTOs created in `pkg/dto/`
- [x] Topic constants added
- [x] Matrix adapter methods implemented
- [x] Event listener handlers added
- [x] Queue command handlers implemented
- [x] Unit tests passing
- [x] TypeScript lib generated
- [x] Manual testing complete
