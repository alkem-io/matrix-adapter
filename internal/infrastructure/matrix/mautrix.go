package matrix

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/config"
	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// MautrixAdapter implements the MatrixPort interface using the mautrix-go library.
type MautrixAdapter struct {
	cfg      *config.Config
	logger   ports.Logger
	as       *appservice.AppService
	idMapper *domain.IDMapper
}

// NewMautrixAdapter creates a new instance of MautrixAdapter.
func NewMautrixAdapter(cfg *config.Config, logger ports.Logger) (*MautrixAdapter, error) {
	// Parse Homeserver URL for client configuration
	hsURL, err := url.Parse(cfg.Matrix.HomeserverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid homeserver URL: %w", err)
	}

	// Use configured homeserver name for room aliases and user IDs
	// This should match Synapse's server_name, not the Docker hostname
	homeserverDomain := cfg.Matrix.HomeserverName
	if homeserverDomain == "" {
		// Fallback to URL hostname if not configured (not recommended)
		homeserverDomain = hsURL.Hostname()
		logger.Warn("SYNAPSE_HOMESERVER_NAME not set, falling back to URL hostname",
			"hostname", homeserverDomain)
	}

	// Create AppService instance
	as := appservice.Create()
	as.HomeserverDomain = homeserverDomain
	// as.HomeserverURL = cfg.Matrix.HomeserverURL // Removed as field doesn't exist
	as.Registration = &appservice.Registration{
		ID:              "alkemio-matrix-adapter",
		URL:             "http://localhost:8080",
		AppToken:        cfg.Matrix.AppServiceToken,
		ServerToken:     cfg.Matrix.HomeserverToken,
		SenderLocalpart: cfg.Matrix.SenderLocalpart,
		Namespaces: appservice.Namespaces{
			UserIDs: []appservice.Namespace{
				{
					Exclusive: false,
					Regex:     "@[0-9a-fA-F-]{36}:.*",
				},
			},
		},
	}

	// Set homeserver URL on the AppService so all clients/intents get it
	if err := as.SetHomeserverURL(cfg.Matrix.HomeserverURL); err != nil {
		return nil, fmt.Errorf("failed to set homeserver URL: %w", err)
	}

	return &MautrixAdapter{
		cfg:      cfg,
		logger:   logger,
		as:       as,
		idMapper: domain.NewIDMapper(homeserverDomain),
	}, nil
}

// Connect initializes the connection to the Matrix homeserver.
func (m *MautrixAdapter) Connect(ctx context.Context) error {
	m.logger.Info("Initializing Matrix AppService connection...")

	// Channel to signal when the server has started or failed
	startedCh := make(chan error, 1)

	// Start the AppService HTTP server in a goroutine
	go func() {
		// Start() blocks until the server stops
		m.as.Start()
		// If we reach here immediately (before readiness check), it means startup failed
		// The readiness check will detect this via the probe
		m.logger.Warn("AppService HTTP server stopped")
	}()

	// Perform readiness check with timeout
	if err := m.waitForServerReady(ctx, startedCh); err != nil {
		return fmt.Errorf("appservice failed to start: %w", err)
	}

	m.logger.Debug("AppService HTTP server is ready")

	// Verify bot connection
	botClient := m.as.BotClient()
	whoami, err := botClient.Whoami(ctx)
	if err != nil {
		m.logger.Warn("Failed to verify bot connection (Whoami)", "error", err)
		// Don't fail hard here, as AS might not be fully registered yet on HS side
	} else {
		m.logger.Info("Matrix AppService connected", "user_id", whoami.UserID)
	}

	return nil
}

// waitForServerReady probes the AppService HTTP server until it's ready or times out.
func (m *MautrixAdapter) waitForServerReady(ctx context.Context, _ chan error) error {
	const (
		timeout      = 5 * time.Second
		pollInterval = 25 * time.Millisecond
	)

	// Build the server address for health check
	serverAddr := m.getServerAddress()
	if serverAddr == "" {
		return fmt.Errorf("appservice host not configured")
	}

	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to connect to the server
		if err := m.probeServer(serverAddr); err == nil {
			return nil // Server is ready
		} else {
			lastErr = err
		}

		time.Sleep(pollInterval)
	}

	if lastErr != nil {
		return fmt.Errorf("timeout waiting for server (last error: %w)", lastErr)
	}
	return fmt.Errorf("timeout waiting for server to become ready")
}

