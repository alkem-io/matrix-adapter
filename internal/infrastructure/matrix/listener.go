// Package matrix provides the Matrix adapter implementation using mautrix-go.
package matrix

import (
	"context"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
)

// EventHandlers holds all the event handler callbacks.
type EventHandlers struct {
	OnMessage         func(msg domain.Message) error
	OnReactionAdded   func(reaction domain.ReactionEvent) error
	OnReactionRemoved func(reaction domain.ReactionRemovedEvent) error
	OnMemberLeft      func(membership domain.MembershipEvent) error
}

// SetEventHandlers sets all event handlers at once.
func (m *MautrixAdapter) SetEventHandlers(handlers EventHandlers) {
	m.eventHandlers = handlers
	m.startEventLoop()
}

// startEventLoop starts the event processing loop if not already started.
func (m *MautrixAdapter) startEventLoop() {
	m.eventLoopOnce.Do(func() {
		m.logger.Info("Starting Matrix event loop")
		go func() {
			for evt := range m.as.Events {
				m.processEvent(evt)
			}
			m.logger.Warn("Matrix event channel closed, event loop exiting")
		}()
	})
}

func (m *MautrixAdapter) processEvent(evt *event.Event) {
	// Ignore own events
	if evt.Sender == m.as.BotMXID() {
		return
	}

	switch evt.Type {
	case event.EventMessage:
		m.handleMessageEvent(evt)
	case event.EventReaction:
		m.handleReactionEvent(evt)
	case event.EventRedaction:
		m.handleRedactionEvent(evt)
	case event.StateMember:
		m.handleMembershipEvent(evt)
	}
}

func (m *MautrixAdapter) handleMessageEvent(evt *event.Event) {
	if m.eventHandlers.OnMessage == nil {
		return
	}

	// Parse content
	content, ok := evt.Content.Raw["body"].(string)
	if !ok {
		m.logger.Warn("Failed to parse message body", "event_id", evt.ID)
		return
	}

	// Parse Sender UUID using IDMapper
	senderUUID := m.idMapper.AlkemioActorID(evt.Sender)
	if senderUUID == uuid.Nil {
		m.logger.Debug("Ignoring message from non-UUID user", "sender", evt.Sender)
		return
	}

	go func(e *event.Event, c string, s uuid.UUID) {
		// Resolve Alkemio room ID from Matrix room ID (HTTP call - must be async)
		alkemioRoomID := m.resolveAlkemioRoomID(context.Background(), e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for message", "room_id", e.RoomID)
			return
		}

		if err := m.eventHandlers.OnMessage(
			domain.Message{
				ID:        e.ID.String(),
				RoomID:    alkemioRoomID.String(),
				SenderID:  s,
				Content:   c,
				Timestamp: time.UnixMilli(e.Timestamp),
			},
		); err != nil {
			m.logger.Error("Error handling message", "error", err)
		}
	}(evt, content, senderUUID)
}

