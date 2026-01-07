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
	"github.com/alkem-io/matrix-adapter-go/internal/infrastructure/alkemiodb"
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

	// ActorResolver for DB-based actor ID mapping (optional, temporary)
	actorResolver *alkemiodb.Adapter

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

	// 3. Initialize IDMapper and optional ActorResolver (must be done BEFORE creating services/handlers)
	// Create the single shared IDMapper
	idMapper := domain.NewIDMapper(cfg.Matrix.HomeserverName)

	// Initialize ActorResolver if enabled (temporary feature for migration period)
	var actorResolverAdapter *alkemiodb.Adapter
	if cfg.ActorResolver.Enabled {
		log.Info("ActorResolver enabled, connecting to Alkemio database...")
		connString := fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=disable",
			cfg.ActorResolver.Username,
			cfg.ActorResolver.Password,
			cfg.ActorResolver.Host,
			cfg.ActorResolver.Port,
			cfg.ActorResolver.Database,
		)
		actorResolverAdapter, err = alkemiodb.NewAdapter(context.Background(), connString, log.(*logger.ZapLogger).Underlying())
		if err != nil {
			return nil, fmt.Errorf("failed to create ActorResolver (ACTOR_ID_MAPPER_ENABLED=true but database unreachable): %w", err)
		}
		log.Info("ActorResolver connected to Alkemio database")

		// Wire ActorResolver to IDMapper and MatrixAdapter
		idMapper.SetActorResolver(actorResolverAdapter)
		matrixAdapter.SetActorResolver(actorResolverAdapter)
		log.Info("ActorResolver wired to IDMapper and MatrixAdapter for DB-based actor ID mapping")
	} else {
		log.Info("ActorResolver disabled, using direct ID mapping")
	}

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
	spaceHandler := queue.NewSpaceHandler(spaceService)
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
		OnMemberLeft:         eventService.HandleMemberLeft,
		OnReadReceiptUpdated: eventService.HandleReadReceiptUpdated,
		OnMessageEdited:      eventService.HandleMessageEdited,
		OnMessageRedacted:    eventService.HandleMessageRedacted,
		OnRoomCreated:        eventService.HandleRoomCreated,
		OnMemberUpdated:      eventService.HandleMemberUpdated,
	})

	return &App{
		cfg:                cfg,
		logger:             log,
		matrixAdapter:      matrixAdapter,
		queueAdapter:       queueAdapter,
		actorResolver:      actorResolverAdapter,
		roomHandler:        roomHandler,
		actorHandler:       actorHandler,
		spaceHandler:       spaceHandler,
		readReceiptHandler: readReceiptHandler,
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
	queue.RegisterRoutes(a.queueAdapter, a.roomHandler, a.actorHandler, a.spaceHandler, a.readReceiptHandler, a.logger)

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

	// Close ActorResolver database connection if enabled
	if a.actorResolver != nil {
		a.actorResolver.Close()
	}

	a.logger.Info("Application stopped")
}