// getServerAddress returns the HTTP address of the AppService server.
func (m *MautrixAdapter) getServerAddress() string {
	if m.as.Host.IsUnixSocket() {
		return "" // Unix sockets need different handling, skip for now
	}
	return m.as.Host.Address()
}

// probeServer attempts to connect to the server to verify it's listening.
func (m *MautrixAdapter) probeServer(addr string) error {
	// Use a TCP connection probe - faster than HTTP and doesn't require valid routes
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// Disconnect closes the connection to the Matrix homeserver.
func (m *MautrixAdapter) Disconnect() error {
	// Stop the AppService HTTP server if running
	m.as.Stop()
	return nil
}

// EnsureUser provisions a user on the homeserver if it doesn't exist
func (m *MautrixAdapter) EnsureUser(ctx context.Context, actor domain.Actor) (id.UserID, error) {
	// Construct Matrix ID from Actor ID (UUID)
	// Format: @uuid:domain
	// Note: We need to ensure the localpart is valid. UUIDs are safe.
	localpart := actor.ID.String()
	userID := id.NewUserID(localpart, m.as.HomeserverDomain)

	// Check if user exists (intent)
	intent := m.as.Intent(userID)

	// Register if needed
	err := intent.EnsureRegistered(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to ensure user registered: %w", err)
	}

	// Update display name if provided
	if actor.DisplayName != "" {
		err := intent.SetDisplayName(ctx, actor.DisplayName)
		if err != nil {
			m.logger.Warn("Failed to set display name", "user_id", userID, "error", err)
		}
	}

	return userID, nil
}

// InviteUser invites a user to a room.
func (m *MautrixAdapter) InviteUser(
	ctx context.Context, roomID id.RoomID, inviterID domain.Actor, inviteeID domain.Actor,
) error {
	inviterUserID, err := m.EnsureUser(ctx, inviterID)
	if err != nil {
		return err
	}
	inviteeUserID, err := m.EnsureUser(ctx, inviteeID)
	if err != nil {
		return err
	}

	intent := m.as.Intent(inviterUserID)
	_, err = intent.InviteUser(
		ctx, roomID, &mautrix.ReqInviteUser{
			UserID: inviteeUserID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to invite user: %w", err)
	}
	return nil
}

// SendMessage sends a message to a room.
func (m *MautrixAdapter) SendMessage(
	ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string,
) (id.EventID, error) {
	userID, err := m.EnsureUser(ctx, senderID)
	if err != nil {
		return "", err
	}
	intent := m.as.Intent(userID)

	resp, err := intent.SendText(ctx, roomID, content)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}
	return resp.EventID, nil
}

// GetAllJoinedRooms returns the list of all rooms the bot has joined.
func (m *MautrixAdapter) GetAllJoinedRooms(ctx context.Context) ([]id.RoomID, error) {
	intent := m.as.BotIntent()
	resp, err := intent.JoinedRooms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get joined rooms: %w", err)
	}
	return resp.JoinedRooms, nil
}

// GetRoomDetails returns details about a room.
func (m *MautrixAdapter) GetRoomDetails(ctx context.Context, roomID id.RoomID) (*domain.Room, error) {
	intent := m.as.BotIntent()

	var name, topic, alias string

	var nameContent event.RoomNameEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateRoomName, "", &nameContent); err == nil {
		name = nameContent.Name
	}

	var topicContent event.TopicEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateTopic, "", &topicContent); err == nil {
		topic = topicContent.Topic
	}

	var aliasContent event.CanonicalAliasEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateCanonicalAlias, "", &aliasContent); err == nil {
		alias = string(aliasContent.Alias)
	}

	return &domain.Room{
		ID:    roomID,
		Name:  name,
		Topic: topic,
		Alias: alias,
	}, nil
}

