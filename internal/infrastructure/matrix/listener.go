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
	// Read receipt and message event handlers (008-read-receipts)
	OnReadReceiptUpdated func(receipt domain.ReadReceiptEvent) error
	OnMessageEdited      func(edit domain.MessageEditedEvent) error
	OnMessageRedacted    func(redaction domain.MessageRedactedEvent) error
	OnRoomCreated        func(room domain.RoomCreatedEvent) error
	OnMemberUpdated      func(membership domain.RoomMemberUpdatedEvent) error
	OnRoomUpdated        func(evt domain.RoomUpdatedEvent) error
	OnSpaceUpdated       func(evt domain.SpaceUpdatedEvent) error
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
	// State events are always processed, even from the bot itself,
	// because the server needs to know about state changes it triggered
	// (e.g. room property updates, member kicks via batch remove).
	switch evt.Type {
	case event.StateRoomName, event.StateRoomAvatar, event.StateTopic:
		m.handleRoomStateEvent(evt)
		return
	case event.StateMember:
		m.handleMembershipEvent(evt)
		return
	}

	// Ignore own events for messages, reactions, and other interactive events
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
	case event.EphemeralEventReceipt:
		m.handleReceiptEvent(evt)
	case event.StateCreate:
		m.handleRoomCreateEvent(evt)
	}
}

func (m *MautrixAdapter) handleMessageEvent(evt *event.Event) {
	// Check if this is a message edit (m.replace relation)
	if m.isMessageEdit(evt) {
		m.handleMessageEditEvent(evt)
		return
	}

	if m.eventHandlers.OnMessage == nil {
		return
	}

	// Parse content
	content, ok := evt.Content.Raw["body"].(string)
	if !ok {
		m.logger.Warn("Failed to parse message body", "event_id", evt.ID)
		return
	}

	// Parse sender UUID from Matrix user ID
	senderUUID := m.resolveActorID(context.Background(), evt.Sender)
	if senderUUID == uuid.Nil {
		m.logger.Debug("Ignoring message from non-UUID user", "sender", evt.Sender)
		return
	}

	// Extract thread ID from m.relates_to if present
	var threadID string
	if relatesTo, ok := evt.Content.Raw["m.relates_to"].(map[string]interface{}); ok {
		// Check for thread relation (MSC3440)
		if relType, ok := relatesTo["rel_type"].(string); ok && relType == "m.thread" {
			if eventID, ok := relatesTo["event_id"].(string); ok {
				threadID = eventID
			}
		}
		// Also check m.in_reply_to for legacy thread support
		if threadID == "" {
			if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
				if eventID, ok := inReplyTo["event_id"].(string); ok {
					threadID = eventID
				}
			}
		}
	}

	go func(e *event.Event, c string, s uuid.UUID, tid string) {
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
				ThreadID:  tid,
			},
		); err != nil {
			m.logger.Error("Error handling message", "error", err)
		}
	}(evt, content, senderUUID, threadID)
}

// isMessageEdit checks if an event is a message edit (m.replace relation).
func (m *MautrixAdapter) isMessageEdit(evt *event.Event) bool {
	relatesTo, ok := evt.Content.Raw["m.relates_to"].(map[string]interface{})
	if !ok {
		return false
	}
	relType, ok := relatesTo["rel_type"].(string)
	return ok && relType == "m.replace"
}

