// Package matrix provides the Matrix adapter implementation using mautrix-go.
package matrix

import (
	"fmt"
	"strings"
	"time"

	"github.com/alkemio/matrix-adapter-go/internal/core/domain"
	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// OnMessage registers a handler for incoming Matrix messages.
func (m *MautrixAdapter) OnMessage(handler func(msg domain.Message) error) {
	// Use the AppService Events channel
	go func() {
		for evt := range m.as.Events {
			m.processEvent(evt, handler)
		}
	}()
}

func (m *MautrixAdapter) processEvent(evt *event.Event, handler func(msg domain.Message) error) {
	if evt.Type != event.EventMessage {
		return
	}

	// Ignore own messages
	if evt.Sender == m.as.BotMXID() {
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
		// Try to get room name from state
		var roomName string

		if err := handler(domain.Message{
			ID:        e.ID.String(),
			RoomID:    e.RoomID.String(),
			RoomName:  roomName,
			SenderID:  s,
			Content:   c,
			Timestamp: time.UnixMilli(e.Timestamp),
		}); err != nil {
			m.logger.Error("Error handling message", "error", err)
		}
	}(evt, content, senderUUID)
}

func (m *MautrixAdapter) parseActorID(mxid id.UserID) (uuid.UUID, error) {
	s := string(mxid)
	// Format: @uuid:domain
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return uuid.Nil, fmt.Errorf("invalid format")
	}
	localpart := strings.TrimPrefix(parts[0], "@")
	return uuid.Parse(localpart)
}
