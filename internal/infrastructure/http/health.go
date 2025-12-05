// Package httpinfra provides HTTP server implementations for health checks and other endpoints.
package httpinfra

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// HealthServer provides liveness and readiness probes.
type HealthServer struct {
	server *http.Server
	logger ports.Logger
}

// NewHealthServer creates a new instance of HealthServer.
func NewHealthServer(port string, logger ports.Logger) *HealthServer {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"/health/live", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		},
	)

	mux.HandleFunc(
		"/health/ready", func(w http.ResponseWriter, _ *http.Request) {
			// Note: In a real implementation, check dependencies (Matrix, RabbitMQ) here
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		},
	)

	return &HealthServer{
		server: &http.Server{
			Addr:              ":" + port,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Start starts the health server in a goroutine.
func (s *HealthServer) Start() {
	go func() {
		s.logger.Info("Starting Health Server", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Health Server failed", "error", err)
		}
	}()
}

// Stop gracefully shuts down the health server.
func (s *HealthServer) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
