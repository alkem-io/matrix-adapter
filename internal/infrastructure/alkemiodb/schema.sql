-- Schema placeholder for SQLC
-- This adapter uses READ-ONLY access to the Alkemio database.
-- No migrations are managed here; the schema is owned by the Alkemio Server.

-- Minimal schema definitions required for SQLC type generation

CREATE TABLE IF NOT EXISTS "user" (
    id UUID PRIMARY KEY,
    "agentId" UUID
);

CREATE TABLE IF NOT EXISTS virtual_contributor (
    id UUID PRIMARY KEY,
    "agentId" UUID
);
