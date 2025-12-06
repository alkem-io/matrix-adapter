# HTTP Contract: DM Webhook Endpoint

**Feature**: 006-room-creation-control | **Date**: 2025-12-06

## Overview

Defines the HTTP endpoint contract for receiving DM request webhooks from the Synapse spam checker module.

---

## 1. Endpoint

**Path**: `/_matrix/app/alkemio/dm-request`  
**Method**: `POST`  
**Content-Type**: `application/json`

**Base URL Configuration**:
- Default: `http://matrix-adapter:8080` (internal container network)
- Configurable via `ADAPTER_WEBHOOK_URL` environment variable in Synapse module

---

## 2. Authentication

**Type**: Bearer Token  
**Header**: `Authorization: Bearer {hs_token}`

The `hs_token` is the **homeserver token** from the AppService registration file (`registration.yaml`). This is a shared secret known to both:
- **Synapse**: Uses it to authenticate requests TO the AppService
- **Adapter**: Uses it to validate incoming webhooks FROM Synapse module

> **Note**: This is the same `hs_token` configured via `MATRIX_HS_TOKEN` environment variable in the adapter.

**Security Notes**:
- Token MUST NOT be logged (Constitution §8)
- Token validation failure returns 401 immediately (no body leak)
- Rate limiting recommended at infrastructure level

---

## 3. Request

### Headers

| Header | Required | Value |
|--------|----------|-------|
| `Authorization` | Yes | `Bearer {hs_token}` |
| `Content-Type` | Yes | `application/json` |
| `X-Request-ID` | No | UUID for tracing (passed as correlation_id) |

### Body Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "DMWebhookRequest",
  "type": "object",
  "required": ["initiator_user_id", "target_user_id", "timestamp"],
  "properties": {
    "initiator_user_id": {
      "type": "string",
      "description": "Full Matrix user ID of the DM initiator (format: @{uuid}:{domain})",
      "pattern": "^@[a-fA-F0-9-]+:[a-zA-Z0-9.-]+$",
      "example": "@550e8400-e29b-41d4-a716-446655440000:matrix.alkemio.org"
    },
    "target_user_id": {
      "type": "string",
      "description": "Full Matrix user ID of the intended DM recipient (format: @{uuid}:{domain})",
      "pattern": "^@[a-fA-F0-9-]+:[a-zA-Z0-9.-]+$",
      "example": "@6ba7b810-9dad-11d1-80b4-00c04fd430c8:matrix.alkemio.org"
    },
    "timestamp": {
      "type": "string",
      "format": "date-time",
      "description": "ISO 8601 timestamp of the request"
    }
  }
}
```

### Example Request

```http
POST /_matrix/app/alkemio/dm-request HTTP/1.1
Host: matrix-adapter:8080
Authorization: Bearer syt_your_hs_token_here
Content-Type: application/json
X-Request-ID: 550e8400-e29b-41d4-a716-446655440000

{
  "initiator_user_id": "@550e8400-e29b-41d4-a716-446655440000:matrix.alkemio.org",
  "target_user_id": "@6ba7b810-9dad-11d1-80b4-00c04fd430c8:matrix.alkemio.org",
  "timestamp": "2024-01-15T10:30:00.000Z"
}
```

---

## 4. Responses

### 200 OK

Request accepted for asynchronous processing. The adapter will publish a `DMRequestedEvent` to RabbitMQ.

```json
{
  "status": "accepted",
  "correlation_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Note**: 200 indicates the webhook was received and will be processed. It does NOT indicate DM creation will succeed.

---

### 400 Bad Request

Malformed JSON or missing required fields.

```json
{
  "error": {
    "code": "INVALID_PAYLOAD",
    "message": "missing required field: initiator_user_id"
  }
}
```

---

### 401 Unauthorized

Invalid or missing authorization token.

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "invalid or missing authorization token"
  }
}
```

**Security**: Response body is intentionally minimal to avoid token enumeration.

---

### 500 Internal Server Error

Adapter failed to process the request (e.g., RabbitMQ unavailable).

```json
{
  "error": {
    "code": "INTERNAL_ERROR",
    "message": "failed to publish event"
  }
}
```

---

## 5. Error Codes

| Code | HTTP Status | Description | Retry? |
|------|-------------|-------------|--------|
| `INVALID_PAYLOAD` | 400 | Malformed request | No |
| `UNAUTHORIZED` | 401 | Invalid/missing token | No |
| `INTERNAL_ERROR` | 500 | Server-side failure | Yes |

---

## 6. Synapse Module Integration

The Synapse spam checker module (`alkemio_room_control.py`) calls this endpoint:

```python
async def _notify_dm_request(
    self,
    initiator_user_id: str,
    target_user_id: str,
) -> bool:
    payload = {
        "initiator_user_id": initiator_user_id,
        "target_user_id": target_user_id,
        "timestamp": datetime.utcnow().isoformat() + "Z",
    }
    
    async with aiohttp.ClientSession() as session:
        async with session.post(
            f"{self.adapter_url}/_matrix/app/alkemio/dm-request",
            json=payload,
            headers={
                "Authorization": f"Bearer {self.hs_token}",
                "Content-Type": "application/json",
            },
            timeout=aiohttp.ClientTimeout(total=5),
        ) as response:
            return response.status == 200
```

---

## 7. Rate Limiting

**Recommended Configuration** (at load balancer/ingress):
- Per-IP: 60 requests/minute
- Global: 1000 requests/minute
- Burst: 10 requests/second

**Synapse Module Deduplication**:
- 30-second cache prevents duplicate requests for same (initiator, target) pair
- Reduces load on adapter during rapid retry scenarios

---

## 8. Monitoring

**Metrics to Track**:
| Metric | Type | Labels |
|--------|------|--------|
| `dm_webhook_requests_total` | Counter | `status` (200/400/401/500) |
| `dm_webhook_duration_seconds` | Histogram | - |
| `dm_webhook_auth_failures_total` | Counter | - |

**Logging**:
- INFO: Successful webhook received (correlation_id, initiator, target)
- WARN: Authentication failure (source IP)
- ERROR: Internal processing failure (correlation_id, error)

---

## 9. Testing

### cURL Examples

**Valid Request**:
```bash
curl -X POST http://localhost:8080/_matrix/app/alkemio/dm-request \
  -H "Authorization: Bearer your_hs_token" \
  -H "Content-Type: application/json" \
  -d '{
    "initiator_user_id": "@550e8400-e29b-41d4-a716-446655440000:localhost",
    "target_user_id": "@6ba7b810-9dad-11d1-80b4-00c04fd430c8:localhost",
    "timestamp": "2024-01-15T10:30:00Z"
  }'
```

**Invalid Token**:
```bash
curl -X POST http://localhost:8080/_matrix/app/alkemio/dm-request \
  -H "Authorization: Bearer invalid_token" \
  -H "Content-Type: application/json" \
  -d '{}' 
# Expected: 401 Unauthorized
```

**Missing Fields**:
```bash
curl -X POST http://localhost:8080/_matrix/app/alkemio/dm-request \
  -H "Authorization: Bearer your_hs_token" \
  -H "Content-Type: application/json" \
  -d '{"initiator_user_id": "@user1:localhost"}'
# Expected: 400 Bad Request
```