// GetRoomMembers returns the list of members in a room.
func (m *MautrixAdapter) GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error) {
	intent := m.as.BotIntent()
	resp, err := intent.JoinedMembers(ctx, roomID)
	if err != nil {
		return nil, err
	}

	members := make([]id.UserID, 0, len(resp.Joined))
	for userID := range resp.Joined {
		members = append(members, userID)
	}
	return members, nil
}

// UpdateRoomState updates the state of a room.
func (m *MautrixAdapter) UpdateRoomState(
	ctx context.Context, roomID id.RoomID, actorID domain.Actor, name, topic, alias string,
) error {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	if name != "" {
		if _, err := intent.SetRoomName(ctx, roomID, name); err != nil {
			return err
		}
	}
	if topic != "" {
		if _, err := intent.SetRoomTopic(ctx, roomID, topic); err != nil {
			return err
		}
	}
	if alias != "" {
		content := event.CanonicalAliasEventContent{
			Alias: id.RoomAlias(alias),
		}
		if _, err := intent.SendStateEvent(ctx, roomID, event.StateCanonicalAlias, "", &content); err != nil {
			return err
		}
	}
	return nil
}

// SendReply sends a reply to a message.
func (m *MautrixAdapter) SendReply(
	ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, threadID id.EventID,
) (id.EventID, error) {
	userID, err := m.EnsureUser(ctx, senderID)
	if err != nil {
		return "", err
	}
	intent := m.as.Intent(userID)

	msgContent := event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    content,
		RelatesTo: &event.RelatesTo{
			InReplyTo: &event.InReplyTo{
				EventID: threadID,
			},
		},
	}

	resp, err := intent.SendMessageEvent(ctx, roomID, event.EventMessage, &msgContent)
	if err != nil {
		return "", fmt.Errorf("failed to send reply: %w", err)
	}
	return resp.EventID, nil
}

// RedactEvent redacts an event.
func (m *MautrixAdapter) RedactEvent(
	ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, reason string,
) error {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	req := mautrix.ReqRedact{
		Reason: reason,
	}

	_, err = intent.RedactEvent(ctx, roomID, eventID, req)
	if err != nil {
		return fmt.Errorf("failed to redact event: %w", err)
	}
	return nil
}

// SendReaction sends a reaction to an event.
func (m *MautrixAdapter) SendReaction(
	ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, emoji string,
) (id.EventID, error) {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return "", err
	}
	intent := m.as.Intent(userID)

	content := event.ReactionEventContent{
		RelatesTo: event.RelatesTo{
			EventID: eventID,
			Key:     emoji,
			Type:    event.RelAnnotation,
		},
	}

	resp, err := intent.SendMessageEvent(ctx, roomID, event.EventReaction, &content)
	if err != nil {
		return "", fmt.Errorf("failed to send reaction: %w", err)
	}
	return resp.EventID, nil
}

// GetMessage retrieves a specific message event.
func (m *MautrixAdapter) GetMessage(ctx context.Context, roomID id.RoomID, eventID id.EventID) (
	*domain.Message, error,
) {
	intent := m.as.BotIntent()
	evt, err := intent.GetEvent(ctx, roomID, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	// Try to parse content - handle nil Parsed case
	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		// Try parsing raw content
		if err := evt.Content.ParseRaw(evt.Type); err != nil {
			return nil, fmt.Errorf("failed to parse message content: %w", err)
		}
		content, ok = evt.Content.Parsed.(*event.MessageEventContent)
		if !ok {
			// Last resort: try raw JSON
			if rawBody, ok := evt.Content.Raw["body"].(string); ok {
				msg := &domain.Message{
					ID:             evt.ID.String(),
					RoomID:         roomID.String(),
					Content:        rawBody,
					SenderMatrixID: evt.Sender.String(),
					Timestamp:      time.UnixMilli(evt.Timestamp),
				}
				// Try to extract thread info from raw content
				if relatesTo, ok := evt.Content.Raw["m.relates_to"].(map[string]interface{}); ok {
					if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
						if eventID, ok := inReplyTo["event_id"].(string); ok {
							msg.ThreadID = eventID
						}
					}
				}
				return msg, nil
			}
			return nil, fmt.Errorf("event is not a message")
		}
	}

	return &domain.Message{
		ID:             evt.ID.String(),
		RoomID:         roomID.String(),
		Content:        content.Body,
		SenderMatrixID: evt.Sender.String(),
		Timestamp:      time.UnixMilli(evt.Timestamp),
	}, nil
}

