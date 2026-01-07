-- Forward Mapping: Actor ID (agent.id) → Entity ID (user.id or virtual_contributor.id)
-- These queries support the Alkemio → Matrix direction

-- name: GetUserIDByAgentID :one
-- Lookup user.id by the agent ID (actor ID)
SELECT id FROM "user" WHERE "agentId" = $1;

-- name: GetVCIDByAgentID :one
-- Lookup virtual_contributor.id by the agent ID (actor ID)
SELECT id FROM virtual_contributor WHERE "agentId" = $1;

-- Reverse Mapping: Entity ID → Actor ID (agent.id)
-- These queries support the Matrix → Alkemio direction

-- name: GetAgentIDByUserID :one
-- Lookup agent ID from user.id (entity ID)
SELECT "agentId" FROM "user" WHERE id = $1;

-- name: GetAgentIDByVCID :one
-- Lookup agent ID from virtual_contributor.id (entity ID)
SELECT "agentId" FROM virtual_contributor WHERE id = $1;
