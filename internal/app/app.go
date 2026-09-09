// Package app manages the application lifecycle, dependency injection, and startup/shutdown logic.
package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	httpinfra "github.com/alkem-io/matrix-adapter/internal/infrastructure/http"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/logger"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/matrix"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/queue"
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
	spaceService := service.NewSpaceService(matrixAdapter, log, idMapper, cfg)
	dmService := service.NewDMService(queueAdapter, log)
	roomCheckService := service.NewRoomCheckService(queueAdapter, idMapper, log)
	readReceiptService := service.NewReadReceiptService(matrixAdapter, log)

	// 5. Initialize Handlers (using shared IDMapper)
	roomHandler := queue.NewRoomHandler(roomService, matrixAdapter, idMapper)
	actorHandler := queue.NewActorHandler(actorService)
	spaceHandler := queue.NewSpaceHandler(spaceService, matrixAdapter, idMapper, cfg)
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

	// Check Room endpoint (synchronous room creation check)
	checkRoomHandler := httpinfra.NewCheckRoomHandler(roomCheckService, cfg.Matrix.HomeserverToken, log)
	checkRoomHandler.RegisterRoutes(router)

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
// The HTTP listener starts in Connect with a transaction gate that drops
// Synapse event deliveries (200 OK, body discarded). This keeps k8s
// probes happy while bot setup runs. Once the outgoing queue is connected
// and routes are registered, EnableEventDelivery opens the gate so real
// events flow through.
//
// Migrations are NOT run here — use cmd/migrate for one-time operations.
func (a *App) Start(ctx context.Context) error {
	a.logger.Info("Starting adapters...")

	// 1. HTTP listener (gate closed) + bot admin + Whoami.
	//    Health probes pass from this point; transactions are dropped.
	if err := a.matrixAdapter.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Matrix: %w", err)
	}

	// 2. Apply bot profile (display name).
	a.matrixAdapter.SetBotProfile(ctx)

	// 3. Connect to RabbitMQ so the event loop has somewhere to publish.
	if err := a.queueAdapter.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Queue: %w", err)
	}

	// 3a. Set queue port on Matrix adapter for reconciliation RPC calls.
	a.matrixAdapter.SetQueuePort(a.queueAdapter)

	// 4. Subscribe queue handlers (incoming commands from Alkemio Server).
	queue.RegisterRoutes(a.queueAdapter, a.roomHandler, a.actorHandler, a.spaceHandler, a.readReceiptHandler, a.logger)

	// 5. Open the transaction gate. From this point, Synapse deliveries
	//    reach mautrix's PutTransaction and flow into the event loop.
	a.matrixAdapter.EnableEventDelivery()

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
