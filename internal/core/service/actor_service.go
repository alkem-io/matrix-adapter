package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

// ActorService handles operations related to actor profile synchronization.
type ActorService struct {
	matrix ports.MatrixPort
	logger ports.Logger
}

// NewActorService creates a new instance of ActorService.
func NewActorService(matrix ports.MatrixPort, logger ports.Logger) *ActorService {
	return &ActorService{
		matrix: matrix,
		logger: logger,
	}
}

// SyncActor ensures an actor exists in Matrix and updates their profile.
// This is an idempotent operation - the Intent API will create the user if
// it doesn't exist, or update the profile if it does.
func (s *ActorService) SyncActor(
	ctx context.Context,
	actorID uuid.UUID,
	displayName string,
	avatarURL string,
) error {
	s.logger.Info("Syncing actor profile",
		"actor_id", actorID,
		"display_name", displayName)

	actor := domain.Actor{
		ID:          actorID,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
	}

	// EnsureUser creates the Matrix user via the Intent API if it doesn't exist
	_, err := s.matrix.EnsureUser(ctx, actor)
	if err != nil {
		return err
	}

	// SetUserProfile updates the display name and avatar
	err = s.matrix.SetUserProfile(ctx, actor)
	if err != nil {
		return err
	}

	s.logger.Info("Actor profile synced successfully", "actor_id", actorID)
	return nil
}
