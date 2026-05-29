package httpinfra

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// CheckRoomHandler handles synchronous room creation check requests from the Synapse module.
type CheckRoomHandler struct {
	roomCheckService *service.RoomCheckService
	hsToken          string
	logger           ports.Logger
}

// NewCheckRoomHandler creates a new instance of CheckRoomHandler.
func NewCheckRoomHandler(
	roomCheckService *service.RoomCheckService,
	hsToken string,
	logger ports.Logger,
) *CheckRoomHandler {
	return &CheckRoomHandler{
		roomCheckService: roomCheckService,
		hsToken:          hsToken,
		logger:           logger,
	}
}

// RegisterRoutes registers the check-room endpoint with the given mux.
func (h *CheckRoomHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /_matrix/app/alkemio/check-room", h.handleCheckRoom)
}

func (h *CheckRoomHandler) handleCheckRoom(w http.ResponseWriter, r *http.Request) {
	if !ValidateBearerToken(r, h.hsToken) {
		h.logger.Warn("check-room: unauthorized request")
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var httpReq dto.CheckRoomHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&httpReq); err != nil {
		h.logger.Warn("check-room: invalid JSON payload", "error", err)
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
		return
	}

	if httpReq.Creator == "" || len(httpReq.Members) == 0 {
		h.logger.Warn("check-room: missing required fields")
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
		return
	}

	members := make([]id.UserID, len(httpReq.Members))
	for i, m := range httpReq.Members {
		members[i] = id.UserID(m)
	}

	domainReq := domain.RoomCheckRequest{
		Creator:  id.UserID(httpReq.Creator),
		Members:  members,
		IsDirect: httpReq.IsDirect,
	}

	resp, err := h.roomCheckService.CheckRoom(r.Context(), domainReq)
	if err != nil {
		if isTimeout(err) {
			h.logger.Error("check-room: server reply timeout", "error", err)
			WriteJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "timeout"})
			return
		}
		h.logger.Error("check-room: internal error", "error", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	httpResp := dto.CheckRoomHTTPResponse{
		Allow:         resp.Allow,
		AlkemioRoomID: resp.AlkemioRoomID,
		Reason:        resp.Reason,
	}

	WriteJSON(w, http.StatusOK, httpResp)
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), "timeout")
}
