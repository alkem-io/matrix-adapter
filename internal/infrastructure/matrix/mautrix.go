package matrix

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // Required by Synapse shared secret registration API (HMAC-SHA1)
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// MautrixAdapter implements the MatrixPort interface using the mautrix-go library.
type MautrixAdapter struct {
	cfg            *config.Config
	logger         ports.Logger
	as             appserviceAPI
	idMapper       *domain.IDMapper
	admin          adminAPI
	botDisplayName string
	eventHandlers  EventHandlers
	eventLoopOnce  sync.Once
	server         *http.Server // our own HTTP server wrapping as.Router
	gateOpen       atomic.Bool  // when false, transactions are ack'd and dropped
	queuePort      ports.QueuePort
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

	// Enable zerolog for mautrix-go internal logging, inheriting the
	// adapter's configured log level so operators can enable DEBUG when
	// investigating Matrix protocol issues.
	var mautrixLogLevel zerolog.Level
	switch strings.ToLower(cfg.App.LogLevel) {
	case "debug":
		mautrixLogLevel = zerolog.DebugLevel
	case "warn", "warning":
		mautrixLogLevel = zerolog.WarnLevel
	case "error":
		mautrixLogLevel = zerolog.ErrorLevel
	default:
		mautrixLogLevel = zerolog.InfoLevel
	}
	as.Log = zerolog.New(zerolog.NewConsoleWriter()).Level(mautrixLogLevel).
		With().Timestamp().Str("component", "mautrix").Logger()

	admin, err := NewSynapseAdmin(cfg.Matrix.HomeserverURL, cfg.Matrix.AppServiceToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create synapse admin client: %w", err)
	}

	return &MautrixAdapter{
		cfg:            cfg,
		logger:         logger,
		as:             &appserviceWrapper{as: as},
		idMapper:       domain.NewIDMapper(homeserverDomain),
		admin:          admin,
		botDisplayName: cfg.Matrix.BotDisplayName,
	}, nil
}

// safePrefix returns the first n characters of s, or all of s if shorter.
func safePrefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// ============================================================================
// Core / Connection
// ============================================================================

// Connect boots the adapter: starts the HTTP server with a transaction
// gate (so k8s probes work immediately), ensures the bot is a Synapse
// admin, and verifies connectivity.
//
// While the gate is closed, incoming Synapse transactions
// (PUT /_matrix/app/v1/transactions/*) are acknowledged with 200 OK and
// dropped — the events they carry are migration noise that needs no
// processing. Everything else (health probes, mautrix query handlers)
// passes through normally.
//
// Lifecycle on app.Start():
//
//	Connect             → HTTP listener (gate closed) + bot admin + Whoami
//	SetBotProfile       → display name
//	EnableEventDelivery → opens the gate; Synapse transactions reach mautrix
func (m *MautrixAdapter) Connect(ctx context.Context) error {
	m.gateOpen.Store(false)

	// Start HTTP server immediately so k8s liveness/readiness probes pass.
	// The gate handler drops Synapse transactions until EnableEventDelivery.
	m.logger.Info("[Connect] Step 1/3: starting HTTP listener (transaction gate closed)")
	if err := m.startGatedServer(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	m.logger.Info("[Connect] Step 1/3: complete")

	return m.connectCore(ctx)
}

// ConnectWithoutListener performs bot admin setup and Whoami verification
// without starting the HTTP listener. Intended for CLI tools (cmd/migrate)
// that don't need k8s probes or event delivery.
func (m *MautrixAdapter) ConnectWithoutListener(ctx context.Context) error {
	return m.connectCore(ctx)
}

func (m *MautrixAdapter) connectCore(ctx context.Context) error {
	m.logger.Info("[Connect] ensuring bot admin status")
	m.ensureBotAdmin(ctx)

	m.logger.Info("[Connect] verifying bot connection (Whoami)")
	botIntent := m.as.BotIntent()
	whoami, err := botIntent.Whoami(ctx)
	if err != nil {
		return fmt.Errorf("connect preflight failed: whoami: %w", err)
	}
	m.logger.Info("[Connect] complete", "user_id", whoami.UserID)

	return nil
}

// startGatedServer starts our own http.Server using the mautrix Router
// (which already has all handlers registered) wrapped in a gate that
// drops transaction PUTs while closed. We never call as.Start() —
// we own the listener.
func (m *MautrixAdapter) startGatedServer(ctx context.Context) error {
	addr := m.as.Host().Address()
	if addr == "" {
		return fmt.Errorf("appservice host not configured")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// While gate is closed, ack Synapse transactions with 200 and drop.
		if !m.gateOpen.Load() && r.Method == http.MethodPut &&
			strings.HasPrefix(r.URL.Path, "/_matrix/app/v1/transactions/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}
		// Everything else (health, mautrix query handlers, transactions
		// after gate opens) goes straight to the mautrix router.
		m.as.Router().ServeHTTP(w, r)
	})

	m.server = &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	startErrCh := make(chan error, 1)
	go func() {
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			startErrCh <- err
		}
	}()

	// Wait for either successful readiness or a fast startup failure
	// (e.g. port already in use).
	readyCh := make(chan error, 1)
	go func() { readyCh <- m.waitForServerReady(ctx) }()

	select {
	case err := <-startErrCh:
		return fmt.Errorf("HTTP listener failed to start: %w", err)
	case err := <-readyCh:
		if err != nil {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = m.server.Shutdown(shutCtx)
			return err
		}
	}
	return nil
}

// SetBotProfile applies the configured bot display name. This is
// intentionally a separate phase: changing the display name fans out an `m.room.member`
// event into every room the bot is currently joined to, so we want
// the joined-room set to be as small as possible (only spaces, after
// leaveBotFromNonSpaceRooms) when this runs.
func (m *MautrixAdapter) SetBotProfile(ctx context.Context) {
	if m.botDisplayName == "" {
		m.logger.Info("[SetBotProfile] skipped (no display name configured)")
		return
	}
	if err := m.as.BotIntent().SetDisplayName(ctx, m.botDisplayName); err != nil {
		m.logger.Warn("[SetBotProfile] SetDisplayName failed (continuing anyway)",
			"display_name", m.botDisplayName, "error", err)
		return
	}
	m.logger.Info("[SetBotProfile] complete", "display_name", m.botDisplayName)
}

// EnableEventDelivery opens the transaction gate so that Synapse
// transactions reach mautrix's PutTransaction handler and flow into
// the event loop. Must be called only after the downstream queue
// (RabbitMQ) is connected and event handlers are registered.
func (m *MautrixAdapter) EnableEventDelivery() {
	m.gateOpen.Store(true)
	m.logger.Info("[EnableEventDelivery] transaction gate open; events flowing")
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
	_, err := m.registerSharedSecretUser(ctx, secret, botLocalpart, "bot-"+safePrefix(secret, 8), true)
	if err == nil {
		m.logger.Info("Bot registered as server admin via shared secret")
		return
	}
	// Ensure the bot is registered as an appservice user before promoting.
	// On a fresh DB, Synapse creates the bot from registration YAML but the
	// M_EXCLUSIVE check blocks external admin API calls until the appservice
	// itself has registered the user via POST /register.
	if err := m.as.BotIntent().EnsureRegistered(ctx); err != nil {
		m.logger.Debug("Bot appservice registration check", "error", err)
	} else {
		m.logger.Debug("Bot appservice registration ensured")
	}

	m.logger.Info("Bot user already exists, promoting via temp admin...")

	// Bot exists but isn't admin — try temp admin approach
	if err := m.promoteViaTemporaryAdmin(ctx, secret); err != nil {
		m.logger.Warn("Failed to promote bot via temp admin",
			"error", err, "bot_mxid", m.as.BotMXID())
	} else {
		m.logger.Info("Bot promoted to server admin", "bot_mxid", m.as.BotMXID())
	}
}

