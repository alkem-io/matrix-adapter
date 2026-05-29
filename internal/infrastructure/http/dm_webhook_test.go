package httpinfra

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

const (
	testHSToken          = "test-homeserver-token"
	testHomeserverDomain = "matrix.alkemio.org"
	testInitiatorUUID    = "550e8400-e29b-41d4-a716-446655440001"
	testTargetUUID       = "660e8400-e29b-41d4-a716-446655440002"
)

func newTestHandler(queue *testutil.MockQueuePort) *DMWebhookHandler {
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper(testHomeserverDomain)
	dmService := service.NewDMService(queue, logger) //nolint:staticcheck // deprecated but kept during transition
	return NewDMWebhookHandler(dmService, idMapper, testHSToken, logger)
}

func TestDMWebhook_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
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

	if queue.PublishedTopic != dto.TopicRoomDMRequested {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomDMRequested, queue.PublishedTopic)
	}

	event, ok := queue.PublishedPayload.(dto.DMRequestedEvent) //nolint:staticcheck // deprecated but kept during transition
	if !ok {
		t.Fatalf("expected DMRequestedEvent payload, got %T", queue.PublishedPayload)
	}

	if event.InitiatorActorID != testInitiatorUUID {
		t.Errorf("expected initiator %s, got %s", testInitiatorUUID, event.InitiatorActorID)
	}
	if event.TargetActorID != testTargetUUID {
		t.Errorf("expected target %s, got %s", testTargetUUID, event.TargetActorID)
	}
}

func TestDMWebhook_Unauthorized_NoHeader(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	handler := newTestHandler(queue)

	body := `{"inviter":"@` + testInitiatorUUID + `:` + testHomeserverDomain + `","invitee":"@` + testTargetUUID + `:` + testHomeserverDomain + `"}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/dm-request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.handleDMRequest(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	if queue.PublishedTopic != "" {
		t.Error("should not publish when unauthorized")
	}
}

func TestDMWebhook_Unauthorized_WrongToken(t *testing.T) {
	queue := &testutil.MockQueuePort{}
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

	if queue.PublishedTopic != "" {
		t.Error("should not publish when token is wrong")
	}
}

func TestDMWebhook_InvalidJSON(t *testing.T) {
	queue := &testutil.MockQueuePort{}
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
	queue := &testutil.MockQueuePort{}
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
	queue := &testutil.MockQueuePort{}
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
	queue := &testutil.MockQueuePort{}
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
	queue := &testutil.MockQueuePort{}
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
	queue := &testutil.MockQueuePort{}
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

func TestValidateBearerToken(t *testing.T) {
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

			result := ValidateBearerToken(req, testHSToken)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}

	t.Run("empty expected token rejects", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer ")
		if ValidateBearerToken(req, "") {
			t.Error("expected false when expected token is empty")
		}
	})
}

func TestExtractActorID(t *testing.T) {
	handler := newTestHandler(&testutil.MockQueuePort{})

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
			result := handler.extractActorID(context.Background(), tc.matrixID)
			if result != tc.expectedID {
				t.Errorf("expected %v, got %v", tc.expectedID, result)
			}
		})
	}
}
