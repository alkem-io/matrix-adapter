// Package app manages the application lifecycle, dependency injection, and startup/shutdown logic.
package app

import (
	"context"
	"fmt"

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
	matrixAdapter ports.MatrixPort
	queueAdapter  ports.QueuePort
	httpServer    *httpinfra.HTTPServer

	// Handlers
	roomHandler  *queue.RoomHandler
	actorHandler *queue.ActorHandler
	spaceHandler *queue.SpaceHandler
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

	// 3. Initialize Services
	roomService := service.NewRoomService(matrixAdapter, log)
	actorService := service.NewActorService(matrixAdapter, log)
	eventService := service.NewEventService(queueAdapter, log, cfg)
	spaceService := service.NewSpaceService(matrixAdapter, log)
	dmService := service.NewDMService(queueAdapter, log)

	// 4. Initialize Handlers
	roomHandler := queue.NewRoomHandler(roomService, matrixAdapter)
	actorHandler := queue.NewActorHandler(actorService)
	spaceHandler := queue.NewSpaceHandler(spaceService)

	// 5. Initialize HTTP Server with health checks and webhook endpoints
	httpServer := httpinfra.NewHTTPServer("8081", log)

	// 6. Register DM Webhook Handler
	idMapper := domain.NewIDMapper(cfg.Matrix.HomeserverName)
	dmWebhookHandler := httpinfra.NewDMWebhookHandler(dmService, idMapper, cfg.Matrix.HomeserverToken, log)
	dmWebhookHandler.RegisterRoutes(httpServer.Mux())

	// 7. Wire up Event Listeners (Matrix -> Queue)
	// This can be done here as it just registers a callback, doesn't start IO usually.
	matrixAdapter.OnMessage(eventService.HandleMessage)

	return &App{
		cfg:           cfg,
		logger:        log,
		matrixAdapter: matrixAdapter,
		queueAdapter:  queueAdapter,
		httpServer:    httpServer,
		roomHandler:   roomHandler,
		actorHandler:  actorHandler,
		spaceHandler:  spaceHandler,
	}, nil
}

// Start starts the application adapters and servers.
func (a *App) Start(ctx context.Context) error {
	a.logger.Info("Starting adapters...")

	if err := a.matrixAdapter.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Matrix: %w", err)
	}

	if err := a.queueAdapter.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Queue: %w", err)
	}

	// Wire up Queue Subscribers
	queue.RegisterRoutes(a.queueAdapter, a.roomHandler, a.actorHandler, a.spaceHandler, a.logger)

	// Start HTTP Server (health checks + webhooks)
	a.httpServer.Start()

	return nil
}

// Stop gracefully stops the application and its components.
func (a *App) Stop(ctx context.Context) {
	a.logger.Info("Stopping application...")

	if err := a.httpServer.Stop(ctx); err != nil {
		a.logger.Error("Failed to stop HTTP server", "error", err)
	}

	if err := a.matrixAdapter.Disconnect(); err != nil {
		a.logger.Error("Failed to disconnect Matrix", "error", err)
	}

	if err := a.queueAdapter.Close(); err != nil {
		a.logger.Error("Failed to close Queue", "error", err)
	}

	a.logger.Info("Application stopped")
}
