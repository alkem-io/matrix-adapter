package service

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter-go/internal/config"
	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// EventService handles incoming Matrix events and publishes them to the queue.
type EventService struct {
	queue  ports.QueuePort
	logger ports.Logger
	cfg    *config.Config
}

// NewEventService creates a new instance of EventService.
func NewEventService(queue ports.QueuePort, logger ports.Logger, cfg *config.Config) *EventService {
	return &EventService{
		queue:  queue,
		logger: logger,
		cfg:    cfg,
	}
}

// HandleMessage processes a message event from Matrix and publishes it to the queue.
func (s *EventService) HandleMessage(msg domain.Message) error {
	// Filter out messages from non-UUID users (e.g. bots, bridges)
	// The domain.Message already has SenderID as UUID, so if it was parsed successfully, it's valid.
	// However, we might want to double check or handle parsing errors upstream.
	// For now, we assume domain.Message is valid.

	payload := dto.MessageReceivedPayload{
		RoomID:   msg.RoomID,
		RoomName: msg.RoomName,
		ActorID:  s.cfg.Matrix.BotActorID,
		Message: dto.Message{
			ID:        msg.ID,
			Message:   msg.Content,
			Sender:    msg.SenderID.String(),
			Timestamp: msg.Timestamp.UnixMilli(),
		},
	}

	s.logger.Info("Publishing message received event", "event_id", msg.ID, "sender_id", msg.SenderID)

	if err := s.queue.Publish("message.received", payload); err != nil {
		return fmt.Errorf("failed to publish message received event: %w", err)
	}

	return nil
}

// ParseActorIDFromMatrixID extracts the UUID from a Matrix ID like @uuid:domain
func (s *EventService) ParseActorIDFromMatrixID(matrixID string) (uuid.UUID, error) {
	// Remove @ and :domain
	parts := strings.Split(matrixID, ":")
	if len(parts) != 2 {
		return uuid.Nil, fmt.Errorf("invalid matrix ID format")
	}
	localpart := strings.TrimPrefix(parts[0], "@")
	return uuid.Parse(localpart)
}