// handleMessageEditEvent handles a message edit event (m.replace).
func (m *MautrixAdapter) handleMessageEditEvent(evt *event.Event) {
	if m.eventHandlers.OnMessageEdited == nil {
		return
	}

	// Parse sender UUID from Matrix user ID
	senderUUID := m.resolveActorID(context.Background(), evt.Sender)
	if senderUUID == uuid.Nil {
		m.logger.Debug("Ignoring edit from non-UUID user", "sender", evt.Sender)
		return
	}

	// Extract original event ID from m.relates_to
	relatesTo, _ := evt.Content.Raw["m.relates_to"].(map[string]interface{})
	originalEventID, _ := relatesTo["event_id"].(string)
	if originalEventID == "" {
		m.logger.Warn("Edit event missing original event ID", "event_id", evt.ID)
		return
	}

	// Extract new content from m.new_content
	newContent := ""
	if newContentMap, ok := evt.Content.Raw["m.new_content"].(map[string]interface{}); ok {
		newContent, _ = newContentMap["body"].(string)
	}
	if newContent == "" {
		// Fallback to main body
		newContent, _ = evt.Content.Raw["body"].(string)
	}

	// Extract thread ID if present
	// Check for explicit m.thread relation first (preferred), then fall back to m.in_reply_to
	var threadID *id.EventID
	if relatesTo != nil {
		// Check for explicit thread relation first (MSC3440)
		if relType, ok := relatesTo["rel_type"].(string); ok && relType == "m.thread" {
			if threadEventID, ok := relatesTo["event_id"].(string); ok {
				tid := id.EventID(threadEventID)
				threadID = &tid
			}
		}
		// Fallback to m.in_reply_to if no explicit thread relation
		if threadID == nil {
			if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
				if threadEventID, ok := inReplyTo["event_id"].(string); ok {
					tid := id.EventID(threadEventID)
					threadID = &tid
				}
			}
		}
	}

	go func(e *event.Event, sender uuid.UUID, origID, content string, tid *id.EventID) {
		ctx := context.Background()
		alkemioRoomID := m.resolveAlkemioRoomID(ctx, e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for edit", "room_id", e.RoomID)
			return
		}

		if err := m.eventHandlers.OnMessageEdited(domain.MessageEditedEvent{
			AlkemioRoomID:   alkemioRoomID,
			OriginalEventID: id.EventID(origID),
			NewEventID:      e.ID,
			SenderID:        sender,
			NewContent:      content,
			ThreadID:        tid,
			Timestamp:       time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling message edit", "error", err)
		}
	}(evt, senderUUID, originalEventID, newContent, threadID)
}

func (m *MautrixAdapter) handleReactionEvent(evt *event.Event) {
	if m.eventHandlers.OnReactionAdded == nil {
		return
	}

	// Parse sender UUID from Matrix user ID
	senderUUID := m.resolveActorID(context.Background(), evt.Sender)
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
	// Parse sender UUID from Matrix user ID
	senderUUID := m.resolveActorID(context.Background(), evt.Sender)
	if senderUUID == uuid.Nil {
		m.logger.Debug("Ignoring redaction from non-UUID user", "sender", evt.Sender)
		return
	}

	// Get the redacted event ID from the redaction
	redactedEventID := m.extractRedactedEventID(evt)
	if redactedEventID == "" {
		return
	}

	// Extract reason if present
	reason := m.extractRedactionReason(evt)

	// Move all blocking HTTP calls inside the goroutine to avoid blocking the event loop
	go func(e *event.Event, s uuid.UUID, redactedID id.EventID, reason string) {
		ctx := context.Background()

		// Get Alkemio room ID first (HTTP call)
		alkemioRoomID := m.resolveAlkemioRoomID(ctx, e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for redaction", "room_id", e.RoomID)
			return
		}

		// Try to get the original event to determine its type (HTTP call)
		originalEvt, err := m.as.BotIntent().GetEvent(ctx, e.RoomID, redactedID)
		if err != nil {
			// Event already redacted or not accessible - emit as message redaction
			m.emitMessageRedaction(e, s, redactedID, alkemioRoomID, reason, nil)
			return
		}

		m.processRedactedEvent(e, s, redactedID, alkemioRoomID, reason, originalEvt)
	}(evt, senderUUID, redactedEventID, reason)
}

// extractRedactedEventID extracts the redacted event ID from a redaction event.
func (m *MautrixAdapter) extractRedactedEventID(evt *event.Event) id.EventID {
	if evt.Redacts != "" {
		return evt.Redacts
	}
	// Try to get from content
	if redacts, ok := evt.Content.Raw["redacts"].(string); ok {
		return id.EventID(redacts)
	}
	return ""
}

// extractRedactionReason extracts the reason from a redaction event.
func (m *MautrixAdapter) extractRedactionReason(evt *event.Event) string {
	if r, ok := evt.Content.Raw["reason"].(string); ok {
		return r
	}
	return ""
}

// processRedactedEvent handles the redaction based on the original event type.
func (m *MautrixAdapter) processRedactedEvent(e *event.Event, s uuid.UUID, redactedID id.EventID, alkemioRoomID uuid.UUID, reason string, originalEvt *event.Event) {
	switch originalEvt.Type {
	case event.EventReaction:
		m.handleReactionRedaction(e, s, redactedID, alkemioRoomID, originalEvt)
	case event.EventMessage:
		threadID := m.extractThreadIDFromMessage(originalEvt)
		m.emitMessageRedaction(e, s, redactedID, alkemioRoomID, reason, threadID)
	default:
		// Unknown event type - emit as generic message redaction
		m.emitMessageRedaction(e, s, redactedID, alkemioRoomID, reason, nil)
	}
}

