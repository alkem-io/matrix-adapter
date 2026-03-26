package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// SpaceService handles operations related to Matrix Spaces.
type SpaceService struct {
	matrix   ports.MatrixPort
	logger   ports.Logger
	idMapper *domain.IDMapper
}

// NewSpaceService creates a new instance of SpaceService.
func NewSpaceService(matrix ports.MatrixPort, logger ports.Logger, idMapper *domain.IDMapper) *SpaceService {
	return &SpaceService{
		matrix:   matrix,
		logger:   logger,
		idMapper: idMapper,
	}
}

// ============================================================================
// Space CRUD Operations (communication.space.*)
// ============================================================================

// CreateSpace creates a new Matrix Space with idempotent alias lookup.
// If the space already exists (alias resolves), it returns success.
func (s *SpaceService) CreateSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
	name, topic, avatarURL string,
	joinRule string,
	isPublic *bool,
	parentContextID *uuid.UUID,
	initialMembers []domain.Actor,
) error {
	s.logger.Info("Creating space with Alkemio Context ID",
		"alkemio_context_id", alkemioContextID,
		"name", name,
		"join_rule", joinRule)

	// Build alias for idempotency check
	alias := s.idMapper.SpaceAlias(alkemioContextID)

	// Check if space already exists (idempotency)
	existingRoomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err == nil {
		// Space already exists - idempotent success
		s.logger.Info("Space already exists (idempotent)",
			"alkemio_context_id", alkemioContextID,
			"existing_room_id", existingRoomID)
		return nil
	}

	// If error is not "not found", return it
	if !domain.IsNotFoundError(err) {
		return fmt.Errorf("failed to check space alias: %w", err)
	}

	// Determine effective join rule (default to invite)
	effectiveJoinRule := joinRule
	if effectiveJoinRule == "" {
		effectiveJoinRule = "invite"
	}

	// Create the space
	spaceRoomID, err := s.matrix.CreateSpace(ctx, alkemioContextID, name, topic, avatarURL, effectiveJoinRule, initialMembers)
	if err != nil {
		return fmt.Errorf("failed to create space: %w", err)
	}

	// Set directory visibility if specified
	if isPublic != nil {
		if err := s.matrix.SetRoomDirectoryVisibility(ctx, spaceRoomID, *isPublic); err != nil {
			s.logger.Warn("Failed to set space directory visibility",
				"alkemio_context_id", alkemioContextID, "is_public", *isPublic, "error", err)
		}
	}

	// If parent context is specified, set up hierarchy
	if parentContextID != nil {
		parentAlias := s.idMapper.SpaceAlias(*parentContextID)
		parentRoomID, err := s.matrix.ResolveAlias(ctx, parentAlias)
		if err != nil {
			s.logger.Warn("Failed to resolve parent space, skipping hierarchy setup",
				"parent_context_id", parentContextID,
				"error", err)
		} else {
			// Add this space as a child of the parent
			if err := s.matrix.AddSpaceChild(ctx, parentRoomID, spaceRoomID, "", false); err != nil {
				s.logger.Warn("Failed to add space as child of parent",
					"parent_room_id", parentRoomID,
					"child_room_id", spaceRoomID,
					"error", err)
			}
			// Set parent relationship on this space
			if err := s.matrix.SetSpaceParent(ctx, spaceRoomID, parentRoomID); err != nil {
				s.logger.Warn("Failed to set space parent",
					"child_room_id", spaceRoomID,
					"parent_room_id", parentRoomID,
					"error", err)
			}
		}
	}

	s.logger.Info("Space created successfully", "alkemio_context_id", alkemioContextID, "room_id", spaceRoomID)
	return nil
}

