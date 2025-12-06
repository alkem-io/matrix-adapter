package httpinfra

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// Test constants
const (
	testHSToken          = "test-homeserver-token"
	testHomeserverDomain = "matrix.alkemio.org"
	testInitiatorUUID    = "550e8400-e29b-41d4-a716-446655440001"
	testTargetUUID       = "660e8400-e29b-41d4-a716-446655440002"
)

// mockQueuePort is a test double for QueuePort.
type mockQueuePort struct {
	publishedTopic   string
	publishedPayload interface{}
	publishError     error
}

func (m *mockQueuePort) Connect(_ context.Context) error { return nil }
func (m *mockQueuePort) Close() error                    { return nil }
func (m *mockQueuePort) Publish(topic string, payload interface{}) error {
	m.publishedTopic = topic
	m.publishedPayload = payload
	return m.publishError
}
func (m *mockQueuePort) Subscribe(_ string, _ ports.MessageHandler) error {
	return nil
}

// mockLogger is a test double for Logger.
type mockLogger struct{}

func (m *mockLogger) Debug(_ string, _ ...interface{})   {}
func (m *mockLogger) Info(_ string, _ ...interface{})    {}
func (m *mockLogger) Warn(_ string, _ ...interface{})    {}
func (m *mockLogger) Error(_ string, _ ...interface{})   {}
func (m *mockLogger) With(_ ...interface{}) ports.Logger { return m }

func newTestHandler(queue *mockQueuePort) *DMWebhookHandler {
	logger := &mockLogger{}
	idMapper := domain.NewIDMapper(testHomeserverDomain)
	dmService := service.NewDMService(queue, logger)
	return NewDMWebhookHandler(dmService, idMapper, testHSToken, logger)
}

func TestDMWebhook_Success(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `","invitee":"@` + testTargetUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if queue.publishedTopic != dto.TopicRoomDMRequested {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomDMRequested, queue.publishedTopic)
	}

	event, ok := queue.publishedPayload.(dto.DMRequestedEvent)
	if !ok {
		t.Fatalf("expected DMRequestedEvent payload, got %T", queue.publishedPayload)
	}

	if event.InitiatorActorID != testInitiatorUUID {
		t.Errorf("expected initiator %s, got %s", testInitiatorUUID, event.InitiatorActorID)
	}
	if event.TargetActorID != testTargetUUID {
		t.Errorf("expected target %s, got %s", testTargetUUID, event.TargetActorID)
	}
}

func TestDMWebhook_Unauthorized_NoHeader(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `","invitee":"@` + testTargetUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	if queue.publishedTopic != "" {
		t.Error("should not publish when unauthorized")
	}
}

func TestDMWebhook_Unauthorized_WrongToken(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `","invitee":"@` + testTargetUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	if queue.publishedTopic != "" {
		t.Error("should not publish when token is wrong")
	}
}

func TestDMWebhook_InvalidJSON(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer "+testHSToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestDMWebhook_MissingInviter(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"invitee":"@` + testTargetUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestDMWebhook_MissingInvitee(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestDMWebhook_InvalidInviterFormat(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@not-a-uuid:` + testHomeserverDomain + `","invitee":"@` + testTargetUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestDMWebhook_InvalidInviteeFormat(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `","invitee":"@not-a-uuid:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestDMWebhook_SameUser(t *testing.T) {
	queue := &mockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `","invitee":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestValidateAuth(t *testing.T) {
	handler := newTestHandler(&mockQueuePort{})

	tests := []struct {
		name       string
		authHeader string
		expected   bool
	}{
		{name: "valid bearer token", authHeader: "Bearer " + testHSToken, expected: true},
		{name: "empty header", authHeader: "", expected: false},
		{name: "wrong token", authHeader: "Bearer wrong-token", expected: false},
		{name: "no bearer prefix", authHeader: testHSToken, expected: false},
		{name: "basic auth", authHeader: "Basic " + testHSToken, expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			result := handler.validateAuth(req)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestExtractActorID(t *testing.T) {
	handler := newTestHandler(&mockQueuePort{})

	tests := []struct {
		name       string
		matrixID   string
		expectedID uuid.UUID
	}{
		{
			name:       "valid matrix ID",
			matrixID:   "@" + testInitiatorUUID + ":" + testHomeserverDomain,
			expectedID: uuid.MustParse(testInitiatorUUID),
		},
		{
			name:       "invalid UUID",
			matrixID:   "@not-a-uuid:" + testHomeserverDomain,
			expectedID: uuid.Nil,
		},
		{
			name:       "empty string",
			matrixID:   "",
			expectedID: uuid.Nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := handler.extractActorID(tc.matrixID)
			if result != tc.expectedID {
				t.Errorf("expected %v, got %v", tc.expectedID, result)
			}
		})
	}
}
