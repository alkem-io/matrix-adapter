#!/usr/bin/env bash
# ============================================================================
# fix_ghost_power_levels_safe.sh — DRAFT, NOT YET TESTED
# ----------------------------------------------------------------------------
# Safe replacement for fix_ghost_power_levels.sql.
#
# The SQL version mutates event_json.json in place, which invalidates
# Synapse's content-hash and produces DatabaseCorruptionError on /sync,
# /state, /messages. This script instead issues NEW m.room.power_levels
# state events through Synapse's normal event-creation path, so each new
# PL event is properly hashed and inserted into the auth chain.
#
# Approach per room:
#   For each affected room, we impersonate the user who originally sent
#   the current m.room.power_levels event. That user, by definition, had
#   PL >= state_default at the time, and in practice almost always still
#   does (and is still joined). All such senders are inside the
#   appservice's user namespace (UUID-style ghosts or the bot itself),
#   so the appservice token can impersonate them via ?user_id=<them>.
#
#   1. SELECT room_id, e.sender FROM events JOIN current_state_events ...
#      WHERE type='m.room.power_levels' AND users_default=0
#   2. GET  /_matrix/client/v3/rooms/{roomId}/state/m.room.power_levels
#      as <sender> — fetches the current PL JSON
#   3. PUT  /_matrix/client/v3/rooms/{roomId}/state/m.room.power_levels
#      as <sender> with users_default merged to 50 — Synapse creates a
#      fresh event with the correct content hash, auth-checked against
#      the sender's existing PL
#
#   This avoids two failure modes of the earlier make_room_admin design:
#     a) The bot is not a member of many rooms (e.g. 1:1 conversation
#        rooms), so make_room_admin only invites it but never joins —
#        subsequent GET state then 403s with "User not in room".
#     b) Joining the bot into every conversation room is intrusive
#        (adds a 3rd member to private 2-user chats).
#
# Pre-conditions:
#   - kubectl context = target cluster
#   - Synapse + matrix-adapter running and healthy
#   - postgres in-cluster pod accessible
#   - jq, curl available locally
#   - The bot user has users.admin=1 in Synapse (it does for acc/prod)
#
# Usage:
#   ./fix_ghost_power_levels_safe.sh acc                 # dry run, lists rooms
#   ./fix_ghost_power_levels_safe.sh acc --limit 5       # try on first 5 only
#   ./fix_ghost_power_levels_safe.sh acc --apply         # full fleet
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
# Path confirmed on acc 2026-05-14: /data/matrix-adapter.yaml
# (referenced from /data/homeserver.yaml -> app_service_config_files).
# Verify the same on prod with: kubectl exec deploy/synapse-deployment --
#   grep '^app_service_config_files:' /data/homeserver.yaml -A1
REGISTRATION_PATH="/data/matrix-adapter.yaml"
echo ">>> Fetching appservice as_token from $REGISTRATION_PATH on synapse pod..."
AS_TOKEN="$(kubectl exec -n "$NAMESPACE" deploy/synapse-deployment -- \
  grep -E '^as_token:' "$REGISTRATION_PATH" | awk '{print $2}' | tr -d ' \r\n')"
if [ -z "$AS_TOKEN" ]; then
  echo "ERROR: could not retrieve as_token from $REGISTRATION_PATH"
  echo "       Confirm the registration path via /data/homeserver.yaml -> app_service_config_files"
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

# URL-encoded bot mxid for ?user_id=
BOT_URI="$(printf '%s' "$BOT_MXID" | jq -sRr @uri)"

# ---- 2a. Pre-flight: confirm as_token + ?user_id=<bot> works on an admin
#         endpoint AND that the impersonated user is recognised as admin.
#         Calls /_synapse/admin/v2/users/<bot> (read-only) and asserts admin=true.
echo ">>> Pre-flight: verifying admin auth via impersonation..."
PRE=$(curl -fsS "${SYNAPSE_URL}/_synapse/admin/v2/users/${BOT_URI}?user_id=${BOT_URI}" \
  -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || true)
if [ -z "$PRE" ] || ! echo "$PRE" | jq -e '.admin == true' >/dev/null 2>&1; then
  echo "ERROR: pre-flight admin auth failed."
  echo "       response: $PRE"
  echo "       Either the bot is not admin (users.admin=1), or impersonation is not honoured here."
  echo "       Fallback: switch to matrixadmin3 password login for the admin endpoint."
  exit 1
fi
echo "    admin auth OK (bot reports admin=true)"