// RespRelations defines the response for relations endpoint
type RespRelations struct {
	Chunk []event.Event `json:"chunk"`
}

// GetReactionEventID finds the event ID of a reaction.
func (m *MautrixAdapter) GetReactionEventID(
	ctx context.Context, roomID id.RoomID, eventID id.EventID, emoji string, senderID domain.Actor,
) (id.EventID, error) {
	intent := m.as.BotIntent()

	// Manually build request for relations
	// BuildURL signature is tricky in this version, so we construct the URL manually.
	// We assume Client.HomeserverURL is set and valid.
	hsURL := strings.TrimSuffix(intent.HomeserverURL.String(), "/")
	u := fmt.Sprintf(
		"%s/_matrix/client/v1/rooms/%s/relations/%s/%s/%s", hsURL, roomID, eventID, event.RelAnnotation,
		event.EventReaction,
	)

	var resp RespRelations
	_, err := intent.MakeRequest(ctx, "GET", u, nil, &resp)
	if err != nil {
		return "", fmt.Errorf("failed to get relations: %w", err)
	}

	senderUserID, err := m.EnsureUser(ctx, senderID)
	if err != nil {
		return "", err
	}

	for _, evt := range resp.Chunk {
		if evt.Sender == senderUserID && evt.Type == event.EventReaction {
			// Try to parse content - handle nil Parsed case
			content, ok := evt.Content.Parsed.(*event.ReactionEventContent)
			if !ok {
				// Try parsing raw content
				if err := evt.Content.ParseRaw(evt.Type); err != nil {
					continue
				}
				content, ok = evt.Content.Parsed.(*event.ReactionEventContent)
				if !ok {
					continue
				}
			}
			if content.RelatesTo.Key == emoji {
				return evt.ID, nil
			}
		}
	}

	return "", fmt.Errorf("reaction not found")
}

// HomeserverDomain returns the homeserver domain for room alias construction.
func (m *MautrixAdapter) HomeserverDomain() string {
	return m.as.HomeserverDomain
}

// SetUserProfile updates the user's display name and avatar.
func (m *MautrixAdapter) SetUserProfile(ctx context.Context, actor domain.Actor) error {
	userID, err := m.EnsureUser(ctx, actor)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	if actor.DisplayName != "" {
		if err := intent.SetDisplayName(ctx, actor.DisplayName); err != nil {
			m.logger.Warn("Failed to set display name", "user_id", userID, "error", err)
		}
	}

	if actor.AvatarURL != "" {
		// AvatarURL should be a mxc:// URL
		if err := intent.SetAvatarURL(ctx, id.ContentURI{}); err != nil {
			m.logger.Warn("Failed to set avatar URL", "user_id", userID, "error", err)
		}
	}

	return nil
}

// ResolveAlias resolves a room alias to a room ID.
func (m *MautrixAdapter) ResolveAlias(ctx context.Context, alias string) (id.RoomID, error) {
	intent := m.as.BotIntent()
	resp, err := intent.ResolveAlias(ctx, id.RoomAlias(alias))
	if err != nil {
		return "", fmt.Errorf("failed to resolve alias %s: %w", alias, err)
	}
	return resp.RoomID, nil
}

// DeleteAlias removes a room alias.
func (m *MautrixAdapter) DeleteAlias(ctx context.Context, alias string) error {
	intent := m.as.BotIntent()
	_, err := intent.DeleteAlias(ctx, id.RoomAlias(alias))
	if err != nil {
		return fmt.Errorf("failed to delete alias %s: %w", alias, err)
	}
	return nil
}

