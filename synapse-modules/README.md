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

[`alkemio_fileservice_provider.py`](./alkemio_fileservice_provider.py) is a Synapse media
[`StorageProvider`](https://element-hq.github.io/synapse/latest/media_repository.html)
(`FileServiceStorageProvider`) that makes the Alkemio file-service the sole durable
store for Matrix media. Synapse's own local media store is kept as an ephemeral
**cache** (an `emptyDir`); durability lives in the file-service.

It is a thin, **stateless** byte bridge between Synapse's media byte I/O and the
file-service — it holds no durable state of its own (the `media_id ↔ document`
mapping lives on the file-service document's opaque `externalReference`):

- **`store_file`** → `POST /internal/file` (multipart) into the reserved
  `matrix_media` bucket, verbatim (`skipImageProcessing=true`), with
  `externalReference = media_id`. Only local user uploads are routed;
  thumbnails / url-cache / remote media stay local-cache-only.
- **`fetch`** → `GET /internal/file/by-reference?ref=<media_id>` (global lookup,
  no bucket id — the server may have re-homed the document into a conversation
  bucket), then streams `GET /internal/file/{id}/content` back through a
  `Responder`. A cache miss in file-service returns `None`.

Reads are streamed with real backpressure (the response-body transport is
registered as a producer with the media consumer) and bounded by **per-request
timeouts only**: `timeout_s` covers connect + response headers, the metadata body
read, and the keep-alive drain, while the streamed content body is bounded by a
**time-to-first-byte** deadline — after the first byte, legitimate slow-client
backpressure governs and no further deadline is imposed. Any miss or failure
(404, non-200, transport error, timeout, malformed body) returns `None`, which
Synapse treats as a cache miss, so the Element media read path degrades rather
than hanging.

There is deliberately **no circuit breaker** and no cross-request state: this
follows the standard Synapse storage-provider model (per-request timeouts +
return-`None`-on-miss, like the mainline `s3_storage_provider`), and a stateful
breaker cannot meaningfully model a `fetch` whose duration is a minutes-long body
stream.

### Deployment

The module is deployed into `/data/modules` (alongside `alkemio_room_control.py`)
and discovered via `PYTHONPATH=/data/modules`. Enable it under
`media_storage_providers` in `homeserver.yaml`:

```yaml
media_storage_providers:
  - module: alkemio_fileservice_provider.FileServiceStorageProvider
    store_local: true
    store_remote: false
    store_synchronous: true
    config:
      file_service_url: "http://file-service:4003"
      matrix_media_bucket_id: "<reserved matrix_media bucket uuid>"
      # optional tuning (seconds, must be finite and > 0):
      #   timeout_s        default 10 — connect + headers, metadata body read,
      #                    drain, and the content stream's time-to-first-byte
      #   store_timeout_s  default 30 — FLOOR for the multipart upload; scaled up
      #                    by file size at an assumed 1 MB/s
      # No other keys are read; unknown keys are ignored.
```

It depends on `treq` / `twisted` (already present in Synapse) and the Synapse
`media` APIs (`StorageProvider`, `Responder`).

### Tests

The provider ships with a companion unit-test suite,
[`test_alkemio_fileservice_provider.py`](./test_alkemio_fileservice_provider.py),
which imports this canonical module directly. It runs in CI (the
`python-modules` job in [`ci-test.yml`](../.github/workflows/ci-test.yml)) and
locally:

```bash
pip install -r synapse-modules/requirements-dev.txt
make test-python
```

It is hermetic — no live file-service, no reactor, no network, and **no Synapse
install**: [`conftest.py`](./conftest.py) registers minimal `sys.modules`
stand-ins for the four Synapse symbols the provider imports at load time, but
only when Synapse is genuinely absent (inside the `matrixdotorg/synapse` image
the real package is used). `twisted` and `treq` are the real libraries, so the
protocols, producers and multipart serialisation are exercised against actual
library behaviour.
