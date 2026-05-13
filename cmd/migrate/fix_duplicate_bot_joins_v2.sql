-- ============================================================================
-- fix_duplicate_bot_joins_v2.sql
-- ----------------------------------------------------------------------------
-- Comprehensive repair for rooms where the appservice bot has duplicate
-- m.room.member "join" events in the event graph.
--
-- v1 fixed event_auth and state_groups_state, but missed event_json.
-- Synapse reads auth_events from the event JSON (not from event_auth table)
-- in check_state_independent_auth_rules (synapse/event_auth.py:247).
-- Events created after the duplicate joins carry BOTH the original and
-- duplicate bot join IDs in their auth_events JSON array. When Synapse
-- tries to create a new event (e.g. EnsureJoined), state resolution
-- surfaces both, and the auth check rejects the new event with:
--   "Event $X has duplicate auth_events for ('m.room.member', '@bot')"
--
-- This script fixes ALL affected tables:
--   1. state_groups_state  (v1 — idempotent if already applied)
--   2. event_auth          (v1 — idempotent if already applied)
--   3. event_json          (NEW — patches auth_events arrays in event JSON)
--   4. current_state_events (NEW — ensures canonical event is current)
--
-- Safe for single-server (non-federated) deployments: event signatures
-- and content hashes are only verified by remote servers.
-- ----------------------------------------------------------------------------
-- USAGE
--   1. Stop the matrix adapter (kubectl scale ... --replicas=0).
--   2. Set the bot user ID for your environment:
--        acc:  bot_mxid = '@00000000-0000-0000-0000-000000000000:matrix-acc.alkem.io'
--        prod: bot_mxid = '@00000000-0000-0000-0000-000000000000:matrix.alkem.io'
--   3. Run:
--        psql -U <user> -d synapse \
--          -v bot_mxid="'@00000000-...:matrix.alkem.io'" \
--          -f fix_duplicate_bot_joins_v2.sql
--   4. ALL verification counts in step 7 must be 0 before COMMIT.
--      If not, ROLLBACK and investigate.
--   5. Restart Synapse (to clear in-memory event/state caches).
--   6. Restart the adapter.
-- ============================================================================

BEGIN;

-- =====================
-- Step 1: Identify canonical (earliest) bot join per room
-- =====================
CREATE TEMP TABLE bot_originals AS
SELECT DISTINCT ON (e.room_id) e.room_id, e.event_id AS original_event_id
FROM events e
JOIN event_json ej ON e.event_id = ej.event_id
WHERE e.type = 'm.room.member'
  AND e.state_key = :'bot_mxid'
  AND ej.json::jsonb->'content'->>'membership' = 'join'
ORDER BY e.room_id, e.origin_server_ts;

-- =====================
-- Step 2: Identify all duplicates (any bot join that isn't the canonical one)
-- =====================
CREATE TEMP TABLE bot_duplicates AS
SELECT e.event_id AS dup_event_id, o.original_event_id, e.room_id
FROM events e
JOIN event_json ej ON e.event_id = ej.event_id
JOIN bot_originals o ON e.room_id = o.room_id
WHERE e.type = 'm.room.member'
  AND e.state_key = :'bot_mxid'
  AND ej.json::jsonb->'content'->>'membership' = 'join'
  AND e.event_id != o.original_event_id;

-- =====================
-- Step 3: Diagnostics — show scope before fixing
-- =====================
SELECT 'duplicate bot joins found' AS diagnostic, count(*) AS count
FROM bot_duplicates
UNION ALL
SELECT 'rooms affected', count(DISTINCT room_id)
FROM bot_duplicates
UNION ALL
SELECT 'state_groups_state refs to fix', count(*)
FROM state_groups_state sgs
JOIN bot_duplicates d ON sgs.event_id = d.dup_event_id
WHERE sgs.type = 'm.room.member'
  AND sgs.state_key = :'bot_mxid'
UNION ALL
SELECT 'event_auth refs to fix', count(*)
FROM event_auth ea
JOIN bot_duplicates d ON ea.auth_id = d.dup_event_id
UNION ALL
SELECT 'event_json auth_events to fix', count(DISTINCT ej.event_id)
FROM event_json ej
JOIN bot_duplicates d ON ej.room_id = d.room_id
WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements_text(ej.json::jsonb -> 'auth_events') ae
    WHERE ae = d.dup_event_id
)
UNION ALL
SELECT 'current_state_events to fix', count(*)
FROM current_state_events cse
JOIN bot_duplicates d ON cse.event_id = d.dup_event_id
WHERE cse.type = 'm.room.member'
  AND cse.state_key = :'bot_mxid';