func (m *MautrixAdapter) handleReactionEvent(evt *event.Event) {
	if m.eventHandlers.OnReactionAdded == nil {
		return
	}

	// Parse sender UUID using IDMapper - skip if not a ghost user
	senderUUID := m.idMapper.AlkemioActorID(evt.Sender)
	if senderUUID == uuid.Nil {
		m.logger.Debug("Ignoring reaction from non-UUID user", "sender", evt.Sender)
		return
	}

	// Parse reaction content using generic helper
	content, ok := parseEventContent[event.ReactionEventContent](evt)
	if !ok {
		m.logger.Warn("Failed to parse reaction content", "event_id", evt.ID)
		return
	}

	// Move all blocking calls inside the goroutine to avoid blocking the event loop
	go func(e *event.Event, c *event.ReactionEventContent, s uuid.UUID) {
		// Get room alias to extract Alkemio room ID (HTTP call - must be async)
		alkemioRoomID := m.resolveAlkemioRoomID(context.Background(), e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for reaction", "room_id", e.RoomID)
			return
		}

		if err := m.eventHandlers.OnReactionAdded(domain.ReactionEvent{
			AlkemioRoomID: alkemioRoomID,
			MessageID:     c.RelatesTo.EventID,
			ReactionID:    e.ID,
			Emoji:         c.RelatesTo.Key,
			SenderActorID: s,
			Timestamp:     time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling reaction", "error", err)
		}
	}(evt, content, senderUUID)
}

func (m *MautrixAdapter) handleRedactionEvent(evt *event.Event) {
	if m.eventHandlers.OnReactionRemoved == nil {
		return
	}

	// Parse sender UUID using IDMapper - skip if not a ghost user
	senderUUID := m.idMapper.AlkemioActorID(evt.Sender)
	if senderUUID == uuid.Nil {
		m.logger.Debug("Ignoring redaction from non-UUID user", "sender", evt.Sender)
		return
	}

	// Get the redacted event ID from the redaction
	redactedEventID := evt.Redacts
	if redactedEventID == "" {
		// Try to get from content
		if redacts, ok := evt.Content.Raw["redacts"].(string); ok {
			redactedEventID = id.EventID(redacts)
		}
	}

	if redactedEventID == "" {
		return
	}

	// Move all blocking HTTP calls inside the goroutine to avoid blocking the event loop
	go func(e *event.Event, s uuid.UUID, redactedID id.EventID) {
		ctx := context.Background()
		var emoji string
		var messageID id.EventID

		// Try to get the original reaction event (HTTP call - must be async)
		originalEvt, err := m.as.BotIntent().GetEvent(ctx, e.RoomID, redactedID)
		if err == nil && originalEvt.Type == event.EventReaction {
			// Parse reaction content using generic helper
			if content, ok := parseEventContent[event.ReactionEventContent](originalEvt); ok {
				emoji = content.RelatesTo.Key
				messageID = content.RelatesTo.EventID
			}
		} else {
			// Could not verify this was a reaction redaction - skip
			return
		}

		// Get Alkemio room ID (HTTP call - must be async)
		alkemioRoomID := m.resolveAlkemioRoomID(ctx, e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for redaction", "room_id", e.RoomID)
			return
		}

		if err := m.eventHandlers.OnReactionRemoved(domain.ReactionRemovedEvent{
			AlkemioRoomID: alkemioRoomID,
			MessageID:     messageID,
			ReactionID:    redactedID,
			Emoji:         emoji,
			SenderActorID: s,
			Timestamp:     time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling reaction removal", "error", err)
		}
	}(evt, senderUUID, redactedEventID)
}

func (m *MautrixAdapter) handleMembershipEvent(evt *event.Event) {
	if m.eventHandlers.OnMemberLeft == nil {
		return
	}

	// Parse membership content using generic helper
	content, ok := parseEventContent[event.MemberEventContent](evt)
	if !ok {
		return
	}

	// Only handle leave and ban events
	if content.Membership != event.MembershipLeave && content.Membership != event.MembershipBan {
		return
	}

	// The state_key is the user who left/was banned
	if evt.StateKey == nil || *evt.StateKey == "" {
		return
	}
	targetUserID := id.UserID(*evt.StateKey)

	// Skip if target is the bot itself
	if targetUserID == m.as.BotMXID() {
		return
	}

	// Parse target user UUID using IDMapper - skip if not a ghost user
	targetUUID := m.idMapper.AlkemioActorID(targetUserID)
	if targetUUID == uuid.Nil {
		m.logger.Debug("Ignoring membership event for non-UUID user", "target", targetUserID)
		return
	}

	// Extract reason if present
	reason := ""
	if content.Reason != "" {
		reason = content.Reason
	}

	// Move HTTP call inside goroutine to avoid blocking the event loop
	go func(e *event.Event, actorID uuid.UUID, reason string) {
		// Get Alkemio room ID (HTTP call - must be async)
		alkemioRoomID := m.resolveAlkemioRoomID(context.Background(), e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for membership event", "room_id", e.RoomID)
			return
		}

		if err := m.eventHandlers.OnMemberLeft(domain.MembershipEvent{
			AlkemioRoomID: alkemioRoomID,
			ActorID:       actorID,
			Reason:        reason,
			Timestamp:     time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling membership event", "error", err)
		}
	}(evt, targetUUID, reason)
}

// resolveAlkemioRoomID gets the Alkemio room UUID from a Matrix room ID.
func (m *MautrixAdapter) resolveAlkemioRoomID(ctx context.Context, roomID id.RoomID) uuid.UUID {
	// Try to get room details to find the alias
	details, err := m.GetRoomDetails(ctx, roomID)
	if err != nil || details.Alias == "" {
		return uuid.Nil
	}

	// Use IDMapper to extract UUID from alias
	return m.idMapper.AlkemioRoomID(details.Alias)
}
