package matrix

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // Required by Synapse shared secret registration API (HMAC-SHA1)
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
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
	cfg            *config.Config
	logger         ports.Logger
	as             *appservice.AppService
	idMapper       *domain.IDMapper
	botDisplayName string
	eventHandlers  EventHandlers
	eventLoopOnce  sync.Once
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
		logger.Warn(
			"SYNAPSE_HOMESERVER_NAME not set, falling back to URL hostname",
			"hostname", homeserverDomain,
		)
	}

	// Create a proper MemoryStateStore with all maps initialized
	stateStore := mautrix.NewMemoryStateStore()

	// Verify the StateStore is properly initialized
	memStore, ok := stateStore.(*mautrix.MemoryStateStore)
	if !ok {
		return nil, fmt.Errorf("failed to create MemoryStateStore: unexpected type")
	}

	// WORKAROUND: mautrix-go v0.26.0 has a bug where NewMemoryStateStore() doesn't
	// initialize the JoinRules map even though SetJoinRules expects it to be non-nil.
	// TODO: Remove this workaround when upgrading to mautrix-go > v0.26.0
	if memStore.JoinRules == nil {
		memStore.JoinRules = make(map[id.RoomID]*event.JoinRulesEventContent)
	}

	// Bot sender localpart is derived from BotActorID (UUID)
	// This ensures bot follows the same ID pattern as all other actors
	botLocalpart := cfg.Matrix.BotActorID

	// Build registration
	registration := &appservice.Registration{
		ID:              "alkemio-matrix-adapter",
		URL:             "http://localhost:8280",
		AppToken:        cfg.Matrix.AppServiceToken,
		ServerToken:     cfg.Matrix.HomeserverToken,
		SenderLocalpart: botLocalpart,
		EphemeralEvents: true, // Enable receiving ephemeral events (m.receipt for read receipts)
		Namespaces: appservice.Namespaces{
			UserIDs: []appservice.Namespace{
				{
					// Bot user - exclusive, only AS can control (security: prevents impersonation)
					Exclusive: true,
					Regex:     fmt.Sprintf("@%s:.*", botLocalpart),
				},
				{
					// Regular UUID users - NOT exclusive so they can login via OIDC/Element
					// Events received via room alias registration instead
					Exclusive: false,
					Regex:     "@[0-9a-fA-F-]{36}:.*",
				},
			},
			RoomAliases: []appservice.Namespace{
				{
					// Room aliases - exclusive, AS receives all events for these rooms
					Exclusive: true,
					Regex:     "#[0-9a-fA-F-]{36}:.*",
				},
			},
		},
	}

	// Create AppService using CreateFull to ensure StateStore is set from the start
	as, err := appservice.CreateFull(
		appservice.CreateOpts{
			Registration:     registration,
			HomeserverDomain: homeserverDomain,
			HomeserverURL:    cfg.Matrix.HomeserverURL,
			HostConfig: appservice.HostConfig{
				Hostname: "0.0.0.0",
				Port:     8280,
			},
			StateStore: stateStore.(appservice.StateStore),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create appservice: %w", err)
	}

	// Enable zerolog for mautrix-go internal logging
	as.Log = zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Str("component", "mautrix").Logger()

	return &MautrixAdapter{
		cfg:            cfg,
		logger:         logger,
		as:             as,
		idMapper:       domain.NewIDMapper(homeserverDomain),
		botDisplayName: cfg.Matrix.BotDisplayName,
	}, nil
}

// ============================================================================
// Core / Connection
// ============================================================================

// Connect initializes the connection to the Matrix homeserver.
func (m *MautrixAdapter) Connect(ctx context.Context) error {
	// Step 1: Ensure bot user exists and is a server admin BEFORE appservice starts.
	// This uses direct HTTP calls to Synapse, independent of the appservice framework.
	m.ensureBotAdmin(ctx)

	// Step 2: Start the AppService
	m.logger.Info("Initializing Matrix AppService connection")

	startedCh := make(chan error, 1)
	go func() {
		m.as.Start()
		m.logger.Warn("AppService HTTP server stopped")
	}()

	if err := m.waitForServerReady(ctx, startedCh); err != nil {
		return fmt.Errorf("appservice failed to start: %w", err)
	}

	// Step 3: Verify bot connection
	botClient := m.as.BotClient()
	whoami, err := botClient.Whoami(ctx)
	if err != nil {
		m.logger.Warn("Failed to verify bot connection (Whoami)", "error", err)
	} else {
		m.logger.Info("Matrix AppService connected", "user_id", whoami.UserID)
	}

	// Step 4: Set bot display name
	if m.botDisplayName != "" {
		botIntent := m.as.BotIntent()
		if err := botIntent.SetDisplayName(ctx, m.botDisplayName); err != nil {
			m.logger.Warn("Failed to set bot display name", "display_name", m.botDisplayName, "error", err)
		} else {
			m.logger.Info("Bot display name set", "display_name", m.botDisplayName)
		}
	}

	return nil
}

// ensureBotAdmin ensures the bot user exists and is a Synapse server admin.
// Flow:
//  1. If bot is already admin → done
//  2. If SYNAPSE_REGISTRATION_SECRET is set → try registering bot as admin directly
//  3. If bot already exists but isn't admin → create temp admin, promote bot, delete temp
//  4. If no secret → log warning with manual instructions
func (m *MautrixAdapter) ensureBotAdmin(ctx context.Context) {
	// Wait for Synapse to become available before checking admin status
	m.waitForSynapse(ctx)

	if m.isBotAdmin(ctx) {
		m.logger.Info("Bot is Synapse server admin", "bot_mxid", m.as.BotMXID())
		return
	}

	secret := m.cfg.Matrix.RegistrationSecret
	if secret == "" {
		m.logger.Warn(
			"Bot is NOT a Synapse server admin and no SYNAPSE_REGISTRATION_SECRET configured. "+
				"To fix, either set SYNAPSE_REGISTRATION_SECRET or run: "+
				"UPDATE users SET admin = 1 WHERE name = '"+m.as.BotMXID().String()+"'; "+
				"then restart Synapse.",
			"bot_mxid", m.as.BotMXID(),
		)
		return
	}

	m.logger.Info("Bot is not a server admin, bootstrapping via shared secret...")

	// Try registering the bot itself as admin (works on fresh deployments)
	botLocalpart := m.cfg.Matrix.BotActorID
	_, err := m.registerSharedSecretUser(ctx, secret, botLocalpart, "bot-"+secret[:8], true)
	if err == nil {
		m.logger.Info("Bot registered as server admin via shared secret")
		return
	}
	m.logger.Info("Bot user already exists, promoting via temp admin...")

	// Bot exists but isn't admin — bootstrap via temp admin user
	if err := m.promoteViaTemporaryAdmin(ctx, secret); err != nil {
		m.logger.Warn("Failed to bootstrap bot admin",
			"error", err, "bot_mxid", m.as.BotMXID())
	} else {
		m.logger.Info("Bot promoted to server admin", "bot_mxid", m.as.BotMXID())
	}
}

