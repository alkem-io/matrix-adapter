-- ============================================================================
-- fix_ghost_power_levels.sql
-- ----------------------------------------------------------------------------
-- Sets users_default to 50 in all rooms where it is currently 0.
--
-- Background: rooms created between ~Jan 2026 and Apr 2026 used presets
-- that left users_default at 0, meaning all ghost users (appservice-managed
-- Matrix users) have PL 0. With PL 0 they cannot:
--   - Send state events (requires PL >= state_default, typically 50)
--   - Invite other users (requires PL >= invite, typically 50)
--
-- After the bot leaves non-space rooms (leaveBotFromNonSpaceRooms migration),
-- the only remaining members are ghosts. If they have PL 0, no one in the
-- room can perform state operations or invite the bot back.
--
-- This script updates the power_levels event JSON in-place, setting
-- users_default from 0 to 50. Since users_default applies to any user
-- NOT explicitly listed in the `users` map, all ghosts immediately
-- inherit PL 50 without needing per-user entries.
--
-- This is safe for a single-server (non-federated) deployment because
-- event signatures are only verified by remote servers.
-- ----------------------------------------------------------------------------
-- USAGE
--   1. Stop the matrix adapter (kubectl scale ... --replicas=0).
--   2. Run inside a single transaction (BEGIN/COMMIT are explicit below).
--   3. Verification query must report 0 remaining. If not, ROLLBACK.
--   4. Restart Synapse (to clear in-memory state caches).
--   5. Restart the adapter.
-- ----------------------------------------------------------------------------
-- ============================================================================

BEGIN;

-- Update users_default from 0 to 50 in all current power_levels events.
UPDATE event_json ej
SET json = jsonb_set(ej.json::jsonb, '{content,users_default}', '50')::text
FROM current_state_events cse
WHERE cse.event_id = ej.event_id
  AND cse.type = 'm.room.power_levels'
  AND (ej.json::jsonb->'content'->>'users_default')::int = 0;

-- Verification: must be 0.
SELECT 'rooms still with users_default=0:' AS check, COUNT(*) AS count
FROM current_state_events cse
JOIN event_json ej ON cse.event_id = ej.event_id
WHERE cse.type = 'm.room.power_levels'
  AND (ej.json::jsonb->'content'->>'users_default')::int = 0;

COMMIT;
