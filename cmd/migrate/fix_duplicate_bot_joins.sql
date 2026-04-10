-- ============================================================================
-- fix_duplicate_bot_joins.sql
-- ----------------------------------------------------------------------------
-- Repairs Synapse rooms where the appservice bot has more than one
-- m.room.member "join" event in its event graph. The duplicates were
-- introduced by an earlier version of `redactCanonicalAliasesFromRooms`
-- that called `admin.JoinRoom` on rooms where the bot was already a
-- member, then issued a state event whose internal `EnsureJoined` call
-- added a second join. This corrupts the auth chain (event_auth) and the
-- state delta cache (state_groups_state).
--
-- The script picks the EARLIEST join per room as canonical, then remaps
-- every reference to any other bot-join in that room to the canonical
-- one. The DELETE in step 4a is required because step 4b's UPDATE would
-- otherwise violate the (event_id, auth_id) unique constraint on
-- event_auth wherever the canonical pair already exists.
--
-- Tested on Synapse 1.x with the matrix-acc.alkem.io homeserver.
-- ----------------------------------------------------------------------------
-- USAGE
--   1. Stop the matrix adapter (kubectl scale ... --replicas=0).
--   2. EDIT the bot user ID below for the target homeserver:
--        acc:  '@00000000-0000-0000-0000-000000000000:matrix-acc.alkem.io'
--        prod: '@00000000-0000-0000-0000-000000000000:matrix.alkem.io'
--      Replace the literal in EVERY occurrence (steps 1, 2, 3, 5).
--   3. Run inside a single transaction (BEGIN/COMMIT are explicit below).
--   4. Step 5 must report 0 / 0 before COMMIT. If it doesn't, ROLLBACK
--      and investigate.
--   5. Restart the adapter.
-- ----------------------------------------------------------------------------
-- ============================================================================

BEGIN;

-- Step 1: pick the canonical (earliest) bot join per room.
CREATE TEMP TABLE bot_originals AS
SELECT DISTINCT ON (e.room_id) e.room_id, e.event_id AS original_event_id
FROM events e
JOIN event_json ej ON e.event_id = ej.event_id
WHERE e.type = 'm.room.member'
  AND e.state_key = '@00000000-0000-0000-0000-000000000000:matrix-acc.alkem.io'
  AND ej.json::jsonb->'content'->>'membership' = 'join'
ORDER BY e.room_id, e.origin_server_ts;

-- Step 2: list every duplicate (any bot join that isn't the canonical one).
CREATE TEMP TABLE bot_duplicates AS
SELECT e.event_id AS dup_event_id, o.original_event_id, e.room_id
FROM events e
JOIN event_json ej ON e.event_id = ej.event_id
JOIN bot_originals o ON e.room_id = o.room_id
WHERE e.type = 'm.room.member'
  AND e.state_key = '@00000000-0000-0000-0000-000000000000:matrix-acc.alkem.io'
  AND ej.json::jsonb->'content'->>'membership' = 'join'
  AND e.event_id != o.original_event_id;

-- Step 3: remap state_groups_state references to the canonical event.
UPDATE state_groups_state sgs
SET event_id = d.original_event_id
FROM bot_duplicates d
WHERE sgs.event_id = d.dup_event_id
  AND sgs.type = 'm.room.member'
  AND sgs.state_key = '@00000000-0000-0000-0000-000000000000:matrix-acc.alkem.io';

-- Step 4a: drop event_auth rows that would otherwise collide with the
--          (event_id, auth_id) unique constraint when we update them in 4b.
DELETE FROM event_auth ea
USING bot_duplicates d
WHERE ea.auth_id = d.dup_event_id
  AND EXISTS (
    SELECT 1 FROM event_auth ea2
    WHERE ea2.event_id = ea.event_id
      AND ea2.auth_id = d.original_event_id
  );

-- Step 4b: remap the remaining event_auth references to the canonical event.
UPDATE event_auth ea
SET auth_id = d.original_event_id
FROM bot_duplicates d
WHERE ea.auth_id = d.dup_event_id;

-- Step 5: verification. BOTH counts MUST be 0 before COMMIT.
SELECT 'state_groups_state remaining:' AS check, count(*) AS count
FROM state_groups_state sgs
JOIN bot_duplicates d ON sgs.event_id = d.dup_event_id
WHERE sgs.type = 'm.room.member'
  AND sgs.state_key = '@00000000-0000-0000-0000-000000000000:matrix-acc.alkem.io'
UNION ALL
SELECT 'event_auth remaining:', count(*)
FROM event_auth ea
JOIN bot_duplicates d ON ea.auth_id = d.dup_event_id;

COMMIT;
