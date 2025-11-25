// Package app manages the application lifecycle, dependency injection, and startup/shutdown logic.
package app

import (
	"context"
	"fmt"

	"github.com/alkemio/matrix-adapter-go/internal/config"
	"github.com/alkemio/matrix-adapter-go/internal/core/ports"
	"github.com/alkemio/matrix-adapter-go/internal/core/service"
	httpinfra "github.com/alkemio/matrix-adapter-go/internal/infrastructure/http"
	"github.com/alkemio/matrix-adapter-go/internal/infrastructure/logger"
	"github.com/alkemio/matrix-adapter-go/internal/infrastructure/matrix"
	"github.com/alkemio/matrix-adapter-go/internal/infrastructure/queue"
)

// App represents the Matrix Adapter application and holds references to all its components.
type App struct {
	cfg           *config.Config
	logger        ports.Logger
	matrixAdapter ports.MatrixPort
	queueAdapter  ports.QueuePort
	healthServer  *httpinfra.HealthServer

	// Handlers
	actorHandler *queue.ActorHandler
	roomHandler  *queue.RoomHandler
	adminHandler *queue.AdminHandler
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
	actorService := service.NewActorService(matrixAdapter, log)
	roomService := service.NewRoomService(matrixAdapter, log)
	adminService := service.NewAdminService(matrixAdapter, log)
	eventService := service.NewEventService(queueAdapter, log, cfg)

	// 4. Initialize Handlers
	actorHandler := queue.NewActorHandler(actorService)
	roomHandler := queue.NewRoomHandler(roomService)
	adminHandler := queue.NewAdminHandler(adminService)

	// 5. Wire up Event Listeners (Matrix -> Queue)
	// This can be done here as it just registers a callback, doesn't start IO usually.
	matrixAdapter.OnMessage(eventService.HandleMessage)

	return &App{
		cfg:           cfg,
		logger:        log,
		matrixAdapter: matrixAdapter,
		queueAdapter:  queueAdapter,
		healthServer:  httpinfra.NewHealthServer("8081", log),
		actorHandler:  actorHandler,
		roomHandler:   roomHandler,
		adminHandler:  adminHandler,
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
	queue.RegisterRoutes(a.queueAdapter, a.actorHandler, a.roomHandler, a.adminHandler, a.logger)

	// Start Health Server
	a.healthServer.Start()

	return nil
}

// Stop gracefully stops the application and its components.
func (a *App) Stop(ctx context.Context) {
	a.logger.Info("Stopping application...")

	if err := a.healthServer.Stop(ctx); err != nil {
		a.logger.Error("Failed to stop health server", "error", err)
	}

	if err := a.matrixAdapter.Disconnect(); err != nil {
		a.logger.Error("Failed to disconnect Matrix", "error", err)
	}

	if err := a.queueAdapter.Close(); err != nil {
		a.logger.Error("Failed to close Queue", "error", err)
	}

	a.logger.Info("Application stopped")
}
