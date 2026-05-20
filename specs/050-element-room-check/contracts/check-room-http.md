# Contract: Check Room HTTP Endpoint

**Direction**: Synapse Module → Adapter
**Transport**: HTTP POST
**Path**: `/_matrix/app/alkemio/check-room`
**Auth**: Bearer token (hs_token, constant-time comparison)

## Request

```json
{
  "creator": "@{uuid}:{domain}",
  "members": ["@{uuid}:{domain}", ...],
  "is_direct": true
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `creator` | string | Valid Matrix user ID, must be a ghost user |
| `members` | []string | 1+ valid Matrix user IDs; for DM exactly 1 |
| `is_direct` | bool | true for DM, false for group |

## Response (200 OK — Allowed)

```json
{
  "allow": true,
  "alkemio_room_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

## Response (200 OK — Rejected)

```json
{
  "allow": false,
  "reason": "messaging disabled for target user"
}
```

## Error Responses

| Status | Condition | Body |
|--------|-----------|------|
| 401 | Missing or invalid Bearer token | `{"error": "unauthorized"}` |
| 400 | Invalid JSON or missing fields | `{"error": "invalid_payload"}` |
| 500 | Internal error (RabbitMQ failure) | `{"error": "internal_error"}` |
| 504 | Server reply timeout (>3s) | `{"error": "timeout"}` |

## Synapse Module Behavior

| Adapter Response | Module Action |
|------------------|---------------|
| 200 + `allow: true` | Inject state into `request_content`, return (room created) |
| 200 + `allow: false` | `raise SynapseError(403, reason, Codes.FORBIDDEN)` |
| 401/400/500 | `raise SynapseError(503, "Service unavailable", Codes.UNKNOWN)` |
| 504 or timeout | `raise SynapseError(503, "Service temporarily unavailable", Codes.UNKNOWN)` |
| Connection error | `raise SynapseError(503, "Service temporarily unavailable", Codes.UNKNOWN)` |
