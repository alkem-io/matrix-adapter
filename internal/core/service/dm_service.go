// Package service provides domain services for the Matrix Adapter.
package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// DMService handles DM request events from Synapse and publishes them to the queue.
//
// Deprecated: Replaced by RoomCheckService and the synchronous check-room flow.
// Kept during server-side transition; remove once the server no longer sends DM webhooks.
type DMService struct {
	queue  ports.QueuePort
	logger ports.Logger
}

// NewDMService creates a new instance of DMService.
func NewDMService(queue ports.QueuePort, logger ports.Logger) *DMService {
	return &DMService{
		queue:  queue,
		logger: logger,
	}
}

// PublishDMRequest publishes a DM requested event to the queue.
// It converts the Matrix user IDs to Alkemio actor IDs and publishes the event.
func (s *DMService) PublishDMRequest(initiatorActorID, targetActorID uuid.UUID) error {
	if initiatorActorID == uuid.Nil {
		return fmt.Errorf("initiator actor ID is required")
	}
	if targetActorID == uuid.Nil {
		return fmt.Errorf("target actor ID is required")
	}
	if initiatorActorID == targetActorID {
		return fmt.Errorf("initiator and target cannot be the same user")
	}

	event := dto.DMRequestedEvent{ //nolint:staticcheck // deprecated but kept during transition
		InitiatorActorID: initiatorActorID.String(),
		TargetActorID:    targetActorID.String(),
		Timestamp:        time.Now().UnixMilli(),
	}

	s.logger.Info("Publishing DM requested event",
		"initiator", initiatorActorID.String(),
		"target", targetActorID.String(),
	)

	if err := s.queue.Publish(dto.TopicRoomDMRequested, event); err != nil {
		return fmt.Errorf("failed to publish DM requested event: %w", err)
	}

	return nil
}
