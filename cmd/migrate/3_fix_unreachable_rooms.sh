#!/usr/bin/env bash
# ============================================================================
# fix_unreachable_rooms.sh
# ----------------------------------------------------------------------------
# Fix rooms where users_default=0 and NO joined user has PL >= 50.
#
# These rooms are unreachable by the normal fix_ghost_power_levels_safe.sh
# because nobody inside the room can send state events. The bot (PL 100)
# has already left.
#
# Approach per room:
#   1. Pick any joined ghost user
#   2. POST /_synapse/admin/v1/rooms/{roomId}/make_room_admin
#      to grant that ghost PL 100 (server-side, bypasses in-room auth)
#   3. PUT m.room.power_levels with users_default=50 as the promoted ghost
#   4. If a non-empty canonical alias exists, PUT empty alias as the ghost
#
# Pre-conditions:
#   - kubectl context = target cluster
#   - Synapse running and healthy
#   - postgres in-cluster pod accessible
#   - jq, curl available locally
#   - The bot user has users.admin=1 in Synapse
#
# Usage:
#   ./fix_unreachable_rooms.sh acc                 # dry run, lists rooms
#   ./fix_unreachable_rooms.sh acc --limit 5       # try on first 5 only
#   ./fix_unreachable_rooms.sh acc --apply         # full fleet
# ============================================================================

set -euo pipefail

ENV="${1:?Usage: $0 <acc|prod> [--limit N | --apply]}"
MODE="${2:-}"
LIMIT_N=""

case "$MODE" in
  --apply)   ;;
  --limit)   LIMIT_N="${3:?--limit requires an integer count}" ;;
  "")        ;;
  *)         echo "ERROR: unknown mode '$MODE'"; exit 1 ;;
esac

case "$ENV" in
  acc)
    EXPECTED_CONTEXT="k8s-scaleway-acceptance"
    ;;
  prod)
    EXPECTED_CONTEXT="k8s-scaleway-production"
    ;;
  *)
    echo "ERROR: env must be 'acc' or 'prod'"; exit 1 ;;
esac

NAMESPACE=default
SYNAPSE_LOCAL_PORT=18008
SYNAPSE_URL="http://127.0.0.1:${SYNAPSE_LOCAL_PORT}"

# ---- safety checks -----------------------------------------------------------

CURRENT_CONTEXT="$(kubectl config current-context)"
if [ "$CURRENT_CONTEXT" != "$EXPECTED_CONTEXT" ]; then
  echo "ERROR: kubectl context is '$CURRENT_CONTEXT', expected '$EXPECTED_CONTEXT'"
  exit 1
fi

POSTGRES_POD="$(kubectl get pod -n "$NAMESPACE" -l app=postgres -o name | head -1 | sed 's|pod/||')"
if [ -z "$POSTGRES_POD" ]; then
  echo "ERROR: no postgres pod found in namespace $NAMESPACE"
  exit 1
fi

# ---- 1. Pull appservice token + bot MXID ------------------------------------

REGISTRATION_PATH="/data/matrix-adapter.yaml"
echo ">>> Fetching appservice as_token from $REGISTRATION_PATH on synapse pod..."
AS_TOKEN="$(kubectl exec -n "$NAMESPACE" deploy/synapse-deployment -- \
  grep -E '^as_token:' "$REGISTRATION_PATH" | awk '{print $2}' | tr -d ' \r\n"')"
if [ -z "$AS_TOKEN" ]; then
  echo "ERROR: could not retrieve as_token from $REGISTRATION_PATH"
  exit 1
fi
echo "    got token (${AS_TOKEN:0:8}...)"

BOT_SENDER=$(kubectl exec -n "$NAMESPACE" deploy/synapse-deployment -- \
  grep -E '^sender_localpart:' "$REGISTRATION_PATH" | awk '{print $2}' | tr -d ' \r\n"')
SERVER_NAME=$(kubectl exec -n "$NAMESPACE" deploy/synapse-deployment -- \
  grep -E '^server_name:' /data/homeserver.yaml | awk '{print $2}' | tr -d ' \r\n"')
BOT_MXID="@${BOT_SENDER}:${SERVER_NAME}"

# ---- 2. Open a local port-forward to Synapse --------------------------------

