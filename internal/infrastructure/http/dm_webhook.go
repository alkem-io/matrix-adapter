// Package httpinfra provides HTTP server implementations for health checks and other endpoints.
package httpinfra

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// DMWebhookHandler handles incoming DM request webhooks from Synapse.
//
// Deprecated: Replaced by CheckRoomHandler and the synchronous check-room flow.
// Kept during server-side transition; remove once the server no longer sends DM webhooks.
type DMWebhookHandler struct {
	dmService *service.DMService //nolint:staticcheck // deprecated but kept during transition
	idMapper  *domain.IDMapper
	hsToken   string
	logger    ports.Logger
}

// NewDMWebhookHandler creates a new instance of DMWebhookHandler.
func NewDMWebhookHandler(
	dmService *service.DMService, //nolint:staticcheck // deprecated but kept during transition
	idMapper *domain.IDMapper,
	hsToken string,
	logger ports.Logger,
) *DMWebhookHandler {
	return &DMWebhookHandler{
		dmService: dmService,
		idMapper:  idMapper,
		hsToken:   hsToken,
		logger:    logger,
	}
}

// RegisterRoutes registers the DM webhook endpoint with the given mux.
func (h *DMWebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /_matrix/app/alkemio/dm-request", h.handleDMRequest)
}

// handleDMRequest processes DM request webhooks from Synapse.
func (h *DMWebhookHandler) handleDMRequest(w http.ResponseWriter, r *http.Request) {
	if !ValidateBearerToken(r, h.hsToken) {
		h.logger.Warn("DM webhook: unauthorized request")
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// Parse the webhook payload
	var payload dto.DMWebhookPayload //nolint:staticcheck // deprecated but kept during transition
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.logger.Warn("DM webhook: invalid JSON payload", "error", err)
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
		return
	}

	// Validate required fields
	if payload.Inviter == "" || payload.Invitee == "" {
		h.logger.Warn("DM webhook: missing required fields",
			"has_inviter", payload.Inviter != "",
			"has_invitee", payload.Invitee != "",
		)
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_required_fields"})
		return
	}

	// Extract actor IDs from Matrix user IDs
	ctx := r.Context()
	initiatorID := h.extractActorID(ctx, payload.Inviter)
	if initiatorID == uuid.Nil {
		h.logger.Warn("DM webhook: invalid inviter format", "inviter", payload.Inviter)
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_inviter_format"})
		return
	}

	targetID := h.extractActorID(ctx, payload.Invitee)
	if targetID == uuid.Nil {
		h.logger.Warn("DM webhook: invalid invitee format", "invitee", payload.Invitee)
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_invitee_format"})
		return
	}

	// Publish DM request event
	if err := h.dmService.PublishDMRequest(initiatorID, targetID); err != nil {
		h.logger.Error("DM webhook: failed to publish event",
			"initiator", initiatorID.String(),
			"target", targetID.String(),
			"error", err,
		)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	h.logger.Info("DM webhook: request forwarded",
		"initiator", initiatorID.String(),
		"target", targetID.String(),
	)

	WriteJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// extractActorID extracts the Alkemio actor UUID from a Matrix user ID.
// Matrix user ID format: @{uuid}:{domain}
func (h *DMWebhookHandler) extractActorID(_ context.Context, matrixUserID string) uuid.UUID {
	return h.idMapper.AlkemioActorID(id.UserID(matrixUserID))
}
