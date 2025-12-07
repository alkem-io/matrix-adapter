# Quickstart: Room Creation Control

**Feature**: 006-room-creation-control | **Date**: 2025-12-06

## Overview

Guide for testing the room creation control feature locally.

---

## Prerequisites

- Go 1.25+
- Docker + Docker Compose
- Running Synapse instance with AppService registration
- RabbitMQ instance
- Alkemio Server (for full flow testing)

---

## 1. Configure Synapse Module

### 1.1 Deploy the Spam Checker

Copy the module to your Synapse installation:

```bash
# From repository root
cp synapse-modules/alkemio_room_control.py /path/to/synapse/modules/
```

### 1.2 Update homeserver.yaml

```yaml
modules:
  - module: alkemio_room_control.AlkemioRoomControl
    config:
      adapter_webhook_url: "http://matrix-adapter:8080"
      # hs_token should match MATRIX_HS_TOKEN in adapter config
      # Use environment variable interpolation if your Synapse supports it,
      # or load from a separate secrets file
      hs_token: !ENV "MATRIX_HS_TOKEN"
      # Matrix user IDs allowed to create rooms (AppService bot)
      allowed_bot_user_ids:
        - "@matrix-adapter:your.matrix.domain"
```

> **Security Note**: Never commit `hs_token` in plaintext. Use Synapse's `!ENV` directive or mount from secrets.

### 1.3 Restart Synapse

```bash
docker-compose restart synapse
# or
systemctl restart synapse
```

---

## 2. Build and Run Adapter

### 2.1 Build

```bash
make build
```

### 2.2 Configure Environment

Create `.env` or set environment variables:

```bash
# Matrix configuration
MATRIX_HOMESERVER_URL=http://localhost:8008
MATRIX_AS_TOKEN=your_as_token
MATRIX_HS_TOKEN=your_hs_token
MATRIX_SERVER_NAME=localhost

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@localhost:5672/

```

### 2.3 Run

```bash
make run
# or
./bin/adapter
```

---

## 3. Test Webhook Endpoint

### 3.1 Verify Health

```bash
curl http://localhost:8080/health
# Expected: {"status":"ok"}
```

### 3.2 Test DM Webhook (Direct)

```bash
curl -X POST http://localhost:8080/_matrix/app/alkemio/dm-request \
  -H "Authorization: Bearer your_hs_token" \
  -H "Content-Type: application/json" \
  -d '{
    "initiator_user_id": "@user1_alkemio-host:localhost",
    "target_user_id": "@user2_alkemio-host:localhost",
    "timestamp": "2024-01-15T10:30:00Z"
  }'
```

**Expected Response** (200 OK):
```json
{
  "status": "accepted",
  "correlation_id": "generated-uuid"
}
```

### 3.3 Verify RabbitMQ Event

Check RabbitMQ management UI or use `rabbitmqadmin`:

```bash
rabbitmqadmin get queue=alkemio.events count=1
```

Should show message on `communication.room.dm.requested` topic.

---

## 4. Test Full DM Flow

### 4.1 Prerequisites
- Alkemio Server running and connected to RabbitMQ
- Alkemio Server implements `communication.room.dm.requested` handler
- Alkemio Server publishes `communication.room.dm.create` commands

### 4.2 Via Element Client

1. Log into Element with a ghost user
2. Start a new DM conversation
3. Observe:
   - Synapse blocks creation (FORBIDDEN)
   - Webhook fires to adapter
   - Event published to RabbitMQ
   - (After Server processes) DM room created

### 4.3 Simulate Server Command

To test without full Alkemio Server, publish directly:

```bash
# Publish CreateDMRoomCommand to RabbitMQ
rabbitmqadmin publish \
  exchange=alkemio \
  routing_key=communication.room.dm.create \
  payload='{
    "initiator_actor_id": "user-uuid-1",
    "target_actor_id": "user-uuid-2",
    "correlation_id": "test-123"
  }'
```

Check adapter logs for room creation.

---

## 5. Run Tests

### 5.1 Unit Tests

```bash
make test
```

### 5.2 Specific Package Tests

```bash
# Webhook handler tests
go test ./internal/infrastructure/http/... -v

# DM handler tests  
go test ./internal/infrastructure/queue/... -v -run TestDM

# Service tests
go test ./internal/core/service/... -v -run TestDM
```

### 5.3 Integration Tests

```bash
# Requires running Synapse and RabbitMQ
make test-integration
```

---

## 6. Verify TypeScript Library

After changes to `pkg/dto/`:

```bash
# Generate TypeScript definitions
make generate

# Check generated types
cat lib/src/dto/generated.ts | grep -A5 "DMRequested"
```

Should include:
```typescript
export interface DMRequestedEvent {
  initiator_actor_id: string;
  target_actor_id: string;
  timestamp: string;
  correlation_id?: string;
}
```

---

## 7. Docker Compose Testing

### 7.1 Full Stack

```yaml
# docker-compose.override.yml
services:
  matrix-adapter:
    build: .
    environment:
      - MATRIX_HOMESERVER_URL=http://synapse:8008
      - RABBITMQ_URL=amqp://rabbitmq:5672/
    ports:
      - "8080:8080"
    depends_on:
      - synapse
      - rabbitmq
```

```bash
docker-compose up -d
```

### 7.2 Test from Container Network

```bash
docker-compose exec synapse curl \
  -X POST http://matrix-adapter:8080/_matrix/app/alkemio/dm-request \
  -H "Authorization: Bearer test_hs_token" \
  -H "Content-Type: application/json" \
  -d '{"initiator_user_id":"@test:localhost","target_user_id":"@test2:localhost","timestamp":"2024-01-15T10:00:00Z"}'
```

---

## 8. Troubleshooting

### Webhook Returns 401

- Verify `hs_token` matches in both Synapse module config and adapter config
- Check adapter logs for authentication errors

### Event Not Published

- Verify RabbitMQ connection in adapter logs
- Check exchange and routing key configuration
- Confirm `communication.room.dm.requested` topic is registered

### Room Not Created

- Verify Alkemio Server is consuming `communication.room.dm.requested`
- Check Alkemio Server logs for authorization decision
- Verify `communication.room.dm.create` is being published
- Check adapter logs for `HandleCreateDMRoom` execution

### Synapse Module Not Loading

- Check Synapse logs for module initialization errors
- Verify Python dependencies (aiohttp) are installed
- Confirm module file path is correct in `homeserver.yaml`

---

## 9. Logs to Monitor

```bash
# Adapter logs
docker-compose logs -f matrix-adapter

# Synapse logs (for module)
docker-compose logs -f synapse | grep -i "alkemio\|dm\|room"

# RabbitMQ
docker-compose exec rabbitmq rabbitmqctl list_queues
```

**Key Log Messages**:
- `"dm webhook received"` - Webhook endpoint hit
- `"dm request event published"` - Event sent to RabbitMQ
- `"create dm room command received"` - Handler processing command
- `"dm room created"` - Success