// findGhostIntentInRoom finds the ghost user with the highest power level in a room.
// Returns their intent and PL, or nil if no ghost users are found.
func (m *MautrixAdapter) findGhostIntentInRoom(ctx context.Context, roomID id.RoomID) (intentAPI, float64) {
	members, err := m.admin.GetRoomMembers(ctx, roomID)
	if err != nil {
		return nil, -1
	}

	// Get power levels to find the most privileged ghost
	powerLevels := make(map[string]float64)
	var usersDefault float64
	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.power_levels"); err == nil && content != nil {
		if users, ok := content["users"].(map[string]interface{}); ok {
			for uid, pl := range users {
				if v, ok := pl.(float64); ok {
					powerLevels[uid] = v
				}
			}
		}
		if d, ok := content["users_default"].(float64); ok {
			usersDefault = d
		}
	}

	botMXID := m.as.BotMXID().String()
	var bestIntent intentAPI
	bestPL := float64(-1)

	for _, member := range members {
		if member == botMXID {
			continue
		}
		userID := id.UserID(member)
		if m.idMapper.AlkemioActorID(userID) == uuid.Nil {
			continue
		}
		pl, ok := powerLevels[member]
		if !ok {
			pl = usersDefault
		}
		if pl > bestPL {
			bestPL = pl
			bestIntent = m.as.Intent(userID)
		}
	}

	return bestIntent, bestPL
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

// isBotAdmin checks if the bot is a Synapse server admin.
func (m *MautrixAdapter) isBotAdmin(ctx context.Context) bool {
	user, err := m.admin.GetUser(ctx, m.as.BotMXID())
	if err != nil {
		return false
	}
	return user.Admin
}

// promoteViaTemporaryAdmin creates a temporary admin user, uses it to promote
// the bot to server admin, then deactivates the temporary user.
func (m *MautrixAdapter) promoteViaTemporaryAdmin(ctx context.Context, secret string) error {
	// Use a unique username each time to avoid conflicts with deactivated leftover users
	bootstrapUser := fmt.Sprintf("alkemio-bootstrap-%d", time.Now().UnixMilli())
	bootstrapPass := "bootstrap-" + safePrefix(secret, 8)

	// Create temporary admin
	adminToken, err := m.registerSharedSecretUser(ctx, secret, bootstrapUser, bootstrapPass, true)
	if err != nil {
		return fmt.Errorf("failed to register bootstrap admin: %w", err)
	}

	tempAdmin, err := NewSynapseAdmin(m.cfg.Matrix.HomeserverURL, adminToken)
	if err != nil {
		return fmt.Errorf("failed to create temp admin client: %w", err)
	}

	// Promote bot
	if err = tempAdmin.SetUserAdmin(ctx, m.as.BotMXID(), true); err != nil {
		return fmt.Errorf("failed to promote bot: %w", err)
	}

	// Deactivate and erase temporary admin
	bootstrapMXID := id.UserID("@" + bootstrapUser + ":" + m.cfg.Matrix.HomeserverName)
	if err = tempAdmin.DeactivateUser(ctx, bootstrapMXID, true); err != nil {
		m.logger.Warn("Failed to deactivate bootstrap admin user",
			"bootstrap_mxid", bootstrapMXID, "error", err)
	} else {
		m.logger.Info("Bootstrap admin user deactivated", "bootstrap_mxid", bootstrapMXID)
	}

	return nil
}

// newDirectClient creates a mautrix.Client that talks directly to Synapse
// without appservice impersonation. Uses config URL so it works before as.Start().
// NOTE: Only used for shared secret registration (non-admin). For admin API, use m.admin.
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
func (m *MautrixAdapter) waitForServerReady(ctx context.Context) error {
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
	if m.as.Host().IsUnixSocket() {
		return "" // Unix sockets need different handling, skip for now
	}
	return m.as.Host().Address()
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
	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.server.Shutdown(ctx)
	}
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
// InviteUser joins a ghost user directly to a room (no invite event).
// Since all users are appservice ghosts, we skip the invite and join directly
// to avoid triggering invite notifications in Element for invisible rooms.
func (m *MautrixAdapter) InviteUser(
	ctx context.Context, roomID id.RoomID, _ domain.Actor, inviteeID domain.Actor,
) error {
	inviteeUserID, err := m.EnsureUser(ctx, inviteeID)
	if err != nil {
		return err
	}

	// Join directly — appservice ghosts don't need invites.
	// This avoids the invite event appearing in /sync before the module can filter it.
	inviteeIntent := m.as.Intent(inviteeUserID)
	if err := inviteeIntent.EnsureJoined(ctx, roomID); err != nil {
		return fmt.Errorf("failed to join user to room: %w", err)
	}

	// Mark room as read to clear join notification from unread count
	if latestEventID, err := m.getLatestEventID(ctx, roomID); err != nil {
		m.logger.Warn("Failed to get latest event for read receipt",
			"room_id", roomID, "error", err)
	} else {
		m.markRoomAsReadForUsers(ctx, roomID, []id.UserID{inviteeUserID}, latestEventID)
	}

	// If bot is still in the room (e.g. room created without initial members),
	// leave now that a real member has joined.
	m.leaveBotIfNotNeeded(ctx, roomID)

	return nil
}

// getLatestEventID retrieves the latest event ID in a room.
// Used to mark rooms as read after joining.
func (m *MautrixAdapter) getLatestEventID(
	ctx context.Context, roomID id.RoomID,
) (id.EventID, error) {
	// Use admin API to get latest event — works regardless of bot membership
	resp, err := m.admin.GetRoomMessages(ctx, roomID, "", "b", 1)
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
	ctx context.Context, intent intentAPI, roomID id.RoomID, eventID id.EventID,
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

// SendMessage sends a message to a room. A non-empty text body is sent as a
// single m.text event; each attachment is sent as its own media event
// (m.image/m.file/...). Returns the primary event ID (the text event if there
// is text, otherwise the first media event).
func (m *MautrixAdapter) SendMessage(
	ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, attachments []domain.Attachment,
) (id.EventID, error) {
	userID, err := m.EnsureUser(ctx, senderID)
	if err != nil {
		return "", err
	}
	intent := m.as.Intent(userID)

	var primaryEventID id.EventID
	if content != "" {
		resp, err := intent.SendText(ctx, roomID, content)
		if err != nil {
			return "", fmt.Errorf("failed to send message: %w", err)
		}
		primaryEventID = resp.EventID
	}

	return m.fanOutAttachments(ctx, intent, roomID, attachments, "", primaryEventID)
}

// fanOutAttachments sends each attachment as its own independent media event.
// There is no atomicity across the fan-out — Matrix/Element have none either: if
// one attachment fails, the events already sent stay in the room and the error
// is returned, so the caller retries the failed part exactly as an Element
// multi-image send behaves (each image is its own event/message). primaryEventID
// is the text event's id (empty for an attachment-only message, in which case
// the first attachment becomes the primary); it is returned resolved.
//
// The whole fan-out (all N<=10 attachments) is fetched, uploaded, and sent
// SEQUENTIALLY within this single send's HandleSendMessage queue-handler
// invocation, on purpose: sequential order preserves the attachments' visible
// order in the room (events are ordered by send). Parallelizing the uploads
// would still require an ordered send afterward for marginal gain on a bounded
// (<=10) attachment list.
func (m *MautrixAdapter) fanOutAttachments(
	ctx context.Context, intent intentAPI, roomID id.RoomID,
	attachments []domain.Attachment, threadID, primaryEventID id.EventID,
) (id.EventID, error) {
	for i := range attachments {
		eventID, err := m.sendAttachment(ctx, intent, roomID, attachments[i], threadID)
		if err != nil {
			return "", fmt.Errorf("attachment %d of %d failed: %w", i+1, len(attachments), err)
		}
		if primaryEventID == "" {
			primaryEventID = eventID
		}
	}
	return primaryEventID, nil
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
// Uses admin API for state reads — works regardless of bot membership.
func (m *MautrixAdapter) GetRoomDetails(ctx context.Context, roomID id.RoomID) (*domain.Room, error) {
	var name, topic, alias, avatarURL string

	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.name"); err == nil && content != nil {
		if v, ok := content["name"].(string); ok {
			name = v
		}
	}

	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.topic"); err == nil && content != nil {
		if v, ok := content["topic"].(string); ok {
			topic = v
		}
	}

	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.avatar"); err == nil && content != nil {
		if v, ok := content["url"].(string); ok {
			avatarURL = v
		}
	}

	// Get alias (uses room_aliases table - same source as ResolveAlias)
	if aliases, err := m.GetRoomAliases(ctx, roomID); err == nil {
		alias = m.selectPreferredAlias(aliases)
	}

	// Get custom io.alkemio.* state events
	customState, _ := m.GetCustomState(ctx, roomID, nil)

	return &domain.Room{
		ID:          roomID,
		Name:        name,
		Topic:       topic,
		AvatarURL:   avatarURL,
		Alias:       alias,
		CustomState: customState,
	}, nil
}

