// Package service provides domain services for the Matrix Adapter.
package service

import (
	"fmt"

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
	payload := dto.MessageReceivedPayload{
		RoomID:   msg.RoomID,
		RoomName: msg.RoomName,
		ActorID:  msg.SenderID.String(),
		Message: dto.Message{
			ID:        msg.ID,
			Message:   msg.Content,
			Sender:    msg.SenderID.String(),
			Timestamp: msg.Timestamp.UnixMilli(),
		},
	}

	s.logger.Debug("Publishing message received event", "event_id", msg.ID, "sender_id", msg.SenderID)

	if err := s.queue.Publish(dto.TopicMessageReceived, payload); err != nil {
		return fmt.Errorf("failed to publish message received event: %w", err)
	}
	s.logger.Debug("Event published successfully", "event_id", msg.ID, "sender_id", msg.SenderID)
	return nil
}

// HandleReactionAdded processes a reaction added event and publishes it to the queue.
func (s *EventService) HandleReactionAdded(evt domain.ReactionEvent) error {
	payload := dto.ReactionAddedEvent{
		AlkemioRoomID: dto.AlkemioRoomID(evt.AlkemioRoomID),
		MessageID:     dto.MessageID(evt.MessageID.String()),
		ReactionID:    dto.ReactionID(evt.ReactionID.String()),
		Emoji:         evt.Emoji,
		SenderActorID: dto.AlkemioActorID(evt.SenderActorID),
		Timestamp:     evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing reaction added event",
		"room_id", evt.AlkemioRoomID,
		"message_id", evt.MessageID,
		"emoji", evt.Emoji,
		"sender", evt.SenderActorID,
	)

	if err := s.queue.Publish(dto.TopicReactionAdded, payload); err != nil {
		return fmt.Errorf("failed to publish reaction added event: %w", err)
	}

	return nil
}

// HandleReactionRemoved processes a reaction removed event and publishes it to the queue.
func (s *EventService) HandleReactionRemoved(evt domain.ReactionRemovedEvent) error {
	payload := dto.ReactionRemovedEvent{
		AlkemioRoomID: dto.AlkemioRoomID(evt.AlkemioRoomID),
		MessageID:     dto.MessageID(evt.MessageID.String()),
		ReactionID:    dto.ReactionID(evt.ReactionID.String()),
		Emoji:         evt.Emoji,
		SenderActorID: dto.AlkemioActorID(evt.SenderActorID),
		Timestamp:     evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing reaction removed event",
		"room_id", evt.AlkemioRoomID,
		"message_id", evt.MessageID,
		"emoji", evt.Emoji,
		"sender", evt.SenderActorID,
	)

	if err := s.queue.Publish(dto.TopicReactionRemoved, payload); err != nil {
		return fmt.Errorf("failed to publish reaction removed event: %w", err)
	}

	return nil
}

// HandleMemberLeft processes a member left event and publishes it to the queue.
func (s *EventService) HandleMemberLeft(evt domain.MembershipEvent) error {
	payload := dto.RoomMemberLeftEvent{
		AlkemioRoomID: dto.AlkemioRoomID(evt.AlkemioRoomID),
		ActorID:       dto.AlkemioActorID(evt.ActorID),
		Reason:        evt.Reason,
		Timestamp:     evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing room member left event",
		"room_id", evt.AlkemioRoomID,
		"actor_id", evt.ActorID,
		"reason", evt.Reason,
	)

	if err := s.queue.Publish(dto.TopicRoomMemberLeft, payload); err != nil {
		return fmt.Errorf("failed to publish room member left event: %w", err)
	}

	return nil
}
