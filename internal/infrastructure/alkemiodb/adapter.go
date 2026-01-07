// Package alkemiodb provides read-only access to the Alkemio database
// for actor ID resolution during the migration period.
package alkemiodb

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/infrastructure/alkemiodb/db"
)

// Adapter implements the ActorResolver port using direct Alkemio database access.
// This is a temporary adapter for the migration period.
// Includes an in-memory cache since mappings are permanent ("married for life").
type Adapter struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	logger  *zap.Logger

	// In-memory cache for permanent mappings (no expiration needed)
	cacheMu       sync.RWMutex
	actorToEntity map[uuid.UUID]uuid.UUID // actorID -> entityID
	entityToActor map[uuid.UUID]uuid.UUID // entityID -> actorID
}

// NewAdapter creates a new Adapter with connection validation.
// Returns an error if the database is unreachable (fail-fast at startup).
func NewAdapter(ctx context.Context, connString string, logger *zap.Logger) (*Adapter, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Validate connection at startup - fail if unreachable
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to connect to Alkemio database: %w", err)
	}

	logger.Info("Connected to Alkemio database for actor ID resolution")

	return &Adapter{
		pool:          pool,
		queries:       db.New(pool),
		logger:        logger,
		actorToEntity: make(map[uuid.UUID]uuid.UUID),
		entityToActor: make(map[uuid.UUID]uuid.UUID),
	}, nil
}

// ResolveActorToEntityID converts an Alkemio actor ID (agent.id) to the corresponding
// entity ID (user.id or virtual_contributor.id).
// Results are cached permanently since mappings never change.
// Checks user table first, then virtual_contributor table.
func (a *Adapter) ResolveActorToEntityID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error) {
	// Check cache first
	a.cacheMu.RLock()
	if entityID, ok := a.actorToEntity[actorID]; ok {
		a.cacheMu.RUnlock()
		a.logger.Debug("Cache hit: actor to entity",
			zap.String("actor_id", actorID.String()),
			zap.String("entity_id", entityID.String()),
		)
		return entityID, nil
	}
	a.cacheMu.RUnlock()

	// Cache miss - query database
	pgActorID := uuidToPgtype(actorID)

	// Try user table first
	userID, err := a.queries.GetUserIDByAgentID(ctx, pgActorID)
	if err == nil {
		entityID := pgtypeToUUID(userID)
		a.cacheMapping(actorID, entityID)
		a.logger.Debug("Resolved actor to user entity (cached)",
			zap.String("actor_id", actorID.String()),
			zap.String("entity_id", entityID.String()),
		)
		return entityID, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		a.logger.Error("Database error looking up user by agent ID",
			zap.String("actor_id", actorID.String()),
			zap.Error(err),
		)
		return uuid.Nil, fmt.Errorf("database error: %w", err)
	}

	// Try virtual_contributor table
	vcID, err := a.queries.GetVCIDByAgentID(ctx, pgActorID)
	if err == nil {
		entityID := pgtypeToUUID(vcID)
		a.cacheMapping(actorID, entityID)
		a.logger.Debug("Resolved actor to virtual contributor entity (cached)",
			zap.String("actor_id", actorID.String()),
			zap.String("entity_id", entityID.String()),
		)
		return entityID, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		a.logger.Error("Database error looking up virtual contributor by agent ID",
			zap.String("actor_id", actorID.String()),
			zap.Error(err),
		)
		return uuid.Nil, fmt.Errorf("database error: %w", err)
	}

	// Actor not found in either table
	a.logger.Warn("Actor ID not found in user or virtual_contributor tables",
		zap.String("actor_id", actorID.String()),
	)
	return uuid.Nil, domain.NewActorNotFoundError(actorID.String())
}

// ResolveEntityToActorID converts an entity ID (user.id or virtual_contributor.id)
// back to the Alkemio actor ID (agent.id).
// Results are cached permanently since mappings never change.
// Checks user table first, then virtual_contributor table.
func (a *Adapter) ResolveEntityToActorID(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error) {
	// Check cache first
	a.cacheMu.RLock()
	if actorID, ok := a.entityToActor[entityID]; ok {
		a.cacheMu.RUnlock()
		a.logger.Debug("Cache hit: entity to actor",
			zap.String("entity_id", entityID.String()),
			zap.String("actor_id", actorID.String()),
		)
		return actorID, nil
	}
	a.cacheMu.RUnlock()

	// Cache miss - query database
	pgEntityID := uuidToPgtype(entityID)

	// Try user table first
	agentID, err := a.queries.GetAgentIDByUserID(ctx, pgEntityID)
	if err == nil {
		actorID := pgtypeToUUID(agentID)
		a.cacheMapping(actorID, entityID)
		a.logger.Debug("Resolved user entity to actor (cached)",
			zap.String("entity_id", entityID.String()),
			zap.String("actor_id", actorID.String()),
		)
		return actorID, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		a.logger.Error("Database error looking up agent ID by user ID",
			zap.String("entity_id", entityID.String()),
			zap.Error(err),
		)
		return uuid.Nil, fmt.Errorf("database error: %w", err)
	}

	// Try virtual_contributor table
	agentID, err = a.queries.GetAgentIDByVCID(ctx, pgEntityID)
	if err == nil {
		actorID := pgtypeToUUID(agentID)
		a.cacheMapping(actorID, entityID)
		a.logger.Debug("Resolved virtual contributor entity to actor (cached)",
			zap.String("entity_id", entityID.String()),
			zap.String("actor_id", actorID.String()),
		)
		return actorID, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		a.logger.Error("Database error looking up agent ID by virtual contributor ID",
			zap.String("entity_id", entityID.String()),
			zap.Error(err),
		)
		return uuid.Nil, fmt.Errorf("database error: %w", err)
	}

	// Entity not found in either table
	a.logger.Warn("Entity ID not found in user or virtual_contributor tables",
		zap.String("entity_id", entityID.String()),
	)
	return uuid.Nil, domain.NewEntityNotFoundError(entityID.String())
}

// Close releases database connection resources.
func (a *Adapter) Close() {
	if a.pool != nil {
		a.pool.Close()
		a.logger.Info("Closed Alkemio database connection")
	}
}

// cacheMapping stores a bidirectional mapping in the cache.
// This is safe to call multiple times with the same values.
func (a *Adapter) cacheMapping(actorID, entityID uuid.UUID) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	a.actorToEntity[actorID] = entityID
	a.entityToActor[entityID] = actorID
}

// Helper functions for UUID conversion

func uuidToPgtype(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{
		Bytes: u,
		Valid: true,
	}
}

func pgtypeToUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return p.Bytes
}
