# Alkemio Synapse Modules

This directory is the **canonical** home of Alkemio's Synapse Python modules:

- [`alkemio_room_control.py`](./alkemio_room_control.py) — spam-checker module that restricts room creation to the Alkemio Matrix Adapter AppService bot ([details below](#alkemio-room-control-module)).
- [`alkemio_fileservice_provider.py`](./alkemio_fileservice_provider.py) — Synapse media `StorageProvider` that bridges Synapse's media byte I/O to the Alkemio file-service ([details below](#alkemio-file-service-media-storage-provider)).

> **Canonical source.** The `.py` files in this directory (plus [`../registration.yaml`](../registration.yaml)) are the single source of truth. Downstream copies are kept in sync automatically by [`.github/workflows/sync-synapse-module.yml`](../.github/workflows/sync-synapse-module.yml) (running [`.scripts/sync-synapse-module.sh`](../.scripts/sync-synapse-module.sh)) whenever any of them changes on `develop`:
>
> - `alkem-io/server` — file-mode copies under `.build/synapse/modules/` (`alkemio_room_control.py`, `alkemio_fileservice_provider.py`) + `.build/synapse/matrix-adapter.yaml`
> - `alkem-io/dev-orchestration` — embedded in `01-synapse-setup-confmap.yml`
> - `alkem-io/infrastructure-operations` — embedded in `01-synapse-setup-confmap.yml`
>
> The two `.py` modules are synced **verbatim** (no per-environment fields). `registration.yaml` is schema-synced — `url`, `as_token`, and `hs_token` stay environment-specific in each downstream. **Both** modules are required in every target: in the ops repos a missing `alkemio_room_control.py: |` or `alkemio_fileservice_provider.py: |` ConfigMap key **fails the sync loudly** (exit 4) rather than skipping quietly, so a target cannot silently stop receiving canonical changes while the job stays green. A target that deliberately does not run the media provider must opt out explicitly with `ALLOW_MISSING_FILESERVICE_PROVIDER=1`, which downgrades the failure to a logged warning.
>
> Do not edit those downstream copies directly. Edit the canonical file here (or `registration.yaml`), merge to `develop`, and review the rolling PR opened in each downstream repo by the Alkemio Infrastructure Bot.

## Alkemio Room Control Module

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
| Ghost User | DM Room | ✅ Allowed (but notifies Adapter for async creation) |

## DM Creation Flow

When a ghost user tries to create a DM in Element:

1. Module intercepts the `createRoom` request
2. Detects it's a DM (`is_direct=true`, single invitee)
3. Sends webhook to Adapter: `POST /_matrix/app/alkemio/dm-request`
4. Don't stop from room creation.
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
```text
Loaded config from AppService 'alkemio-matrix-adapter': sender=matrix-adapter, url=http://matrix-adapter:8280
AlkemioRoomControl initialized - AppService: @matrix-adapter:alkemio.io, Adapter: http://matrix-adapter:8280, Token: configured
```

If the AppService is not found, you'll see:
```text
AppService 'alkemio-matrix-adapter' not found! Check registration.yaml is loaded.
```

## Alkemio File-Service Media Storage Provider

`alkemio_fileservice_provider.py` stores local original media in file-service.
Keep Synapse's existing media volume. This feature introduces no volume cutover
or media purger.

- `store_file` streams standard treq multipart into the reserved `matrix_media`
  bucket with `externalReference=media_id` and `skipImageProcessing=true`.
  It awaits HTTP 201. The provider row stays in this bucket; conversation copies
  reference the same content-addressed bytes.
- `fetch` looks up the reference in that bucket explicitly. A 404 returns `None`;
  other failures propagate. Synapse's `SimpleHttpClient.get_file` downloads into
  a temporary disk file with `max_upload_size`, then the stock `FileResponder`
  serves and closes it. Failures close it too. This is disk spooling, not direct
  network-to-network streaming.
- Thumbnails, URL-cache entries and remote media use Synapse's existing storage.

### Deployment

Place the module on `PYTHONPATH=/data/modules` and configure:

```yaml
media_storage_providers:
  - module: alkemio_fileservice_provider.FileServiceStorageProvider
    store_local: true
    store_remote: false
    store_synchronous: true
    config:
      file_service_url: "http://file-service:4003"
      matrix_media_bucket_id: "<reserved matrix_media bucket uuid>"
      store_timeout_s: 30  # optional multipart request timeout
```

Reads use Synapse's normal HTTP deadlines. The old custom transport timeout,
producer and thread-pool settings are retired. Dependencies are treq, Twisted
and Synapse's media APIs, already available in the pinned Synapse image.

### Verification

`make test-python` exercises routing, scoped lookup, byte-preserving upload,
error propagation and file cleanup with mocked HTTP and import-only Synapse
shims. It does not establish live wire compatibility.

A separate local check on Synapse v1.132.0 on 2026-09-22 used real treq multipart,
SimpleHttpClient and FileResponder against file-service. Empty, 1 MiB binary and
PNG originals retained identical SHA256 hashes through store and fetch, and the
responder files closed. Real Element and Alkemio browser verification remains a
separate release requirement.
