#!/usr/bin/env bash
# ============================================================================
# 6_fix_direct_room_account_data.sh
# ----------------------------------------------------------------------------
# Set m.direct account data for existing direct conversation rooms.
#
# The adapter never set m.direct account data when creating direct rooms,
# so Element/Matrix clients don't recognize them as DMs (not bucketed under
# "People"). This script retroactively fixes that.
#
# Direct rooms are identified by:
#   - io.alkemio.visibility = {visible: true} (conversation room)
#   - Exactly 2 joined non-bot members (DM, not group)
#
# For each room, both participants get the room added to their m.direct
# account data under the other participant's user ID.
#
# Pre-conditions:
#   - kubectl context = target cluster
#   - Synapse running and healthy
#   - postgres in-cluster pod accessible
#   - jq, curl available locally
#
# Usage:
#   ./6_fix_direct_room_account_data.sh acc                 # dry run
#   ./6_fix_direct_room_account_data.sh acc --limit 5       # try first 5
#   ./6_fix_direct_room_account_data.sh acc --apply         # full fleet
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

# ---- 3. List direct conversation rooms (visible=true, exactly 2 non-bot members)

echo ">>> Querying direct conversation rooms ..."
ROOMS_RAW=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT cse_v.room_id || '|' || m1.state_key || '|' || m2.state_key
FROM current_state_events cse_v
JOIN event_json ej_v ON ej_v.event_id = cse_v.event_id
-- Exactly 2 joined non-bot members
CROSS JOIN LATERAL (
  SELECT array_agg(cse_m.state_key ORDER BY cse_m.state_key) AS members
  FROM current_state_events cse_m
  JOIN event_json ej_m ON ej_m.event_id = cse_m.event_id
  WHERE cse_m.room_id = cse_v.room_id
    AND cse_m.type = 'm.room.member'
    AND cse_m.state_key != '${BOT_MXID}'
    AND ej_m.json::jsonb->'content'->>'membership' = 'join'
) mem
-- Join to extract individual members
JOIN current_state_events m1_cse ON m1_cse.room_id = cse_v.room_id
  AND m1_cse.type = 'm.room.member'
  AND m1_cse.state_key = mem.members[1]
JOIN current_state_events m2_cse ON m2_cse.room_id = cse_v.room_id
  AND m2_cse.type = 'm.room.member'
  AND m2_cse.state_key = mem.members[2]
-- Rename for output
CROSS JOIN LATERAL (VALUES (m1_cse.state_key)) AS m1(state_key)
CROSS JOIN LATERAL (VALUES (m2_cse.state_key)) AS m2(state_key)
WHERE cse_v.type = 'io.alkemio.visibility'
  AND ej_v.json::jsonb->'content'->'visible' = 'true'::jsonb
  AND array_length(mem.members, 1) = 2
  -- Exclude spaces
  AND NOT EXISTS (
    SELECT 1 FROM current_state_events cse_c
    JOIN event_json ej_c ON ej_c.event_id = cse_c.event_id
    WHERE cse_c.room_id = cse_v.room_id
      AND cse_c.type = 'm.room.create'
      AND ej_c.json::jsonb->'content'->>'type' = 'm.space'
  )
ORDER BY cse_v.room_id;
")
ROOM_COUNT=$(printf '%s\n' "$ROOMS_RAW" | grep -c . || true)
echo "    $ROOM_COUNT direct conversation rooms found"

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
  echo "DRY-RUN. Sample of first 10 rooms (room | user1 | user2):"
  head -10 <<< "$ROOMS_RAW" | column -ts '|'
  echo
  echo "Re-run with --limit N or --apply to execute."
  exit 0
fi

# ---- 4. For each room: set m.direct account data for both participants ------

echo ">>> Setting m.direct account data ..."
OK=0
FAIL=0
SKIP=0
declare -a FAILED_ROOMS=()

# set_m_direct <user_mxid> <other_user_mxid> <room_id>
# GET current m.direct, append room if missing, PUT back.
set_m_direct() {
  local USER="$1" OTHER="$2" ROOM_ID="$3"
  local USER_URI OTHER_KEY
  USER_URI=$(printf '%s' "$USER" | jq -sRr @uri)

  # GET current m.direct — distinguish 404 (no data yet) from transient errors
  local CURRENT HTTP_CODE
  HTTP_CODE=$(curl -sS -o /tmp/mdirect_resp -w '%{http_code}' \
    "${SYNAPSE_URL}/_matrix/client/v3/user/${USER_URI}/account_data/m.direct?user_id=${USER_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || echo "000")
  CURRENT=$(cat /tmp/mdirect_resp 2>/dev/null || echo '')

  case "$HTTP_CODE" in
    200)
      if ! echo "$CURRENT" | jq -e '.' >/dev/null 2>&1; then
        CURRENT='{}'
      fi
      ;;
    404)
      CURRENT='{}'
      ;;
    *)
      return 2
      ;;
  esac

  # Check if room already listed under the other user
  local ALREADY
  ALREADY=$(printf '%s' "$CURRENT" | jq -r --arg other "$OTHER" --arg room "$ROOM_ID" \
    '(.[$other] // []) | map(select(. == $room)) | length')
  if [ "$ALREADY" != "0" ]; then
    return 1  # already set
  fi

  # Append room
  local UPDATED
  UPDATED=$(printf '%s' "$CURRENT" | jq -c --arg other "$OTHER" --arg room "$ROOM_ID" \
    '.[$other] = ((.[$other] // []) + [$room])')

  # PUT back
  local PUT_RESP
  PUT_RESP=$(curl -fsS -X PUT \
    "${SYNAPSE_URL}/_matrix/client/v3/user/${USER_URI}/account_data/m.direct?user_id=${USER_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$UPDATED" 2>/dev/null || true)

  # Synapse returns {} on success
  if [ "$PUT_RESP" = "{}" ] || echo "$PUT_RESP" | jq -e '. == {}' >/dev/null 2>&1; then
    return 0
  else
    echo "$PUT_RESP"
    return 2
  fi
}

while IFS='|' read -r ROOM_ID USER1 USER2; do
  [ -z "$ROOM_ID" ] && continue

  ROOM_UPDATED=false
  ROOM_FAILED=false

  # Set for user1 → user2
  rc=0; set_m_direct "$USER1" "$USER2" "$ROOM_ID" || rc=$?
  case $rc in
    0) ROOM_UPDATED=true ;;
    1) ;; # already set
    *) ROOM_FAILED=true ;;
  esac

  # Set for user2 → user1
  rc=0; set_m_direct "$USER2" "$USER1" "$ROOM_ID" || rc=$?
  case $rc in
    0) ROOM_UPDATED=true ;;
    1) ;; # already set
    *) ROOM_FAILED=true ;;
  esac

  if [ "$ROOM_FAILED" = "true" ]; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|$USER1|$USER2")
  elif [ "$ROOM_UPDATED" = "true" ]; then
    OK=$((OK+1))
    if [ $((OK % 50)) -eq 0 ]; then
      echo "    progress: ok=$OK fail=$FAIL skip=$SKIP"
    fi
  else
    SKIP=$((SKIP+1))
  fi

  sleep 0.2
done <<< "$ROOMS_RAW"

echo
echo "=== summary ==="
echo "OK (updated):       $OK"
echo "SKIP (already set): $SKIP"
echo "FAIL:               $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo
  echo "First 20 failures:"
  for r in "${FAILED_ROOMS[@]:0:20}"; do echo "  $r"; done
fi