// GetRoomMembers returns the list of members in a room.
// Uses admin API — works regardless of bot membership.
func (m *MautrixAdapter) GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error) {
	return m.admin.GetRoomMemberIDs(ctx, roomID)
}

// UpdateRoomState updates the state of a room.
// nil pointers mean "no change"; non-nil (including empty string) means "set this value".
func (m *MautrixAdapter) UpdateRoomState(
	ctx context.Context, roomID id.RoomID, _ domain.Actor, name, topic, avatarURL, joinRule *string,
) error {
	intent := m.getIntentForRoom(ctx, roomID)

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
	ctx context.Context, intent intentAPI, roomID id.RoomID, eventType event.Type, value string,
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

	// Value is empty — send empty content to clear the state.
	// The /state/ endpoint doesn't return event_id, so we can't redact directly.
	// Sending empty content effectively clears the property.
	var emptyContent interface{}
	switch eventType {
	case event.StateRoomName:
		emptyContent = map[string]interface{}{"name": ""}
	case event.StateTopic:
		emptyContent = map[string]interface{}{"topic": ""}
	case event.StateRoomAvatar:
		emptyContent = &event.RoomAvatarEventContent{}
	default:
		emptyContent = map[string]interface{}{}
	}
	_, err := intent.SendStateEvent(ctx, roomID, eventType, "", emptyContent)
	return err
}

// ============================================================================
// Message & Reaction Operations
// ============================================================================

// SendReply sends a reply to a message. A non-empty text body is sent as a
// single threaded m.text event; each attachment is sent as its own threaded
// media event. Returns the primary event ID (the text event if there is text,
// otherwise the first media event).
func (m *MautrixAdapter) SendReply(
	ctx context.Context, roomID id.RoomID, senderID domain.Actor, content string, threadID id.EventID,
	attachments []domain.Attachment,
) (id.EventID, error) {
	userID, err := m.EnsureUser(ctx, senderID)
	if err != nil {
		return "", err
	}
	intent := m.as.Intent(userID)

	var primaryEventID id.EventID
	if content != "" {
		msgContent := event.MessageEventContent{
			MsgType:   event.MsgText,
			Body:      content,
			RelatesTo: threadRelation(threadID),
		}

		resp, err := intent.SendMessageEvent(ctx, roomID, event.EventMessage, &msgContent)
		if err != nil {
			return "", fmt.Errorf("failed to send reply: %w", err)
		}
		primaryEventID = resp.EventID
	}

	return m.fanOutAttachments(ctx, intent, roomID, attachments, threadID, primaryEventID)
}

// ============================================================================
// Media (byte bridge) — stateless: fetch from file-service, push to Synapse
// ============================================================================

var errAttachmentTooLarge = errors.New("attachment exceeds max size")

// countingCapReader streams from r, tracking bytes read and failing once more
// than max bytes have been read so an oversized document fails the upload
// instead of being buffered. n is the exact number of bytes streamed.
//
// A known-length oversize document is rejected up front (before streaming). An
// unknown-length (chunked) document that only reveals it is oversize mid-stream
// will have streamed up to max bytes to Synapse before this trips; those bytes
// are unreferenced by any event and are reclaimed by Synapse media retention —
// an accepted bounded cost of streaming, not worth a pre-buffering pass.
type countingCapReader struct {
	r      io.Reader
	max, n int64
}

func (c *countingCapReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > c.max {
		return n, errAttachmentTooLarge
	}
	return n, err
}

// sendAttachment fetches a document's bytes from file-service, uploads them to
// the homeserver, and sends a media event carrying the mxc URL, file info, and
// the io.alkemio.document_id breadcrumb. When threadID is non-empty the event
// is threaded under it.
func (m *MautrixAdapter) sendAttachment(
	ctx context.Context, intent intentAPI, roomID id.RoomID, att domain.Attachment,
	threadID id.EventID,
) (id.EventID, error) {
	resp, err := m.openDocumentFetch(ctx, att.DocumentID)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// Use the most specific media type available. The server-declared att.MimeType
	// is authoritative when specific; fall back to the file-service response
	// Content-Type when att is generic/empty (some object stores serve
	// application/octet-stream regardless of the real type). The FULL resolved type
	// (with any params such as "; charset=utf-8") drives the upload Content-Type so
	// charset survives for text; the BARE type (params stripped) drives the event
	// msgtype and info.mimetype, which are conventionally unparameterized.
	contentType := resolveMediaMime(resp.Header.Get("Content-Type"), att.MimeType)

	maxBytes := m.cfg.MaxAttachmentBytes()
	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
		return "", attachmentTooLargeError(att.DocumentID, maxBytes)
	}
	reader := &countingCapReader{r: resp.Body, max: maxBytes}
	// Upload via the SENDER ghost intent (not the appservice bot), so the media
	// blob is owned by the acting user's account — attributing quota/retention
	// correctly and avoiding a single-account purge stripping every bridged blob.
	//
	// Always upload with an unknown (streamed/chunked) length rather than
	// trusting file-service's declared Content-Length: if the real body is
	// shorter than the declared length, a declared upload would EOF early and
	// fail. countingCapReader still enforces the per-attachment max, and the
	// declared > cap case is already rejected above.
	up, err := intent.UploadMedia(ctx, mautrix.ReqUploadMedia{
		Content:       reader,
		ContentLength: -1,
		ContentType:   contentType,
	})
	if err != nil {
		if errors.Is(err, errAttachmentTooLarge) || reader.n > maxBytes {
			return "", attachmentTooLargeError(att.DocumentID, maxBytes)
		}
		return "", fmt.Errorf("failed to upload media: %w", err)
	}
	// info.size is the bytes actually streamed, never the caller-declared att.Size.
	// The bare type (params stripped) is used for info.mimetype and msgtype.
	content := buildMediaContent(att, up.ContentURI, baseType(contentType), reader.n, threadID)
	sent, err := intent.SendMessageEvent(ctx, roomID, event.EventMessage, content)
	if err != nil {
		return "", fmt.Errorf("failed to send media event: %w", err)
	}
	return sent.EventID, nil
}