// KickUser kicks a user from a room.
func (m *MautrixAdapter) KickUser(ctx context.Context, roomID id.RoomID, userID id.UserID, reason string) error {
	intent := m.as.BotIntent()
	_, err := intent.KickUser(ctx, roomID, &mautrix.ReqKickUser{
		UserID: userID,
		Reason: reason,
	})
	if err != nil {
		return fmt.Errorf("failed to kick user %s from room %s: %w", userID, roomID, err)
	}
	return nil
}

// GetRoomMessages retrieves all messages from a room, including their reactions.
func (m *MautrixAdapter) GetRoomMessages(ctx context.Context, roomID id.RoomID) ([]domain.Message, error) {
	intent := m.as.BotIntent()

	// Get messages using the messages endpoint
	resp, err := intent.Messages(ctx, roomID, "", "", mautrix.DirectionBackward, nil, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get room messages: %w", err)
	}

	// First pass: collect messages and track indices by event ID
	messageIndices := make(map[string]int)
	messages := make([]domain.Message, 0, len(resp.Chunk))

	for _, evt := range resp.Chunk {
		if evt.Type != event.EventMessage {
			continue
		}

		msg := m.parseMessageEvent(evt, roomID)
		if msg != nil {
			msg.Reactions = []domain.Reaction{} // Initialize empty slice
			messages = append(messages, *msg)
			messageIndices[msg.ID] = len(messages) - 1
		}
	}

	// Second pass: collect reactions and attach to their parent messages
	for _, evt := range resp.Chunk {
		if evt.Type != event.EventReaction {
			continue
		}

		reaction := m.parseReactionEvent(evt, roomID)
		if reaction == nil {
			continue
		}

		// Find parent message and attach reaction
		parentMsgID := reaction.MessageID.String()
		if idx, exists := messageIndices[parentMsgID]; exists {
			messages[idx].Reactions = append(messages[idx].Reactions, *reaction)
		}
	}

	// Log warning if message count exceeds 1000 for future pagination tracking
	if len(messages) >= 1000 {
		m.logger.Warn("Room has 1000+ messages, pagination may be needed in future", "room_id", roomID, "count", len(messages))
	}

	return messages, nil
}

// parseReactionEvent extracts a domain.Reaction from a Matrix reaction event.
func (m *MautrixAdapter) parseReactionEvent(evt *event.Event, roomID id.RoomID) *domain.Reaction {
	// Try to parse content
	content, ok := evt.Content.Parsed.(*event.ReactionEventContent)
	if !ok {
		// Try parsing raw content
		if err := evt.Content.ParseRaw(evt.Type); err != nil {
			return nil
		}
		content, ok = evt.Content.Parsed.(*event.ReactionEventContent)
		if !ok {
			return nil
		}
	}

	if content.RelatesTo.EventID == "" {
		return nil
	}

	return &domain.Reaction{
		ID:             evt.ID,
		RoomID:         roomID,
		MessageID:      content.RelatesTo.EventID,
		Emoji:          content.RelatesTo.Key,
		SenderMatrixID: evt.Sender.String(),
		Timestamp:      time.UnixMilli(evt.Timestamp),
	}
}

// parseMessageEvent extracts a domain.Message from a Matrix event.
func (m *MautrixAdapter) parseMessageEvent(evt *event.Event, roomID id.RoomID) *domain.Message {
	body := m.extractMessageBody(evt)
	if body == "" {
		return nil
	}

	msg := &domain.Message{
		ID:             evt.ID.String(),
		RoomID:         roomID.String(),
		Content:        body,
		SenderMatrixID: evt.Sender.String(),
		Timestamp:      time.UnixMilli(evt.Timestamp),
	}

	// Check for thread/reply from parsed content
	if content, ok := evt.Content.Parsed.(*event.MessageEventContent); ok {
		if content.RelatesTo != nil && content.RelatesTo.InReplyTo != nil {
			msg.ThreadID = content.RelatesTo.InReplyTo.EventID.String()
		}
	}

	return msg
}