// handleReactionRedaction handles a redaction of a reaction event.
func (m *MautrixAdapter) handleReactionRedaction(e *event.Event, s uuid.UUID, redactedID id.EventID, alkemioRoomID uuid.UUID, originalEvt *event.Event) {
	if m.eventHandlers.OnReactionRemoved == nil {
		return
	}
	content, ok := parseEventContent[event.ReactionEventContent](originalEvt)
	if !ok {
		return
	}
	if err := m.eventHandlers.OnReactionRemoved(domain.ReactionRemovedEvent{
		AlkemioRoomID: alkemioRoomID,
		MessageID:     content.RelatesTo.EventID,
		ReactionID:    redactedID,
		Emoji:         content.RelatesTo.Key,
		SenderActorID: s,
		Timestamp:     time.UnixMilli(e.Timestamp),
	}); err != nil {
		m.logger.Error("Error handling reaction removal", "error", err)
	}
}

// extractThreadIDFromMessage extracts the thread ID from a message event if present.
// Checks for explicit m.thread relation first (MSC3440), then falls back to m.in_reply_to.
func (m *MautrixAdapter) extractThreadIDFromMessage(originalEvt *event.Event) *id.EventID {
	content, ok := originalEvt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return nil
	}
	if content.RelatesTo == nil {
		return nil
	}
	// Check for explicit thread relation first (MSC3440)
	if content.RelatesTo.Type == event.RelThread && content.RelatesTo.EventID != "" {
		eventID := content.RelatesTo.EventID
		return &eventID
	}
	// Fallback to m.in_reply_to
	if content.RelatesTo.InReplyTo != nil {
		return &content.RelatesTo.InReplyTo.EventID
	}
	return nil
}

// emitMessageRedaction emits a message redacted event.
func (m *MautrixAdapter) emitMessageRedaction(e *event.Event, redactor uuid.UUID, redactedID id.EventID, alkemioRoomID uuid.UUID, reason string, threadID *id.EventID) {
	if m.eventHandlers.OnMessageRedacted == nil {
		return
	}

	if err := m.eventHandlers.OnMessageRedacted(domain.MessageRedactedEvent{
		AlkemioRoomID:    alkemioRoomID,
		RedactedEventID:  redactedID,
		RedactionEventID: e.ID,
		RedactorID:       redactor,
		Reason:           reason,
		ThreadID:         threadID,
		Timestamp:        time.UnixMilli(e.Timestamp),
	}); err != nil {
		m.logger.Error("Error handling message redaction", "error", err)
	}
}

func (m *MautrixAdapter) handleMembershipEvent(evt *event.Event) {
	// Parse membership content using generic helper
	content, ok := parseEventContent[event.MemberEventContent](evt)
	if !ok {
		return
	}

	// The state_key is the user whose membership changed
	if evt.StateKey == nil || *evt.StateKey == "" {
		return
	}
	targetUserID := id.UserID(*evt.StateKey)

	// Skip if target is the bot itself
	if targetUserID == m.as.BotMXID() {
		return
	}

	// Parse target user UUID from Matrix user ID
	targetUUID := m.resolveActorID(context.Background(), targetUserID)
	if targetUUID == uuid.Nil {
		m.logger.Debug("Ignoring membership event for non-UUID user", "target", targetUserID)
		return
	}

	// Parse sender UUID (who performed the action)
	senderUUID := m.resolveActorID(context.Background(), evt.Sender)
	// Note: senderUUID may be Nil for system actions, that's OK

	// Extract reason if present
	reason := ""
	if content.Reason != "" {
		reason = content.Reason
	}

	membership := string(content.Membership)

	// Move HTTP call inside goroutine to avoid blocking the event loop
	go func(e *event.Event, memberID, senderID uuid.UUID, membershipState, _ string) {
		// Get Alkemio room ID (HTTP call - must be async)
		alkemioRoomID := m.resolveAlkemioRoomID(context.Background(), e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for membership event", "room_id", e.RoomID)
			return
		}

		// Emit OnMemberUpdated for all membership changes (join, invite, knock, leave, ban)
		if m.eventHandlers.OnMemberUpdated != nil {
			if err := m.eventHandlers.OnMemberUpdated(domain.RoomMemberUpdatedEvent{
				AlkemioRoomID: alkemioRoomID,
				MemberID:      memberID,
				SenderID:      senderID,
				Membership:    membershipState,
				Timestamp:     time.UnixMilli(e.Timestamp),
			}); err != nil {
				m.logger.Error("Error handling member updated event", "error", err)
			}
		}
	}(evt, targetUUID, senderUUID, membership, reason)
}