// GetSpace retrieves space details including members and children.
func (s *SpaceService) GetSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
) (*domain.Space, error) {
	s.logger.Info("Getting space", "alkemio_context_id", alkemioContextID)

	// Build alias and resolve to Matrix room ID
	alias := s.idMapper.SpaceAlias(alkemioContextID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return nil, domain.NewSpaceNotFoundError(alkemioContextID.String())
	}

	// Get space details
	space, err := s.matrix.GetSpaceDetails(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get space details: %w", err)
	}
	space.AlkemioContextID = alkemioContextID

	// Get space members and map to Alkemio actor IDs
	members, err := s.matrix.GetSpaceMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get space members: %w", err)
	}

	space.MemberIDs = make([]uuid.UUID, 0, len(members))
	for _, memberID := range members {
		// Extract UUID from Matrix user ID (@uuid:domain)
		if actorUUID := s.idMapper.AlkemioActorID(memberID); actorUUID != uuid.Nil {
			space.MemberIDs = append(space.MemberIDs, actorUUID)
		}
	}

	// Get space children
	children, err := s.matrix.GetSpaceChildren(ctx, roomID)
	if err != nil {
		s.logger.Warn("Failed to get space children", "room_id", roomID, "error", err)
		children = []domain.SpaceChild{}
	}
	space.Children = children

	return space, nil
}

// UpdateSpace updates space metadata.
func (s *SpaceService) UpdateSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
	name, topic, avatarURL *string,
	joinRule *string,
	isPublic *bool,
) error {
	s.logger.Info("Updating space", "alkemio_context_id", alkemioContextID)

	// Resolve alias to get Matrix room ID
	alias := s.idMapper.SpaceAlias(alkemioContextID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		return domain.NewSpaceNotFoundError(alkemioContextID.String())
	}

	err = s.matrix.UpdateSpaceState(ctx, roomID, name, topic, avatarURL, joinRule)
	if err != nil {
		return fmt.Errorf("failed to update space: %w", err)
	}

	// Set directory visibility if specified
	if isPublic != nil {
		if err := s.matrix.SetRoomDirectoryVisibility(ctx, roomID, *isPublic); err != nil {
			s.logger.Warn("Failed to set space directory visibility",
				"alkemio_context_id", alkemioContextID, "is_public", *isPublic, "error", err)
		}
	}

	return nil
}

// DeleteSpace kicks all members, leaves the space, and removes the alias.
func (s *SpaceService) DeleteSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
	reason string,
) error {
	s.logger.Info("Deleting space", "alkemio_context_id", alkemioContextID, "reason", reason)

	// Resolve alias to get Matrix room ID
	alias := s.idMapper.SpaceAlias(alkemioContextID)
	roomID, err := s.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		// Space doesn't exist - idempotent success
		if domain.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to resolve space alias: %w", err)
	}

	// Get space members to kick
	members, err := s.matrix.GetSpaceMembers(ctx, roomID)
	if err != nil {
		s.logger.Warn("Failed to get space members for kick", "room_id", roomID, "error", err)
	} else {
		// Kick all members
		for _, memberID := range members {
			if err := s.matrix.KickFromSpace(ctx, roomID, memberID, reason); err != nil {
				s.logger.Warn("Failed to kick user from space", "user_id", memberID, "room_id", roomID, "error", err)
			}
		}
	}

	// Delete the alias
	if err := s.matrix.DeleteAlias(ctx, alias); err != nil {
		s.logger.Warn("Failed to delete space alias", "alias", alias, "error", err)
	}

	s.logger.Info("Space deleted successfully", "alkemio_context_id", alkemioContextID)
	return nil
}

// ListSpaces returns a paginated list of Alkemio context IDs.
func (s *SpaceService) ListSpaces(
	ctx context.Context,
	cursor string,
) ([]uuid.UUID, string, error) {
	s.logger.Info("Listing spaces", "cursor", cursor)

	// Get all joined rooms
	rooms, err := s.matrix.GetAllJoinedRooms(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list spaces: %w", err)
	}

	// Extract Alkemio context IDs from space aliases
	alkemioContextIDs := s.extractAlkemioContextIDs(ctx, rooms)

	return alkemioContextIDs, "", nil
}

// extractAlkemioContextIDs extracts Alkemio context UUIDs from Matrix spaces.
func (s *SpaceService) extractAlkemioContextIDs(ctx context.Context, rooms []id.RoomID) []uuid.UUID {
	alkemioContextIDs := make([]uuid.UUID, 0)

	for _, roomID := range rooms {
		space, err := s.matrix.GetSpaceDetails(ctx, roomID)
		if err != nil {
			continue
		}

		contextID := s.idMapper.AlkemioContextID(space.Alias)
		if contextID != uuid.Nil {
			alkemioContextIDs = append(alkemioContextIDs, contextID)
		}
	}

	return alkemioContextIDs
}