# ---- 3. List rooms needing a fix (room_id|impersonator per line) -----------
# Per room, pick the best impersonator from inside Matrix:
#   (a) PL-event sender, if they are still joined to the room — most likely
#       to retain PL >= state_default and have rich room context.
#   (b) Fallback: any explicit user in the PL `users` map with PL >= 50 who
#       is currently joined.
#   (c) Empty string if neither — those rooms are unreachable from inside
#       Matrix (no joined user can send a new PL event); they will be logged
#       as failures during the apply loop.
echo ">>> Querying rooms with users_default=0 + best impersonator ..."
ROOMS_RAW=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT cse.room_id || '|' || COALESCE(
  -- (a) PL event's original sender, if still joined
  (CASE WHEN EXISTS (
    SELECT 1 FROM current_state_events cse_s
    JOIN event_json ej_s ON ej_s.event_id = cse_s.event_id
    WHERE cse_s.room_id = cse.room_id
      AND cse_s.type = 'm.room.member'
      AND cse_s.state_key = e.sender
      AND ej_s.json::jsonb->'content'->>'membership' = 'join'
  ) THEN e.sender END),
  -- (b) Any joined user with explicit PL >= 50 in the PL users map
  (SELECT u.key FROM jsonb_each_text(COALESCE(ej.json::jsonb->'content'->'users', '{}'::jsonb)) u
   WHERE u.value::int >= 50
     AND EXISTS (
       SELECT 1 FROM current_state_events cse_u
       JOIN event_json ej_u ON ej_u.event_id = cse_u.event_id
       WHERE cse_u.room_id = cse.room_id
         AND cse_u.type = 'm.room.member'
         AND cse_u.state_key = u.key
         AND ej_u.json::jsonb->'content'->>'membership' = 'join'
     )
   LIMIT 1),
  -- (c) No reachable impersonator
  ''
)
FROM current_state_events cse
JOIN events e ON e.event_id = cse.event_id
JOIN event_json ej ON ej.event_id = cse.event_id
WHERE cse.type = 'm.room.power_levels'
  AND (ej.json::jsonb->'content'->>'users_default')::int = 0
ORDER BY cse.room_id;
")
ROOM_COUNT=$(printf '%s\n' "$ROOMS_RAW" | grep -c . || true)
echo "    $ROOM_COUNT rooms have users_default=0"

if [ "$ROOM_COUNT" = "0" ]; then
  echo "Nothing to do."
  exit 0
fi

if [ -n "$LIMIT_N" ]; then
  # NOTE: heredoc instead of printf | head to avoid SIGPIPE on printf
  # under `set -e -o pipefail` (head closes stdin after N lines).
  ROOMS_RAW=$(head -n "$LIMIT_N" <<< "$ROOMS_RAW")
  ROOM_COUNT="$LIMIT_N"
  echo "    --limit $LIMIT_N → will only process first $LIMIT_N"
fi

if [ "$MODE" != "--apply" ] && [ "$MODE" != "--limit" ]; then
  echo
  echo "DRY-RUN. Sample of first 5 affected rooms (room | PL-event sender):"
  head -5 <<< "$ROOMS_RAW" | column -ts '|'
  echo
  echo "Re-run with --limit N or --apply to execute."
  exit 0
fi

# ---- 4. For each room: GET PL as the PL sender, PUT new PL with users_default=50
echo ">>> Applying fix ..."
OK=0
FAIL=0
declare -a FAILED_ROOMS=()

while IFS='|' read -r ROOM_ID SENDER; do
  [ -z "$ROOM_ID" ] && continue
  # Rooms with no in-room user able to send a PL state event — count as failure.
  if [ -z "$SENDER" ]; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|unreachable|no joined user with PL >= state_default in this room")
    continue
  fi
  ROOM_URI=$(printf '%s' "$ROOM_ID" | jq -sRr @uri)
  SENDER_URI=$(printf '%s' "$SENDER" | jq -sRr @uri)

  # 4a. Fetch the current PL state event as the original sender.
  CURRENT_PL=$(curl -fsS \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/state/m.room.power_levels?user_id=${SENDER_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || true)
  if [ -z "$CURRENT_PL" ] || ! echo "$CURRENT_PL" | jq -e . >/dev/null 2>&1; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|get_pl|sender=$SENDER|$(printf '%s' "$CURRENT_PL" | head -c 160)")
    continue
  fi

  # 4b. Already at users_default=50? (race / already-fixed) — skip.
  if [ "$(printf '%s' "$CURRENT_PL" | jq -r '.users_default // 0')" = "50" ]; then
    OK=$((OK+1))
    continue
  fi

  # 4c. PUT a fresh PL state event with users_default merged to 50.
  NEW_PL=$(printf '%s' "$CURRENT_PL" | jq -c '.users_default = 50')
  PUT_RESP=$(curl -fsS -X PUT \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/state/m.room.power_levels?user_id=${SENDER_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$NEW_PL" 2>/dev/null || true)

  if echo "$PUT_RESP" | jq -e '.event_id' >/dev/null 2>&1; then
    OK=$((OK+1))
    if [ $((OK % 50)) -eq 0 ]; then
      echo "    progress: ok=$OK fail=$FAIL"
    fi
  else
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|put_pl|sender=$SENDER|$(printf '%s' "$PUT_RESP" | head -c 160)")
  fi

  sleep 0.2  # 5 req/sec — matches the Phase-2 mutation throttle; safe for prod load
done <<< "$ROOMS_RAW"

echo
echo "=== summary ==="
echo "OK:   $OK"
echo "FAIL: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "First 20 failures:"
  for r in "${FAILED_ROOMS[@]:0:20}"; do echo "$r"; done
fi

# ---- 5. Verify --------------------------------------------------------------
echo
echo ">>> Verifying via DB (current_state_events + event_json) ..."
REMAINING=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT count(*)
FROM current_state_events cse
JOIN event_json ej ON cse.event_id = ej.event_id
WHERE cse.type = 'm.room.power_levels'
  AND (ej.json::jsonb->'content'->>'users_default')::int = 0;
")
echo "Remaining rooms with users_default=0: $REMAINING (should be 0 modulo failures)"

echo
echo ">>> Smoke check for DatabaseCorruptionError on synapse log (last 2m) ..."
COUNT=$(kubectl logs -n "$NAMESPACE" deploy/synapse-deployment --since=2m 2>&1 | grep -c DatabaseCorruptionError || true)
echo "DatabaseCorruptionError count: $COUNT (must be 0)"