echo ">>> Opening port-forward to synapse:8008 → localhost:${SYNAPSE_LOCAL_PORT}..."
kubectl port-forward -n "$NAMESPACE" svc/synapse "${SYNAPSE_LOCAL_PORT}:8008" >/tmp/pf-synapse.log 2>&1 &
PF_PID=$!
trap 'kill "$PF_PID" 2>/dev/null || true' EXIT
sleep 2
if ! curl -sf "${SYNAPSE_URL}/_matrix/client/versions" >/dev/null; then
  echo "ERROR: synapse not reachable via port-forward; see /tmp/pf-synapse.log"
  exit 1
fi

BOT_URI="$(printf '%s' "$BOT_MXID" | jq -sRr @uri)"

# ---- 2a. Pre-flight ---------------------------------------------------------

echo ">>> Pre-flight: verifying admin auth via impersonation (bot=$BOT_MXID)..."
PRE=$(curl -fsS "${SYNAPSE_URL}/_synapse/admin/v2/users/${BOT_URI}?user_id=${BOT_URI}" \
  -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || true)
if [ -z "$PRE" ] || ! echo "$PRE" | jq -e '.admin == true' >/dev/null 2>&1; then
  echo "ERROR: pre-flight admin auth failed. response: $PRE"
  exit 1
fi
echo "    admin auth OK"

# ---- 3. List stuck rooms: users_default=0, no joined user with PL >= 50 -----

echo ">>> Querying unreachable rooms (users_default=0, no joined user with PL >= 50) ..."
ROOMS_RAW=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT cse.room_id || '|' || COALESCE(
  -- Pick any joined user
  (SELECT cse_m.state_key
   FROM current_state_events cse_m
   JOIN event_json ej_m ON ej_m.event_id = cse_m.event_id
   WHERE cse_m.room_id = cse.room_id
     AND cse_m.type = 'm.room.member'
     AND ej_m.json::jsonb->'content'->>'membership' = 'join'
   LIMIT 1),
  ''
) || '|' || COALESCE(
  -- Check if canonical alias is set
  (SELECT ej_ca.json::jsonb->'content'->>'alias'
   FROM current_state_events cse_ca
   JOIN event_json ej_ca ON ej_ca.event_id = cse_ca.event_id
   WHERE cse_ca.room_id = cse.room_id
     AND cse_ca.type = 'm.room.canonical_alias'
     AND COALESCE(ej_ca.json::jsonb->'content'->>'alias', '') != ''),
  ''
)
FROM current_state_events cse
JOIN event_json ej ON ej.event_id = cse.event_id
WHERE cse.type = 'm.room.power_levels'
  AND (ej.json::jsonb->'content'->>'users_default')::int = 0
  -- No joined user with effective PL >= 50
  AND NOT EXISTS (
    SELECT 1
    FROM current_state_events cse_m
    JOIN event_json ej_m ON ej_m.event_id = cse_m.event_id
    WHERE cse_m.room_id = cse.room_id
      AND cse_m.type = 'm.room.member'
      AND ej_m.json::jsonb->'content'->>'membership' = 'join'
      AND COALESCE(
        (ej.json::jsonb->'content'->'users'->>cse_m.state_key)::int,
        0
      ) >= 50
  )
ORDER BY cse.room_id;
")
ROOM_COUNT=$(printf '%s\n' "$ROOMS_RAW" | grep -c . || true)
echo "    $ROOM_COUNT unreachable rooms found"

if [ "$ROOM_COUNT" = "0" ]; then
  echo "Nothing to do."
  exit 0
fi

if [ -n "$LIMIT_N" ]; then
  ROOMS_RAW=$(head -n "$LIMIT_N" <<< "$ROOMS_RAW")
  ROOM_COUNT="$LIMIT_N"
  echo "    --limit $LIMIT_N → will only process first $LIMIT_N"
fi

if [ "$MODE" != "--apply" ] && [ "$MODE" != "--limit" ]; then
  echo
  echo "DRY-RUN. Rooms (room | ghost_to_promote | canonical_alias):"
  printf '%s\n' "$ROOMS_RAW" | column -ts '|'
  echo
  echo "Re-run with --limit N or --apply to execute."
  exit 0
fi

# ---- 4. For each room: make_room_admin → fix PL → clear alias ---------------

echo ">>> Fixing unreachable rooms ..."
OK=0
FAIL=0
SKIP=0
ALIAS_OK=0
declare -a FAILED_ROOMS=()