// extractMessageBody gets the message body from an event, trying multiple approaches.
func (m *MautrixAdapter) extractMessageBody(evt *event.Event) string {
	// Try parsed content first
	if content, ok := evt.Content.Parsed.(*event.MessageEventContent); ok {
		return content.Body
	}

	// Parse from raw content if Parsed is nil
	if err := evt.Content.ParseRaw(evt.Type); err == nil {
		if content, ok := evt.Content.Parsed.(*event.MessageEventContent); ok {
			return content.Body
		}
	}

	// Try raw JSON as last resort
	if rawBody, ok := evt.Content.Raw["body"].(string); ok {
		return rawBody
	}

	return ""
}

// GetReaction retrieves details of a specific reaction.
func (m *MautrixAdapter) GetReaction(ctx context.Context, roomID id.RoomID, reactionID id.EventID) (*domain.Reaction, error) {
	intent := m.as.BotIntent()

	evt, err := intent.GetEvent(ctx, roomID, reactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reaction event: %w", err)
	}

	if evt.Type != event.EventReaction {
		return nil, fmt.Errorf("event is not a reaction")
	}

	// Try to parse content - handle nil Parsed case
	content, ok := evt.Content.Parsed.(*event.ReactionEventContent)
	if !ok {
		// Try parsing raw content
		if err := evt.Content.ParseRaw(evt.Type); err != nil {
			return nil, fmt.Errorf("failed to parse reaction content: %w", err)
		}
		content, ok = evt.Content.Parsed.(*event.ReactionEventContent)
		if !ok {
			return nil, fmt.Errorf("failed to parse reaction content after ParseRaw")
		}
	}

	return &domain.Reaction{
		ID:             evt.ID,
		RoomID:         roomID,
		MessageID:      content.RelatesTo.EventID,
		Emoji:          content.RelatesTo.Key,
		SenderMatrixID: evt.Sender.String(),
		Timestamp:      time.UnixMilli(evt.Timestamp),
	}, nil
}

// CreateRoomWithAlias creates a new room with a specific alias based on Alkemio room ID.
func (m *MautrixAdapter) CreateRoomWithAlias(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	roomType string,
	name, topic string,
	initialMembers []domain.Actor,
) (id.RoomID, error) {
	// Use IDMapper for consistent alias construction
	aliasLocalpart := m.idMapper.RoomAliasLocalpart(alkemioRoomID)

	// Use bot intent for creating rooms
	intent := m.as.BotIntent()

	// Prepare initial invites
	invites := make([]id.UserID, 0, len(initialMembers))
	for _, member := range initialMembers {
		userID, err := m.EnsureUser(ctx, member)
		if err != nil {
			return "", fmt.Errorf("failed to ensure member %s: %w", member.ID, err)
		}
		invites = append(invites, userID)
	}

	// Determine preset based on room type
	preset := "public_chat"
	isDirect := false
	if roomType == "direct" {
		preset = "trusted_private_chat"
		isDirect = true
	}

	req := &mautrix.ReqCreateRoom{
		Name:          name,
		Topic:         topic,
		Preset:        preset,
		IsDirect:      isDirect,
		RoomAliasName: aliasLocalpart,
		Invite:        invites,
	}

	resp, err := intent.CreateRoom(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create room with alias: %w", err)
	}

	m.logger.Info("Room created with alias",
		"room_id", resp.RoomID,
		"alias", m.idMapper.RoomAlias(alkemioRoomID),
		"alkemio_room_id", alkemioRoomID)

	return resp.RoomID, nil
}

// ============================================================================
// Space Operations (MSC1772)
// ============================================================================

