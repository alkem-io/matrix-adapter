package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

// DeviceService owns Matrix device revocation (contract membership-revocation §4):
// central deletion of all of an actor's devices, and the nightly idle sweep.
type DeviceService struct {
	matrix ports.MatrixPort
	logger ports.Logger
}

// NewDeviceService creates a new instance of DeviceService.
func NewDeviceService(matrix ports.MatrixPort, logger ports.Logger) *DeviceService {
	return &DeviceService{matrix: matrix, logger: logger}
}

// RevokeActorDevices deletes ALL of the actor's Matrix devices, invalidating
// access and refresh tokens. Zero devices is a success with an empty result.
func (s *DeviceService) RevokeActorDevices(ctx context.Context, actorID uuid.UUID, reason string) ([]string, error) {
	deviceIDs, err := s.matrix.RevokeActorDevices(ctx, actorID)
	if err != nil {
		s.logger.Error("Failed to revoke actor devices",
			"actor_id", actorID, "reason", reason, "error", err)
		return nil, err
	}
	s.logger.Info("Actor devices revoked",
		"actor_id", actorID, "reason", reason, "deleted_count", len(deviceIDs))
	return deviceIDs, nil
}

// Sweep deletes devices idle longer than the given duration (nightly CronJob;
// spec FR-018). Devices without a recorded last use are skipped and counted;
// the bot is never touched; a dry run reports without deleting.
func (s *DeviceService) Sweep(ctx context.Context, idle time.Duration, dryRun bool) (domain.SweepReport, error) {
	return s.matrix.SweepDevices(ctx, idle, dryRun)
}