func attachmentTooLargeError(documentID string, maxBytes int64) error {
	return fmt.Errorf("document %s exceeds max attachment size of %d bytes", documentID, maxBytes)
}

// fileServiceFetchTimeout bounds connection establishment and the wait for
// file-service response headers. The streamed response body remains governed by
// the send context rather than a fixed wall-clock timeout.
const fileServiceFetchTimeout = 60 * time.Second

func newFileServiceHTTPClient(responseHeaderTimeout time.Duration) *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: fileServiceFetchTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: time.Second,
	}}
}

// fileServiceHTTPClient bounds connection phases and header wait without a
// Client.Timeout, which would also cap reading a large body while it is streamed
// onward to Synapse.
var fileServiceHTTPClient = newFileServiceHTTPClient(fileServiceFetchTimeout)

// openDocumentFetch performs the GET against the file-service internal content
// endpoint (GET {FILE_SERVICE_URL}/internal/file/{id}/content) and returns the
// response with its headers available and the body unread so sendAttachment can
// stream it directly to the homeserver.
// A non-200 response is turned into an error here (and its body closed) so no
// budget is reserved for a failed fetch. The caller MUST close resp.Body on the
// success path. Connection setup and response-header wait are transport-bounded;
// reading the success body is bounded by the request context.
func (m *MautrixAdapter) openDocumentFetch(ctx context.Context, documentID string) (*http.Response, error) {
	if m.cfg == nil || m.cfg.FileService.URL == "" {
		return nil, fmt.Errorf("file-service URL not configured (set FILE_SERVICE_URL)")
	}
	baseURL := m.cfg.FileService.URL
	// Defense-in-depth on the internal fetch path: document ids are UUIDs, so
	// validate the shape and path-escape before interpolating into the URL.
	if _, err := uuid.Parse(documentID); err != nil {
		return nil, fmt.Errorf("invalid document id %q: %w", documentID, err)
	}
	endpoint := fmt.Sprintf("%s/internal/file/%s/content",
		strings.TrimRight(baseURL, "/"), url.PathEscape(documentID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build file-service request: %w", err)
	}

	resp, err := fileServiceHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document %s from file-service: %w", documentID, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("file-service returned status %d fetching document %s", resp.StatusCode, documentID)
	}
	return resp, nil
}

// buildMediaContent constructs the raw event content for a media message:
// msgtype + body + url(mxc) + info, plus the io.alkemio.document_id breadcrumb
// used for lookup-free read-translation of our own outbound media. A map is
// used (rather than event.MessageEventContent) so custom fields can be set as
// top-level event properties.
// bareMimeType is the resolved media type with any parameters stripped
// (see baseType) — Matrix info.mimetype and msgtype are conventionally bare.
func buildMediaContent(
	att domain.Attachment, mxc id.ContentURI, bareMimeType string, size int64,
	threadID id.EventID,
) map[string]any {
	info := map[string]any{
		"mimetype": bareMimeType,
		"size":     size,
	}
	if att.Width != nil {
		info["w"] = *att.Width
	}
	if att.Height != nil {
		info["h"] = *att.Height
	}

	// HandleSendMessage rejects empty document ids, and openDocumentFetch validates
	// the UUID before the media content is built, so this breadcrumb is never blank.
	content := map[string]any{
		"msgtype":                mediaMsgType(bareMimeType),
		"body":                   att.DisplayName,
		"url":                    mxc.String(),
		"info":                   info,
		"io.alkemio.document_id": att.DocumentID,
	}

	if threadID != "" {
		content["m.relates_to"] = threadRelation(threadID)
	}

	return content
}

// threadRelation is the single typed construction used by text and media
// replies, keeping their MSC3440 relation shape identical.
func threadRelation(threadID id.EventID) *event.RelatesTo {
	return &event.RelatesTo{
		Type:    event.RelThread,
		EventID: threadID,
		InReplyTo: &event.InReplyTo{
			EventID: threadID,
		},
		IsFallingBack: true,
	}
}

// resolveMediaMime picks the most specific media type and returns it in
// canonical form: a lowercased "type/subtype" with any meaningful parameters
// (e.g. "; charset=utf-8") preserved. The server-declared att.MimeType is
// authoritative when specific; otherwise the file-service response Content-Type
// is used. Empty, params-only, malformed, and application/octet-stream values
// are treated as non-specific. The result drives the upload Content-Type, the
// Matrix msgtype, and info.mimetype uniformly so they never disagree — and
// because the type is lowercased, case-insensitive inputs (e.g. "Image/JPEG")
// still classify correctly in mediaMsgType.
func resolveMediaMime(responseContentType, attMimeType string) string {
	att := normalizeMediaType(attMimeType)
	resp := normalizeMediaType(responseContentType)
	if isSpecificMediaType(att) {
		return att
	}
	if isSpecificMediaType(resp) {
		return resp
	}
	if att != "" {
		return att
	}
	if resp != "" {
		return resp
	}
	return "application/octet-stream"
}

// normalizeMediaType returns a lowercased, canonical "type/subtype[; params]"
// for a Content-Type, or "" when there is no valid media type. Valid input is
// canonicalized (preserving parameters such as charset). When only a PARAMETER
// is malformed, the bare "type/subtype" is recovered and RE-PARSED with
// mime.ParseMediaType so a malformed BASE type (bad token chars, whitespace,
// extra segments, control characters) still collapses to "" rather than leaking
// into the upload Content-Type header or the event's info.mimetype.
func normalizeMediaType(contentType string) string {
	if mediatype, params, err := mime.ParseMediaType(contentType); err == nil && validMediaType(mediatype) {
		return mime.FormatMediaType(mediatype, params)
	}
	// Parse failed — possibly only a parameter was malformed. Recover the base
	// (before the first ';') and re-validate it via ParseMediaType.
	base := contentType
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = base[:i]
	}
	if mediatype, _, err := mime.ParseMediaType(base); err == nil && validMediaType(mediatype) {
		return mediatype
	}
	return ""
}

// validMediaType reports whether t is "type/subtype" with a non-empty subtype
// (mime.ParseMediaType accepts a bare type like "image" with no subtype).
func validMediaType(t string) bool {
	slash := strings.IndexByte(t, '/')
	return slash > 0 && slash < len(t)-1
}