// resolveAlkemioRoomID gets the Alkemio room UUID from a Matrix room ID.
// Uses GetRoomAliases() which queries room_aliases table - same source as ResolveAlias().
// Prefers aliases matching the Alkemio UUID pattern, falls back to first alias if none match.
func (m *MautrixAdapter) resolveAlkemioRoomID(ctx context.Context, roomID id.RoomID) uuid.UUID {
	aliases, err := m.GetRoomAliases(ctx, roomID)
	if err != nil || len(aliases) == 0 {
		return uuid.Nil
	}

	alias := m.selectPreferredAlias(aliases)
	if alias == "" {
		return uuid.Nil
	}

	return m.idMapper.AlkemioRoomID(alias)
}

// resolveActorID converts a Matrix user ID to an Alkemio actor UUID.
// Returns uuid.Nil if the user ID is not a valid ghost user.
func (m *MautrixAdapter) resolveActorID(_ context.Context, userID id.UserID) uuid.UUID {
	return m.idMapper.AlkemioActorID(userID)
}

// handleReceiptEvent handles m.receipt ephemeral events (read receipts).
func (m *MautrixAdapter) handleReceiptEvent(evt *event.Event) {
	if m.eventHandlers.OnReadReceiptUpdated == nil {
		return
	}

	// Parse receipt content
	content, ok := evt.Content.Parsed.(*event.ReceiptEventContent)
	if !ok || content == nil {
		m.logger.Debug("Failed to parse receipt content", "event_id", evt.ID)
		return
	}

	// Process each event's receipts
	for eventID, receipts := range *content {
		for receiptType, users := range receipts {
			// Only process m.read and m.read.thread receipts
			if receiptType != event.ReceiptTypeRead && receiptType != event.ReceiptTypeReadPrivate {
				continue
			}

			for userID, receipt := range users {
				// Skip bot's own receipts
				if userID == m.as.BotMXID() {
					continue
				}

				// Parse user UUID from Matrix user ID
				actorUUID := m.resolveActorID(context.Background(), userID)
				if actorUUID == uuid.Nil {
					continue
				}

				// Extract thread ID if present
				var threadID *id.EventID
				if receipt.ThreadID != "" && receipt.ThreadID != "main" {
					tid := receipt.ThreadID
					threadID = &tid
				}

				go func(roomID id.RoomID, evtID id.EventID, actor uuid.UUID, ts int64, tid *id.EventID) {
					ctx := context.Background()
					alkemioRoomID := m.resolveAlkemioRoomID(ctx, roomID)
					if alkemioRoomID == uuid.Nil {
						return
					}

					if err := m.eventHandlers.OnReadReceiptUpdated(domain.ReadReceiptEvent{
						AlkemioRoomID: alkemioRoomID,
						UserID:        actor,
						EventID:       evtID,
						ThreadID:      tid,
						Timestamp:     time.UnixMilli(ts),
					}); err != nil {
						m.logger.Error("Error handling read receipt", "error", err)
					}
				}(evt.RoomID, eventID, actorUUID, receipt.Timestamp.UnixMilli(), threadID)
			}
		}
	}
}

