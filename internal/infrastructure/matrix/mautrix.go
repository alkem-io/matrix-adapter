package matrix

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/alkem-io/matrix-adapter-go/internal/config"
	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// MautrixAdapter implements the MatrixPort interface using the mautrix-go library.
type MautrixAdapter struct {
	cfg    *config.Config
	logger ports.Logger
	as     *appservice.AppService
}

// NewMautrixAdapter creates a new instance of MautrixAdapter.
func NewMautrixAdapter(cfg *config.Config, logger ports.Logger) (*MautrixAdapter, error) {
	// Parse Homeserver URL to get domain
	hsURL, err := url.Parse(cfg.Matrix.HomeserverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid homeserver URL: %w", err)
	}

	// Create AppService instance
	as := appservice.Create()
	as.HomeserverDomain = hsURL.Hostname()
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

	return &MautrixAdapter{
		cfg:    cfg,
		logger: logger,
		as:     as,
	}, nil
}

// Connect initializes the connection to the Matrix homeserver.
func (m *MautrixAdapter) Connect(ctx context.Context) error {
	m.logger.Info("Initializing Matrix AppService connection...")

	// Initialize the AppService
	// m.as.Init() is not available, we assume Create() did enough or we use Start() later.
	// However, we need to set the HomeserverURL on the BotClient if it wasn't set.
	// The AppService struct doesn't have HomeserverURL, but the Client does.
	// When we call BotClient(), it returns a client. We should ensure it has the URL.

	// Actually, we should probably use CreateFull or manually configure the client.
	// For now, let's just set it on the bot client.
	m.as.BotClient().HomeserverURL, _ = url.Parse(m.cfg.Matrix.HomeserverURL)

	// Start the AppService (this starts the HTTP server for transactions)
	go m.as.Start()

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

// Disconnect closes the connection to the Matrix homeserver.
func (m *MautrixAdapter) Disconnect() error {
	// AppService doesn't have a strict disconnect, but we can stop the HTTP server if we started one
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

// CreateRoom creates a new room on the homeserver.
func (m *MautrixAdapter) CreateRoom(
	ctx context.Context, actorID domain.Actor, name string, metadata map[string]string,
) (id.RoomID, error) {
	// Get intent for the actor
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return "", err
	}
	intent := m.as.Intent(userID)

	// Create room options
	req := &mautrix.ReqCreateRoom{
		Name:   name,
		Preset: "public_chat", // Default to public for now
	}

	if topic, ok := metadata["topic"]; ok {
		req.Topic = topic
	}
	if alias, ok := metadata["alias"]; ok {
		req.RoomAliasName = alias
	}

	resp, err := intent.CreateRoom(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create room: %w", err)
	}

	return resp.RoomID, nil
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

// InviteUserByID invites a user (by Matrix ID) to a room.
func (m *MautrixAdapter) InviteUserByID(
	ctx context.Context, roomID id.RoomID, inviterID domain.Actor, inviteeID id.UserID,
) error {
	inviterUserID, err := m.EnsureUser(ctx, inviterID)
	if err != nil {
		return err
	}

	intent := m.as.Intent(inviterUserID)
	_, err = intent.InviteUser(
		ctx, roomID, &mautrix.ReqInviteUser{
			UserID: inviteeID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to invite user: %w", err)
	}
	return nil
}

// JoinRoom joins a user to a room.
func (m *MautrixAdapter) JoinRoom(ctx context.Context, roomID id.RoomID, actorID domain.Actor) error {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	_, err = intent.JoinRoomByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to join room: %w", err)
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

// LeaveRoom leaves a room.
func (m *MautrixAdapter) LeaveRoom(ctx context.Context, roomID id.RoomID, actorID domain.Actor) error {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	_, err = intent.LeaveRoom(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to leave room: %w", err)
	}
	return nil
}

// GetUserJoinedRooms returns the list of rooms a user has joined.
func (m *MautrixAdapter) GetUserJoinedRooms(ctx context.Context, actorID domain.Actor) ([]id.RoomID, error) {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return nil, err
	}
	intent := m.as.Intent(userID)

	resp, err := intent.JoinedRooms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get joined rooms: %w", err)
	}
	return resp.JoinedRooms, nil
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

// ForgetRoom forgets a room.
func (m *MautrixAdapter) ForgetRoom(ctx context.Context, roomID id.RoomID, actorID domain.Actor) error {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	if _, err := intent.LeaveRoom(ctx, roomID); err != nil {
		// Ignore if already left?
		m.logger.Warn("Failed to leave room before forgetting", "error", err)
	}

	if _, err := intent.ForgetRoom(ctx, roomID); err != nil {
		return fmt.Errorf("failed to forget room: %w", err)
	}
	return nil
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

	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return nil, fmt.Errorf("event is not a message")
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
	hsURL := intent.HomeserverURL.String()
	// Ensure no trailing slash
	if hsURL[len(hsURL)-1] == '/' {
		hsURL = hsURL[:len(hsURL)-1]
	}
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
			content, ok := evt.Content.Parsed.(*event.ReactionEventContent)
			if ok && content.RelatesTo.Key == emoji {
				return evt.ID, nil
			}
		}
	}

	return "", fmt.Errorf("reaction not found")
}

// CreateDirectRoom creates a direct message room.
func (m *MautrixAdapter) CreateDirectRoom(
	ctx context.Context, initiator domain.Actor, receiver domain.Actor,
) (id.RoomID, error) {
	initiatorID, err := m.EnsureUser(ctx, initiator)
	if err != nil {
		return "", err
	}
	receiverID, err := m.EnsureUser(ctx, receiver)
	if err != nil {
		return "", err
	}

	intent := m.as.Intent(initiatorID)
	resp, err := intent.CreateRoom(
		ctx, &mautrix.ReqCreateRoom{
			Preset:   "trusted_private_chat",
			IsDirect: true,
			Invite:   []id.UserID{receiverID},
			Topic:    "Direct Message",
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create DM room: %w", err)
	}

	return resp.RoomID, nil
}

// GetDirectRooms returns the list of DM rooms for an actor.
func (m *MautrixAdapter) GetDirectRooms(ctx context.Context, actorID domain.Actor) (map[id.UserID]id.RoomID, error) {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return nil, err
	}
	intent := m.as.Intent(userID)

	var directContent map[id.UserID][]id.RoomID
	if err := intent.GetAccountData(ctx, "m.direct", &directContent); err != nil {
		return map[id.UserID]id.RoomID{}, nil
	}

	result := make(map[id.UserID]id.RoomID)
	for user, rooms := range directContent {
		if len(rooms) > 0 {
			result[user] = rooms[0]
		}
	}
	return result, nil
}
