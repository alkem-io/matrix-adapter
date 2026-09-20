// Package matrix provides the Matrix adapter implementation using mautrix-go.
package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// reconciling tracks rooms currently undergoing reconciliation to prevent concurrent attempts.
var reconciling sync.Map

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
			for evt := range m.as.Events() {
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
	case event.EventMessage, event.EventSticker:
		// m.sticker is its own event type but mautrix treats it as equivalent to
		// m.room.message (MessageEventContent with body/url/info). Route it through
		// the same handler so an inbound sticker flows as image-like media.
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

	// Parse content + attachment via the shared inbound-media helper, which
	// applies the present-but-empty-body rule.
	content, attachment, ok := extractInboundMessage(evt, m.idMapper)
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
	threadID := extractThreadID(evt)

	var attachments []domain.Attachment
	if attachment != nil {
		attachments = []domain.Attachment{*attachment}
	}

	go func(e *event.Event, c string, s uuid.UUID, tid string, atts []domain.Attachment) {
		// Resolve Alkemio room ID from Matrix room ID (HTTP call - must be async)
		alkemioRoomID := m.resolveOrReconcile(context.Background(), e.RoomID)
		if alkemioRoomID == uuid.Nil {
			m.logger.Warn("Could not resolve Alkemio room ID for message", "room_id", e.RoomID)
			return
		}

		if err := m.eventHandlers.OnMessage(
			domain.Message{
				ID:          e.ID.String(),
				RoomID:      alkemioRoomID.String(),
				SenderID:    s,
				Content:     c,
				Timestamp:   time.UnixMilli(e.Timestamp),
				ThreadID:    tid,
				Attachments: atts,
			},
		); err != nil {
			m.logger.Error("Error handling message", "error", err)
		}
	}(evt, content, senderUUID, threadID, attachments)
}

// extractThreadID reads the thread/reply parent event id from a message event,
// used by BOTH the live-sync path (handleMessageEvent) and the read path
// (parseMessageEvent) so the two can't diverge. It prefers the RAW m.relates_to,
// which is present on live-sync events AND on Synapse-fetched read-path events —
// the latter arrive with Content.Parsed == nil (the shared inbound helper reads
// Content.Raw and, on its fallback, ALREADY-parsed content; it never triggers
// ParseRaw, so it cannot populate Parsed as a side effect on the shared event —
// see inboundBody), so reading Parsed alone would
// silently drop thread linkage on real reads (F1). It falls back to the parsed
// content's RelatesTo for parsed-only events (e.g. unit-constructed events with
// no raw map). The explicit m.thread relation (MSC3440) wins over the legacy
// m.in_reply_to fallback. Returns "" when the event carries no thread/reply.
func extractThreadID(evt *event.Event) string {
	if tid := threadIDFromRaw(evt); tid != "" {
		return tid
	}
	return threadIDFromParsed(evt)
}

// threadIDFromRaw reads the thread/reply parent id from the raw m.relates_to.
func threadIDFromRaw(evt *event.Event) string {
	relatesTo, ok := evt.Content.Raw["m.relates_to"].(map[string]interface{})
	if !ok {
		return ""
	}
	// An m.thread relation with a MISSING or EMPTY event_id names no parent, so it
	// must fall through to the m.in_reply_to branch below rather than short-circuit
	// with "". Returning the empty event_id here would skip that fallback — which
	// the previous inline extractor reached — and silently lose thread linkage.
	if relType, ok := relatesTo["rel_type"].(string); ok && relType == "m.thread" {
		if eventID, ok := relatesTo["event_id"].(string); ok && eventID != "" {
			return eventID
		}
	}
	if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
		if eventID, ok := inReplyTo["event_id"].(string); ok {
			return eventID
		}
	}
	return ""
}

