#!/usr/bin/env bash
# ============================================================================
# redact_canonical_aliases.sh
# ----------------------------------------------------------------------------
# Clear m.room.canonical_alias state events from non-space rooms.
#
# After Part 2 (server sync mutation) clears room names on direct
# conversations, Element falls back to displaying the canonical alias
# (a UUID like #a1610b03-...:matrix-acc.alkem.io) instead of computing
# the name from the member list. Clearing the canonical alias lets
# Element render "Valentin" instead of "#a1610b03-...".
#
# Space rooms are skipped — they have proper display names and the alias
# is harmless there.
#
# Approach per room:
#   For each non-space room that has a m.room.canonical_alias state event,
#   we impersonate a joined user with sufficient power level and PUT an
#   empty canonical alias state event. Synapse creates a proper event
#   with correct content hash and auth chain.
#
#   Impersonator selection (same strategy as fix_ghost_power_levels_safe.sh):
#     (a) The sender of the current canonical_alias event, if still joined
#     (b) Any joined user with explicit PL >= state_default (50)
#     (c) Empty — room is unreachable, logged as failure
#
# Pre-conditions:
#   - kubectl context = target cluster
#   - Synapse + matrix-adapter running and healthy
#   - postgres in-cluster pod accessible
#   - jq, curl available locally
#   - The bot user has users.admin=1 in Synapse
#
# Usage:
#   ./redact_canonical_aliases.sh acc                 # dry run, lists rooms
#   ./redact_canonical_aliases.sh acc --limit 5       # try on first 5 only
#   ./redact_canonical_aliases.sh acc --apply         # full fleet
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

# ---- 2a. Pre-flight: confirm appservice auth works ---------------------------

BOT_SENDER=$(kubectl exec -n "$NAMESPACE" deploy/synapse-deployment -- \
  grep -E '^sender_localpart:' "$REGISTRATION_PATH" | awk '{print $2}' | tr -d ' \r\n"')
SERVER_NAME=$(kubectl exec -n "$NAMESPACE" deploy/synapse-deployment -- \
  grep -E '^server_name:' /data/homeserver.yaml | awk '{print $2}' | tr -d ' \r\n"')
BOT_MXID="@${BOT_SENDER}:${SERVER_NAME}"
BOT_URI="$(printf '%s' "$BOT_MXID" | jq -sRr @uri)"

echo ">>> Pre-flight: verifying admin auth via impersonation (bot=$BOT_MXID)..."
PRE=$(curl -fsS "${SYNAPSE_URL}/_synapse/admin/v2/users/${BOT_URI}?user_id=${BOT_URI}" \
  -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || true)
if [ -z "$PRE" ] || ! echo "$PRE" | jq -e '.admin == true' >/dev/null 2>&1; then
  echo "ERROR: pre-flight admin auth failed. response: $PRE"
  exit 1
fi
echo "    admin auth OK"

# ---- 3. List non-space rooms with canonical alias + best impersonator --------
#
# Excludes spaces (m.room.create content has type=m.space).
# For each room, picks an impersonator who can send state events:
#   (a) The sender of the current canonical_alias event, if still joined
#   (b) Any joined user with explicit PL >= 50
#   (c) Empty — unreachable