// CreateSpace creates a Matrix Space room with the given parameters.
func (m *MautrixAdapter) CreateSpace(
	ctx context.Context,
	alkemioContextID uuid.UUID,
	name, topic, avatarURL string,
	joinRule string,
	initialMembers []domain.Actor,
) (id.RoomID, error) {
	// Use IDMapper for consistent alias construction
	aliasLocalpart := m.idMapper.SpaceAliasLocalpart(alkemioContextID)

	intent := m.as.BotIntent()

	// Prepare initial invites
	invites := make([]id.UserID, 0, len(initialMembers))
	for _, member := range initialMembers {
		userID, err := m.EnsureUser(ctx, member)
		if err != nil {
			m.logger.Warn("Failed to ensure member for space invite", "member_id", member.ID, "error", err)
			continue
		}
		invites = append(invites, userID)
	}

	// Map join rule to Matrix preset
	preset := "private_chat"
	if joinRule == "public" {
		preset = "public_chat"
	}

	// Create room with space type
	req := &mautrix.ReqCreateRoom{
		Name:          name,
		Topic:         topic,
		Preset:        preset,
		RoomAliasName: aliasLocalpart,
		Invite:        invites,
		CreationContent: map[string]interface{}{
			"type": "m.space",
		},
	}

	// Set initial state events
	initialState := make([]*event.Event, 0)

	// Add join rule state event
	if joinRule != "" {
		joinRuleContent := &event.JoinRulesEventContent{
			JoinRule: event.JoinRule(joinRule),
		}
		initialState = append(initialState, &event.Event{
			Type:    event.StateJoinRules,
			Content: event.Content{Parsed: joinRuleContent},
		})
	}

	// Add avatar state event if provided
	if avatarURL != "" {
		avatarContent := &event.RoomAvatarEventContent{
			URL: id.ContentURIString(avatarURL),
		}
		initialState = append(initialState, &event.Event{
			Type:    event.StateRoomAvatar,
			Content: event.Content{Parsed: avatarContent},
		})
	}

	if len(initialState) > 0 {
		req.InitialState = initialState
	}

	resp, err := intent.CreateRoom(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create space: %w", err)
	}

	m.logger.Info("Space created",
		"room_id", resp.RoomID,
		"alias", m.idMapper.SpaceAlias(alkemioContextID),
		"alkemio_context_id", alkemioContextID)

	return resp.RoomID, nil
}

// GetSpaceDetails retrieves space metadata and state.
func (m *MautrixAdapter) GetSpaceDetails(ctx context.Context, roomID id.RoomID) (*domain.Space, error) {
	intent := m.as.BotIntent()

	var name, topic, alias, avatarURL, joinRule string

	var nameContent event.RoomNameEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateRoomName, "", &nameContent); err == nil {
		name = nameContent.Name
	}

	var topicContent event.TopicEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateTopic, "", &topicContent); err == nil {
		topic = topicContent.Topic
	}

	var aliasContent event.CanonicalAliasEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateCanonicalAlias, "", &aliasContent); err == nil {
		alias = string(aliasContent.Alias)
	}

	var avatarContent event.RoomAvatarEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateRoomAvatar, "", &avatarContent); err == nil {
		avatarURL = string(avatarContent.URL)
	}

	var joinRuleContent event.JoinRulesEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateJoinRules, "", &joinRuleContent); err == nil {
		joinRule = string(joinRuleContent.JoinRule)
	}

	return &domain.Space{
		ID:        roomID,
		Name:      name,
		Topic:     topic,
		Alias:     alias,
		AvatarURL: avatarURL,
		JoinRule:  joinRule,
	}, nil
}

// GetSpaceMembers returns the list of members in a space.
func (m *MautrixAdapter) GetSpaceMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error) {
	return m.GetRoomMembers(ctx, roomID)
}

// UpdateSpaceState updates space name, topic, avatar, or join rule.
func (m *MautrixAdapter) UpdateSpaceState(ctx context.Context, roomID id.RoomID, name, topic, avatarURL, joinRule string) error {
	intent := m.as.BotIntent()

	if name != "" {
		if _, err := intent.SetRoomName(ctx, roomID, name); err != nil {
			return fmt.Errorf("failed to set space name: %w", err)
		}
	}

	if topic != "" {
		if _, err := intent.SetRoomTopic(ctx, roomID, topic); err != nil {
			return fmt.Errorf("failed to set space topic: %w", err)
		}
	}

	if avatarURL != "" {
		avatarContent := &event.RoomAvatarEventContent{
			URL: id.ContentURIString(avatarURL),
		}
		if _, err := intent.SendStateEvent(ctx, roomID, event.StateRoomAvatar, "", avatarContent); err != nil {
			return fmt.Errorf("failed to set space avatar: %w", err)
		}
	}

	if joinRule != "" {
		joinRuleContent := &event.JoinRulesEventContent{
			JoinRule: event.JoinRule(joinRule),
		}
		if _, err := intent.SendStateEvent(ctx, roomID, event.StateJoinRules, "", joinRuleContent); err != nil {
			return fmt.Errorf("failed to set space join rule: %w", err)
		}
	}

	return nil
}

