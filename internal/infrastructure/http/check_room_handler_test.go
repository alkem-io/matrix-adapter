package httpinfra

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

const (
	checkTestCreatorUUID = "550e8400-e29b-41d4-a716-446655440001"
	checkTestMemberUUID  = "660e8400-e29b-41d4-a716-446655440002"
	checkTestRoomUUID    = "770e8400-e29b-41d4-a716-446655440003"
)

func newCheckRoomHandler(queue *testutil.MockQueuePort) *CheckRoomHandler {
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper(testHomeserverDomain)
	roomCheckService := service.NewRoomCheckService(queue, idMapper, logger)
	return NewCheckRoomHandler(roomCheckService, testHSToken, logger)
}

func TestCheckRoom_Success_Allow(t *testing.T) {
	resp := dto.CheckRoomResponse{Allow: true, AlkemioRoomID: checkTestRoomUUID}
	respBytes, _ := json.Marshal(resp)

	queue := &testutil.MockQueuePort{PublishAndWaitResponse: respBytes}
	handler := newCheckRoomHandler(queue)

	body := `{"creator":"@` + checkTestCreatorUUID + `:` + testHomeserverDomain + `","members":["@` + checkTestMemberUUID + `:` + testHomeserverDomain + `"],"is_direct":true}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var httpResp dto.CheckRoomHTTPResponse
	if err := json.NewDecoder(rr.Body).Decode(&httpResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !httpResp.Allow {
		t.Error("expected allow=true")
	}
	if httpResp.AlkemioRoomID != checkTestRoomUUID {
		t.Errorf("expected alkemioRoomID %s, got %s", checkTestRoomUUID, httpResp.AlkemioRoomID)
	}
}

func TestCheckRoom_Success_Reject(t *testing.T) {
	resp := dto.CheckRoomResponse{Allow: false, Reason: "duplicate DM"}
	respBytes, _ := json.Marshal(resp)

	queue := &testutil.MockQueuePort{PublishAndWaitResponse: respBytes}
	handler := newCheckRoomHandler(queue)

	body := `{"creator":"@` + checkTestCreatorUUID + `:` + testHomeserverDomain + `","members":["@` + checkTestMemberUUID + `:` + testHomeserverDomain + `"],"is_direct":true}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var httpResp dto.CheckRoomHTTPResponse
	if err := json.NewDecoder(rr.Body).Decode(&httpResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if httpResp.Allow {
		t.Error("expected allow=false")
	}
	if httpResp.Reason != "duplicate DM" {
		t.Errorf("expected reason 'duplicate DM', got %q", httpResp.Reason)
	}
}

func TestCheckRoom_Unauthorized(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	handler := newCheckRoomHandler(queue)

	body := `{"creator":"@test:example.com","members":["@m:example.com"],"is_direct":false}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString(body))

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestCheckRoom_InvalidJSON(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	handler := newCheckRoomHandler(queue)

	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer "+testHSToken)

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestCheckRoom_MissingCreator(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	handler := newCheckRoomHandler(queue)

	body := `{"members":["@` + checkTestMemberUUID + `:` + testHomeserverDomain + `"],"is_direct":false}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestCheckRoom_EmptyMembers(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	handler := newCheckRoomHandler(queue)

	body := `{"creator":"@` + checkTestCreatorUUID + `:` + testHomeserverDomain + `","members":[],"is_direct":false}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestCheckRoom_Timeout(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishAndWaitError: fmt.Errorf("timeout")}
	handler := newCheckRoomHandler(queue)

	body := `{"creator":"@` + checkTestCreatorUUID + `:` + testHomeserverDomain + `","members":["@` + checkTestMemberUUID + `:` + testHomeserverDomain + `"],"is_direct":true}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusGatewayTimeout {
		t.Errorf("expected status %d, got %d", http.StatusGatewayTimeout, rr.Code)
	}
}

func TestCheckRoom_InternalError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishAndWaitError: fmt.Errorf("connection refused")}
	handler := newCheckRoomHandler(queue)

	body := `{"creator":"@` + checkTestCreatorUUID + `:` + testHomeserverDomain + `","members":["@` + checkTestMemberUUID + `:` + testHomeserverDomain + `"],"is_direct":true}`
	req := httptest.NewRequest(http.MethodPost, "/_matrix/app/alkemio/check-room", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testHSToken)

	rr := httptest.NewRecorder()
	handler.handleCheckRoom(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}
