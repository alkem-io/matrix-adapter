# Feature Specification: Space & Room Parameters, Bot Architecture, Custom State

**Feature Branch**: `013-space-room-params`
**Created**: 2026-03-25
**Updated**: 2026-03-30
**Status**: Implemented

## Overview

This feature encompasses a series of interconnected changes to the Matrix adapter:

1. **joinRule wiring** for rooms (matching spaces)
2. **isPublic** (directory visibility) for rooms and spaces
3. **Bot architecture overhaul** — bot leaves rooms, operates as server admin
4. **Custom state API** — generic `io.alkemio.*` state events
5. **Sync visibility filtering** — Synapse module hides rooms from Element
6. **SynapseAdmin client** — encapsulated admin API operations
7. **Bot display name** and **admin bootstrap** on startup

## Changes Summary

### 1. joinRule End-to-End for Rooms

Wire the existing `joinRule` DTO field through handler → service → port → adapter for both `createRoom` and `updateRoom`. For DM rooms, `joinRule` is ignored (always private).

### 2. isPublic (Directory Visibility)

Add `is_public` parameter to all create/update room/space operations. Controls whether a room appears in Matrix's public room directory ("Explore rooms" in Element). Uses `PUT /_matrix/client/v3/directory/list/room/{roomId}`. Requires `room_list_publication_rules` in Synapse config to allow the bot.

### 3. Bot Architecture

- **Bot leaves rooms after creation** — stays only in spaces
- **Bot is Synapse server admin** — required for reading room state/aliases/messages without membership
- **Admin bootstrap** — auto-promotes bot via shared secret registration at startup
- **Ghost user intents** — writes (state events, kicks) use the ghost user with highest power level
- **No invite events** — ghost users join directly via `EnsureJoined` (no invite notification)

### 4. Custom State API (`io.alkemio.*`)

Generic API for setting/getting custom state events on rooms and spaces:
- `custom_state` map on create/update requests
- `custom_state` returned in get responses
- Standalone endpoints: `communication.room.state.set/get`, `communication.space.state.set/get`
- Only `io.alkemio.*` prefixed event types are allowed

### 5. Sync Visibility Filtering

Synapse module (`alkemio_room_control.py`) monkey-patches `SyncHandler.get_sync_result_builder` to:
- Hide rooms with `io.alkemio.visibility: {visible: false}` from `/sync`
- Exempt the bot user (sees all rooms)
- Custom state is set as `InitialState` at room creation (before members join)

### 6. SynapseAdmin Client

Encapsulated Synapse Admin API operations in `internal/infrastructure/matrix/synapse_admin.go`:
- User operations: GetUser, SetUserAdmin, DeactivateUser
- Room operations: ListRooms, GetRoomMembers, GetRoomState, GetRoomMessages, GetEvent, GetRelations
- Registration: GetRegistrationNonce, RegisterWithMAC
- All room reads migrated from BotIntent to admin API

### 7. Additional Changes

- **Bot display name**: `MATRIX_BOT_DISPLAY_NAME` env var (default: "Alkemio")
- **COMMUNICATION_SPACE_UPDATED** event for space property changes (separate from room updates)
- **Pointer semantics** for update operations: `nil` = no change, `""` = clear value
- **No canonical alias** on rooms — aliases are for internal lookups only
- **Startup cleanup**: redact canonical aliases, bot leaves non-space rooms
- **Shared AliasResolver** for DRY handler code
- **Removed pgx dependency** and broken `cmd/fix-canonical-aliases`

## New Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MATRIX_BOT_DISPLAY_NAME` | Bot display name in Matrix | `Alkemio` |
| `SYNAPSE_SERVER_SHARED_SECRET` | Synapse registration_shared_secret for admin bootstrap | - |

## New API Topics

| Topic | Direction | Description |
|-------|-----------|-------------|
| `communication.room.state.set` | Command | Set io.alkemio.* state on a room |
| `communication.room.state.get` | Command | Get io.alkemio.* state from a room |
| `communication.space.state.set` | Command | Set io.alkemio.* state on a space |
| `communication.space.state.get` | Command | Get io.alkemio.* state from a space |
| `communication.space.updated` | Event | Space property change (name, avatar, topic) |

## Synapse Configuration Requirements

```yaml
# Allow bot to publish rooms to directory
room_list_publication_rules:
  - user_id: "@00000000-0000-0000-0000-000000000000:your.domain"
    action: allow
  - action: deny

# Required for admin bootstrap (optional if bot is manually set as admin)
registration_shared_secret: "your-secret"
```

## Architecture Rules Added

- All Synapse Admin API calls MUST go through `SynapseAdmin` (`synapse_admin.go`)
- Bare HTTP requests to `/_synapse/admin/` are forbidden outside this package
- Bot MUST NOT be a member of non-space rooms (leaves after setup)
- All room state reads use admin API (no BotIntent.StateEvent)
- Room state writes use ghost user with highest power level