-- =====================
-- Step 4: Fix state_groups_state (idempotent — 0 rows if v1 was applied)
-- =====================
UPDATE state_groups_state sgs
SET event_id = d.original_event_id
FROM bot_duplicates d
WHERE sgs.event_id = d.dup_event_id
  AND sgs.type = 'm.room.member'
  AND sgs.state_key = :'bot_mxid';

-- =====================
-- Step 5: Fix event_auth (idempotent — 0 rows if v1 was applied)
-- =====================

-- 5a: Delete rows that would violate unique constraint after remap
DELETE FROM event_auth ea
USING bot_duplicates d
WHERE ea.auth_id = d.dup_event_id
  AND EXISTS (
    SELECT 1 FROM event_auth ea2
    WHERE ea2.event_id = ea.event_id
      AND ea2.auth_id = d.original_event_id
  );

-- 5b: Remap remaining references
UPDATE event_auth ea
SET auth_id = d.original_event_id
FROM bot_duplicates d
WHERE ea.auth_id = d.dup_event_id;

-- =====================
-- Step 6: Fix auth_events arrays inside event JSON (NEW)
--
-- For each event whose auth_events JSON array contains a duplicate
-- bot join event ID: replace the duplicate with the canonical and
-- deduplicate. This is THE fix for the "duplicate auth_events" error
-- since Synapse reads auth_events from event JSON, not event_auth table.
-- =====================
UPDATE event_json ej
SET json = (
    jsonb_set(
        ej.json::jsonb,
        '{auth_events}',
        (
            SELECT COALESCE(
                jsonb_agg(DISTINCT to_jsonb(fixed_id)),
                '[]'::jsonb
            )
            FROM (
                SELECT CASE
                    WHEN d.dup_event_id IS NOT NULL THEN d.original_event_id
                    ELSE ae
                END AS fixed_id
                FROM jsonb_array_elements_text(ej.json::jsonb -> 'auth_events') ae
                LEFT JOIN bot_duplicates d ON ae = d.dup_event_id
            ) sub
        )
    )
)::text
WHERE ej.room_id IN (SELECT room_id FROM bot_duplicates)
  AND EXISTS (
    SELECT 1
    FROM jsonb_array_elements_text(ej.json::jsonb -> 'auth_events') ae
    JOIN bot_duplicates d ON ae = d.dup_event_id
  );

-- =====================
-- Step 7: Fix current_state_events (NEW)
--
-- If the "current" m.room.member event for the bot points to a
-- duplicate, remap it to the canonical. This table is Synapse's
-- fast-path for current room state lookups.
-- =====================
UPDATE current_state_events cse
SET event_id = d.original_event_id
FROM bot_duplicates d
WHERE cse.event_id = d.dup_event_id
  AND cse.type = 'm.room.member'
  AND cse.state_key = :'bot_mxid';

-- =====================
-- Step 8: Verification — ALL counts must be 0
-- =====================
SELECT 'state_groups_state remaining' AS check, count(*) AS must_be_zero
FROM state_groups_state sgs
JOIN bot_duplicates d ON sgs.event_id = d.dup_event_id
WHERE sgs.type = 'm.room.member'
  AND sgs.state_key = :'bot_mxid'
UNION ALL
SELECT 'event_auth remaining', count(*)
FROM event_auth ea
JOIN bot_duplicates d ON ea.auth_id = d.dup_event_id
UNION ALL
SELECT 'event_json auth_events remaining', count(DISTINCT ej.event_id)
FROM event_json ej
JOIN bot_duplicates d ON ej.room_id = d.room_id
WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements_text(ej.json::jsonb -> 'auth_events') ae
    WHERE ae = d.dup_event_id
)
UNION ALL
SELECT 'current_state_events remaining', count(*)
FROM current_state_events cse
JOIN bot_duplicates d ON cse.event_id = d.dup_event_id
WHERE cse.type = 'm.room.member'
  AND cse.state_key = :'bot_mxid';

COMMIT;
