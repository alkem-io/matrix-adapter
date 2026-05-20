package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

const roomCheckTimeout = 3 * time.Second

// RoomCheckService handles the synchronous room creation check flow:
// receives a check request from the HTTP handler, converts Matrix IDs to actor UUIDs,
// calls the server via RabbitMQ request-reply, and returns allow/deny.
type RoomCheckService struct {
	queue    ports.QueuePort
	idMapper *domain.IDMapper
	logger   ports.Logger
}

// NewRoomCheckService creates a new instance of RoomCheckService.
func NewRoomCheckService(queue ports.QueuePort, idMapper *domain.IDMapper, logger ports.Logger) *RoomCheckService {
	return &RoomCheckService{
		queue:    queue,
		idMapper: idMapper,
		logger:   logger,
	}
}

// CheckRoom forwards the room creation check to the server via RabbitMQ request-reply.
func (s *RoomCheckService) CheckRoom(ctx context.Context, req domain.RoomCheckRequest) (*domain.RoomCheckResponse, error) {
	creatorActorID := s.idMapper.AlkemioActorID(req.Creator)
	if creatorActorID == uuid.Nil {
		return nil, fmt.Errorf("invalid creator user ID: %s", req.Creator)
	}

	memberActorIDs := make([]string, 0, len(req.Members))
	for _, member := range req.Members {
		actorID := s.idMapper.AlkemioActorID(member)
		if actorID == uuid.Nil {
			return nil, fmt.Errorf("invalid member user ID: %s", member)
		}
		memberActorIDs = append(memberActorIDs, actorID.String())
	}

	rmqReq := dto.CheckRoomRequest{
		CreatorActorID: creatorActorID.String(),
		MemberActorIDs: memberActorIDs,
		IsDirect:       req.IsDirect,
	}

	s.logger.Info("Sending room check to server",
		"creator", creatorActorID.String(),
		"members", len(memberActorIDs),
		"isDirect", req.IsDirect,
	)

	respBytes, err := s.queue.PublishAndWait(ctx, dto.TopicRoomCheck, rmqReq, roomCheckTimeout)
	if err != nil {
		return nil, fmt.Errorf("room check RPC failed: %w", err)
	}

	var rmqResp dto.CheckRoomResponse
	if err := json.Unmarshal(respBytes, &rmqResp); err != nil {
		return nil, fmt.Errorf("failed to parse room check response: %w", err)
	}

	resp := &domain.RoomCheckResponse{
		Allow:         rmqResp.Allow,
		AlkemioRoomID: rmqResp.AlkemioRoomID,
		Reason:        rmqResp.Reason,
	}

	if resp.Allow {
		s.logger.Info("Room check approved",
			"alkemioRoomID", resp.AlkemioRoomID,
		)
	} else {
		s.logger.Info("Room check rejected",
			"reason", resp.Reason,
		)
	}

	return resp, nil
}