echo ">>> Querying non-space rooms with canonical alias set ..."
ROOMS_RAW=$(kubectl exec -n "$NAMESPACE" "$POSTGRES_POD" -- psql -U synapse-db -d synapse -tAc "
SELECT cse.room_id || '|' || COALESCE(
  -- (a) canonical_alias event sender, if still joined
  (CASE WHEN EXISTS (
    SELECT 1 FROM current_state_events cse_m
    JOIN event_json ej_m ON ej_m.event_id = cse_m.event_id
    WHERE cse_m.room_id = cse.room_id
      AND cse_m.type = 'm.room.member'
      AND cse_m.state_key = e.sender
      AND ej_m.json::jsonb->'content'->>'membership' = 'join'
  ) THEN e.sender END),
  -- (b) Any joined user with effective PL >= 50 (explicit or via users_default)
  (SELECT cse_u.state_key
   FROM current_state_events cse_u
   JOIN event_json ej_u ON ej_u.event_id = cse_u.event_id
   JOIN current_state_events cse_pl ON cse_pl.room_id = cse_u.room_id
     AND cse_pl.type = 'm.room.power_levels'
   JOIN event_json ej_pl ON ej_pl.event_id = cse_pl.event_id
   WHERE cse_u.room_id = cse.room_id
     AND cse_u.type = 'm.room.member'
     AND ej_u.json::jsonb->'content'->>'membership' = 'join'
     AND COALESCE(
       (ej_pl.json::jsonb->'content'->'users'->>cse_u.state_key)::int,
       (ej_pl.json::jsonb->'content'->>'users_default')::int,
       0
     ) >= 50
   LIMIT 1),
  -- (c) No reachable impersonator
  ''
)
FROM current_state_events cse
JOIN events e ON e.event_id = cse.event_id
JOIN event_json ej ON ej.event_id = cse.event_id
WHERE cse.type = 'm.room.canonical_alias'
  -- Has a non-empty alias set
  AND COALESCE(ej.json::jsonb->'content'->>'alias', '') != ''
  -- Exclude spaces
  AND NOT EXISTS (
    SELECT 1 FROM current_state_events cse_c
    JOIN event_json ej_c ON ej_c.event_id = cse_c.event_id
    WHERE cse_c.room_id = cse.room_id
      AND cse_c.type = 'm.room.create'
      AND ej_c.json::jsonb->'content'->>'type' = 'm.space'
  )
ORDER BY cse.room_id;
")
ROOM_COUNT=$(printf '%s\n' "$ROOMS_RAW" | grep -c . || true)
echo "    $ROOM_COUNT non-space rooms have canonical alias set"

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
  echo "DRY-RUN. Sample of first 10 affected rooms (room | impersonator):"
  head -10 <<< "$ROOMS_RAW" | column -ts '|'
  echo
  echo "Re-run with --limit N or --apply to execute."
  exit 0
fi

# ---- 4. For each room: PUT empty canonical alias as the impersonator ---------

echo ">>> Clearing canonical aliases ..."
OK=0
FAIL=0
SKIP=0
declare -a FAILED_ROOMS=()

while IFS='|' read -r ROOM_ID SENDER; do
  [ -z "$ROOM_ID" ] && continue

  if [ -z "$SENDER" ]; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|unreachable|no joined user with PL >= state_default")
    continue
  fi

  ROOM_URI=$(printf '%s' "$ROOM_ID" | jq -sRr @uri)
  SENDER_URI=$(printf '%s' "$SENDER" | jq -sRr @uri)

  # 4a. Verify the canonical alias is still set (might have been cleared by a re-run)
  CURRENT=$(curl -fsS \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/state/m.room.canonical_alias?user_id=${SENDER_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" 2>/dev/null || true)
  if [ -z "$CURRENT" ] || ! echo "$CURRENT" | jq -e . >/dev/null 2>&1; then
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|get_alias|sender=$SENDER|$(printf '%s' "$CURRENT" | head -c 160)")
    continue
  fi

  CURRENT_ALIAS=$(printf '%s' "$CURRENT" | jq -r '.alias // ""')
  if [ -z "$CURRENT_ALIAS" ]; then
    SKIP=$((SKIP+1))
    continue
  fi

  # 4b. PUT empty canonical alias — clears it
  PUT_RESP=$(curl -fsS -X PUT \
    "${SYNAPSE_URL}/_matrix/client/v3/rooms/${ROOM_URI}/state/m.room.canonical_alias?user_id=${SENDER_URI}" \
    -H "Authorization: Bearer $AS_TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{}' 2>/dev/null || true)

  if echo "$PUT_RESP" | jq -e '.event_id' >/dev/null 2>&1; then
    OK=$((OK+1))
    if [ $((OK % 50)) -eq 0 ]; then
      echo "    progress: ok=$OK fail=$FAIL skip=$SKIP"
    fi
  else
    FAIL=$((FAIL+1))
    FAILED_ROOMS+=("$ROOM_ID|put_alias|sender=$SENDER|$(printf '%s' "$PUT_RESP" | head -c 160)")
  fi

  sleep 0.2
done <<< "$ROOMS_RAW"

echo
echo "=== summary ==="
echo "OK (cleared):  $OK"
echo "SKIP (already): $SKIP"
echo "FAIL:          $FAIL"
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
FROM current_state_events cse
JOIN event_json ej ON cse.event_id = ej.event_id
WHERE cse.type = 'm.room.canonical_alias'
  AND COALESCE(ej.json::jsonb->'content'->>'alias', '') != ''
  AND NOT EXISTS (
    SELECT 1 FROM current_state_events cse_c
    JOIN event_json ej_c ON ej_c.event_id = cse_c.event_id
    WHERE cse_c.room_id = cse.room_id
      AND cse_c.type = 'm.room.create'
      AND ej_c.json::jsonb->'content'->>'type' = 'm.space'
  );
")
echo "Remaining non-space rooms with canonical alias: $REMAINING (should be 0 modulo failures)"
