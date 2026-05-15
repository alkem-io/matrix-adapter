#!/usr/bin/env bash
# ============================================================================
# leave_conversations.sh
# ----------------------------------------------------------------------------
# Remove the appservice bot from conversation rooms so Element renders
# DM names correctly (shows the other participant, not "Valentin and Alkemio").
#
# Conversation rooms are identified by io.alkemio.visibility = {visible: true},
# which Part 2 (server sync mutation) sets on CONVERSATION, CONVERSATION_DIRECT,
# and CONVERSATION_GROUP rooms. All other rooms have visible=false and are left
# untouched (callout, updates, post, calendar_event, spaces).
#
# WARNING: after the bot leaves a room, it CANNOT rejoin via admin.JoinRoom
# (Synapse requires the sender to be joined for the internal invite). The only
# way back in is a ghost user with PL >= 50 inviting the bot. Make sure
# fix_ghost_power_levels has been applied first.
#
# Approach per room:
#   1. Query rooms where:
#      - io.alkemio.visibility = {visible: true}
#      - Bot is currently joined
#   2. POST /_matrix/client/v3/rooms/{roomId}/leave?user_id=<bot>
#      using the appservice token to impersonate the bot
#
# Pre-conditions:
#   - kubectl context = target cluster
#   - Synapse + matrix-adapter running and healthy
#   - postgres in-cluster pod accessible
#   - jq, curl available locally
#   - fix_ghost_power_levels already applied (ghosts need PL >= 50)
#   - redact_canonical_aliases already applied (otherwise Element shows UUID)
#   - Part 2 server sync already run (io.alkemio.visibility set)
#
# Usage:
#   ./leave_conversations.sh acc                 # dry run, lists rooms
#   ./leave_conversations.sh acc --limit 5       # try on first 5 only
#   ./leave_conversations.sh acc --apply         # full fleet
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
    BOT_MXID="@00000000-0000-0000-0000-000000000000:matrix-acc.alkem.io"
    EXPECTED_CONTEXT="k8s-scaleway-acceptance"
    ;;
  prod)
    BOT_MXID="@00000000-0000-0000-0000-000000000000:matrix.alkem.io"
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

# ---- 1. Pull appservice token from the Synapse pod's registration file -----

REGISTRATION_PATH="/data/matrix-adapter.yaml"
echo ">>> Fetching appservice as_token from $REGISTRATION_PATH on synapse pod..."
AS_TOKEN="$(kubectl exec -n "$NAMESPACE" deploy/synapse-deployment -- \
  grep -E '^as_token:' "$REGISTRATION_PATH" | awk '{print $2}' | tr -d ' \r\n')"
if [ -z "$AS_TOKEN" ]; then
  echo "ERROR: could not retrieve as_token from $REGISTRATION_PATH"
  exit 1
fi
echo "    got token (${AS_TOKEN:0:8}...)"

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

# ---- 2a. Pre-flight: confirm appservice auth works --------------------------

echo ">>> Pre-flight: verifying admin auth via impersonation (bot=$BOT_MXID)..."
PRE=$(curl -fsS "${SYNAPSE_URL}/_synapse/admin/v2/users/${BOT_URI}?user_id=${BOT_URI}" \
  -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || true)
if [ -z "$PRE" ] || ! echo "$PRE" | jq -e '.admin == true' >/dev/null 2>&1; then
  echo "ERROR: pre-flight admin auth failed. response: $PRE"
  exit 1
fi
echo "    admin auth OK"

# ---- 3. List conversation rooms where bot is joined -------------------------
#
# Conversation rooms: io.alkemio.visibility = {visible: true}
# Bot must be currently joined.
# Exclude spaces (safety net).

echo ">>> Querying conversation rooms where bot is joined ..."
ROOMS_RAW=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT cse_v.room_id || '|' || count_members.cnt
FROM current_state_events cse_v
JOIN event_json ej_v ON ej_v.event_id = cse_v.event_id
-- Bot is joined
JOIN current_state_events cse_bot ON cse_bot.room_id = cse_v.room_id
  AND cse_bot.type = 'm.room.member'
  AND cse_bot.state_key = '${BOT_MXID}'
JOIN event_json ej_bot ON ej_bot.event_id = cse_bot.event_id
  AND ej_bot.json::jsonb->'content'->>'membership' = 'join'