// handleRoomCreateEvent handles m.room.create state events.
func (m *MautrixAdapter) handleRoomCreateEvent(evt *event.Event) {
	if m.eventHandlers.OnRoomCreated == nil {
		return
	}

	// Parse creator UUID from Matrix user ID
	creatorUUID := m.resolveActorID(context.Background(), evt.Sender)
	if creatorUUID == uuid.Nil {
		m.logger.Debug("Ignoring room create from non-UUID user", "sender", evt.Sender)
		return
	}

	// Parse room create content
	content, ok := parseEventContent[event.CreateEventContent](evt)
	if !ok {
		m.logger.Debug("Failed to parse room create content", "event_id", evt.ID)
		return
	}

	// Determine room type
	roomType := "room"
	if content.Type == "m.space" {
		roomType = "space"
	}

	go func(e *event.Event, creator uuid.UUID, rType string) {
		ctx := context.Background()
		alkemioRoomID := m.resolveAlkemioRoomID(ctx, e.RoomID)
		if alkemioRoomID == uuid.Nil {
			return
		}

		// Fetch room name and topic from state (best-effort)
		name, topic := m.getRoomNameAndTopic(ctx, e.RoomID)

		if err := m.eventHandlers.OnRoomCreated(domain.RoomCreatedEvent{
			AlkemioRoomID: alkemioRoomID,
			MatrixRoomID:  e.RoomID,
			CreatorID:     creator,
			RoomType:      rType,
			Name:          name,
			Topic:         topic,
			Timestamp:     time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling room create", "error", err)
		}
	}(evt, creatorUUID, roomType)
}

// stateChange holds the parsed property change from a room state event.
type stateChange struct {
	DisplayName *string
	AvatarURL   *string
	Topic       *string
}

// parseStateChange extracts the changed property from a room state event.
// Returns nil if the event content cannot be parsed.
func parseStateChange(e *event.Event) *stateChange {
	switch e.Type {
	case event.StateRoomName:
		content, ok := parseEventContent[event.RoomNameEventContent](e)
		if !ok {
			return nil
		}
		return &stateChange{DisplayName: &content.Name}

	case event.StateRoomAvatar:
		content, ok := parseEventContent[event.RoomAvatarEventContent](e)
		if !ok {
			return nil
		}
		url := string(content.URL)
		return &stateChange{AvatarURL: &url}

	case event.StateTopic:
		content, ok := parseEventContent[event.TopicEventContent](e)
		if !ok {
			return nil
		}
		return &stateChange{Topic: &content.Topic}
	}
	return nil
}

// handleRoomStateEvent handles m.room.name, m.room.avatar, and m.room.topic state events.
// Detects whether the room is a space and routes to OnSpaceUpdated or OnRoomUpdated accordingly.
func (m *MautrixAdapter) handleRoomStateEvent(evt *event.Event) {
	if m.eventHandlers.OnRoomUpdated == nil && m.eventHandlers.OnSpaceUpdated == nil {
		return
	}

	go func(e *event.Event) {
		change := parseStateChange(e)
		if change == nil {
			m.logger.Debug("Failed to parse state event content", "event_id", e.ID)
			return
		}

		ctx := context.Background()
		isSpace := m.isSpaceRoom(ctx, e.RoomID)

		alkemioID := m.resolveAlkemioRoomID(ctx, e.RoomID)
		if alkemioID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio ID for state event",
				"room_id", e.RoomID, "event_type", e.Type.Type, "is_space", isSpace)
			return
		}

		if isSpace && m.eventHandlers.OnSpaceUpdated != nil {
			if err := m.eventHandlers.OnSpaceUpdated(domain.SpaceUpdatedEvent{
				AlkemioContextID: alkemioID,
				DisplayName:      change.DisplayName,
				AvatarURL:        change.AvatarURL,
				Topic:            change.Topic,
				Timestamp:        time.UnixMilli(e.Timestamp),
			}); err != nil {
				m.logger.Error("Error handling space state event",
					"error", err, "event_type", e.Type.Type, "room_id", e.RoomID)
			}
		} else if !isSpace && m.eventHandlers.OnRoomUpdated != nil {
			if err := m.eventHandlers.OnRoomUpdated(domain.RoomUpdatedEvent{
				AlkemioRoomID: alkemioID,
				DisplayName:   change.DisplayName,
				AvatarURL:     change.AvatarURL,
				Topic:         change.Topic,
				Timestamp:     time.UnixMilli(e.Timestamp),
			}); err != nil {
				m.logger.Error("Error handling room state event",
					"error", err, "event_type", e.Type.Type, "room_id", e.RoomID)
			}
		}
	}(evt)
}

// isSpaceRoom checks if a Matrix room is a space by inspecting its m.room.create event.
func (m *MautrixAdapter) isSpaceRoom(ctx context.Context, roomID id.RoomID) bool {
	intent := m.as.BotIntent()
	var createContent event.CreateEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateCreate, "", &createContent); err != nil {
		return false
	}
	return createContent.Type == "m.space"
}

// getRoomNameAndTopic fetches the room name and topic from state events.
// Returns empty strings if the state events are absent or fetching fails.
func (m *MautrixAdapter) getRoomNameAndTopic(ctx context.Context, roomID id.RoomID) (name, topic string) {
	intent := m.as.BotIntent()

	// Fetch room name (best-effort)
	var nameContent event.RoomNameEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateRoomName, "", &nameContent); err == nil {
		name = nameContent.Name
	}

	// Fetch room topic (best-effort)
	var topicContent event.TopicEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateTopic, "", &topicContent); err == nil {
		topic = topicContent.Topic
	}

	return name, topic
}