// threadIDFromParsed reads the thread/reply parent id from parsed content — the
// fallback for events that arrive parsed-only (no raw map).
func threadIDFromParsed(evt *event.Event) string {
	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok || content.RelatesTo == nil {
		return ""
	}
	if content.RelatesTo.Type == event.RelThread && content.RelatesTo.EventID != "" {
		return content.RelatesTo.EventID.String()
	}
	if content.RelatesTo.InReplyTo != nil {
		return content.RelatesTo.InReplyTo.EventID.String()
	}
	return ""
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

	// Extract thread ID via the shared *id.EventID helper (single source of truth
	// for the m.thread → event_id / m.in_reply_to → event_id parse; returns nil when
	// there is no thread/reply). Note this is distinct from originalEventID above
	// (the m.replace target — the edited message id): for an m.replace event rel_type
	// is "m.replace", so the m.thread branch never matches and this falls through to
	// the m.in_reply_to fallback.
	threadID := m.extractThreadIDFromMessage(evt)

	go func(e *event.Event, sender uuid.UUID, origID, content string, tid *id.EventID) {
		ctx := context.Background()
		alkemioRoomID := m.resolveOrReconcile(ctx, e.RoomID)
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
		alkemioRoomID := m.resolveOrReconcile(context.Background(), e.RoomID)
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
		alkemioRoomID := m.resolveOrReconcile(ctx, e.RoomID)
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
	case event.EventMessage, event.EventSticker:
		// A sticker is a message; redacting one preserves its thread linkage the
		// same way a redacted m.room.message does (the default branch would drop it).
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

// extractThreadIDFromMessage extracts the thread ID from a message event if present,
// as an *id.EventID (nil when the event carries no thread/reply). It uses the
// same raw-preferring extractor as live sync and reads, because GetEvent responses
// can carry m.relates_to only in Content.Raw.
func (m *MautrixAdapter) extractThreadIDFromMessage(originalEvt *event.Event) *id.EventID {
	tid := extractThreadID(originalEvt)
	if tid == "" {
		return nil
	}
	eventID := id.EventID(tid)
	return &eventID
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
		alkemioRoomID := m.resolveOrReconcile(context.Background(), e.RoomID)
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

// ResolveAlkemioID resolves a raw Matrix room or space id back to the Alkemio
// UUID encoded in its alias. It repeats resolveAlkemioRoomID's alias lookup —
// room and space aliases use the identical #uuid:domain pattern, so the same
// resolution works for either, and the caller already knows from context (the
// children_are_spaces flag on a hierarchy convergence request) which kind of
// id it is asking about — but keeps apart the two outcomes
// resolveAlkemioRoomID deliberately collapses, because this method's caller
// uses the answer to decide whether to remove state:
//
//	(uuid.Nil, nil) — the room carries no Alkemio-patterned alias. A confirmed
//	ghost: exactly the state a deleted discussion's child edge is left in, and
//	the only answer that may drive a prune.
//
//	(uuid.Nil, err) — the alias read itself failed. The room's identity is
//	unknown, not absent; reporting it as a ghost would let a transient
//	homeserver fault masquerade as proof that a live edge is prunable.
func (m *MautrixAdapter) ResolveAlkemioID(ctx context.Context, roomID id.RoomID) (uuid.UUID, error) {
	aliases, err := m.GetRoomAliases(ctx, roomID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to read aliases for room %s: %w", roomID, err)
	}
	if len(aliases) == 0 {
		return uuid.Nil, nil
	}

	alias := m.selectPreferredAlias(aliases)
	if alias == "" {
		return uuid.Nil, nil
	}

	return m.idMapper.AlkemioRoomID(alias), nil
}

// resolveOrReconcile tries to resolve the Alkemio room ID from the room alias.
// If no alias exists, checks for io.alkemio.pending state event and triggers reconciliation.
// Returns uuid.Nil if the room is not an Alkemio room or reconciliation is already in progress.
func (m *MautrixAdapter) resolveOrReconcile(ctx context.Context, roomID id.RoomID) uuid.UUID {
	alkemioRoomID := m.resolveAlkemioRoomID(ctx, roomID)
	if alkemioRoomID != uuid.Nil {
		return alkemioRoomID
	}

	// No alias — check for io.alkemio.pending state
	customState, err := m.GetCustomState(ctx, roomID, []string{"io.alkemio.pending"})
	if err != nil {
		m.logger.Debug("resolveOrReconcile: failed to check pending state",
			"room_id", roomID, "error", err)
		return uuid.Nil
	}

	pendingState, ok := customState["io.alkemio.pending"]
	if !ok {
		return uuid.Nil
	}

	rawID, _ := pendingState["alkemio_room_id"].(string)
	pendingUUID, err := uuid.Parse(rawID)
	if err != nil || pendingUUID == uuid.Nil {
		m.logger.Warn("resolveOrReconcile: invalid alkemio_room_id in pending state",
			"room_id", roomID, "raw_id", rawID)
		return uuid.Nil
	}

	// Prevent concurrent reconciliation for the same room
	if _, loaded := reconciling.LoadOrStore(roomID, true); loaded {
		m.logger.Debug("resolveOrReconcile: reconciliation already in progress",
			"room_id", roomID)
		return uuid.Nil
	}

	go func() { //nolint:gosec // G118: intentional background goroutine outlives the request
		defer reconciling.Delete(roomID)

		reconcileCtx := context.Background()
		creatorUserID, err := m.getRoomCreator(reconcileCtx, roomID)
		if err != nil {
			m.logger.Error("resolveOrReconcile: failed to get room creator",
				"room_id", roomID, "error", err)
			return
		}

		if err := m.ReconcileRoom(reconcileCtx, roomID, pendingUUID, creatorUserID); err != nil {
			m.logger.Error("resolveOrReconcile: reconciliation failed",
				"room_id", roomID, "alkemio_room_id", pendingUUID, "error", err)
			return
		}

		creatorActorID := m.idMapper.AlkemioActorID(creatorUserID)
		name, topic := m.getRoomNameAndTopic(reconcileCtx, roomID)

		if m.eventHandlers.OnRoomCreated != nil {
			evt := domain.RoomCreatedEvent{
				AlkemioRoomID: pendingUUID,
				MatrixRoomID:  roomID,
				CreatorID:     creatorActorID,
				RoomType:      "room",
				Name:          name,
				Topic:         topic,
				Timestamp:     time.Now(),
			}
			for attempt := 1; attempt <= 3; attempt++ {
				if err := m.eventHandlers.OnRoomCreated(evt); err != nil {
					m.logger.Error("resolveOrReconcile: failed to emit RoomCreatedEvent",
						"room_id", roomID, "attempt", attempt, "error", err)
					if attempt < 3 {
						time.Sleep(time.Duration(attempt) * time.Second)
						continue
					}
				}
				break
			}
		}

		m.logger.Info("resolveOrReconcile: reconciliation and event emission complete",
			"room_id", roomID, "alkemio_room_id", pendingUUID)
	}()

	return uuid.Nil
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
					alkemioRoomID := m.resolveOrReconcile(ctx, roomID)
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
		alkemioRoomID := m.resolveOrReconcile(ctx, e.RoomID)
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
		isSpace, err := m.isSpaceRoom(ctx, e.RoomID)
		if err != nil {
			m.logger.Warn("Could not determine room type for state event, skipping",
				"room_id", e.RoomID, "event_type", e.Type.Type, "error", err)
			return
		}

		alkemioID := m.resolveOrReconcile(ctx, e.RoomID)
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
// Uses admin API — works regardless of bot membership.
// Returns (true/false, nil) on success, or (false, err) when the room type cannot be determined.
func (m *MautrixAdapter) isSpaceRoom(ctx context.Context, roomID id.RoomID) (bool, error) {
	content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.create")
	if err != nil {
		return false, fmt.Errorf("failed to read m.room.create for %s: %w", roomID, err)
	}
	if content == nil {
		return false, nil
	}
	roomType, _ := content["type"].(string)
	return roomType == "m.space", nil
}

// Media m.room.message msgtypes carrying an attachment (mxc:// URL + file info).
// Shared by the inbound path (mediaMsgTypes) and the outbound MIME→msgtype
// mapping (mediaMsgType in mautrix.go) so the two never drift.
const (
	msgTypeImage = "m.image"
	msgTypeVideo = "m.video"
	msgTypeAudio = "m.audio"
	msgTypeFile  = "m.file"
)

// mediaMsgTypes is the set of m.room.message msgtypes that carry a media
// attachment (an mxc:// URL + file info).
var mediaMsgTypes = map[string]struct{}{
	msgTypeImage: {},
	msgTypeFile:  {},
	msgTypeVideo: {},
	msgTypeAudio: {},
}

// extractAttachment surfaces a raw media reference from a message event, or nil
// if the event is not a media message (or carries neither an mxc URL nor a
// document id). It reads the event's raw content:
//   - url(mxc, OUR homeserver only) → MediaID (the Synapse media id; the
//     server's re-home key)
//   - info → mimetype/size/w/h
//   - io.alkemio.document_id → DocumentID (an UNAUTHENTICATED hint, see below)
//
// The mxc URL must be hosted on OUR homeserver (idMapper.LocalMediaID). This is
// NOT federation support — this deployment's homeserver is not federated and a
// foreign mxc cannot legitimately arrive. It is input validation: event content
// is user-controlled, so a local member can put a foreign-looking mxc in an
// event, and MediaID is a BARE media id that the Alkemio server resolves by
// externalReference against media its OWN Synapse stored. Surfacing a foreign
// media id would either miss (attachment silently lost) or COLLIDE with an
// unrelated local media id and resolve to the WRONG document. A foreign (or
// unparseable) url therefore surfaces no MediaID at all, exactly like a non-mxc
// url.
//
// io.alkemio.document_id is an UNAUTHENTICATED HINT, and the adapter says so
// rather than pretending otherwise. It marks an echo of our own outbound media
// (the server routes DocumentID-present events down the coalesce path instead of
// re-homing), but nothing here can authenticate it:
//
//   - Any member of the room can stamp the field on an event they send.
//   - The adapter CANNOT tell its own sends from a human's. It impersonates
//     actor X *as* X — IDMapper.UserID(actorID) is @<actor-uuid>:<homeserver>,
//     the very same MXID that actor gets when they log into Element via OIDC
//     (which is why the appservice UUID namespace is registered NON-exclusive,
//     see the registration in mautrix.go). There is no separate ghost identity
//     to match against, and Synapse gives an appservice no "you sent this"
//     marker on the /transactions push. So a sender-shape test would be true for
//     every message from every user — which is exactly what the previous
//     isOwnAppserviceUser gate was, and why it was removed rather than tightened.
//
// The AUTHORITY is the Alkemio server, which is where the ownership facts live:
// its outbound/echo branch requires the named document to be in the message's
// room bucket AND createdBy == the message sender
// (message.attachment.service.ts, coalesceOutboundEcho / the read-path resolve).
// A forged breadcrumb dies there. The adapter's job is to surface what the event
// says, labelled honestly — not to advertise a guarantee it cannot keep.
//
// An m.sticker (event.EventSticker) is handled as image media: it carries the
// same url+info shape as an m.image but has NO msgtype field, so the
// mediaMsgTypes gate is bypassed for stickers. A sticker with no info.mimetype
// leaves att.MimeType empty — the adapter does not guess (a sticker may be
// webp/gif/Lottie, not PNG); the true content-type is resolved downstream at
// re-home, where file-service stores the actual blob content-type.
//
// The adapter never resolves these refs — it only surfaces them.
func extractAttachment(evt *event.Event, idMapper *domain.IDMapper) *domain.Attachment {
	raw := evt.Content.Raw
	if raw == nil {
		return nil
	}
	isSticker := evt.Type == event.EventSticker
	if !isSticker {
		msgtype, _ := raw["msgtype"].(string)
		if _, ok := mediaMsgTypes[msgtype]; !ok {
			return nil
		}
	}

	att := &domain.Attachment{DisplayName: attachmentDisplayName(raw)}

	// url (mxc://<our homeserver>/<media_id>) → MediaID. A foreign-homeserver or
	// unparseable url yields "" (see LocalMediaID) and surfaces no MediaID.
	if urlStr, ok := raw["url"].(string); ok {
		att.MediaID = idMapper.LocalMediaID(urlStr)
	}

	applyMediaInfo(att, raw)

	// io.alkemio.document_id → DocumentID, surfaced verbatim for EVERY sender (it
	// is a hint, not a credential — see the doc comment). Our own outbound media
	// already lives in file-service as document D, and the server routes echoes
	// (DocumentID present) to the *coalesce* path — stamp externalReference=media_id
	// onto D and drop the provider's staging twin — which needs BOTH refs, so
	// MediaID stays populated alongside DocumentID. The server distinguishes echo
	// (DocumentID present → coalesce) from Element-origin (DocumentID absent →
	// re-home) by DocumentID, never by MediaID, so surfacing both is unambiguous;
	// it then authorizes the DocumentID against bucket + createdBy before acting.
	if docID, ok := raw["io.alkemio.document_id"].(string); ok && docID != "" {
		att.DocumentID = docID
	}

	// A media msgtype with neither a parseable mxc URL nor a surfaced document id
	// carries no reference the server can act on — drop it rather than emit a
	// dead attachment record.
	if att.MediaID == "" && att.DocumentID == "" {
		return nil
	}

	return att
}

// attachmentDisplayName resolves a media event's display name. MSC2530: modern
// media events carry the filename in a dedicated top-level `filename` field and
// use `body` for a human caption. Prefer `filename`; fall back to `body` for
// legacy media where the body IS the filename.
//
// The fallback is gated on `filename` being ABSENT, not on it being empty.
// Presence is what distinguishes the two event shapes: a string `filename`
// declares the event MSC2530-shaped, which makes `body` a human CAPTION. Falling
// back to it because the declared filename happened to be "" would hand the
// server a caption ("look at this sunset") to use as a FILENAME. An empty
// display name is the honest answer for a degenerate `filename: ""` and is
// already a supported outcome here (a media event whose body is absent or
// non-string yields one too), so the server's own naming fallback handles it.
//
// The predicate stays a type assertion, so a non-string `filename` (null, a
// number) reads as absent and keeps the legacy body fallback — only a media
// event that actually declares a string filename opts into caption semantics.
//
// This resolves the DISPLAY NAME only. Content is a separate concern and always
// carries the event body verbatim (see extractInboundMessage): whether a body
// that equals the filename is "really" a caption is the renderer's inference to
// make, never a fact this adapter may destroy.
func attachmentDisplayName(raw map[string]interface{}) string {
	if filename, ok := raw["filename"].(string); ok {
		return filename
	}
	if body, ok := raw["body"].(string); ok {
		return body
	}
	return ""
}

// applyMediaInfo copies the media event's info block (mime/size/dimensions) onto
// the attachment. A missing or malformed info block leaves the fields zeroed.
func applyMediaInfo(att *domain.Attachment, raw map[string]interface{}) {
	info, ok := raw["info"].(map[string]interface{})
	if !ok {
		return
	}
	if mime, ok := info["mimetype"].(string); ok {
		att.MimeType = mime
	}
	// info.size is asserted by the SENDING CLIENT, so it is attacker-controlled.
	// It travels to the Alkemio server's IMessageAttachment.size, a GraphQL Int —
	// 32-bit — so an out-of-range value would break the server's whole response
	// rather than just this attachment. Clamp it to "unknown" (0) at the boundary,
	// exactly like the dimensions below.
	if size, ok := rawInt64(info["size"]); ok && size <= math.MaxInt32 {
		att.Size = size
	}
	att.Width = rawPixelPtr(info["w"])
	att.Height = rawPixelPtr(info["h"])
}

// extractInboundMessage computes the inbound message Content and optional media
// attachment for an m.room.message event. It is the single source of truth
// shared by the live-sync path (handleMessageEvent) and the read path
// (parseMessageEvent), so the two can't diverge (F10).
//
// ok is false only when the event carries neither a present body nor an
// attachment — the caller then drops the event. A present-but-empty body ("")
// IS forwarded (ok=true, empty Content): only a genuinely absent/non-string
// body with no attachment is dropped (F3/F4).
//
// A media event's body is ALWAYS surfaced as Content, exactly as it was before
// attachments existed. An earlier revision dropped it when it equalled the
// resolved display name (the MSC2530 "body == filename means no caption" rule),
// which BLANKED the message for any consumer reading Content without the new
// attachments array — on the previous behaviour that consumer saw the
// filename/caption. Both facts now travel in the same payload: Content carries
// the body verbatim, and attachments[i].DisplayName carries the resolved
// filename, so a consumer that reads attachments can apply MSC2530 itself
// (Content == DisplayName ⇒ no caption ⇒ render the attachment only). That is a
// rendering decision, and it belongs to the renderer, not to this adapter —
// destroying one of the two strings here is the only choice that cannot be
// undone downstream.
//
// A sticker's body is ALWAYS alt-text describing the image, never message text:
// it feeds only the attachment DisplayName, so Content is forced empty. A normal
// sticker (url present) → attachment + empty Content. A url-less sticker (e.g.
// E2EE content.file, or a non-parseable url) has no extractable media and no
// surface-able Content, so it returns ok=false. That ok=false is what drops it on
// the LIVE-SYNC path (handleMessageEvent, which has no isBlankMessage guard). On
// the read/scan paths parseMessageEvent discards ok, so the url-less sticker
// becomes a blank (empty-content, no-attachment) Message instead — which
// isBlankMessage then excludes from timeline/scan results.
func extractInboundMessage(
	evt *event.Event, idMapper *domain.IDMapper,
) (content string, attachment *domain.Attachment, ok bool) {
	body, bodyPresent := inboundBody(evt)
	attachment = extractAttachment(evt, idMapper)

	if !bodyPresent && attachment == nil {
		return "", nil, false
	}

	if evt.Type == event.EventSticker {
		// A sticker is media; its body is alt-text, never Content. With no
		// extractable attachment (url-less/E2EE), there is nothing to surface.
		if attachment == nil {
			return "", nil, false
		}
		return "", attachment, true
	}

	return body, attachment, true
}

// inboundBody returns the message body and whether a body is present. A body is
// present when the raw event carries a "body" string (even the empty string — a
// present-but-empty body is forwarded), or when ALREADY-parsed content yields a
// non-empty body (covers events that arrive parsed but without a raw map, e.g.
// in unit tests). This mirrors develop's `, ok` semantics.
//
// The parsed fallback reads evt.Content.Parsed DIRECTLY and deliberately does
// NOT go through parseEventContent, whose ParseRaw fallback would populate
// Content.Parsed as a side effect. The event is shared with the caller (and with
// extractThreadID, whose contract says this helper never triggers ParseRaw), so
// this read stays observation-only.
func inboundBody(evt *event.Event) (string, bool) {
	if evt.Content.Raw != nil {
		if b, ok := evt.Content.Raw["body"].(string); ok {
			return b, true
		}
	}
	if c, ok := evt.Content.Parsed.(*event.MessageEventContent); ok && c.Body != "" {
		return c.Body, true
	}
	return "", false
}

// rawInt64 coerces a JSON-decoded numeric value (float64 from encoding/json, or
// an int produced by in-process construction) to an int64. It only carries
// sizes/dimensions, which are non-negative: a NaN/Inf, negative, or int64-range-
// overflowing float (e.g. a hostile info.size of 1e30) is rejected as (0, false)
// rather than surfacing an implementation-defined garbage/negative int64. The
// upper bound is `>=` because float64(math.MaxInt64) rounds UP to 2^63, so exactly
// 2^63 would otherwise pass and int64(2^63) wraps to MinInt64.
func rawInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= float64(math.MaxInt64) {
			return 0, false
		}
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		// Int64 already errors on out-of-range values; keep returning false there.
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

// rawPixelPtr returns a pointer to v as a pixel dimension (info.w / info.h), or
// nil when v is not a usable dimension.
//
// info.w/info.h are asserted by the SENDING CLIENT — any member of the room can
// put anything there — and they flow verbatim through dto.ReceivedAttachment
// into the Alkemio server's IMessageAttachment.width/height, which are GraphQL
// `Int`: THIRTY-TWO bit. rawInt64's int64 bound is therefore not enough; a value
// above MaxInt32 breaks the server's response for the whole query. Two rules:
//
//   - > math.MaxInt32 → nil. Out of the wire type's range.
//   - <= 0 → nil. No image is zero or negative pixels wide; a non-positive
//     dimension is what an unmeasured source reports, and the server already
//     treats non-positive dims as ABSENT — so say "absent" here rather than
//     shipping a 0 it will discard anyway. (rawInt64 already rejects negatives,
//     NaN/Inf and int64 overflow; this narrows to the pixel domain.)
//
// int is 64-bit on every platform this builds for, so the conversion below is
// exact once the MaxInt32 bound has been applied.
func rawPixelPtr(v any) *int {
	i, ok := rawInt64(v)
	if !ok || i <= 0 || i > math.MaxInt32 {
		return nil
	}
	x := int(i)
	return &x
}

// getRoomNameAndTopic fetches the room name and topic from state events.
// Returns empty strings if the state events are absent or fetching fails.
func (m *MautrixAdapter) getRoomNameAndTopic(ctx context.Context, roomID id.RoomID) (name, topic string) {
	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.name"); err == nil && content != nil {
		name, _ = content["name"].(string)
	}
	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.topic"); err == nil && content != nil {
		topic, _ = content["topic"].(string)
	}
	return name, topic
}
