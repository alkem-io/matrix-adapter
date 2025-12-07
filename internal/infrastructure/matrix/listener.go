// Package matrix provides the Matrix adapter implementation using mautrix-go.
package matrix

import (
	"context"
	"fmt"
	"strings"
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
		go func() {
			for evt := range m.as.Events {
				m.processEvent(evt)
			}
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
		return
	}

	// Parse Sender UUID
	senderUUID, err := m.parseActorID(evt.Sender)
	if err != nil {
		m.logger.Warn("Ignoring message from invalid user", "sender", evt.Sender, "error", err)
		return
	}

	go func(e *event.Event, c string, s uuid.UUID) {
		var roomName string

		if err := m.eventHandlers.OnMessage(
			domain.Message{
				ID:        e.ID.String(),
				RoomID:    e.RoomID.String(),
				RoomName:  roomName,
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

	// Parse sender UUID - skip if not a ghost user
	senderUUID, err := m.parseActorID(evt.Sender)
	if err != nil {
		m.logger.Debug("Ignoring reaction from non-ghost user", "sender", evt.Sender)
		return
	}

	// Parse reaction content using generic helper
	content, ok := parseEventContent[event.ReactionEventContent](evt)
	if !ok {
		m.logger.Warn("Failed to parse reaction content")
		return
	}

	// Get room alias to extract Alkemio room ID
	alkemioRoomID := m.resolveAlkemioRoomID(context.Background(), evt.RoomID)
	if alkemioRoomID == uuid.Nil {
		m.logger.Warn("Could not resolve Alkemio room ID for reaction", "room_id", evt.RoomID)
		return
	}

	go func(e *event.Event, c *event.ReactionEventContent, s uuid.UUID, roomID uuid.UUID) {
		if err := m.eventHandlers.OnReactionAdded(domain.ReactionEvent{
			AlkemioRoomID: roomID,
			MessageID:     c.RelatesTo.EventID,
			ReactionID:    e.ID,
			Emoji:         c.RelatesTo.Key,
			SenderActorID: s,
			Timestamp:     time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling reaction", "error", err)
		}
	}(evt, content, senderUUID, alkemioRoomID)
}

func (m *MautrixAdapter) handleRedactionEvent(evt *event.Event) {
	if m.eventHandlers.OnReactionRemoved == nil {
		return
	}

	// Parse sender UUID - skip if not a ghost user
	senderUUID, err := m.parseActorID(evt.Sender)
	if err != nil {
		m.logger.Debug("Ignoring redaction from non-ghost user", "sender", evt.Sender)
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
		m.logger.Debug("Redaction event missing redacted event ID")
		return
	}

	// Try to look up the original reaction to get emoji and target message
	// If we can't find it, we still publish with empty emoji
	ctx := context.Background()
	var emoji string
	var messageID id.EventID

	// Try to get the original reaction event
	originalEvt, err := m.as.BotIntent().GetEvent(ctx, evt.RoomID, redactedEventID)
	if err == nil && originalEvt.Type == event.EventReaction {
		// Parse reaction content using generic helper
		if content, ok := parseEventContent[event.ReactionEventContent](originalEvt); ok {
			emoji = content.RelatesTo.Key
			messageID = content.RelatesTo.EventID
		}
	} else {
		// Could not verify this was a reaction redaction - skip
		// (We only want to publish reaction removed events for actual reactions)
		m.logger.Debug("Redaction is not for a reaction event, ignoring", "redacted_id", redactedEventID)
		return
	}

	// Get Alkemio room ID
	alkemioRoomID := m.resolveAlkemioRoomID(ctx, evt.RoomID)
	if alkemioRoomID == uuid.Nil {
		m.logger.Warn("Could not resolve Alkemio room ID for redaction", "room_id", evt.RoomID)
		return
	}

	go func(e *event.Event, s uuid.UUID, roomID uuid.UUID, emoji string, msgID id.EventID, reactionID id.EventID) {
		if err := m.eventHandlers.OnReactionRemoved(domain.ReactionRemovedEvent{
			AlkemioRoomID: roomID,
			MessageID:     msgID,
			ReactionID:    reactionID,
			Emoji:         emoji,
			SenderActorID: s,
			Timestamp:     time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling reaction removal", "error", err)
		}
	}(evt, senderUUID, alkemioRoomID, emoji, messageID, redactedEventID)
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

	// Parse target user UUID - skip if not a ghost user
	targetUUID, err := m.parseActorID(targetUserID)
	if err != nil {
		m.logger.Debug("Ignoring membership event for non-ghost user", "target", targetUserID)
		return
	}

	// Get Alkemio room ID
	alkemioRoomID := m.resolveAlkemioRoomID(context.Background(), evt.RoomID)
	if alkemioRoomID == uuid.Nil {
		m.logger.Warn("Could not resolve Alkemio room ID for membership event", "room_id", evt.RoomID)
		return
	}

	// Extract reason if present
	reason := ""
	if content.Reason != "" {
		reason = content.Reason
	}

	go func(e *event.Event, actorID uuid.UUID, roomID uuid.UUID, reason string) {
		if err := m.eventHandlers.OnMemberLeft(domain.MembershipEvent{
			AlkemioRoomID: roomID,
			ActorID:       actorID,
			Reason:        reason,
			Timestamp:     time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling membership event", "error", err)
		}
	}(evt, targetUUID, alkemioRoomID, reason)
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

func (m *MautrixAdapter) parseActorID(mxid id.UserID) (uuid.UUID, error) {
	s := string(mxid)
	// Format: @uuid:domain
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return uuid.Nil, fmt.Errorf("invalid format")
	}
	localpart := strings.TrimPrefix(parts[0], "@")
	return uuid.Parse(localpart)
}
