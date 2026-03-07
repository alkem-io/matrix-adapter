// Package httpinfra provides HTTP server implementations for health checks and other endpoints.
package httpinfra

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// DMWebhookHandler handles incoming DM request webhooks from Synapse.
type DMWebhookHandler struct {
	dmService *service.DMService
	idMapper  *domain.IDMapper
	hsToken   string
	logger    ports.Logger
}

// NewDMWebhookHandler creates a new instance of DMWebhookHandler.
func NewDMWebhookHandler(
	dmService *service.DMService,
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
	// Validate authorization
	if !h.validateAuth(r) {
		h.logger.Warn("DM webhook: unauthorized request")
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Parse the webhook payload
	var payload dto.DMWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.logger.Warn("DM webhook: invalid JSON payload", "error", err)
		http.Error(w, `{"error":"invalid_payload"}`, http.StatusBadRequest)
		return
	}

	// Validate required fields
	if payload.Inviter == "" || payload.Invitee == "" {
		h.logger.Warn("DM webhook: missing required fields",
			"has_inviter", payload.Inviter != "",
			"has_invitee", payload.Invitee != "",
		)
		http.Error(w, `{"error":"missing_required_fields"}`, http.StatusBadRequest)
		return
	}

	// Extract actor IDs from Matrix user IDs
	ctx := r.Context()
	initiatorID := h.extractActorID(ctx, payload.Inviter)
	if initiatorID == uuid.Nil {
		h.logger.Warn("DM webhook: invalid inviter format", "inviter", payload.Inviter)
		http.Error(w, `{"error":"invalid_inviter_format"}`, http.StatusBadRequest)
		return
	}

	targetID := h.extractActorID(ctx, payload.Invitee)
	if targetID == uuid.Nil {
		h.logger.Warn("DM webhook: invalid invitee format", "invitee", payload.Invitee)
		http.Error(w, `{"error":"invalid_invitee_format"}`, http.StatusBadRequest)
		return
	}

	// Publish DM request event
	if err := h.dmService.PublishDMRequest(initiatorID, targetID); err != nil {
		h.logger.Error("DM webhook: failed to publish event",
			"initiator", initiatorID.String(),
			"target", targetID.String(),
			"error", err,
		)
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}

	h.logger.Info("DM webhook: request forwarded",
		"initiator", initiatorID.String(),
		"target", targetID.String(),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// validateAuth checks the Authorization header for a valid Bearer token.
func (h *DMWebhookHandler) validateAuth(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	// Expected format: "Bearer <token>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(h.hsToken)) == 1
}

// extractActorID extracts the Alkemio actor UUID from a Matrix user ID.
// Matrix user ID format: @{uuid}:{domain}
func (h *DMWebhookHandler) extractActorID(_ context.Context, matrixUserID string) uuid.UUID {
	return h.idMapper.AlkemioActorID(id.UserID(matrixUserID))
}