// baseType strips any parameters (e.g. "; charset=utf-8") from a media type,
// returning the bare "type/subtype". Matrix FileInfo.mimetype and the event
// msgtype are conventionally a bare type — a parameterized value can defeat a
// client's exact-match preview/thumbnail logic — whereas the upload Content-Type
// must keep params (charset) intact.
func baseType(s string) string {
	if i := strings.IndexByte(s, ';'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// isSpecificMediaType reports whether a normalized media type is more specific
// than application/octet-stream. normalized is "" or "type/subtype[; params]".
func isSpecificMediaType(normalized string) bool {
	base := baseType(normalized)
	return base != "" && base != "application/octet-stream"
}

// mediaMsgType maps a MIME type to the appropriate Matrix message msgtype.
func mediaMsgType(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return msgTypeImage
	case strings.HasPrefix(mime, "video/"):
		return msgTypeVideo
	case strings.HasPrefix(mime, "audio/"):
		return msgTypeAudio
	default:
		return msgTypeFile
	}
}

// RedactEvent redacts (deletes) an event from a room.
//
// A media message fans out to a primary event plus one event per attachment;
// deleting the message redacts only the primary. The attachment events are not
// cascade-redacted here (a stateless adapter cannot reliably rediscover them) —
// they are unreferenced media reclaimed by Synapse media retention.
func (m *MautrixAdapter) RedactEvent(
	ctx context.Context, roomID id.RoomID, actorID domain.Actor, eventID id.EventID, reason string,
) error {
	userID, err := m.EnsureUser(ctx, actorID)
	if err != nil {
		return err
	}
	intent := m.as.Intent(userID)

	req := mautrix.ReqRedact{Reason: reason}

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
//
// It routes through parseMessageEvent — the same path as GetRoomMessages /
// GetLastMessage / GetThreadMessages — so a media event returns its attachment
// (mxc→MediaID, io.alkemio.document_id→DocumentID) consistently, rather than
// just its body text.
func (m *MautrixAdapter) GetMessage(ctx context.Context, roomID id.RoomID, eventID id.EventID) (
	*domain.Message, error,
) {
	evt, err := m.admin.GetEvent(ctx, roomID, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	if malformedMessageBody(evt) {
		return nil, fmt.Errorf("event is not a message")
	}

	// Only m.room.message events resolve to a Message (with attachments for media).
	// A non-message event (reaction, m.sticker, ...) errors rather than being
	// coerced to bare text — resolving a sticker to its alt-text would drop the
	// image and surface a phantom text message. The server tracks message ids
	// distinctly, so message.get is not called with such ids.
	msg := m.parseMessageEvent(evt, roomID)
	if msg == nil {
		return nil, fmt.Errorf("event is not a message")
	}
	return msg, nil
}

// malformedMessageBody reports whether a message-type event carries a "body" that
// is present but not a string — a corrupt event that is not a usable message
// (develop errored here). An ABSENT body is NOT malformed: a bodyless/redacted
// message event still resolves to an empty-content Message.
//
// A MEDIA event (non-empty mxc "url") is NEVER malformed on body grounds: it is a
// valid attachment that parseMessageEvent/extractInboundMessage surface via the
// url, and the timeline scans (GetRoomMessages/GetLastMessage) return it. Firing
// here on a media event with a corrupt/non-string body would make GetMessage
// error on a message the other read paths return, an inconsistent read path.
// Only a non-string body with NO url is malformed.
func malformedMessageBody(evt *event.Event) bool {
	if evt.Type != event.EventMessage || evt.Content.Raw == nil {
		return false
	}
	if url, ok := evt.Content.Raw["url"].(string); ok && url != "" {
		return false
	}
	raw, present := evt.Content.Raw["body"]
	if !present {
		return false
	}
	_, isString := raw.(string)
	return !isString
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
	chunk, err := m.admin.GetRelations(ctx, roomID, eventID, event.RelAnnotation, event.EventReaction)
	if err != nil {
		return "", fmt.Errorf("failed to get relations: %w", err)
	}

	senderUserID, err := m.EnsureUser(ctx, senderID)
	if err != nil {
		return "", err
	}

	return m.findReactionByEmojiAndSender(chunk, senderUserID, emoji)
}

// findReactionByEmojiAndSender searches for a specific reaction in a list of events.
func (m *MautrixAdapter) findReactionByEmojiAndSender(
	events []*event.Event, senderUserID id.UserID, emoji string,
) (id.EventID, error) {
	for _, evt := range events {
		if evt.Sender != senderUserID || evt.Type != event.EventReaction {
			continue
		}
		content, ok := parseEventContent[event.ReactionEventContent](evt)
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
	return m.as.HomeserverDomain()
}

// Router returns the AppService HTTP router for registering custom endpoints.
// Custom endpoints will be served on the same port as the AppService transaction API.
func (m *MautrixAdapter) Router() *http.ServeMux {
	return m.as.Router()
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
	// Requires Synapse room_list_publication_rules to allow the bot user.
	// Without it, Synapse v1.126.0+ returns 403 by default.
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

// SetCustomState sets custom io.alkemio.* state events on a room.
// Only event types with the "io.alkemio." prefix are allowed.
// Uses BotIntent if bot is in the room, otherwise finds a ghost user.
func (m *MautrixAdapter) SetCustomState(ctx context.Context, roomID id.RoomID, state map[string]map[string]interface{}) error {
	// Try bot first; if it fails (not in room), find a ghost user
	intent := m.getIntentForRoom(ctx, roomID)

	for eventType, content := range state {
		if !strings.HasPrefix(eventType, "io.alkemio.") {
			return fmt.Errorf("custom state event type must have io.alkemio. prefix, got: %s", eventType)
		}
		_, err := intent.SendStateEvent(ctx, roomID, event.Type{
			Type:  eventType,
			Class: event.StateEventType,
		}, "", content)
		if err != nil {
			return fmt.Errorf("failed to set custom state %s: %w", eventType, err)
		}
	}
	return nil
}

// getIntentForRoom returns an intent that has access to a room with at least
// the given power level. Tries BotIntent first (for spaces where bot is a member),
// then falls back to finding a joined ghost user with sufficient PL.
// If no suitable intent is found, admin-joins the bot (PL 100 as room creator).
func (m *MautrixAdapter) getIntentForRoom(ctx context.Context, roomID id.RoomID, minPL ...float64) intentAPI {
	requiredPL := float64(50) // default: state events require PL 50
	if len(minPL) > 0 {
		requiredPL = minPL[0]
	}
	botIntent := m.as.BotIntent()

	// Check if bot is in the room via admin API
	members, err := m.admin.GetRoomMembers(ctx, roomID)
	if err == nil {
		botMXID := m.as.BotMXID().String()
		for _, member := range members {
			if member == botMXID {
				m.logger.Debug("getIntentForRoom: using bot (member of room)", "room_id", roomID)
				return botIntent
			}
		}
	}

	// Bot not in room — find a ghost user with sufficient PL
	intent, pl := m.findGhostIntentInRoom(ctx, roomID)
	if intent != nil && pl >= requiredPL {
		m.logger.Debug("getIntentForRoom: using ghost user", "room_id", roomID, "power_level", pl)
		return intent
	}

	// No ghost with sufficient PL — admin-join the bot (PL 100 as room creator).
	// The bot will leave once a real member is added (via leaveBotIfNotNeeded).
	if intent != nil {
		m.logger.Debug("getIntentForRoom: ghost PL too low, admin-joining bot",
			"room_id", roomID, "ghost_pl", pl)
	} else {
		m.logger.Debug("getIntentForRoom: no ghost user found, admin-joining bot",
			"room_id", roomID)
	}
	if err := m.admin.JoinRoom(ctx, roomID, m.as.BotMXID()); err != nil {
		m.logger.Debug("getIntentForRoom: admin join failed",
			"room_id", roomID, "error", err)
	} else {
		// Sync the StateStore so EnsureJoined (called by SendStateEvent, etc.)
		// sees the bot as already joined and doesn't create a duplicate join event.
		if err := m.as.SetMembership(ctx, roomID, m.as.BotMXID(), event.MembershipJoin); err != nil {
			m.logger.Error("getIntentForRoom: failed to sync StateStore after admin join — risk of duplicate join",
				"room_id", roomID, "error", err)
			if intent != nil {
				return intent // fall back to ghost to avoid duplicate-join risk
			}
		}
	}
	return botIntent
}

// GetCustomState retrieves io.alkemio.* state events from a room.
// Uses Synapse admin API — works regardless of bot membership.
func (m *MautrixAdapter) GetCustomState(ctx context.Context, roomID id.RoomID, eventTypes []string) (map[string]map[string]interface{}, error) {
	return m.admin.GetCustomState(ctx, roomID, eventTypes)
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
	intent := m.getIntentForRoom(ctx, roomID)
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
	// Get messages using admin API — works regardless of bot membership
	resp, err := m.admin.GetRoomMessages(ctx, roomID, "", "b", 1000)
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
		// Skip blank preview messages (present-but-empty body, no attachment):
		// this is a history scan, so it must not surface them (A1). GetMessage
		// by id still returns them.
		if msg == nil || isBlankMessage(msg) {
			continue
		}
		msg.Reactions = []domain.Reaction{} // Initialize empty slice
		messages = append(messages, *msg)
		messageIndices[msg.ID] = len(messages) - 1
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
	// Progressive fetch: start small, expand if needed
	// In most cases, the last message is within the first few events
	batchSizes := []int{5, 10, 20, 50, 200}
	var allEvents []*event.Event
	var from string

	for _, batchSize := range batchSizes {
		resp, err := m.admin.GetRoomMessages(ctx, roomID, from, "b", batchSize)
		if err != nil {
			return nil, fmt.Errorf("failed to get last message: %w", err)
		}

		allEvents = append(allEvents, resp.Chunk...)

		// Check if we found a non-blank message in this batch. A blank preview
		// message (present-but-empty body, no attachment) must NOT be surfaced as
		// the room's last message (A1) — keep scanning older events for a real one.
		for i, evt := range resp.Chunk {
			if evt.Type != event.EventMessage {
				continue
			}
			msg := m.parseMessageEvent(evt, roomID)
			if msg == nil || isBlankMessage(msg) {
				continue
			}
			// Record stats: total events scanned = previous batches + position in current batch
			eventsScanned := len(allEvents) - len(resp.Chunk) + i + 1
			lastMsgStats.record(eventsScanned, true, m.logger)
			// Found a real message - collect reactions from all fetched events and return
			return m.buildLastMessageWithReactions(allEvents, msg, roomID)
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

// buildLastMessageWithReactions attaches reactions to the message already parsed
// by GetLastMessage while scanning the timeline.
func (m *MautrixAdapter) buildLastMessageWithReactions(
	events []*event.Event, msg *domain.Message, roomID id.RoomID,
) (*domain.Message, error) {
	if msg == nil {
		return nil, nil
	}
	msg.Reactions = []domain.Reaction{}

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
	if evt.Type != event.EventMessage {
		return nil
	}

	// Shared inbound-media helper (F10): applies MSC2530 caption semantics and
	// the present-but-empty-body rule. Read paths keep bodyless message events as
	// blank Messages so GetMessage can return them through this same population
	// path; timeline scans exclude them with isBlankMessage below.
	content, attachment, _ := extractInboundMessage(evt, m.isOwnAppserviceUser(evt.Sender))

	msg := &domain.Message{
		ID:             evt.ID.String(),
		RoomID:         roomID.String(),
		Content:        content,
		SenderMatrixID: evt.Sender.String(),
		Timestamp:      time.UnixMilli(evt.Timestamp),
	}
	if attachment != nil {
		msg.Attachments = []domain.Attachment{*attachment}
	}

	// Thread linkage (MSC3440): read from the same helper the live-sync path uses,
	// which prefers the RAW m.relates_to. Reading Content.Parsed alone (as this
	// path used to) silently dropped the thread/parent linkage on Synapse-fetched
	// read-path events, whose Parsed is nil — so threaded replies rendered as
	// top-level in GetMessage/GetRoomMessages/GetThreadMessages (F1).
	msg.ThreadID = extractThreadID(evt)

	return msg
}

// isBlankMessage reports whether a parsed message carries nothing renderable:
// empty Content AND no attachment. The SCANNING read paths (GetRoomMessages,
// GetLastMessage, and GetThreadMessages replies) skip such messages so a
// present-but-empty-body event never surfaces as a room's latest/preview message
// — restoring develop's behavior, where parseMessageEvent returned nil for an
// empty body and these scans never saw it (A1).
//
// By-id fetches (GetMessage, and the explicitly-requested thread root) still
// return blank messages, and live-sync (handleMessageEvent) still forwards them:
// a present-but-empty body is a real event (F3/F4), just not a preview-worthy one.
func isBlankMessage(msg *domain.Message) bool {
	return msg.Content == "" && len(msg.Attachments) == 0
}

// GetReaction retrieves details of a specific reaction.
func (m *MautrixAdapter) GetReaction(ctx context.Context, roomID id.RoomID, reactionID id.EventID) (
	*domain.Reaction, error,
) {
	evt, err := m.admin.GetEvent(ctx, roomID, reactionID)
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
	// First, get the thread root message
	rootEvt, err := m.admin.GetEvent(ctx, roomID, threadRootID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread root message: %w", err)
	}

	// Parse the root message. A blank (present-but-empty body, no attachment)
	// root — e.g. a redacted thread root — is dropped so the thread view does not
	// show a spurious empty first message (matches develop and the reply scan
	// below).
	var rootMsg *domain.Message
	if parsed := m.parseMessageEvent(rootEvt, roomID); parsed != nil && !isBlankMessage(parsed) {
		parsed.Reactions = []domain.Reaction{}
		rootMsg = parsed
	}

	// Get thread replies using admin relations API
	chunk, err := m.admin.GetRelations(ctx, roomID, threadRootID, event.RelThread, event.EventMessage)

	messages := make([]domain.Message, 0)
	if err != nil {
		// If no relations found, return just the root message
		m.logger.Debug("No thread relations found, returning only root", "thread_root_id", threadRootID)
		if rootMsg != nil {
			messages = append(messages, *rootMsg)
		}
		return messages, nil
	}

	// Parse thread reply messages (relations API returns newest-first). This is a
	// history scan, so blank preview messages (present-but-empty body, no
	// attachment) are skipped (A1); the explicitly-requested thread root above is
	// kept regardless, matching GetMessage-by-id semantics.
	for _, evt := range chunk {
		if evt.Type != event.EventMessage {
			continue
		}

		msg := m.parseMessageEvent(evt, roomID)
		if msg == nil || isBlankMessage(msg) {
			continue
		}
		msg.Reactions = []domain.Reaction{}
		msg.ThreadID = threadRootID.String()
		messages = append(messages, *msg)
	}

	// Append root message last so the server's .reverse() puts it first
	if rootMsg != nil {
		messages = append(messages, *rootMsg)
	}

	return messages, nil
}

// CreateRoomWithAlias creates a new room with a specific alias based on Alkemio room ID.
func (m *MautrixAdapter) CreateRoomWithAlias(
	ctx context.Context,
	alkemioRoomID uuid.UUID,
	roomType string,
	name, topic, avatarURL, joinRule string,
	customState map[string]map[string]interface{},
	initialMembers []domain.Actor,
) (id.RoomID, error) {
	// Use IDMapper for consistent alias construction
	// Use bot intent for creating rooms
	intent := m.as.BotIntent()

	// Ensure all members exist in Matrix (register ghost users).
	// Members are joined individually after room creation to avoid
	// Synapse's per-room invite rate limit (default burst_count: 10).
	memberUserIDs := make([]id.UserID, 0, len(initialMembers))
	for _, member := range initialMembers {
		userID, err := m.EnsureUser(ctx, member)
		if err != nil {
			return "", fmt.Errorf("failed to ensure member %s: %w", member.ID, err)
		}
		memberUserIDs = append(memberUserIDs, userID)
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
		PowerLevelOverride: &event.PowerLevelsEventContent{
			UsersDefault: 50,
		},
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

	// Add custom io.alkemio.* state events (must be set before members join,
	// so visibility filtering is active from the start).
	for eventType, content := range customState {
		if strings.HasPrefix(eventType, "io.alkemio.") {
			req.InitialState = append(req.InitialState, &event.Event{
				Type:    event.Type{Type: eventType, Class: event.StateEventType},
				Content: event.Content{Parsed: content},
			})
		}
	}

	resp, err := intent.CreateRoom(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create room with alias: %w", err)
	}

	// Set alias via the shared helper (not via RoomAliasName in create request,
	// which auto-sets canonical alias and breaks DM member-name display).
	fullAlias := m.idMapper.RoomAlias(alkemioRoomID)
	if err := m.SetRoomAlias(ctx, resp.RoomID, fullAlias); err != nil {
		return "", fmt.Errorf("failed to set alias on room %s (%s): %w", resp.RoomID, fullAlias, err)
	}

	m.registerDirectRoomParticipants(ctx, resp.RoomID, isDirect, memberUserIDs)

	joinedCount := m.autoJoinAndMarkRead(ctx, resp.RoomID, memberUserIDs)

	// Bot leaves only if other members are present — an empty room becomes
	// unreachable if the last member leaves (Synapse loses server tracking).
	if joinedCount > 0 {
		if _, err := intent.LeaveRoom(ctx, resp.RoomID); err != nil {
			m.logger.Warn("Failed to leave room after creation",
				"room_id", resp.RoomID, "error", err)
		}
	} else {
		m.logger.Info("Bot staying in room (no other members joined yet)",
			"room_id", resp.RoomID)
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
// Returns the number of members successfully joined.
func (m *MautrixAdapter) autoJoinAndMarkRead(ctx context.Context, roomID id.RoomID, invites []id.UserID) int {
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
	return len(joinedUsers)
}

// leaveBotIfNotNeeded checks if the bot is in a non-space room and leaves
// if at least one ghost user is also present. This ensures the bot doesn't
// appear as a member in rooms visible to users.
func (m *MautrixAdapter) leaveBotIfNotNeeded(ctx context.Context, roomID id.RoomID) {
	if isSpace, _ := m.isSpaceRoom(ctx, roomID); isSpace {
		return // Bot stays in spaces
	}

	members, err := m.admin.GetRoomMembers(ctx, roomID)
	if err != nil {
		return
	}

	botMXID := m.as.BotMXID().String()
	botIsMember := false
	ghostCount := 0

	for _, member := range members {
		if member == botMXID {
			botIsMember = true
			continue
		}
		if m.idMapper.AlkemioActorID(id.UserID(member)) != uuid.Nil {
			ghostCount++
		}
	}

	if botIsMember && ghostCount > 0 {
		intent := m.as.BotIntent()
		if _, err := intent.LeaveRoom(ctx, roomID); err != nil {
			m.logger.Warn("Failed to leave room after member added",
				"room_id", roomID, "error", err)
		} else {
			m.logger.Debug("Bot left room after member joined", "room_id", roomID)
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
	var directContent map[string][]id.RoomID
	err = intent.GetAccountData(ctx, "m.direct", &directContent)
	if err != nil {
		m.logger.Debug("No m.direct account data for user", "user_id", user1ID)
		return "", nil
	}

	directRooms, exists := directContent[user2ID.String()]
	if !exists || len(directRooms) == 0 {
		return "", nil
	}

	return m.findRoomWithBothUsers(ctx, directRooms, user1ID, user2ID)
}

// findRoomWithBothUsers checks a list of room IDs to find one where both users are members.
func (m *MautrixAdapter) findRoomWithBothUsers(
	ctx context.Context,
	roomIDs []id.RoomID,
	user1ID, user2ID id.UserID,
) (id.RoomID, error) {
	for _, roomID := range roomIDs {
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

func (m *MautrixAdapter) registerDirectRoomParticipants(
	ctx context.Context, roomID id.RoomID, isDirect bool, memberUserIDs []id.UserID,
) {
	if !isDirect || len(memberUserIDs) != 2 {
		return
	}
	m.setDirectRoomAccountData(ctx, roomID, memberUserIDs[0], memberUserIDs[1])
	m.setDirectRoomAccountData(ctx, roomID, memberUserIDs[1], memberUserIDs[0])
}

// setDirectRoomAccountData adds a room to a user's m.direct account data,
// registering the other user as the DM counterpart.
func (m *MautrixAdapter) setDirectRoomAccountData(
	ctx context.Context,
	roomID id.RoomID,
	userID, otherUserID id.UserID,
) {
	userIntent := m.as.Intent(userID)

	var directContent map[string][]id.RoomID
	if err := userIntent.GetAccountData(ctx, "m.direct", &directContent); err != nil {
		if !errors.Is(err, mautrix.MNotFound) {
			m.logger.Warn("Failed to read m.direct account data",
				"user_id", userID, "other_user_id", otherUserID, "error", err)
			return
		}
		directContent = make(map[string][]id.RoomID)
	}
	if directContent == nil {
		directContent = make(map[string][]id.RoomID)
	}

	otherKey := otherUserID.String()
	for _, existingID := range directContent[otherKey] {
		if existingID == roomID {
			return
		}
	}
	directContent[otherKey] = append(directContent[otherKey], roomID)

	if err := userIntent.SetAccountData(ctx, "m.direct", directContent); err != nil {
		m.logger.Warn("Failed to set m.direct account data",
			"user_id", userID, "other_user_id", otherUserID, "room_id", roomID, "error", err)
	}
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
// Does NOT set canonical alias — aliases are for internal lookups only,
// not for client display (which should show room name or member names).
func (m *MautrixAdapter) SetRoomAlias(ctx context.Context, roomID id.RoomID, alias string) error {
	intent := m.as.BotIntent()

	_, err := intent.CreateAlias(ctx, id.RoomAlias(alias), roomID)
	if err != nil {
		return fmt.Errorf("failed to create room alias: %w", err)
	}

	m.logger.Info("Room alias set", "room_id", roomID, "alias", alias)
	return nil
}

// ============================================================================
// Reconciliation (Element-initiated room creation)
// ============================================================================

// SetQueuePort sets the queue port for RabbitMQ RPC calls during reconciliation.
// Must be called before the event listener starts processing transactions.
func (m *MautrixAdapter) SetQueuePort(queuePort ports.QueuePort) {
	m.queuePort = queuePort
}

// ReconcileRoom completes setup for a room created via Element's check flow.
// Uses the creator's intent (PL 100 as room creator) for all state operations
// since the bot cannot admin-join invite-only rooms.
func (m *MautrixAdapter) ReconcileRoom(
	ctx context.Context, roomID id.RoomID, alkemioRoomID uuid.UUID, creatorUserID id.UserID,
) error {
	m.logger.Info("Starting room reconciliation",
		"room_id", roomID, "alkemio_room_id", alkemioRoomID, "creator", creatorUserID)

	if m.queuePort == nil {
		return fmt.Errorf("reconcile: queue port not configured")
	}

	creatorIntent := m.as.Intent(creatorUserID)

	// 1. Get room info from server
	roomInfoReq := dto.GetRoomInfoRequest{
		AlkemioRoomID: alkemioRoomID.String(),
	}

	respBytes, err := m.queuePort.PublishAndWait(ctx, dto.TopicRoomInfo, roomInfoReq, 3*time.Second)
	if err != nil {
		return fmt.Errorf("reconcile: get room info failed: %w", err)
	}

	var roomInfo dto.GetRoomInfoResponse
	if err := json.Unmarshal(respBytes, &roomInfo); err != nil {
		return fmt.Errorf("reconcile: failed to parse room info: %w", err)
	}

	// 2. EnsureUser + EnsureJoined for each member
	memberUserIDs := make([]id.UserID, 0, len(roomInfo.Members))
	for _, member := range roomInfo.Members {
		actorUUID, err := uuid.Parse(member.ActorID)
		if err != nil {
			return fmt.Errorf("reconcile: invalid actor ID %q: %w", member.ActorID, err)
		}

		actor := domain.Actor{
			ID:          actorUUID,
			DisplayName: member.DisplayName,
		}
		userID, err := m.EnsureUser(ctx, actor)
		if err != nil {
			return fmt.Errorf("reconcile: EnsureUser failed for %s: %w", member.ActorID, err)
		}

		if userID != creatorUserID {
			if _, err := creatorIntent.InviteUser(ctx, roomID, &mautrix.ReqInviteUser{UserID: userID}); err != nil {
				m.logger.Warn("reconcile: invite failed, will attempt EnsureJoined anyway",
					"user_id", userID, "room_id", roomID, "error", err)
			}
		}
		memberIntent := m.as.Intent(userID)
		if err := memberIntent.EnsureJoined(ctx, roomID); err != nil {
			return fmt.Errorf("reconcile: EnsureJoined failed for %s: %w", userID, err)
		}
		memberUserIDs = append(memberUserIDs, userID)
	}

	// 3. Set m.direct account data for DMs
	if roomInfo.IsDirect {
		m.registerDirectRoomParticipants(ctx, roomID, true, memberUserIDs)
	}

	// 4. Set power levels via creator intent (creator has PL 100 as room creator)
	plContent := &event.PowerLevelsEventContent{
		Users: map[id.UserID]int{
			creatorUserID: 50,
		},
		UsersDefault: 50,
	}
	if _, err := creatorIntent.SendStateEvent(ctx, roomID, event.StatePowerLevels, "", plContent); err != nil {
		return fmt.Errorf("reconcile: set power levels failed: %w", err)
	}

	// 5. Set alias via creator intent
	alias := m.idMapper.RoomAlias(alkemioRoomID)
	if err := m.SetRoomAlias(ctx, roomID, alias); err != nil {
		return fmt.Errorf("reconcile: set alias failed: %w", err)
	}

	m.logger.Info("Room reconciliation complete",
		"room_id", roomID, "alkemio_room_id", alkemioRoomID,
		"members", len(memberUserIDs), "is_direct", roomInfo.IsDirect)
	return nil
}

// getRoomCreator reads the m.room.create state event and returns the room creator's user ID.
func (m *MautrixAdapter) getRoomCreator(ctx context.Context, roomID id.RoomID) (id.UserID, error) {
	stateEvents, err := m.admin.GetRoomState(ctx, roomID, "m.room.create")
	if err != nil {
		return "", fmt.Errorf("failed to get m.room.create state: %w", err)
	}
	if len(stateEvents) == 0 {
		return "", fmt.Errorf("no m.room.create state event in room %s", roomID)
	}

	var createEvent struct {
		Content struct {
			Creator string `json:"creator"`
		} `json:"content"`
		Sender string `json:"sender"`
	}
	if err := json.Unmarshal(stateEvents[0], &createEvent); err != nil {
		return "", fmt.Errorf("failed to parse m.room.create: %w", err)
	}

	creator := createEvent.Content.Creator
	if creator == "" {
		creator = createEvent.Sender
	}
	if creator == "" {
		return "", fmt.Errorf("no creator found in m.room.create for room %s", roomID)
	}

	return id.UserID(creator), nil
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

	// Ensure all members exist in Matrix (register ghost users).
	// Members are joined individually after creation to avoid
	// Synapse's per-room invite rate limit (default burst_count: 10).
	memberUserIDs := make([]id.UserID, 0, len(initialMembers))
	for _, member := range initialMembers {
		userID, err := m.EnsureUser(ctx, member)
		if err != nil {
			m.logger.Warn("Failed to ensure member for space", "member_id", member.ID, "error", err)
			continue
		}
		memberUserIDs = append(memberUserIDs, userID)
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
		CreationContent: map[string]interface{}{
			"type": "m.space",
		},
		PowerLevelOverride: &event.PowerLevelsEventContent{
			UsersDefault: 50,
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

	joinedCount := m.autoJoinAndMarkRead(ctx, resp.RoomID, memberUserIDs)

	m.logger.Info(
		"Space created",
		"room_id", resp.RoomID,
		"alias", m.idMapper.SpaceAlias(alkemioContextID),
		"alkemio_context_id", alkemioContextID,
		"members_joined", joinedCount,
	)

	return resp.RoomID, nil
}

// GetSpaceDetails retrieves space metadata and state.
// Uses admin API for state reads — works regardless of bot membership.
func (m *MautrixAdapter) GetSpaceDetails(ctx context.Context, roomID id.RoomID) (*domain.Space, error) {
	var name, topic, alias, avatarURL, joinRule string

	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.name"); err == nil && content != nil {
		if v, ok := content["name"].(string); ok {
			name = v
		}
	}

	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.topic"); err == nil && content != nil {
		if v, ok := content["topic"].(string); ok {
			topic = v
		}
	}

	// Get alias (uses room_aliases table - same source as ResolveAlias)
	if aliases, err := m.GetRoomAliases(ctx, roomID); err == nil {
		alias = m.selectPreferredAlias(aliases)
	}

	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.avatar"); err == nil && content != nil {
		if v, ok := content["url"].(string); ok {
			avatarURL = v
		}
	}

	if content, err := m.admin.GetStateEventContent(ctx, roomID, "m.room.join_rules"); err == nil && content != nil {
		if v, ok := content["join_rule"].(string); ok {
			joinRule = v
		}
	}

	// Get custom io.alkemio.* state events
	customState, _ := m.GetCustomState(ctx, roomID, nil)

	return &domain.Space{
		ID:          roomID,
		Name:        name,
		Topic:       topic,
		Alias:       alias,
		AvatarURL:   avatarURL,
		JoinRule:    joinRule,
		CustomState: customState,
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

		// Determine if child is a space by checking its creation content via admin API
		childRoomID := id.RoomID(stateKey)
		child.IsSpace, _ = m.isSpaceRoom(ctx, childRoomID)

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
		Via:       []string{m.as.HomeserverDomain()},
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
	intent := m.getIntentForRoom(ctx, childID)

	content := &event.SpaceParentEventContent{
		Via:       []string{m.as.HomeserverDomain()},
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
	ctx context.Context, intent intentAPI, roomID id.RoomID,
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
	ctx context.Context, intent intentAPI, roomID id.RoomID,
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
	ctx context.Context, intent intentAPI, roomID id.RoomID,
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
	ctx context.Context, intent intentAPI, roomID id.RoomID,
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
	ctx context.Context, intent intentAPI, roomIDs []id.RoomID,
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