-- Count other joined members (excluding bot)
CROSS JOIN LATERAL (
  SELECT count(*) AS cnt
  FROM current_state_events cse_m
  JOIN event_json ej_m ON ej_m.event_id = cse_m.event_id
  WHERE cse_m.room_id = cse_v.room_id
    AND cse_m.type = 'm.room.member'
    AND cse_m.state_key != '${BOT_MXID}'
    AND ej_m.json::jsonb->'content'->>'membership' = 'join'
) count_members
WHERE cse_v.type = 'io.alkemio.visibility'
  AND ej_v.json::jsonb->'content'->'visible' = 'true'::jsonb
  -- Exclude spaces (safety)
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
echo "    $ROOM_COUNT conversation rooms with bot joined"

if [ "$ROOM_COUNT" = "0" ]; then
  echo "Nothing to do."
  exit 0
fi

# Show member count distribution
ROOMS_WITH_OTHERS=$(printf '%s\n' "$ROOMS_RAW" | awk -F'|' '$2 > 0' | wc -l | tr -d ' ')
ROOMS_BOT_ONLY=$(printf '%s\n' "$ROOMS_RAW" | awk -F'|' '$2 == 0' | wc -l | tr -d ' ')
echo "    $ROOMS_WITH_OTHERS rooms have other joined members (will leave)"
echo "    $ROOMS_BOT_ONLY rooms have bot as only member (will SKIP — no one to operate room after)"

if [ -n "$LIMIT_N" ]; then
  ROOMS_RAW=$(head -n "$LIMIT_N" <<< "$ROOMS_RAW")
  ROOM_COUNT="$LIMIT_N"
  echo "    --limit $LIMIT_N → will only process first $LIMIT_N"
fi

if [ "$MODE" != "--apply" ] && [ "$MODE" != "--limit" ]; then
  echo
  echo "DRY-RUN. Sample of first 10 rooms (room | other_member_count):"
  head -10 <<< "$ROOMS_RAW" | column -ts '|'
  echo
  echo "Re-run with --limit N or --apply to execute."
  exit 0
fi

# ---- 4. For each room: POST /leave as the bot -------------------------------

echo ">>> Leaving conversation rooms ..."
OK=0
FAIL=0
SKIP=0
declare -a FAILED_ROOMS=()

while IFS='|' read -r ROOM_ID MEMBER_COUNT; do
  [ -z "$ROOM_ID" ] && continue

  # Skip rooms where bot is the only member — leaving would orphan the room
  if [ "$MEMBER_COUNT" = "0" ]; then
    SKIP=$((SKIP+1))
    continue
  fi

  ROOM_URI=$(printf '%s' "$ROOM_ID" | jq -sRr @uri)

  LEAVE_RESP=$(curl -fsS -X POST \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/leave?user_id=${BOT_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{}' 2>/dev/null || true)

  # Synapse returns {} on successful leave
  if [ "$LEAVE_RESP" = "{}" ] || echo "$LEAVE_RESP" | jq -e 'has("event_id") or . == {}' >/dev/null 2>&1; then
    OK=$((OK+1))
    if [ $((OK % 50)) -eq 0 ]; then
      echo "    progress: ok=$OK fail=$FAIL skip=$SKIP"
    fi
  else
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|members=$MEMBER_COUNT|$(printf '%s' "$LEAVE_RESP" | head -c 160)")
  fi

  sleep 0.2
done <<< "$ROOMS_RAW"

echo
echo "=== summary ==="
echo "OK (left):      $OK"
echo "SKIP (bot-only): $SKIP"
echo "FAIL:           $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo
  echo "First 20 failures:"
  for r in "${FAILED_ROOMS[@]:0:20}"; do echo "  $r"; done
fi

# ---- 5. Verify ---------------------------------------------------------------

echo
echo ">>> Verifying via DB ..."
REMAINING=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT count(*)
FROM current_state_events cse_v
JOIN event_json ej_v ON ej_v.event_id = cse_v.event_id
JOIN current_state_events cse_bot ON cse_bot.room_id = cse_v.room_id
  AND cse_bot.type = 'm.room.member'
  AND cse_bot.state_key = '${BOT_MXID}'
JOIN event_json ej_bot ON ej_bot.event_id = cse_bot.event_id
  AND ej_bot.json::jsonb->'content'->>'membership' = 'join'
WHERE cse_v.type = 'io.alkemio.visibility'
  AND ej_v.json::jsonb->'content'->'visible' = 'true'::jsonb
  AND NOT EXISTS (
    SELECT 1 FROM current_state_events cse_c
    JOIN event_json ej_c ON ej_c.event_id = cse_c.event_id
    WHERE cse_c.room_id = cse_v.room_id
      AND cse_c.type = 'm.room.create'
      AND ej_c.json::jsonb->'content'->>'type' = 'm.space'
  );
")
echo "Conversation rooms with bot still joined: $REMAINING (should be 0 modulo bot-only skips)"