while IFS='|' read -r ROOM_ID GHOST ALIAS; do
  [ -z "$ROOM_ID" ] && continue

  if [ -z "$GHOST" ]; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|no_members|room has zero joined users")
    continue
  fi

  ROOM_URI=$(printf '%s' "$ROOM_ID" | jq -sRr @uri)
  GHOST_URI=$(printf '%s' "$GHOST" | jq -sRr @uri)

  # 4a. Admin-join the bot into the room (make_room_admin requires a local admin in the room)
  JOIN_RESP=$(curl -fsS -X POST \
    "${SYNAPSE_URL}/_synapse/admin/v1/join/${ROOM_URI}?user_id=${BOT_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"user_id\": \"$BOT_MXID\"}" 2>/dev/null || true)

  if ! echo "$JOIN_RESP" | jq -e '.room_id' >/dev/null 2>&1; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|admin_join|$(printf '%s' "$JOIN_RESP" | head -c 160)")
    continue
  fi

  sleep 0.3

  # 4b. make_room_admin — promote the ghost to PL 100
  ADMIN_RESP=$(curl -fsS -X POST \
    "${SYNAPSE_URL}/_synapse/admin/v1/rooms/${ROOM_URI}/make_room_admin?user_id=${BOT_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"user_id\": \"$GHOST\"}" 2>/dev/null || true)

  if echo "$ADMIN_RESP" | jq -e '.errcode' >/dev/null 2>&1; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|make_admin|ghost=$GHOST|$(printf '%s' "$ADMIN_RESP" | head -c 160)")
    continue
  fi

  sleep 0.3

  # 4c. GET current PL as the promoted ghost
  CURRENT_PL=$(curl -fsS \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/state/m.room.power_levels?user_id=${GHOST_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || true)

  if [ -z "$CURRENT_PL" ] || ! echo "$CURRENT_PL" | jq -e . >/dev/null 2>&1; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|get_pl|ghost=$GHOST|$(printf '%s' "$CURRENT_PL" | head -c 160)")
    continue
  fi

  # 4d. PUT PL with users_default=50
  NEW_PL=$(printf '%s' "$CURRENT_PL" | jq -c '.users_default = 50')
  PUT_RESP=$(curl -fsS -X PUT \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/state/m.room.power_levels?user_id=${GHOST_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$NEW_PL" 2>/dev/null || true)

  if ! echo "$PUT_RESP" | jq -e '.event_id' >/dev/null 2>&1; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|put_pl|ghost=$GHOST|$(printf '%s' "$PUT_RESP" | head -c 160)")
    continue
  fi

  OK=$((OK+1))

  # 4e. Clear canonical alias if present
  if [ -n "$ALIAS" ]; then
    sleep 0.2
    ALIAS_RESP=$(curl -fsS -X PUT \
      "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/state/m.room.canonical_alias?user_id=${GHOST_URI}" \
      -H "Authorization: Bearer $AS_TOKEN" \
      -H 'Content-Type: application/json' \
      -d '{}' 2>/dev/null || true)

    if echo "$ALIAS_RESP" | jq -e '.event_id' >/dev/null 2>&1; then
      ALIAS_OK=$((ALIAS_OK+1))
    else
      echo "    WARN: PL fixed but alias clear failed for $ROOM_ID: $(printf '%s' "$ALIAS_RESP" | head -c 100)"
    fi
  fi

  # 4f. Bot leaves the room (was only joined to enable make_room_admin)
  sleep 0.2
  curl -fsS -X POST \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/leave?user_id=${BOT_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{}' >/dev/null 2>&1 || true

  sleep 0.2
done <<< "$ROOMS_RAW"

echo
echo "=== summary ==="
echo "OK (PL fixed):     $OK"
echo "Aliases cleared:   $ALIAS_OK"
echo "FAIL:              $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo
  echo "Failures:"
  for r in "${FAILED_ROOMS[@]}"; do echo "  $r"; done
fi

# ---- 5. Verify ---------------------------------------------------------------

echo
echo ">>> Verifying via DB ..."
REMAINING=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT count(*)
FROM current_state_events cse
JOIN event_json ej ON ej.event_id = cse.event_id
WHERE cse.type = 'm.room.power_levels'
  AND (ej.json::jsonb->'content'->>'users_default')::int = 0
  AND NOT EXISTS (
    SELECT 1
    FROM current_state_events cse_m
    JOIN event_json ej_m ON ej_m.event_id = cse_m.event_id
    WHERE cse_m.room_id = cse.room_id
      AND cse_m.type = 'm.room.member'
      AND ej_m.json::jsonb->'content'->>'membership' = 'join'
      AND COALESCE(
        (ej.json::jsonb->'content'->'users'->>cse_m.state_key)::int,
        0
      ) >= 50
  );
")
echo "Remaining unreachable rooms with users_default=0: $REMAINING (should be 0)"