// ============================================================================
// Hierarchy Operations (communication.hierarchy.*)
// ============================================================================

// SetParent establishes parent-child relationship for a room or subspace.
func (s *SpaceService) SetParent(
	ctx context.Context,
	childID string,
	isSpace bool,
	parentContextID uuid.UUID,
	order string,
	suggested bool,
) error {
	s.logger.Info("Setting parent relationship",
		"child_id", childID,
		"is_space", isSpace,
		"parent_context_id", parentContextID)

	// Resolve parent space
	parentAlias := s.idMapper.SpaceAlias(parentContextID)
	parentRoomID, err := s.matrix.ResolveAlias(ctx, parentAlias)
	if err != nil {
		return domain.NewParentNotFoundError(parentContextID.String())
	}

	// Resolve child (either room or space)
	var childRoomID id.RoomID
	if isSpace {
		// Parse as UUID and resolve space alias
		childUUID, err := uuid.Parse(childID)
		if err != nil {
			return fmt.Errorf("invalid child space ID: %w", err)
		}
		childAlias := s.idMapper.SpaceAlias(childUUID)
		childRoomID, err = s.matrix.ResolveAlias(ctx, childAlias)
		if err != nil {
			return domain.NewChildNotFoundError(childID)
		}
	} else {
		// Parse as UUID and resolve room alias
		childUUID, err := uuid.Parse(childID)
		if err != nil {
			return fmt.Errorf("invalid child room ID: %w", err)
		}
		roomAlias := s.idMapper.RoomAlias(childUUID)
		childRoomID, err = s.matrix.ResolveAlias(ctx, roomAlias)
		if err != nil {
			return domain.NewChildNotFoundError(childID)
		}
	}

	// Add child to parent space
	if err := s.matrix.AddSpaceChild(ctx, parentRoomID, childRoomID, order, suggested); err != nil {
		return fmt.Errorf("failed to add child to space: %w", err)
	}

	// Set parent on child
	if err := s.matrix.SetSpaceParent(ctx, childRoomID, parentRoomID); err != nil {
		return fmt.Errorf("failed to set parent on child: %w", err)
	}

	s.logger.Info("Parent relationship set successfully",
		"child_room_id", childRoomID,
		"parent_room_id", parentRoomID)
	return nil
}

// ============================================================================
// Batch Membership Operations (communication.space.member.batch.*)
// ============================================================================

// BatchAddMember adds an actor to multiple spaces.
func (s *SpaceService) BatchAddMember(
	ctx context.Context,
	actorID uuid.UUID,
	contextIDs []uuid.UUID,
) map[string]error {
	results := make(map[string]error)
	actor := domain.NewActor(actorID)

	for _, contextID := range contextIDs {
		alias := s.idMapper.SpaceAlias(contextID)
		spaceRoomID, err := s.matrix.ResolveAlias(ctx, alias)
		if err != nil {
			results[contextID.String()] = domain.ErrSpaceNotFound
			continue
		}

		err = s.matrix.InviteToSpace(ctx, spaceRoomID, actor)
		results[contextID.String()] = err
	}

	return results
}

// BatchRemoveMember removes an actor from multiple spaces.
func (s *SpaceService) BatchRemoveMember(
	ctx context.Context,
	actorID uuid.UUID,
	contextIDs []uuid.UUID,
	reason string,
) map[string]error {
	results := make(map[string]error)
	userMatrixID := s.idMapper.UserID(actorID)

	for _, contextID := range contextIDs {
		alias := s.idMapper.SpaceAlias(contextID)
		spaceRoomID, err := s.matrix.ResolveAlias(ctx, alias)
		if err != nil {
			results[contextID.String()] = domain.ErrSpaceNotFound
			continue
		}

		err = s.matrix.KickFromSpace(ctx, spaceRoomID, userMatrixID, reason)
		results[contextID.String()] = err
	}

	return results
}
