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
	// Convert thread ID to pointer if present
	var threadID *string
	if msg.ThreadID != "" {
		threadID = &msg.ThreadID
	}

	payload := dto.MessageReceivedPayload{
		RoomID:   msg.RoomID,
		RoomName: msg.RoomName,
		ActorID:  msg.SenderID.String(),
		Message: dto.Message{
			ID:        msg.ID,
			Message:   msg.Content,
			ThreadID:  threadID,
			Sender:    msg.SenderID.String(),
			Timestamp: msg.Timestamp.UnixMilli(),
		},
	}

	s.logger.Debug(
		"Publishing message received event", "event_id", msg.ID, "sender_id", msg.SenderID, "thread_id", threadID,
	)

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

// HandleReadReceiptUpdated processes a read receipt update event and publishes it to the queue.
func (s *EventService) HandleReadReceiptUpdated(evt domain.ReadReceiptEvent) error {
	var threadID *dto.MessageID
	if evt.ThreadID != nil {
		tid := dto.MessageID(evt.ThreadID.String())
		threadID = &tid
	}

	payload := dto.ReadReceiptUpdatedEvent{
		AlkemioRoomID: dto.AlkemioRoomID(evt.AlkemioRoomID),
		ActorID:       dto.AlkemioActorID(evt.UserID),
		EventID:       dto.MessageID(evt.EventID.String()),
		ThreadID:      threadID,
		Timestamp:     evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing read receipt updated event",
		"alkemio_room_id", evt.AlkemioRoomID,
		"actor_id", evt.UserID,
		"event_id", evt.EventID,
		"thread_id", threadID,
	)

	if err := s.queue.Publish(dto.TopicReadReceiptUpdated, payload); err != nil {
		return fmt.Errorf("failed to publish read receipt updated event: %w", err)
	}

	return nil
}

// HandleMessageEdited processes a message edited event and publishes it to the queue.
func (s *EventService) HandleMessageEdited(evt domain.MessageEditedEvent) error {
	var threadID *dto.MessageID
	if evt.ThreadID != nil {
		tid := dto.MessageID(evt.ThreadID.String())
		threadID = &tid
	}

	payload := dto.MessageEditedEvent{
		AlkemioRoomID:     dto.AlkemioRoomID(evt.AlkemioRoomID),
		SenderActorID:     dto.AlkemioActorID(evt.SenderID),
		OriginalMessageID: dto.MessageID(evt.OriginalEventID.String()),
		NewMessageID:      dto.MessageID(evt.NewEventID.String()),
		NewContent:        evt.NewContent,
		ThreadID:          threadID,
		Timestamp:         evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing message edited event",
		"alkemio_room_id", evt.AlkemioRoomID,
		"original_message_id", evt.OriginalEventID,
		"sender_actor_id", evt.SenderID,
	)

	if err := s.queue.Publish(dto.TopicMessageEdited, payload); err != nil {
		return fmt.Errorf("failed to publish message edited event: %w", err)
	}

	return nil
}

// HandleMessageRedacted processes a message redacted event and publishes it to the queue.
func (s *EventService) HandleMessageRedacted(evt domain.MessageRedactedEvent) error {
	var threadID *dto.MessageID
	if evt.ThreadID != nil {
		tid := dto.MessageID(evt.ThreadID.String())
		threadID = &tid
	}

	payload := dto.MessageRedactedEvent{
		AlkemioRoomID:      dto.AlkemioRoomID(evt.AlkemioRoomID),
		RedactorActorID:    dto.AlkemioActorID(evt.RedactorID),
		RedactedMessageID:  dto.MessageID(evt.RedactedEventID.String()),
		RedactionMessageID: dto.MessageID(evt.RedactionEventID.String()),
		Reason:             evt.Reason,
		ThreadID:           threadID,
		Timestamp:          evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing message redacted event",
		"alkemio_room_id", evt.AlkemioRoomID,
		"redacted_message_id", evt.RedactedEventID,
		"redactor_actor_id", evt.RedactorID,
	)

	if err := s.queue.Publish(dto.TopicMessageRedacted, payload); err != nil {
		return fmt.Errorf("failed to publish message redacted event: %w", err)
	}

	return nil
}

// HandleRoomCreated processes a room created event and publishes it to the queue.
func (s *EventService) HandleRoomCreated(evt domain.RoomCreatedEvent) error {
	payload := dto.RoomCreatedEvent{
		AlkemioRoomID:  dto.AlkemioRoomID(evt.AlkemioRoomID),
		CreatorActorID: dto.AlkemioActorID(evt.CreatorID),
		RoomType:       evt.RoomType,
		Name:           evt.Name,
		Topic:          evt.Topic,
		Timestamp:      evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing room created event",
		"alkemio_room_id", evt.AlkemioRoomID,
		"creator_actor_id", evt.CreatorID,
		"room_type", evt.RoomType,
	)

	if err := s.queue.Publish(dto.TopicRoomCreated, payload); err != nil {
		return fmt.Errorf("failed to publish room created event: %w", err)
	}

	return nil
}

// HandleMemberUpdated processes a membership updated event and publishes it to the queue.
func (s *EventService) HandleMemberUpdated(evt domain.RoomMemberUpdatedEvent) error {
	payload := dto.RoomMemberUpdatedEvent{
		AlkemioRoomID: dto.AlkemioRoomID(evt.AlkemioRoomID),
		MemberActorID: dto.AlkemioActorID(evt.MemberID),
		SenderActorID: dto.AlkemioActorID(evt.SenderID),
		Membership:    evt.Membership,
		Timestamp:     evt.Timestamp.UnixMilli(),
	}

	s.logger.Debug(
		"Publishing room member updated event",
		"alkemio_room_id", evt.AlkemioRoomID,
		"member_actor_id", evt.MemberID,
		"membership", evt.Membership,
	)

	if err := s.queue.Publish(dto.TopicRoomMemberUpdated, payload); err != nil {
		return fmt.Errorf("failed to publish room member updated event: %w", err)
	}

	return nil
}