// waitForSynapse retries connecting to Synapse until it responds or context is cancelled.
func (m *MautrixAdapter) waitForSynapse(ctx context.Context) {
	client := m.newDirectClient("")
	urlPath := client.BuildClientURL("v3", "login")

	for {
		_, err := client.MakeRequest(ctx, http.MethodGet, urlPath, nil, nil)
		if err == nil {
			return
		}

		// Check if context was cancelled
		if ctx.Err() != nil {
			m.logger.Error("Context cancelled while waiting for Synapse")
			return
		}

		m.logger.Info("Waiting for Synapse to become available...", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// isBotAdmin checks if the bot is a Synapse server admin by querying its own user record.
// Uses a direct HTTP client (not appservice framework) so it works before as.Start().
func (m *MautrixAdapter) isBotAdmin(ctx context.Context) bool {
	client := m.newDirectClient(m.cfg.Matrix.AppServiceToken)
	botMXID := m.as.BotMXID()
	var resp struct {
		Admin bool `json:"admin"`
	}
	urlPath := client.BuildURL(mautrix.SynapseAdminURLPath{"v2", "users", botMXID})
	_, err := client.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp)
	if err != nil {
		return false
	}
	return resp.Admin
}

// promoteViaTemporaryAdmin creates a temporary admin user, uses it to promote
// the bot to server admin, then deactivates the temporary user.
func (m *MautrixAdapter) promoteViaTemporaryAdmin(ctx context.Context, secret string) error {
	// Use a unique username each time to avoid conflicts with deactivated leftover users
	bootstrapUser := fmt.Sprintf("alkemio-bootstrap-%d", time.Now().UnixMilli())
	bootstrapPass := "bootstrap-" + secret[:8]

	// Create temporary admin
	adminToken, err := m.registerSharedSecretUser(ctx, secret, bootstrapUser, bootstrapPass, true)
	if err != nil {
		return fmt.Errorf("failed to register bootstrap admin: %w", err)
	}

	adminClient := m.newDirectClient(adminToken)

	// Promote bot
	botMXID := m.as.BotMXID()
	urlPath := adminClient.BuildURL(mautrix.SynapseAdminURLPath{"v2", "users", botMXID})
	_, err = adminClient.MakeRequest(ctx, http.MethodPut, urlPath, map[string]interface{}{
		"admin": true,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to promote bot: %w", err)
	}

	// Deactivate and erase temporary admin
	bootstrapMXID := "@" + bootstrapUser + ":" + m.cfg.Matrix.HomeserverName
	urlPath = adminClient.BuildURL(mautrix.SynapseAdminURLPath{"v1", "deactivate", bootstrapMXID})
	_, err = adminClient.MakeRequest(ctx, http.MethodPost, urlPath, map[string]interface{}{
		"erase": true,
	}, nil)
	if err != nil {
		m.logger.Warn("Failed to deactivate bootstrap admin user",
			"bootstrap_mxid", bootstrapMXID, "error", err)
	} else {
		m.logger.Info("Bootstrap admin user deactivated", "bootstrap_mxid", bootstrapMXID)
	}

	return nil
}

// newDirectClient creates a mautrix.Client that talks directly to Synapse
// without appservice impersonation. Uses config URL so it works before as.Start().
func (m *MautrixAdapter) newDirectClient(accessToken string) *mautrix.Client {
	hsURL, _ := url.Parse(m.cfg.Matrix.HomeserverURL)
	return &mautrix.Client{
		HomeserverURL: hsURL,
		AccessToken:   accessToken,
		Client:        http.DefaultClient,
	}
}

// registerSharedSecretUser registers a user via Synapse's shared secret API.
// Uses HMAC-SHA1 as required by the Synapse registration endpoint.
func (m *MautrixAdapter) registerSharedSecretUser(
	ctx context.Context, secret, username, password string, admin bool,
) (string, error) {
	client := m.newDirectClient("")

	// Get nonce
	var nonceResp struct {
		Nonce string `json:"nonce"`
	}
	urlPath := client.BuildURL(mautrix.SynapseAdminURLPath{"v1", "register"})
	_, err := client.MakeRequest(ctx, http.MethodGet, urlPath, nil, &nonceResp)
	if err != nil {
		return "", fmt.Errorf("failed to get registration nonce: %w", err)
	}

	// Compute HMAC-SHA1: nonce + \0 + username + \0 + password + \0 + "admin"|"notadmin"
	adminStr := "notadmin"
	if admin {
		adminStr = "admin"
	}
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(nonceResp.Nonce + "\x00" + username + "\x00" + password + "\x00" + adminStr))
	macHex := hex.EncodeToString(mac.Sum(nil))

	// Register
	var regResp struct {
		AccessToken string `json:"access_token"`
	}
	_, err = client.MakeRequest(ctx, http.MethodPost, urlPath, map[string]interface{}{
		"nonce":    nonceResp.Nonce,
		"username": username,
		"password": password,
		"admin":    admin,
		"mac":      macHex,
	}, &regResp)
	if err != nil {
		return "", fmt.Errorf("failed to register user: %w", err)
	}

	return regResp.AccessToken, nil
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

// ============================================================================
// User Operations
// ============================================================================

// EnsureUser provisions a user on the homeserver if it doesn't exist
func (m *MautrixAdapter) EnsureUser(ctx context.Context, actor domain.Actor) (id.UserID, error) {
	// Use centralized IDMapper for consistent user ID construction
	userID := m.idMapper.UserID(actor.ID)

	// Check if user exists (intent)
	intent := m.as.Intent(userID)

	// Register if needed
	if err := intent.EnsureRegistered(ctx); err != nil {
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

// InviteUser invites a user to a room and auto-joins them.
// Since all users are appservice ghosts, they are automatically joined after invitation.
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

	inviterIntent := m.as.Intent(inviterUserID)
	_, err = inviterIntent.InviteUser(
		ctx, roomID, &mautrix.ReqInviteUser{
			UserID: inviteeUserID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to invite user: %w", err)
	}

	// Auto-join the invitee since they are an appservice ghost user
	inviteeIntent := m.as.Intent(inviteeUserID)
	if err := inviteeIntent.EnsureJoined(ctx, roomID); err != nil {
		return fmt.Errorf("failed to join room after invite: %w", err)
	}

	// Mark room as read to clear invite notification from unread count
	// Get latest event once, then send receipt for this user
	if latestEventID, err := m.getLatestEventID(ctx, roomID); err != nil {
		m.logger.Warn("Failed to get latest event for read receipt",
			"room_id", roomID,
			"error", err,
		)
	} else {
		m.markRoomAsReadForUsers(ctx, roomID, []id.UserID{inviteeUserID}, latestEventID)
	}

	return nil
}

// getLatestEventID retrieves the latest event ID in a room.
// Used to mark rooms as read after joining.
func (m *MautrixAdapter) getLatestEventID(
	ctx context.Context, roomID id.RoomID,
) (id.EventID, error) {
	// Use bot intent to get latest event (any user can read room state)
	intent := m.as.BotIntent()
	resp, err := intent.Messages(ctx, roomID, "", "", mautrix.DirectionBackward, nil, 1)
	if err != nil {
		return "", fmt.Errorf("failed to get latest event: %w", err)
	}

	if len(resp.Chunk) == 0 {
		return "", nil // No events in room yet
	}

	return resp.Chunk[0].ID, nil
}

// markAsRead sends m.read receipt (triggers EDU) and sets m.fully_read marker (queryable)
// for a single user in a room. This is the single source of truth for marking messages as read.
func (m *MautrixAdapter) markAsRead(
	ctx context.Context, intent *appservice.IntentAPI, roomID id.RoomID, eventID id.EventID,
) {
	if err := intent.SendReceipt(ctx, roomID, eventID, event.ReceiptTypeRead, nil); err != nil {
		m.logger.Warn("Failed to send m.read receipt",
			"room_id", roomID, "event_id", eventID, "error", err)
	}
	if err := intent.SetReadMarkers(ctx, roomID, &mautrix.ReqSetReadMarkers{
		FullyRead: eventID,
	}); err != nil {
		m.logger.Warn("Failed to set m.fully_read marker",
			"room_id", roomID, "event_id", eventID, "error", err)
	}
}

// markRoomAsReadForUsers marks a room as read for multiple users using a single event ID.
// Used for room/space creation to clear invite notifications for all members.
func (m *MautrixAdapter) markRoomAsReadForUsers(
	ctx context.Context, roomID id.RoomID, userIDs []id.UserID, eventID id.EventID,
) {
	if eventID == "" {
		return
	}

	for _, userID := range userIDs {
		m.markAsRead(ctx, m.as.Intent(userID), roomID, eventID)
	}

	m.logger.Debug("Marked room as read for users after join",
		"room_id", roomID,
		"event_id", eventID,
		"user_count", len(userIDs),
	)
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

// ============================================================================
// Room Operations
// ============================================================================

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

	var name, topic, alias, avatarURL string

	var nameContent event.RoomNameEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateRoomName, "", &nameContent); err == nil {
		name = nameContent.Name
	}

	var topicContent event.TopicEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateTopic, "", &topicContent); err == nil {
		topic = topicContent.Topic
	}

	var avatarContent event.RoomAvatarEventContent
	if err := intent.StateEvent(ctx, roomID, event.StateRoomAvatar, "", &avatarContent); err == nil {
		avatarURL = string(avatarContent.URL)
	}

	// Get alias (uses room_aliases table - same source as ResolveAlias)
	if aliases, err := m.GetRoomAliases(ctx, roomID); err == nil {
		alias = m.selectPreferredAlias(aliases)
	}

	return &domain.Room{
		ID:        roomID,
		Name:      name,
		Topic:     topic,
		AvatarURL: avatarURL,
		Alias:     alias,
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
// nil pointers mean "no change"; non-nil (including empty string) means "set this value".
func (m *MautrixAdapter) UpdateRoomState(
	ctx context.Context, roomID id.RoomID, actorID domain.Actor, name, topic, avatarURL, joinRule *string,
) error {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	if name != nil {
		if err := m.setOrRedactState(ctx, intent, roomID, event.StateRoomName, *name); err != nil {
			return fmt.Errorf("failed to set room name: %w", err)
		}
	}
	if topic != nil {
		if err := m.setOrRedactState(ctx, intent, roomID, event.StateTopic, *topic); err != nil {
			return fmt.Errorf("failed to set room topic: %w", err)
		}
	}
	if avatarURL != nil {
		if err := m.setOrRedactState(ctx, intent, roomID, event.StateRoomAvatar, *avatarURL); err != nil {
			return fmt.Errorf("failed to set room avatar: %w", err)
		}
	}
	if joinRule != nil && *joinRule != "" {
		joinRuleContent := &event.JoinRulesEventContent{
			JoinRule: event.JoinRule(*joinRule),
		}
		if _, err := intent.SendStateEvent(ctx, roomID, event.StateJoinRules, "", joinRuleContent); err != nil {
			return fmt.Errorf("failed to set room join rule: %w", err)
		}
	}
	return nil
}

// setOrRedactState sets a state event value, or redacts the existing state event
// if the value is empty. Redacting (instead of setting to empty) ensures clients
// fall back to their default behavior (e.g. showing member names for unnamed rooms).
func (m *MautrixAdapter) setOrRedactState(
	ctx context.Context, intent *appservice.IntentAPI, roomID id.RoomID, eventType event.Type, value string,
) error {
	if value != "" {
		var content interface{}
		switch eventType {
		case event.StateRoomName:
			content = map[string]interface{}{"name": value}
		case event.StateTopic:
			content = map[string]interface{}{"topic": value}
		case event.StateRoomAvatar:
			content = &event.RoomAvatarEventContent{URL: id.ContentURIString(value)}
		default:
			content = map[string]interface{}{}
		}
		_, err := intent.SendStateEvent(ctx, roomID, eventType, "", content)
		return err
	}

	// Value is empty — redact the current state event to fully remove it.
	// In Matrix, state events are keyed by (type, state_key), so there is
	// only one "current" state event. Redacting it removes the property
	// entirely, causing clients to fall back to defaults (e.g. member names for unnamed DMs).
	existing, err := intent.FullStateEvent(ctx, roomID, eventType, "")
	if err != nil || existing == nil || existing.ID == "" {
		// No existing state event — nothing to redact
		return nil
	}
	_, err = intent.RedactEvent(ctx, roomID, existing.ID)
	return err
}

// ============================================================================
// Message & Reaction Operations
// ============================================================================

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

	// Try to parse using generic helper
	if content, ok := parseEventContent[event.MessageEventContent](evt); ok {
		msg := &domain.Message{
			ID:             evt.ID.String(),
			RoomID:         roomID.String(),
			Content:        content.Body,
			SenderMatrixID: evt.Sender.String(),
			Timestamp:      time.UnixMilli(evt.Timestamp),
		}
		// Extract thread ID from RelatesTo (same logic as fallback path)
		if content.RelatesTo != nil && content.RelatesTo.InReplyTo != nil {
			msg.ThreadID = content.RelatesTo.InReplyTo.EventID.String()
		}
		return msg, nil
	}

	// Fallback: try raw JSON body
	return m.parseMessageFromRaw(evt, roomID)
}

// parseMessageFromRaw extracts a message from raw event content as a fallback.
func (m *MautrixAdapter) parseMessageFromRaw(evt *event.Event, roomID id.RoomID) (*domain.Message, error) {
	rawBody, ok := evt.Content.Raw["body"].(string)
	if !ok {
		return nil, fmt.Errorf("event is not a message")
	}

	msg := &domain.Message{
		ID:             evt.ID.String(),
		RoomID:         roomID.String(),
		Content:        rawBody,
		SenderMatrixID: evt.Sender.String(),
		Timestamp:      time.UnixMilli(evt.Timestamp),
	}

	// Try to extract thread info from raw content
	msg.ThreadID = m.extractThreadIDFromRaw(evt)

	return msg, nil
}

// extractThreadIDFromRaw extracts thread ID from raw event content.
func (m *MautrixAdapter) extractThreadIDFromRaw(evt *event.Event) string {
	relatesTo, ok := evt.Content.Raw["m.relates_to"].(map[string]interface{})
	if !ok {
		return ""
	}
	inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{})
	if !ok {
		return ""
	}
	threadID, _ := inReplyTo["event_id"].(string)
	return threadID
}

// RespRelations defines the response for relations endpoint
type RespRelations struct {
	Chunk []event.Event `json:"chunk"`
}

// relationsURLFormat is the format string for the Matrix relations API endpoint.
const relationsURLFormat = "%s/_matrix/client/v1/rooms/%s/relations/%s/%s/%s"

// buildRelationsURL constructs a URL for the Matrix relations API.
func (m *MautrixAdapter) buildRelationsURL(
	intent *appservice.IntentAPI, roomID id.RoomID, eventID id.EventID, relType event.RelationType,
	eventType event.Type,
) string {
	hsURL := strings.TrimSuffix(intent.HomeserverURL.String(), "/")
	return fmt.Sprintf(relationsURLFormat, hsURL, roomID, eventID, relType, eventType)
}

// parseEventContent attempts to parse event content, trying Parsed first then ParseRaw.
// Returns the parsed content and true if successful, nil and false otherwise.
func parseEventContent[T any](evt *event.Event) (*T, bool) {
	// Try parsed content first
	if content, ok := evt.Content.Parsed.(*T); ok {
		return content, true
	}

	// Try parsing raw content
	if err := evt.Content.ParseRaw(evt.Type); err != nil {
		return nil, false
	}

	content, ok := evt.Content.Parsed.(*T)
	return content, ok
}

// GetReactionEventID finds the event ID of a reaction.
func (m *MautrixAdapter) GetReactionEventID(
	ctx context.Context, roomID id.RoomID, eventID id.EventID, emoji string, senderID domain.Actor,
) (id.EventID, error) {
	intent := m.as.BotIntent()
	u := m.buildRelationsURL(intent, roomID, eventID, event.RelAnnotation, event.EventReaction)

	var resp RespRelations
	_, err := intent.MakeRequest(ctx, "GET", u, nil, &resp)
	if err != nil {
		return "", fmt.Errorf("failed to get relations: %w", err)
	}

	senderUserID, err := m.EnsureUser(ctx, senderID)
	if err != nil {
		return "", err
	}

	return m.findReactionByEmojiAndSender(resp.Chunk, senderUserID, emoji)
}

// findReactionByEmojiAndSender searches for a specific reaction in a list of events.
func (m *MautrixAdapter) findReactionByEmojiAndSender(
	events []event.Event, senderUserID id.UserID, emoji string,
) (id.EventID, error) {
	for _, evt := range events {
		if evt.Sender != senderUserID || evt.Type != event.EventReaction {
			continue
		}
		content, ok := parseEventContent[event.ReactionEventContent](&evt)
		if ok && content.RelatesTo.Key == emoji {
			return evt.ID, nil
		}
	}
	return "", fmt.Errorf("reaction not found")
}

// ============================================================================
// Helpers & Utilities
// ============================================================================

// HomeserverDomain returns the homeserver domain for room alias construction.
func (m *MautrixAdapter) HomeserverDomain() string {
	return m.as.HomeserverDomain
}

// Router returns the AppService HTTP router for registering custom endpoints.
// Custom endpoints will be served on the same port as the AppService transaction API.
func (m *MautrixAdapter) Router() *http.ServeMux {
	return m.as.Router
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
// Direction: Alias -> Room ID (uses room_aliases table)
func (m *MautrixAdapter) ResolveAlias(ctx context.Context, alias string) (id.RoomID, error) {
	intent := m.as.BotIntent()
	resp, err := intent.ResolveAlias(ctx, id.RoomAlias(alias))
	if err != nil {
		return "", fmt.Errorf("failed to resolve alias %s: %w", alias, err)
	}
	return resp.RoomID, nil
}

// GetRoomAliases gets all aliases for a room ID.
// The bot must be a Synapse server admin to query rooms it has left.
func (m *MautrixAdapter) GetRoomAliases(ctx context.Context, roomID id.RoomID) ([]string, error) {
	intent := m.as.BotIntent()
	aliasResp, err := intent.GetAliases(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases for room %s: %w", roomID, err)
	}
	aliases := make([]string, len(aliasResp.Aliases))
	for i, a := range aliasResp.Aliases {
		aliases[i] = string(a)
	}
	return aliases, nil
}

// selectPreferredAlias selects the preferred alias from a list.
// Prefers Alkemio UUID-formatted aliases, falls back to first alias if none match.
func (m *MautrixAdapter) selectPreferredAlias(aliases []string) string {
	if len(aliases) == 0 {
		return ""
	}

	// Prefer Alkemio-patterned alias
	for _, alias := range aliases {
		if m.idMapper.AlkemioRoomID(alias) != uuid.Nil {
			return alias
		}
	}

	// Fallback to first alias
	return aliases[0]
}

// SetRoomDirectoryVisibility sets whether a room appears in the public room directory.
// Uses PUT /_matrix/client/v3/directory/list/room/{roomId}.
func (m *MautrixAdapter) SetRoomDirectoryVisibility(ctx context.Context, roomID id.RoomID, isPublic bool) error {
	// TODO: Currently returns 403 from Synapse — needs investigation
	intent := m.as.BotIntent()
	visibility := "private"
	if isPublic {
		visibility = "public"
	}
	urlPath := intent.BuildClientURL("v3", "directory", "list", "room", roomID)
	_, err := intent.MakeRequest(ctx, http.MethodPut, urlPath, map[string]string{"visibility": visibility}, nil)
	if err != nil {
		return fmt.Errorf("failed to set room directory visibility: %w", err)
	}
	return nil
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
	_, err := intent.KickUser(
		ctx, roomID, &mautrix.ReqKickUser{
			UserID: userID,
			Reason: reason,
		},
	)
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
		m.logger.Warn(
			"Room has 1000+ messages, pagination may be needed in future", "room_id", roomID, "count", len(messages),
		)
	}

	return messages, nil
}

// lastMessageStats tracks how many events were needed to find the last message.
// Used for observability and future optimization of batch sizes.
type lastMessageStats struct {
	sync.Mutex
	// Histogram buckets: events needed to find message (0-5, 6-10, 11-20, 21-50, 51-200, not found)
	bucket5     int64
	bucket10    int64
	bucket20    int64
	bucket50    int64
	bucket200   int64
	notFound    int64
	totalCalls  int64
	logInterval int64
}

var lastMsgStats = &lastMessageStats{logInterval: 100}

func (s *lastMessageStats) record(eventsNeeded int, found bool, logger ports.Logger) {
	s.Lock()
	defer s.Unlock()

	s.totalCalls++

	switch {
	case !found:
		s.notFound++
	case eventsNeeded <= 5:
		s.bucket5++
	case eventsNeeded <= 10:
		s.bucket10++
	case eventsNeeded <= 20:
		s.bucket20++
	case eventsNeeded <= 50:
		s.bucket50++
	default:
		s.bucket200++
	}

	// Log stats periodically
	if s.totalCalls%s.logInterval == 0 {
		logger.Info("GetLastMessage stats",
			"total_calls", s.totalCalls,
			"bucket_0-5", s.bucket5,
			"bucket_6-10", s.bucket10,
			"bucket_11-20", s.bucket20,
			"bucket_21-50", s.bucket50,
			"bucket_51-200", s.bucket200,
			"not_found", s.notFound,
		)
	}
}

// GetLastMessage retrieves the most recent message in a room.
// Returns nil if the room has no messages.
func (m *MautrixAdapter) GetLastMessage(ctx context.Context, roomID id.RoomID) (*domain.Message, error) {
	intent := m.as.BotIntent()

	// Progressive fetch: start small, expand if needed
	// In most cases, the last message is within the first few events
	batchSizes := []int{5, 10, 20, 50, 200}
	var allEvents []*event.Event
	var from string

	for _, batchSize := range batchSizes {
		resp, err := intent.Messages(ctx, roomID, from, "", mautrix.DirectionBackward, nil, batchSize)
		if err != nil {
			return nil, fmt.Errorf("failed to get last message: %w", err)
		}

		allEvents = append(allEvents, resp.Chunk...)

		// Check if we found a message in this batch
		for i, evt := range resp.Chunk {
			if evt.Type == event.EventMessage {
				// Record stats: total events scanned = previous batches + position in current batch
				eventsScanned := len(allEvents) - len(resp.Chunk) + i + 1
				lastMsgStats.record(eventsScanned, true, m.logger)
				// Found a message - collect reactions from all fetched events and return
				return m.buildLastMessageWithReactions(allEvents, roomID)
			}
		}

		// No message found yet - continue with next batch if there are more events
		if resp.End == "" || len(resp.Chunk) < batchSize {
			break // No more events to fetch
		}
		from = resp.End
	}

	lastMsgStats.record(len(allEvents), false, m.logger)
	return nil, nil // No messages in room
}

// buildLastMessageWithReactions finds the last message and attaches its reactions.
func (m *MautrixAdapter) buildLastMessageWithReactions(events []*event.Event, roomID id.RoomID) (*domain.Message, error) {
	// Find the first m.room.message event
	var msg *domain.Message
	for _, evt := range events {
		if evt.Type == event.EventMessage {
			msg = m.parseMessageEvent(evt, roomID)
			if msg != nil {
				msg.Reactions = []domain.Reaction{}
				break
			}
		}
	}

	if msg == nil {
		return nil, nil
	}

	// Collect reactions for this message
	for _, evt := range events {
		if evt.Type != event.EventReaction {
			continue
		}

		reaction := m.parseReactionEvent(evt, roomID)
		if reaction != nil && reaction.MessageID.String() == msg.ID {
			msg.Reactions = append(msg.Reactions, *reaction)
		}
	}

	return msg, nil
}

// maxConcurrentLastMessageFetches limits parallel requests to avoid overwhelming the homeserver.
const maxConcurrentLastMessageFetches = 10

// GetBatchLastMessages retrieves the most recent message for multiple rooms in parallel.
// Limits concurrency to avoid overwhelming the homeserver with too many simultaneous requests.
func (m *MautrixAdapter) GetBatchLastMessages(
	ctx context.Context, roomIDs []id.RoomID,
) (map[id.RoomID]*domain.Message, map[id.RoomID]error) {
	results := make(map[id.RoomID]*domain.Message)
	errors := make(map[id.RoomID]error)

	if len(roomIDs) == 0 {
		return results, errors
	}

	// Use channels to collect results from goroutines
	type result struct {
		roomID id.RoomID
		msg    *domain.Message
		err    error
	}
	resultCh := make(chan result, len(roomIDs))

	// Semaphore to limit concurrent requests
	sem := make(chan struct{}, maxConcurrentLastMessageFetches)

	// Launch parallel goroutines for each room (bounded by semaphore)
	var wg sync.WaitGroup
	for _, roomID := range roomIDs {
		wg.Add(1)
		go func(rid id.RoomID) {
			defer wg.Done()
			// Acquire semaphore with context awareness
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultCh <- result{roomID: rid, err: ctx.Err()}
				return
			}
			msg, err := m.GetLastMessage(ctx, rid)
			resultCh <- result{roomID: rid, msg: msg, err: err}
		}(roomID)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	for r := range resultCh {
		if r.err != nil {
			errors[r.roomID] = r.err
		} else {
			results[r.roomID] = r.msg
		}
	}

	m.logger.Debug(
		"Retrieved batch last messages",
		"rooms_requested", len(roomIDs),
		"rooms_found", len(results),
		"rooms_failed", len(errors),
	)

	return results, errors
}

// parseReactionEvent extracts a domain.Reaction from a Matrix reaction event.
func (m *MautrixAdapter) parseReactionEvent(evt *event.Event, roomID id.RoomID) *domain.Reaction {
	content, ok := parseEventContent[event.ReactionEventContent](evt)
	if !ok || content.RelatesTo.EventID == "" {
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

	// Check for thread relation (MSC3440) first, then fallback to m.in_reply_to
	if content, ok := evt.Content.Parsed.(*event.MessageEventContent); ok && content.RelatesTo != nil {
		// Check for explicit m.thread relation first
		if content.RelatesTo.Type == "m.thread" && content.RelatesTo.EventID != "" {
			msg.ThreadID = content.RelatesTo.EventID.String()
		} else if content.RelatesTo.InReplyTo != nil {
			// Fallback to m.in_reply_to for older clients
			msg.ThreadID = content.RelatesTo.InReplyTo.EventID.String()
		}
	}

	return msg
}

// extractMessageBody gets the message body from an event, trying multiple approaches.
func (m *MautrixAdapter) extractMessageBody(evt *event.Event) string {
	// Try the generic parser first
	if content, ok := parseEventContent[event.MessageEventContent](evt); ok {
		return content.Body
	}

	// Try raw JSON as last resort
	if rawBody, ok := evt.Content.Raw["body"].(string); ok {
		return rawBody
	}

	return ""
}

// GetReaction retrieves details of a specific reaction.
func (m *MautrixAdapter) GetReaction(ctx context.Context, roomID id.RoomID, reactionID id.EventID) (
	*domain.Reaction, error,
) {
	intent := m.as.BotIntent()

	evt, err := intent.GetEvent(ctx, roomID, reactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reaction event: %w", err)
	}

	if evt.Type != event.EventReaction {
		return nil, fmt.Errorf("event is not a reaction")
	}

	content, ok := parseEventContent[event.ReactionEventContent](evt)
	if !ok {
		return nil, fmt.Errorf("failed to parse reaction content")
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

// GetThreadMessages retrieves all messages in a thread, including the thread root.
func (m *MautrixAdapter) GetThreadMessages(
	ctx context.Context, roomID id.RoomID, threadRootID id.EventID,
) ([]domain.Message, error) {
	intent := m.as.BotIntent()

	// First, get the thread root message
	rootEvt, err := intent.GetEvent(ctx, roomID, threadRootID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread root message: %w", err)
	}

	messages := make([]domain.Message, 0)

	// Parse the root message
	rootMsg := m.parseMessageEvent(rootEvt, roomID)
	if rootMsg != nil {
		rootMsg.Reactions = []domain.Reaction{} // Initialize empty slice
		messages = append(messages, *rootMsg)
	}

	// Get thread replies using relations API
	u := m.buildRelationsURL(intent, roomID, threadRootID, event.RelThread, event.EventMessage)

	var resp RespRelations
	_, err = intent.MakeRequest(ctx, "GET", u, nil, &resp)
	if err != nil {
		// If no relations found, return just the root message
		m.logger.Debug("No thread relations found, returning only root", "thread_root_id", threadRootID)
		return messages, nil
	}

	// Parse thread reply messages
	for _, evt := range resp.Chunk {
		if evt.Type != event.EventMessage {
			continue
		}

		msg := m.parseMessageEvent(&evt, roomID)
		if msg != nil {
			msg.Reactions = []domain.Reaction{} // Initialize empty slice
			msg.ThreadID = threadRootID.String()
			messages = append(messages, *msg)
		}
	}

	return messages, nil
}

// CreateRoomWithAlias creates a new room with a specific alias based on Alkemio room ID.
func (m *MautrixAdapter) CreateRoomWithAlias(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	roomType string,
	name, topic, avatarURL, joinRule string,
	initialMembers []domain.Actor,
) (id.RoomID, error) {
	// Use IDMapper for consistent alias construction
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

	// When an explicit joinRule is provided for non-direct rooms,
	// use private_chat preset and let the join_rules state event control visibility
	if joinRule != "" && !isDirect {
		preset = "private_chat"
	}

	req := &mautrix.ReqCreateRoom{
		Name:     name,
		Topic:    topic,
		Preset:   preset,
		IsDirect: isDirect,
		Invite:   invites,
	}

	// Add join rule state event if provided (following CreateSpace pattern)
	if joinRule != "" {
		joinRuleContent := &event.JoinRulesEventContent{
			JoinRule: event.JoinRule(joinRule),
		}
		req.InitialState = append(req.InitialState, &event.Event{
			Type:    event.StateJoinRules,
			Content: event.Content{Parsed: joinRuleContent},
		})
	}

	// Add avatar state event if provided
	if avatarURL != "" {
		avatarContent := &event.RoomAvatarEventContent{
			URL: id.ContentURIString(avatarURL),
		}
		req.InitialState = append(req.InitialState, &event.Event{
			Type:    event.StateRoomAvatar,
			Content: event.Content{Parsed: avatarContent},
		})
	}

	resp, err := intent.CreateRoom(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create room with alias: %w", err)
	}

	// Set alias manually after creation (without canonical alias).
	// Using RoomAliasName in the create request auto-sets canonical alias,
	// which causes clients to display the alias instead of member names for DMs.
	fullAlias := m.idMapper.RoomAlias(alkemioRoomID)
	if _, err := intent.CreateAlias(ctx, id.RoomAlias(fullAlias), resp.RoomID); err != nil {
		m.logger.Warn("Failed to set alias on room",
			"room_id", resp.RoomID, "alias", fullAlias, "error", err)
	}

	m.autoJoinAndMarkRead(ctx, resp.RoomID, invites)

	// Bot leaves the room — the appservice still receives events via ghost users
	// matching the user namespace. This keeps the bot out of member lists.
	if _, err := intent.LeaveRoom(ctx, resp.RoomID); err != nil {
		m.logger.Warn("Failed to leave room after creation",
			"room_id", resp.RoomID, "error", err)
	}

	m.logger.Info(
		"Room created",
		"room_id", resp.RoomID,
		"alias", fullAlias,
		"alkemio_room_id", alkemioRoomID,
	)

	return resp.RoomID, nil
}

// autoJoinAndMarkRead auto-joins invited members and marks the room as read for them.
func (m *MautrixAdapter) autoJoinAndMarkRead(ctx context.Context, roomID id.RoomID, invites []id.UserID) {
	joinedUsers := make([]id.UserID, 0, len(invites))
	for _, memberUserID := range invites {
		memberIntent := m.as.Intent(memberUserID)
		if err := memberIntent.EnsureJoined(ctx, roomID); err != nil {
			m.logger.Warn("Failed to auto-join member to room",
				"room_id", roomID, "user_id", memberUserID, "error", err)
		} else {
			joinedUsers = append(joinedUsers, memberUserID)
		}
	}

	if len(joinedUsers) > 0 {
		if latestEventID, err := m.getLatestEventID(ctx, roomID); err != nil {
			m.logger.Warn("Failed to get latest event for read receipts",
				"room_id", roomID, "error", err)
		} else {
			m.markRoomAsReadForUsers(ctx, roomID, joinedUsers, latestEventID)
		}
	}
}

// FindExistingDirectRoom finds an existing direct room between two users.
// It checks if user1 has a direct room where user2 is also a member.
// Returns the room ID if found, or empty string if no direct room exists.
func (m *MautrixAdapter) FindExistingDirectRoom(
	ctx context.Context,
	user1 domain.Actor,
	user2 domain.Actor,
) (id.RoomID, error) {
	user1ID, err := m.EnsureUser(ctx, user1)
	if err != nil {
		return "", fmt.Errorf("failed to ensure user1: %w", err)
	}
	user2ID, err := m.EnsureUser(ctx, user2)
	if err != nil {
		return "", fmt.Errorf("failed to ensure user2: %w", err)
	}

	// Get the direct rooms for user1 from their account data
	intent := m.as.Intent(user1ID)

	// Fetch the m.direct account data
	var directContent map[string][]string
	err = intent.GetAccountData(ctx, "m.direct", &directContent)
	if err != nil {
		// No m.direct data means no direct rooms
		m.logger.Debug("No m.direct account data for user", "user_id", user1ID)
		return "", nil
	}

	// Look for rooms with user2
	directRooms, exists := directContent[user2ID.String()]
	if !exists || len(directRooms) == 0 {
		return "", nil
	}

	// Check each direct room to find one where both users are members
	return m.findRoomWithBothUsers(ctx, directRooms, user1ID, user2ID)
}

// findRoomWithBothUsers checks a list of room IDs to find one where both users are members.
func (m *MautrixAdapter) findRoomWithBothUsers(
	ctx context.Context,
	roomIDs []string,
	user1ID, user2ID id.UserID,
) (id.RoomID, error) {
	for _, roomIDStr := range roomIDs {
		roomID := id.RoomID(roomIDStr)

		if m.roomContainsBothUsers(ctx, roomID, user1ID, user2ID) {
			m.logger.Info(
				"Found existing direct room between users",
				"room_id", roomID,
				"user1", user1ID,
				"user2", user2ID,
			)
			return roomID, nil
		}
	}
	return "", nil
}

// roomContainsBothUsers checks if a room contains both specified users.
func (m *MautrixAdapter) roomContainsBothUsers(
	ctx context.Context,
	roomID id.RoomID,
	user1ID, user2ID id.UserID,
) bool {
	members, err := m.GetRoomMembers(ctx, roomID)
	if err != nil {
		m.logger.Debug(
			"Failed to get members for direct room, skipping",
			"room_id", roomID, "error", err,
		)
		return false
	}

	hasUser1, hasUser2 := false, false
	for _, member := range members {
		if member == user1ID {
			hasUser1 = true
		}
		if member == user2ID {
			hasUser2 = true
		}
		if hasUser1 && hasUser2 {
			return true
		}
	}
	return false
}

// SetRoomAlias sets a room alias for an existing room.
func (m *MautrixAdapter) SetRoomAlias(ctx context.Context, roomID id.RoomID, alias string) error {
	intent := m.as.BotIntent()

	// Add the alias to the room
	_, err := intent.CreateAlias(ctx, id.RoomAlias(alias), roomID)
	if err != nil {
		return fmt.Errorf("failed to create room alias: %w", err)
	}

	// Set it as the canonical alias
	content := event.CanonicalAliasEventContent{
		Alias: id.RoomAlias(alias),
	}
	_, err = intent.SendStateEvent(ctx, roomID, event.StateCanonicalAlias, "", &content)
	if err != nil {
		m.logger.Warn(
			"Failed to set canonical alias, alias was still created",
			"room_id", roomID, "alias", alias, "error", err,
		)
	}

	m.logger.Info("Room alias set", "room_id", roomID, "alias", alias)
	return nil
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
		initialState = append(
			initialState, &event.Event{
				Type:    event.StateJoinRules,
				Content: event.Content{Parsed: joinRuleContent},
			},
		)
	}

	// Add avatar state event if provided
	if avatarURL != "" {
		avatarContent := &event.RoomAvatarEventContent{
			URL: id.ContentURIString(avatarURL),
		}
		initialState = append(
			initialState, &event.Event{
				Type:    event.StateRoomAvatar,
				Content: event.Content{Parsed: avatarContent},
			},
		)
	}

	if len(initialState) > 0 {
		req.InitialState = initialState
	}

	resp, err := intent.CreateRoom(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create space: %w", err)
	}

	// Auto-join initial members (they were only invited, need to accept)
	joinedUsers := make([]id.UserID, 0, len(invites))
	for _, memberUserID := range invites {
		memberIntent := m.as.Intent(memberUserID)
		if err := memberIntent.EnsureJoined(ctx, resp.RoomID); err != nil {
			m.logger.Warn("Failed to auto-join member to space",
				"room_id", resp.RoomID,
				"user_id", memberUserID,
				"error", err,
			)
			// Continue with other members, don't fail the whole operation
		} else {
			joinedUsers = append(joinedUsers, memberUserID)
		}
	}

	// Mark space as read for all joined users to clear invite notifications
	// Optimized: get latest event once, send receipts for all users
	if len(joinedUsers) > 0 {
		if latestEventID, err := m.getLatestEventID(ctx, resp.RoomID); err != nil {
			m.logger.Warn("Failed to get latest event for read receipts",
				"room_id", resp.RoomID,
				"error", err,
			)
		} else {
			m.markRoomAsReadForUsers(ctx, resp.RoomID, joinedUsers, latestEventID)
		}
	}

	m.logger.Info(
		"Space created",
		"room_id", resp.RoomID,
		"alias", m.idMapper.SpaceAlias(alkemioContextID),
		"alkemio_context_id", alkemioContextID,
		"members_joined", len(joinedUsers),
	)

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

	// Get alias (uses room_aliases table - same source as ResolveAlias)
	if aliases, err := m.GetRoomAliases(ctx, roomID); err == nil {
		alias = m.selectPreferredAlias(aliases)
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
// nil pointers mean "no change"; non-nil (including empty string) means "set this value".
func (m *MautrixAdapter) UpdateSpaceState(
	ctx context.Context, roomID id.RoomID, name, topic, avatarURL, joinRule *string,
) error {
	intent := m.as.BotIntent()

	if name != nil {
		if err := m.setOrRedactState(ctx, intent, roomID, event.StateRoomName, *name); err != nil {
			return fmt.Errorf("failed to set space name: %w", err)
		}
	}

	if topic != nil {
		if err := m.setOrRedactState(ctx, intent, roomID, event.StateTopic, *topic); err != nil {
			return fmt.Errorf("failed to set space topic: %w", err)
		}
	}

	if avatarURL != nil {
		if err := m.setOrRedactState(ctx, intent, roomID, event.StateRoomAvatar, *avatarURL); err != nil {
			return fmt.Errorf("failed to set space avatar: %w", err)
		}
	}

	if joinRule != nil && *joinRule != "" {
		joinRuleContent := &event.JoinRulesEventContent{
			JoinRule: event.JoinRule(*joinRule),
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
		content, ok := parseEventContent[event.SpaceChildEventContent](evt)
		if !ok || len(content.Via) == 0 {
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
func (m *MautrixAdapter) AddSpaceChild(
	ctx context.Context, spaceID id.RoomID, childID id.RoomID, order string, suggested bool,
) error {
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

	botIntent := m.as.BotIntent()
	_, err = botIntent.InviteUser(
		ctx, spaceID, &mautrix.ReqInviteUser{
			UserID: inviteeUserID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to invite user to space: %w", err)
	}

	// Auto-join the invitee since they are an appservice ghost user
	inviteeIntent := m.as.Intent(inviteeUserID)
	if err := inviteeIntent.EnsureJoined(ctx, spaceID); err != nil {
		return fmt.Errorf("failed to join space after invite: %w", err)
	}

	// Mark space as read to clear invite notification from unread count
	if latestEventID, err := m.getLatestEventID(ctx, spaceID); err != nil {
		m.logger.Warn("Failed to get latest event for read receipt",
			"room_id", spaceID,
			"error", err,
		)
	} else {
		m.markRoomAsReadForUsers(ctx, spaceID, []id.UserID{inviteeUserID}, latestEventID)
	}

	return nil
}

// KickFromSpace kicks a user from a space.
func (m *MautrixAdapter) KickFromSpace(ctx context.Context, spaceID id.RoomID, userID id.UserID, reason string) error {
	return m.KickUser(ctx, spaceID, userID, reason)
}

// ============================================================================
// Read Receipt Operations (008-read-receipts)
// ============================================================================

// SendReadReceipt sends a read receipt for a message in a room.
// If threadRootID is provided, sends an m.read receipt with thread_id for thread-level tracking.
// Otherwise, sends a standard m.read receipt for room-level tracking.
func (m *MautrixAdapter) SendReadReceipt(
	ctx context.Context, actor domain.Actor, roomID id.RoomID, eventID id.EventID, threadRootID *id.EventID,
) error {
	// Use centralized IDMapper for user ID
	userID := m.idMapper.UserID(actor.ID)
	intent := m.as.Intent(userID)

	// Ensure user is registered
	if err := intent.EnsureRegistered(ctx); err != nil {
		return fmt.Errorf("failed to ensure user registered: %w", err)
	}

	if threadRootID != nil {
		// Thread-level: use SendReceipt with thread context (MSC3771)
		m.logger.Debug("Sending thread-level read receipt",
			"room_id", roomID, "event_id", eventID,
			"thread_root_id", *threadRootID, "user_id", userID)
		content := &mautrix.ReqSendReceipt{
			ThreadID: threadRootID.String(),
		}
		if err := intent.SendReceipt(ctx, roomID, eventID, event.ReceiptTypeRead, content); err != nil {
			return fmt.Errorf("failed to send thread read receipt: %w", err)
		}
	} else {
		// Room-level: send m.read receipt (triggers EDU) + set m.fully_read (queryable)
		m.logger.Debug("Sending room-level read receipt with read marker",
			"room_id", roomID, "event_id", eventID, "user_id", userID)
		m.markAsRead(ctx, intent, roomID, eventID)
	}

	return nil
}

// getFullyReadMarker queries the m.fully_read room account data for a user in a room.
// Returns nil if no marker is set (user has never marked anything as read in this room).
func (m *MautrixAdapter) getFullyReadMarker(
	ctx context.Context, intent *appservice.IntentAPI, roomID id.RoomID,
) *id.EventID {
	var result struct {
		EventID id.EventID `json:"event_id"`
	}
	err := intent.GetRoomAccountData(ctx, roomID, "m.fully_read", &result)
	if err != nil {
		m.logger.Debug("No m.fully_read marker found for room",
			"room_id", roomID, "error", err)
		return nil
	}
	if result.EventID == "" {
		return nil
	}
	m.logger.Debug("Found m.fully_read marker",
		"room_id", roomID, "event_id", result.EventID)
	return &result.EventID
}

// countUnreadMessages counts m.room.message events backward from the latest event,
// stopping when the receipt event ID is found or 200 events have been scanned.
// If receiptEventID is nil, counts ALL non-self messages (for rooms with no receipt).
// Returns (count, true) if receipt found or all events scanned.
// Returns (count-so-far, false) if 200-event cap hit without finding receipt.
func (m *MautrixAdapter) countUnreadMessages(
	ctx context.Context, intent *appservice.IntentAPI, roomID id.RoomID,
	receiptEventID *id.EventID, userID id.UserID,
) (int, bool) {
	if receiptEventID == nil {
		m.logger.Debug("No read receipt found for user in room, counting all messages",
			"room_id", roomID, "user_id", userID)
	}

	batchSizes := []int{5, 10, 20, 50, 200}
	var from string
	count := 0
	totalScanned := 0

	for _, batchSize := range batchSizes {
		select {
		case <-ctx.Done():
			return count, false
		default:
		}

		resp, err := intent.Messages(ctx, roomID, from, "", mautrix.DirectionBackward, nil, batchSize)
		if err != nil {
			m.logger.Error("Failed to fetch messages for unread count",
				"error", err, "room_id", roomID, "batch_size", batchSize)
			return count, false
		}

		for _, evt := range resp.Chunk {
			totalScanned++
			// Check if this is the receipt target
			if receiptEventID != nil && evt.ID == *receiptEventID {
				m.logger.Debug("Found read receipt position",
					"room_id", roomID,
					"receipt_event_id", *receiptEventID,
					"events_scanned", totalScanned,
					"unread_count", count,
				)
				return count, true
			}
			// Count message events not from the user
			if evt.Type == event.EventMessage && evt.Sender != userID {
				count++
			}
		}

		// No more events to fetch
		if resp.End == "" || len(resp.Chunk) < batchSize {
			m.logger.Debug("Scanned all available events",
				"room_id", roomID,
				"events_scanned", totalScanned,
				"unread_count", count,
				"has_receipt", receiptEventID != nil,
			)
			return count, true
		}
		from = resp.End
	}

	// Progressive batch cap hit without finding receipt (max ~285 events across all batches)
	m.logger.Debug("Reached progressive fetch cap without finding receipt",
		"room_id", roomID,
		"receipt_event_id", receiptEventID,
		"events_scanned", totalScanned,
		"partial_count", count,
	)
	return count, false
}

// bootstrapFullyReadMarker sets m.fully_read to the latest message in the room.
// Called when m.fully_read doesn't exist and Synapse reports 0 unread,
// so future queries can use self-calculation.
func (m *MautrixAdapter) bootstrapFullyReadMarker(
	ctx context.Context, intent *appservice.IntentAPI, roomID id.RoomID,
) {
	// Fetch the latest message to get its event ID
	resp, err := intent.Messages(ctx, roomID, "", "", mautrix.DirectionBackward, nil, 1)
	if err != nil || len(resp.Chunk) == 0 {
		return
	}
	latestEventID := resp.Chunk[0].ID
	if err := intent.SetReadMarkers(ctx, roomID, &mautrix.ReqSetReadMarkers{
		FullyRead: latestEventID,
	}); err != nil {
		m.logger.Warn("Failed to bootstrap m.fully_read marker",
			"error", err, "room_id", roomID, "event_id", latestEventID)
		return
	}
	m.logger.Debug("Bootstrapped m.fully_read marker from latest event",
		"room_id", roomID, "event_id", latestEventID)
}

// extractSynapseNotificationCount extracts the notification count from a sync joined room.
// Prefers MSC2654 unread count over standard notification count.
func extractSynapseNotificationCount(joinedRoom *mautrix.SyncJoinedRoom) int {
	if joinedRoom == nil {
		return 0
	}
	count := 0
	if joinedRoom.UnreadNotifications != nil {
		count = joinedRoom.UnreadNotifications.NotificationCount
	}
	if joinedRoom.MSC2654UnreadCount != nil {
		count = *joinedRoom.MSC2654UnreadCount
	}
	return count
}

// getSynapseUnreadCount performs a /sync call to get Synapse's notification_count for a single room.
// Used as fallback when self-calculation isn't possible.
func (m *MautrixAdapter) getSynapseUnreadCount(
	ctx context.Context, intent *appservice.IntentAPI, roomID id.RoomID,
) (int, error) {
	filter := &mautrix.Filter{
		Room: &mautrix.RoomFilter{
			Rooms:     []id.RoomID{roomID},
			Timeline:  &mautrix.FilterPart{Limit: 0},
			State:     &mautrix.FilterPart{Limit: 0},
			Ephemeral: &mautrix.FilterPart{Limit: 0},
		},
		Presence: &mautrix.FilterPart{Limit: 0},
	}
	filterResp, err := intent.CreateFilter(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to create sync filter: %w", err)
	}
	syncResp, err := intent.SyncRequest(ctx, 0, "", filterResp.FilterID, false, "")
	if err != nil {
		return 0, fmt.Errorf("failed to sync: %w", err)
	}
	if joinedRoom, ok := syncResp.Rooms.Join[roomID]; ok {
		return extractSynapseNotificationCount(joinedRoom), nil
	}
	return 0, nil
}

// getSynapseBatchUnreadCounts performs a single /sync call to get Synapse's notification_count for multiple rooms.
// Used as fallback when self-calculation isn't possible.
func (m *MautrixAdapter) getSynapseBatchUnreadCounts(
	ctx context.Context, intent *appservice.IntentAPI, roomIDs []id.RoomID,
) (map[id.RoomID]int, error) {
	filter := &mautrix.Filter{
		Room: &mautrix.RoomFilter{
			Rooms:     roomIDs,
			Timeline:  &mautrix.FilterPart{Limit: 0},
			State:     &mautrix.FilterPart{Limit: 0},
			Ephemeral: &mautrix.FilterPart{Limit: 0},
		},
		Presence: &mautrix.FilterPart{Limit: 0},
	}
	filterResp, err := intent.CreateFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync filter: %w", err)
	}
	syncResp, err := intent.SyncRequest(ctx, 0, "", filterResp.FilterID, false, "")
	if err != nil {
		return nil, fmt.Errorf("failed to sync: %w", err)
	}
	results := make(map[id.RoomID]int)
	for _, rid := range roomIDs {
		if joinedRoom, ok := syncResp.Rooms.Join[rid]; ok {
			results[rid] = extractSynapseNotificationCount(joinedRoom)
		}
	}
	return results, nil
}

// GetUnreadCounts retrieves unread message counts for a room and optionally specific threads.
// Self-calculates unread counts from read receipt position and message timeline,
// falling back to Synapse's notification_count when self-calculation isn't possible.
//
// Note: Thread-level unread counts require MSC3773 support which may not be available
// on all homeservers. If thread counts are requested but not available, the thread
// map will be empty (not an error).
func (m *MautrixAdapter) GetUnreadCounts(
	ctx context.Context, actor domain.Actor, roomID id.RoomID, threadRootIDs []id.EventID,
) (*domain.UnreadCountSummary, error) {
	userID := m.idMapper.UserID(actor.ID)
	intent := m.as.Intent(userID)

	if err := intent.EnsureRegistered(ctx); err != nil {
		m.logger.Error("Failed to ensure user is registered",
			"error", err, "user_id", userID, "actor_id", actor.ID)
		return nil, fmt.Errorf("failed to ensure user registered: %w", err)
	}

	summary := &domain.UnreadCountSummary{
		RoomUnreadCount:    0,
		ThreadUnreadCounts: make(map[id.EventID]int),
	}

	// Get read position from m.fully_read room account data
	receiptEventID := m.getFullyReadMarker(ctx, intent, roomID)

	var count int
	var calculationMethod string

	if receiptEventID == nil {
		// No m.fully_read marker — user has never read via this adapter version.
		// Fall back to Synapse's notification_count to avoid marking everything as unread.
		calculationMethod = "synapse-fallback-no-marker"
		fallbackCount, fallbackErr := m.getSynapseUnreadCount(ctx, intent, roomID)
		if fallbackErr != nil {
			m.logger.Error("Failed to get Synapse fallback unread count",
				"error", fallbackErr, "room_id", roomID, "actor_id", actor.ID)
		} else {
			count = fallbackCount
		}
		// Bootstrap: if Synapse says 0 unread, set m.fully_read to latest message
		// so future queries use self-calculation
		if count == 0 {
			m.bootstrapFullyReadMarker(ctx, intent, roomID)
		}
		m.logger.Debug("No m.fully_read marker, using Synapse fallback",
			"room_id", roomID, "actor_id", actor.ID, "fallback_count", count)
	} else {
		// Self-calculate unread count via progressive timeline fetch
		calculationMethod = "self-calculated"
		var found bool
		count, found = m.countUnreadMessages(ctx, intent, roomID, receiptEventID, userID)

		if !found {
			// Receipt not found in 200 events — fall back to Synapse's notification_count
			calculationMethod = "synapse-fallback-receipt-not-found"
			fallbackCount, fallbackErr := m.getSynapseUnreadCount(ctx, intent, roomID)
			if fallbackErr != nil {
				m.logger.Error("Failed to get Synapse fallback unread count",
					"error", fallbackErr, "room_id", roomID, "actor_id", actor.ID)
				count = 0
			} else {
				count = fallbackCount
			}
			m.logger.Debug("Receipt not found in 200 events, using Synapse fallback",
				"room_id", roomID, "actor_id", actor.ID,
				"receipt_event_id", receiptEventID, "fallback_count", count)
		}
	}

	summary.RoomUnreadCount = count

	// Thread-level unread counts: not yet supported (MSC3773)
	if len(threadRootIDs) > 0 {
		m.logger.Debug("Thread-level unread counts requested but not yet supported by homeserver",
			"room_id", roomID, "actor_id", actor.ID,
			"requested_threads", len(threadRootIDs))
	}

	m.logger.Debug("Retrieved unread counts",
		"room_id", roomID, "actor_id", actor.ID,
		"room_unread", summary.RoomUnreadCount,
		"receipt_event_id", receiptEventID,
		"calculation_method", calculationMethod,
	)

	return summary, nil
}

// maxConcurrentUnreadFetches limits parallel message fetch requests for batch unread counts.
const maxConcurrentUnreadFetches = 10

// GetBatchUnreadCounts retrieves unread counts for multiple rooms.
// Self-calculates from read receipt positions and message timelines,
// falling back to Synapse's notification_count when self-calculation isn't possible.
func (m *MautrixAdapter) GetBatchUnreadCounts(
	ctx context.Context, actor domain.Actor, roomIDs []id.RoomID,
) (map[id.RoomID]int, map[id.RoomID]error) {
	results := make(map[id.RoomID]int)
	errors := make(map[id.RoomID]error)

	if len(roomIDs) == 0 {
		return results, errors
	}

	userID := m.idMapper.UserID(actor.ID)
	intent := m.as.Intent(userID)

	if err := intent.EnsureRegistered(ctx); err != nil {
		m.logger.Error("Failed to ensure user registered for batch unread", "error", err, "user_id", userID)
		for _, roomID := range roomIDs {
			errors[roomID] = fmt.Errorf("failed to ensure user registered: %w", err)
		}
		return results, errors
	}

	// Pre-fetch Synapse fallback counts for all rooms in a single sync call
	synapseCounts, synapseFallbackErr := m.getSynapseBatchUnreadCounts(ctx, intent, roomIDs)
	if synapseFallbackErr != nil {
		m.logger.Warn("Failed to get Synapse fallback counts, will use 0 for fallback",
			"error", synapseFallbackErr, "actor_id", actor.ID)
	}

	// Process each room in parallel with semaphore
	type batchResult struct {
		roomID id.RoomID
		count  int
		err    error
	}
	resultCh := make(chan batchResult, len(roomIDs))
	sem := make(chan struct{}, maxConcurrentUnreadFetches)

	var wg sync.WaitGroup
	for _, roomID := range roomIDs {
		wg.Add(1)
		go func(rid id.RoomID) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultCh <- batchResult{roomID: rid, err: ctx.Err()}
				return
			}

			// Get read position from m.fully_read room account data
			receiptEventID := m.getFullyReadMarker(ctx, intent, rid)

			var count int
			var calculationMethod string

			if receiptEventID == nil {
				// No m.fully_read marker — fall back to Synapse to avoid marking everything as unread
				calculationMethod = "synapse-fallback-no-marker"
				if synapseCounts != nil {
					count = synapseCounts[rid]
				}
				// Bootstrap: if Synapse says 0 unread, set m.fully_read to latest message
				if count == 0 {
					m.bootstrapFullyReadMarker(ctx, intent, rid)
				}
				m.logger.Debug("No m.fully_read marker, using Synapse fallback",
					"room_id", rid, "actor_id", actor.ID, "fallback_count", count)
			} else {
				calculationMethod = "self-calculated"
				var found bool
				count, found = m.countUnreadMessages(ctx, intent, rid, receiptEventID, userID)

				if !found {
					calculationMethod = "synapse-fallback-receipt-not-found"
					if synapseCounts != nil {
						count = synapseCounts[rid]
					} else {
						count = 0
					}
					m.logger.Debug("Receipt not found in 200 events, using Synapse fallback",
						"room_id", rid, "actor_id", actor.ID,
						"receipt_event_id", receiptEventID, "fallback_count", count)
				}
			}

			m.logger.Debug("Batch unread count for room",
				"actor_id", actor.ID, "room_id", rid,
				"unread_count", count,
				"receipt_event_id", receiptEventID,
				"calculation_method", calculationMethod,
			)

			resultCh <- batchResult{roomID: rid, count: count}
		}(roomID)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for r := range resultCh {
		if r.err != nil {
			errors[r.roomID] = r.err
		} else {
			results[r.roomID] = r.count
		}
	}

	m.logger.Debug("Retrieved batch unread counts",
		"actor_id", actor.ID,
		"rooms_requested", len(roomIDs),
		"rooms_found", len(results),
		"rooms_failed", len(errors),
	)

	return results, errors
}
