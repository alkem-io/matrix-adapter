# Alkemio Room Control Module for Synapse

This is a Synapse spam checker module that restricts room creation to the Alkemio Matrix Adapter AppService bot.

> **Canonical source.** This file (and [`../registration.yaml`](../registration.yaml)) is the single source of truth. Three downstream copies are kept in sync automatically by [`.github/workflows/sync-synapse-module.yml`](../.github/workflows/sync-synapse-module.yml) whenever either file changes on `develop`:
>
> - `alkem-io/server` — `.build/synapse/modules/alkemio_room_control.py` + `.build/synapse/matrix-adapter.yaml`
> - `alkem-io/dev-orchestration` — both embedded in `01-synapse-setup-confmap.yml`
> - `alkem-io/infrastructure-operations` — both embedded in `01-synapse-setup-confmap.yml`
>
> Do not edit those copies directly. Edit this file (or `registration.yaml`), merge to `develop`, and review the rolling PR opened in each downstream repo by the Alkemio Infrastructure Bot. For `registration.yaml`, only schema fields are synced — `url`, `as_token`, and `hs_token` stay environment-specific in each downstream.

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

The module enforces the governed-operations-authority contract
(`agents-hq/specs/069-matrix-governance-hardening/contracts/governed-operations-authority.md` §3).
A room is **governed** iff `io.alkemio.entity` is present in its current state;
the **bot** is the `sender_localpart` of AppService `alkemio-matrix-adapter`.

| Callback | Rule |
|----------|------|
| `on_create_room` | admins and the bot bypass; `m.space` and rooms-in-spaces denied to users; standalone rooms mediated by `POST /_matrix/app/alkemio/check-room` (server room check) and stamped `io.alkemio.pending` |
| `user_may_invite` | governed room ⇒ **deny** unless the invitee is the bot (repair rejoin); ungoverned ⇒ allow |
| `check_event_allowed` | bot ⇒ allow. Else: any `io.alkemio.*` state event ⇒ **deny** (every room). Governed room: any state event ⇒ **deny** except `m.room.member` with `state_key == sender` and membership `join`/`leave`; timeline events ⇒ allow. Ungoverned ⇒ allow |
| `user_may_join_room` | not registered — Synapse join rules decide (restricted/invite) |

Consequences on governed rooms: kicks, bans, invites-as-state, power-level,
join-rule, name/topic/avatar, space-link and marker changes by any ordinary
session are refused with `M_FORBIDDEN`; self-join (via restricted allow or
invite) and self-leave work; sending, reacting and redacting one's own
messages work (whose events may be redacted is the power ladder's job).

## AppService alias namespaces

The registration claims two exclusive room-alias namespaces (both required
for the adapter to receive events and create aliases):

- `#<uuid>` — canonical room lookup alias (rooms and spaces)
- `#t_<uuid>` — the cutover-stable thread-room alias (target-alignment A-3)

## Room Creation Flow (mediated)

When a ghost user creates a standalone room (DM/group) from a Matrix client:

1. `on_create_room` intercepts the request (admins and the bot bypass)
2. The module synchronously calls the adapter: `POST /_matrix/app/alkemio/check-room`
3. The adapter asks the Alkemio server for consent/dedup over RabbitMQ
4. On rejection the creation fails with `M_FORBIDDEN`
5. On approval the module strips invites, injects the power-level override and
   the `io.alkemio.visibility` + `io.alkemio.pending` markers
6. The adapter reconciles the room after creation (bot admin, ladder, markers,
   aliases), removing the creator's administrative power

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