// GetSpaceChildren returns child rooms and subspaces of a space.
func (m *MautrixAdapter) GetSpaceChildren(ctx context.Context, roomID id.RoomID) ([]domain.SpaceChild, error) {
	intent := m.as.BotIntent()

	// Get all state events for the room
	stateMap, err := intent.State(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get space state: %w", err)
	}

	children := make([]domain.SpaceChild, 0)

	// Get m.space.child events from the state map
	childEvents, ok := stateMap[event.StateSpaceChild]
	if !ok {
		return children, nil
	}

	for stateKey, evt := range childEvents {
		// Try to parse content - handle nil Parsed case
		content, ok := evt.Content.Parsed.(*event.SpaceChildEventContent)
		if !ok {
			// Try parsing raw content
			if err := evt.Content.ParseRaw(evt.Type); err != nil {
				continue
			}
			content, ok = evt.Content.Parsed.(*event.SpaceChildEventContent)
			if !ok {
				continue
			}
		}

		// Check if child is active (has "via" servers)
		if len(content.Via) == 0 {
			continue
		}

		child := domain.SpaceChild{
			ChildID:   stateKey,
			Order:     content.Order,
			Suggested: content.Suggested,
		}

		// Determine if child is a space by checking its creation content
		childRoomID := id.RoomID(stateKey)
		var creationContent event.CreateEventContent
		if err := intent.StateEvent(ctx, childRoomID, event.StateCreate, "", &creationContent); err == nil {
			child.IsSpace = creationContent.Type == "m.space"
		}

		children = append(children, child)
	}

	return children, nil
}

// AddSpaceChild adds a room or subspace as a child of a space.
func (m *MautrixAdapter) AddSpaceChild(ctx context.Context, spaceID id.RoomID, childID id.RoomID, order string, suggested bool) error {
	intent := m.as.BotIntent()

	content := &event.SpaceChildEventContent{
		Via:       []string{m.as.HomeserverDomain},
		Order:     order,
		Suggested: suggested,
	}

	_, err := intent.SendStateEvent(ctx, spaceID, event.StateSpaceChild, string(childID), content)
	if err != nil {
		return fmt.Errorf("failed to add space child: %w", err)
	}

	return nil
}

// SetSpaceParent sets the parent space for a room or subspace (m.space.parent state event).
func (m *MautrixAdapter) SetSpaceParent(ctx context.Context, childID id.RoomID, parentID id.RoomID) error {
	intent := m.as.BotIntent()

	content := &event.SpaceParentEventContent{
		Via:       []string{m.as.HomeserverDomain},
		Canonical: true,
	}

	_, err := intent.SendStateEvent(ctx, childID, event.StateSpaceParent, string(parentID), content)
	if err != nil {
		return fmt.Errorf("failed to set space parent: %w", err)
	}

	return nil
}

// InviteToSpace invites a user to a space.
func (m *MautrixAdapter) InviteToSpace(ctx context.Context, spaceID id.RoomID, invitee domain.Actor) error {
	inviteeUserID, err := m.EnsureUser(ctx, invitee)
	if err != nil {
		return fmt.Errorf("failed to ensure invitee user: %w", err)
	}

	intent := m.as.BotIntent()
	_, err = intent.InviteUser(ctx, spaceID, &mautrix.ReqInviteUser{
		UserID: inviteeUserID,
	})
	if err != nil {
		return fmt.Errorf("failed to invite user to space: %w", err)
	}

	return nil
}

// KickFromSpace kicks a user from a space.
func (m *MautrixAdapter) KickFromSpace(ctx context.Context, spaceID id.RoomID, userID id.UserID, reason string) error {
	return m.KickUser(ctx, spaceID, userID, reason)
}
