// Package app manages the application lifecycle, dependency injection, and startup/shutdown logic.
package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/alkem-io/matrix-adapter-go/internal/config"
	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	httpinfra "github.com/alkem-io/matrix-adapter-go/internal/infrastructure/http"
	"github.com/alkem-io/matrix-adapter-go/internal/infrastructure/logger"
	"github.com/alkem-io/matrix-adapter-go/internal/infrastructure/matrix"
	"github.com/alkem-io/matrix-adapter-go/internal/infrastructure/queue"
)

// App represents the Matrix Adapter application and holds references to all its components.
type App struct {
	cfg           *config.Config
	logger        ports.Logger
	matrixAdapter *matrix.MautrixAdapter
	queueAdapter  ports.QueuePort

	// Handlers
	roomHandler        *queue.RoomHandler
	actorHandler       *queue.ActorHandler
	spaceHandler       *queue.SpaceHandler
	readReceiptHandler *queue.ReadReceiptHandler
}

// NewApp initializes a new instance of the App with the given configuration.
// It sets up the logger, adapters, services, and handlers.
func NewApp(cfg *config.Config) (*App, error) {
	// 1. Initialize Logger
	log, err := logger.NewZapLogger(cfg.App.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}
	log.Info("Initializing Matrix Adapter Application", "env", cfg.App.Environment)

	// 2. Initialize Adapters
	matrixAdapter, err := matrix.NewMautrixAdapter(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create Matrix adapter: %w", err)
	}

	queueAdapter, err := queue.NewWatermillAdapter(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create Queue adapter: %w", err)
	}

	// 3. Initialize IDMapper
	idMapper := domain.NewIDMapper(cfg.Matrix.HomeserverName)

	// 4. Initialize Services (using shared IDMapper)
	roomService := service.NewRoomService(matrixAdapter, log, idMapper)
	actorService := service.NewActorService(matrixAdapter, log)
	eventService := service.NewEventService(queueAdapter, log, cfg)
	spaceService := service.NewSpaceService(matrixAdapter, log, idMapper)
	dmService := service.NewDMService(queueAdapter, log)
	readReceiptService := service.NewReadReceiptService(matrixAdapter, log)

	// 5. Initialize Handlers (using shared IDMapper)
	roomHandler := queue.NewRoomHandler(roomService, matrixAdapter, idMapper)
	actorHandler := queue.NewActorHandler(actorService)
	spaceHandler := queue.NewSpaceHandler(spaceService, matrixAdapter, idMapper)
	readReceiptHandler := queue.NewReadReceiptHandler(readReceiptService, matrixAdapter, idMapper, log)

	// 6. Register all HTTP endpoints on AppService router (single port 8280)
	router := matrixAdapter.Router()

	// Health endpoints for k8s probes
	router.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	router.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// DM Webhook endpoint (using shared IDMapper)
	dmWebhookHandler := httpinfra.NewDMWebhookHandler(dmService, idMapper, cfg.Matrix.HomeserverToken, log)
	dmWebhookHandler.RegisterRoutes(router)

	// 7. Wire up Event Listeners (Matrix -> Queue)
	matrixAdapter.SetEventHandlers(matrix.EventHandlers{
		OnMessage:            eventService.HandleMessage,
		OnReactionAdded:      eventService.HandleReactionAdded,
		OnReactionRemoved:    eventService.HandleReactionRemoved,
		OnReadReceiptUpdated: eventService.HandleReadReceiptUpdated,
		OnMessageEdited:      eventService.HandleMessageEdited,
		OnMessageRedacted:    eventService.HandleMessageRedacted,
		OnRoomCreated:        eventService.HandleRoomCreated,
		OnMemberUpdated:      eventService.HandleMemberUpdated,
		OnRoomUpdated:        eventService.HandleRoomUpdated,
		OnSpaceUpdated:       eventService.HandleSpaceUpdated,
	})

	return &App{
		cfg:                cfg,
		logger:             log,
		matrixAdapter:      matrixAdapter,
		queueAdapter:       queueAdapter,
		roomHandler:        roomHandler,
		actorHandler:       actorHandler,
		spaceHandler:       spaceHandler,
		readReceiptHandler: readReceiptHandler,
	}, nil
}

// Start starts the application adapters and servers.
//
// Ordering is deliberate. Matrix mutating operations (migrations, display
// name) run while the AppService HTTP listener is still down, so any
// events they generate queue in Synapse's `application_services_txns`
// instead of fanning into our event loop mid-mutation. The listener
// comes up only once the outgoing queue is connected and routes are
// registered; at that point Synapse drains its queue in order.
//
// RunMigrations runs BEFORE SetBotProfile so the leave-non-space-rooms
// migration shrinks the bot's joined-room set first, which dramatically
// reduces the display-name fan-out radius.
func (a *App) Start(ctx context.Context) error {
	a.logger.Info("Starting adapters...")

	// 1. Bot admin + Whoami. No room state touched yet.
	if err := a.matrixAdapter.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Matrix: %w", err)
	}

	// 2. One-time room state migrations. Disable/replace this call
	//    once the deployment is known to be clean.
	a.matrixAdapter.RunMigrations(ctx)

	// 3. Apply bot profile (display name). Done after migrations so the
	//    fan-out only hits the rooms the bot is still in (spaces).
	a.matrixAdapter.SetBotProfile(ctx)

	// 4. Connect to RabbitMQ so the event loop has somewhere to publish.
	if err := a.queueAdapter.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Queue: %w", err)
	}

	// 5. Subscribe queue handlers (incoming commands from Alkemio Server).
	queue.RegisterRoutes(a.queueAdapter, a.roomHandler, a.actorHandler, a.spaceHandler, a.readReceiptHandler, a.logger)

	// 6. Bring the AppService HTTP listener online. Synapse will start
	//    delivering everything it queued during steps 1-3, and our event
	//    loop (already running, sitting on an empty channel) will drain it.
	if err := a.matrixAdapter.StartListener(ctx); err != nil {
		return fmt.Errorf("failed to start Matrix listener: %w", err)
	}

	return nil
}

// Stop gracefully stops the application and its components.
func (a *App) Stop(_ context.Context) {
	a.logger.Info("Stopping application...")

	if err := a.matrixAdapter.Disconnect(); err != nil {
		a.logger.Error("Failed to disconnect Matrix", "error", err)
	}

	if err := a.queueAdapter.Close(); err != nil {
		a.logger.Error("Failed to close Queue", "error", err)
	}

	a.logger.Info("Application stopped")
}
