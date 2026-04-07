package httpinfra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

type healthMockLogger struct{}

func (l *healthMockLogger) Debug(_ string, _ ...interface{}) {}
func (l *healthMockLogger) Info(_ string, _ ...interface{})  {}
func (l *healthMockLogger) Warn(_ string, _ ...interface{})  {}
func (l *healthMockLogger) Error(_ string, _ ...interface{}) {}
func (l *healthMockLogger) With(_ ...interface{}) ports.Logger {
	return l
}

func TestNewHTTPServer(t *testing.T) {
	srv := NewHTTPServer("8080", &healthMockLogger{})
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.Mux() == nil {
		t.Fatal("expected non-nil mux")
	}
}

func TestHealthLive(t *testing.T) {
	srv := NewHTTPServer("0", &healthMockLogger{})
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("expected 'OK', got %q", w.Body.String())
	}
}

func TestHealthReady(t *testing.T) {
	srv := NewHTTPServer("0", &healthMockLogger{})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("expected 'OK', got %q", w.Body.String())
	}
}

func TestHTTPServer_StartAndStop(t *testing.T) {
	srv := NewHTTPServer("0", &healthMockLogger{})
	srv.Start()
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestDMWebhookHandler_RegisterRoutes(t *testing.T) {
	handler := NewDMWebhookHandler(nil, nil, "token", &healthMockLogger{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Verify the route was registered by making a request
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should get 401 (unauthorized) not 404 (route not found)
	if w.Code == http.StatusNotFound {
		t.Error("route was not registered — got 404")
	}
}
