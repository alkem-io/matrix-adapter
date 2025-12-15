// Package service provides domain services for the Matrix Adapter.
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// ReadReceiptService handles read receipt operations.
type ReadReceiptService struct {
	matrix ports.MatrixPort
	logger ports.Logger
}

// NewReadReceiptService creates a new instance of ReadReceiptService.
func NewReadReceiptService(matrix ports.MatrixPort, logger ports.Logger) *ReadReceiptService {
	return &ReadReceiptService{
		matrix: matrix,
		logger: logger,
	}
}

// MarkMessageRead marks a message as read for a user.
// If threadRootID is provided, it marks a thread-level receipt.
func (s *ReadReceiptService) MarkMessageRead(
	ctx context.Context,
	actorID uuid.UUID,
	roomID id.RoomID,
	eventID id.EventID,
	threadRootID *id.EventID,
) error {
	s.logger.Debug("Marking message as read",
		"actor_id", actorID,
		"room_id", roomID,
		"event_id", eventID,
		"thread_root_id", threadRootID,
	)

	// Create actor for the Matrix port
	actor := domain.Actor{ID: actorID}

	// Send the read receipt via Matrix adapter
	err := s.matrix.SendReadReceipt(ctx, actor, roomID, eventID, threadRootID)
	if err != nil {
		return fmt.Errorf("failed to send read receipt: %w", err)
	}

	s.logger.Debug("Message marked as read successfully",
		"actor_id", actorID,
		"room_id", roomID,
		"event_id", eventID,
	)

	return nil
}

// GetUnreadCounts retrieves unread message counts for a user in a room.
// If threadRootIDs is provided, it returns counts for those specific threads.
func (s *ReadReceiptService) GetUnreadCounts(
	ctx context.Context,
	actorID uuid.UUID,
	roomID id.RoomID,
	threadRootIDs []id.EventID,
) (*domain.UnreadCountSummary, error) {
	s.logger.Debug("Getting unread counts",
		"actor_id", actorID,
		"room_id", roomID,
		"thread_count", len(threadRootIDs),
	)

	// Create actor for the Matrix port
	actor := domain.Actor{ID: actorID}

	// Get unread counts from Matrix adapter
	summary, err := s.matrix.GetUnreadCounts(ctx, actor, roomID, threadRootIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get unread counts: %w", err)
	}

	s.logger.Debug("Got unread counts",
		"actor_id", actorID,
		"room_id", roomID,
		"room_count", summary.RoomUnreadCount,
		"thread_counts", len(summary.ThreadUnreadCounts),
	)

	return summary, nil
}
