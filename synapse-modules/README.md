# Alkemio Room Control Module for Synapse

This is a Synapse spam checker module that restricts room creation to the Alkemio Matrix Adapter AppService bot.

## Overview

- **All users are ghost users** provisioned by the AppService
- **Community rooms**: Only created via Alkemio Server → Adapter → AppService bot
- **DM rooms**: Users can initiate from Element, module notifies Adapter, Server decides, bot creates room

## Installation

1. Copy `alkemio_room_control.py` to your Synapse modules directory (e.g., `/etc/synapse/modules/`)

2. Install the `aiohttp` dependency (for webhook calls):
   ```bash
   pip install aiohttp
   ```

3. Add to your `homeserver.yaml`:

```yaml
# Zero-config (recommended) - all values auto-detected from AppService registration
modules:
  - module: alkemio_room_control.AlkemioRoomControl
    config: {}
```

4. Restart Synapse

## Zero-Config Auto-Detection

The module **automatically loads all configuration** from the AppService with id `alkemio-matrix-adapter`:

| Config Value | Auto-Detected From |
|--------------|-------------------|
| `homeserver_domain` | Synapse's `server_name` |
| `appservice_sender` | AppService's `sender_localpart` |
| `adapter_url` | AppService's registered `url` |
| `hs_token` | AppService's `hs_token` |

**Important**: Your AppService registration must have `id: alkemio-matrix-adapter` in `registration.yaml`.

## Behavior

| User Type | Room Type | Result |
|-----------|-----------|--------|
| AppService Bot | Any | ✅ Allowed |
| Ghost User | Community Room | ❌ Blocked |
| Ghost User | DM Room | ❌ Blocked (but notifies Adapter for async creation) |

## DM Creation Flow

When a ghost user tries to create a DM in Element:

1. Module intercepts the `createRoom` request
2. Detects it's a DM (`is_direct=true`, single invitee)
3. Sends webhook to Adapter: `POST /_matrix/app/alkemio/dm-request`
4. Returns `M_FORBIDDEN` to client
5. Adapter notifies Alkemio Server via RabbitMQ
6. Server decides and commands Adapter to create room
7. AppService bot creates the DM room
8. User sees new room in Element

## Webhook Payload

```json
POST /_matrix/app/alkemio/dm-request
Authorization: Bearer <hs_token>
{
    "inviter": "@uuid1:alkemio.matrix.host",
    "invitee": "@uuid2:alkemio.matrix.host"
}
```

## Error Messages

When a user tries to create a room, they will receive:
- Error code: `M_FORBIDDEN`
- For DMs: The room will be created asynchronously by the bot if approved

## Testing

```bash
# As a ghost user trying to create a community room (should fail)
curl -X POST "https://synapse/_matrix/client/v3/createRoom" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Test Room"}'

# Expected: {"errcode": "M_FORBIDDEN", ...}

# As a ghost user trying to create a DM (should fail but trigger webhook)
curl -X POST "https://synapse/_matrix/client/v3/createRoom" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"is_direct": true, "invite": ["@other-uuid:domain"]}'

# Expected: {"errcode": "M_FORBIDDEN", ...} but adapter receives webhook
```

## Troubleshooting

Check Synapse logs for module activity:
```bash
grep -i "AlkemioRoomControl" /var/log/synapse/homeserver.log
grep -i "Auto-detected" /var/log/synapse/homeserver.log
grep -i "DM creation attempt" /var/log/synapse/homeserver.log
```

Expected log on startup:
```
Loaded config from AppService 'alkemio-matrix-adapter': sender=matrix-adapter, url=http://matrix-adapter:8080
AlkemioRoomControl initialized - AppService: @matrix-adapter:alkemio.io, Adapter: http://matrix-adapter:8080, Token: configured
```

If the AppService is not found, you'll see:
```
AppService 'alkemio-matrix-adapter' not found! Check registration.yaml is loaded.
```
