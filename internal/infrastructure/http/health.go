// Package httpinfra provides HTTP server implementations for health checks and other endpoints.
package httpinfra

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// HTTPServer provides the main HTTP server for health checks and webhooks.
type HTTPServer struct {
	server *http.Server
	mux    *http.ServeMux
	logger ports.Logger
}

// NewHTTPServer creates a new instance of HTTPServer with health endpoints.
func NewHTTPServer(port string, logger ports.Logger) *HTTPServer {
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

	return &HTTPServer{
		mux: mux,
		server: &http.Server{
			Addr:              ":" + port,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Mux returns the underlying ServeMux to allow additional route registration.
func (s *HTTPServer) Mux() *http.ServeMux {
	return s.mux
}

// Start starts the HTTP server in a goroutine.
func (s *HTTPServer) Start() {
	go func() {
		s.logger.Info("Starting HTTP Server", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP Server failed", "error", err)
		}
	}()
}

// Stop gracefully shuts down the HTTP server.
func (s *HTTPServer) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// HealthServer is an alias for HTTPServer for backward compatibility.
// Deprecated: Use HTTPServer instead.
type HealthServer = HTTPServer

// NewHealthServer creates a new instance of HTTPServer (alias for backward compatibility).
// Deprecated: Use NewHTTPServer instead.
func NewHealthServer(port string, logger ports.Logger) *HTTPServer {
	return NewHTTPServer(port, logger)
}
