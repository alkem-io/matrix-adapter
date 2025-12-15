# Quickstart: Read Receipts & Message Events

**Feature**: 008-read-receipts

## Prerequisites

- Running Matrix Homeserver (Synapse)
- RabbitMQ instance
- `mautrix-go` v0.26.0+

## Configuration

No new configuration variables required. The feature uses existing Matrix and RabbitMQ credentials.

## Running the Service

```bash
# Build
make build

# Run
./bin/adapter
```

## Testing Read Receipts

### 1. Mark a Message as Read

Publish a command to `matrix.room.message.read`:

```bash
# Example payload
{
  "actor_id": "550e8400-e29b-41d4-a716-446655440000",
  "alkemio_room_id": "123e4567-e89b-12d3-a456-426614174000",
  "message_id": "$event_id:example.com"
}
```

### 2. Get Unread Counts

Publish a command to `matrix.room.unread_counts.get`:

```bash
# Example payload
{
  "actor_id": "550e8400-e29b-41d4-a716-446655440000",
  "alkemio_room_id": "123e4567-e89b-12d3-a456-426614174000"
}
```

### 3. Verify Events

Monitor the `matrix.room.receipt.updated` topic. When you mark a message as read (or use a Matrix client to do so), an event should appear:

```json
{
  "room_id": "!room:example.com",
  "user_id": "@user:example.com",
  "event_id": "$event_id:example.com",
  "timestamp": 1678900000000
}
```

## Troubleshooting

- **"Message not found" error**: Ensure the `message_id` exists in the room and the user has joined the room.
- **No receipt event**: Check if the user is a ghost user managed by the adapter. Events from external users are also captured but ensure the listener is active.
- **Unread count is 0**: Ensure there are messages *after* the marked event in the timeline.
