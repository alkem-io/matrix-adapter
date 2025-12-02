package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// RoomHandler handles queue messages related to room operations.
type RoomHandler struct {
	service *service.RoomService
	matrix  ports.MatrixPort
}

// NewRoomHandler creates a new instance of RoomHandler.
func NewRoomHandler(service *service.RoomService, matrix ports.MatrixPort) *RoomHandler {
	return &RoomHandler{
		service: service,
		matrix:  matrix,
	}
}

// ============================================================================
// Room Handlers (communication.room.*)
// ============================================================================

// HandleCreateRoom handles communication.room.create topic.
func (h *RoomHandler) HandleCreateRoom(payload []byte) (interface{}, error) {
	var req dto.CreateRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}

	// Convert initial members to domain actors
	var initialMembers []domain.Actor
	for _, memberID := range req.InitialMembers {
		if memberID.UUID() != uuid.Nil {
			initialMembers = append(initialMembers, domain.Actor{ID: memberID.UUID()})
		}
	}

	err := h.service.CreateRoomWithAlkemioID(
		context.Background(),
		req.AlkemioRoomID.UUID(),
		string(req.Type),
		req.Name,
		req.Topic,
		initialMembers,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.CreateRoomResponse{BaseResponse: dto.NewSuccessResponse()}, nil
}

// HandleGetRoom handles communication.room.get topic.
func (h *RoomHandler) HandleGetRoom(payload []byte) (interface{}, error) {
	var req dto.GetRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}

	room, err := h.service.GetRoomWithMessages(context.Background(), req.AlkemioRoomID.UUID())
	if err != nil {
		return MapServiceError(err), nil
	}

	// Convert domain messages to DTOs
	messages := make([]dto.MessageDto, 0, len(room.Messages))
	for _, msg := range room.Messages {
		msgDTO := dto.MessageDto{
			ID:            dto.MessageID(msg.ID),
			Content:       msg.Content,
			SenderActorID: dto.AlkemioActorID(msg.SenderID),
			Timestamp:     msg.Timestamp,
		}
		// Convert reactions
		for _, r := range msg.Reactions {
			reactionDTO := dto.ReactionDto{
				ID:            dto.ReactionID(r.ID.String()),
				Emoji:         r.Emoji,
				SenderActorID: dto.AlkemioActorID(r.SenderID),
				Timestamp:     r.Timestamp,
			}
			msgDTO.Reactions = append(msgDTO.Reactions, reactionDTO)
		}
		messages = append(messages, msgDTO)
	}

	// Convert member UUIDs to AlkemioActorID
	memberActorIDs := make([]dto.AlkemioActorID, 0, len(room.MemberIDs))
	for _, memberID := range room.MemberIDs {
		memberActorIDs = append(memberActorIDs, dto.AlkemioActorID(memberID))
	}

	return dto.GetRoomResponse{
		BaseResponse:   dto.NewSuccessResponse(),
		AlkemioRoomID:  dto.AlkemioRoomID(room.AlkemioID),
		DisplayName:    room.Name,
		MemberActorIDs: memberActorIDs,
		Messages:       messages,
	}, nil
}

// HandleUpdateRoom handles communication.room.update topic.
func (h *RoomHandler) HandleUpdateRoom(payload []byte) (interface{}, error) {
	var req dto.UpdateRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}

	err := h.service.UpdateRoomMetadata(
		context.Background(),
		req.AlkemioRoomID.UUID(),
		req.Name,
		req.Topic,
		req.IsPublic,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.UpdateRoomResponse{BaseResponse: dto.NewSuccessResponse()}, nil
}

// HandleDeleteRoom handles communication.room.delete topic.
func (h *RoomHandler) HandleDeleteRoom(payload []byte) (interface{}, error) {
	var req dto.DeleteRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}

	err := h.service.DeleteRoomFully(context.Background(), req.AlkemioRoomID.UUID(), req.Reason)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.DeleteRoomResponse{BaseResponse: dto.NewSuccessResponse()}, nil
}

// HandleListRooms handles communication.room.list topic.
func (h *RoomHandler) HandleListRooms(payload []byte) (interface{}, error) {
	var req dto.ListRoomsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	alkemioRoomIDs, nextCursor, err := h.service.ListRooms(context.Background(), req.Limit, req.Cursor)
	if err != nil {
		return MapServiceError(err), nil
	}

	// Convert to DTO type
	roomIDs := make([]dto.AlkemioRoomID, 0, len(alkemioRoomIDs))
	for _, roomID := range alkemioRoomIDs {
		roomIDs = append(roomIDs, dto.AlkemioRoomID(roomID))
	}

	return dto.ListRoomsResponse{
		BaseResponse:   dto.NewSuccessResponse(),
		AlkemioRoomIDs: roomIDs,
		NextCursor:     nextCursor,
	}, nil
}

// ============================================================================
// Message Handlers (communication.message.*)
// ============================================================================

// HandleSendMessage handles communication.message.send topic.
func (h *RoomHandler) HandleSendMessage(payload []byte) (interface{}, error) {
	var req dto.SendMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}
	if req.SenderActorID.UUID() == uuid.Nil {
		return NewInvalidParamError("sender_actor_id is required"), nil
	}
	if req.Content == "" {
		return NewInvalidParamError("content is required"), nil
	}

	// Resolve room alias to Matrix room ID
	alias := fmt.Sprintf("#%s:%s", req.AlkemioRoomID.String(), h.matrix.HomeserverDomain())
	roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
	if err != nil {
		return NewRoomNotFoundError(req.AlkemioRoomID.String()), nil
	}

	sender := domain.Actor{ID: req.SenderActorID.UUID()}

	var eventID id.EventID
	if req.ParentMessageID != nil && *req.ParentMessageID != "" {
		// Send as reply/thread
		eventID, err = h.service.SendReply(
			context.Background(),
			roomID,
			sender,
			req.Content,
			id.EventID(*req.ParentMessageID),
		)
	} else {
		// Send as regular message
		eventID, err = h.service.SendMessage(context.Background(), roomID, sender, req.Content)
	}

	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.SendMessageResponse{
		BaseResponse: dto.NewSuccessResponse(),
		MessageID:    dto.MessageID(eventID.String()),
		Timestamp:    time.Now().UTC(),
	}, nil
}

// HandleGetMessage handles communication.message.get topic.
func (h *RoomHandler) HandleGetMessage(payload []byte) (interface{}, error) {
	var req dto.GetMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}
	if req.MessageID == "" {
		return NewInvalidParamError("message_id is required"), nil
	}

	// Resolve room alias to Matrix room ID
	alias := fmt.Sprintf("#%s:%s", req.AlkemioRoomID.String(), h.matrix.HomeserverDomain())
	roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
	if err != nil {
		return NewRoomNotFoundError(req.AlkemioRoomID.String()), nil
	}

	msg, err := h.service.GetMessage(context.Background(), roomID, id.EventID(req.MessageID))
	if err != nil {
		return MapServiceError(err), nil
	}

	msgDTO := dto.MessageDto{
		ID:            dto.MessageID(msg.ID),
		Content:       msg.Content,
		SenderActorID: dto.AlkemioActorID(msg.SenderID),
		Timestamp:     msg.Timestamp,
	}

	return dto.GetMessageResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Message:      msgDTO,
	}, nil
}

// HandleDeleteMessage handles communication.message.delete topic.
func (h *RoomHandler) HandleDeleteMessage(payload []byte) (interface{}, error) {
	var req dto.DeleteMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}
	if req.MessageID == "" {
		return NewInvalidParamError("message_id is required"), nil
	}
	if req.SenderActorID.UUID() == uuid.Nil {
		return NewInvalidParamError("sender_actor_id is required"), nil
	}

	// Resolve room alias to Matrix room ID
	alias := fmt.Sprintf("#%s:%s", req.AlkemioRoomID.String(), h.matrix.HomeserverDomain())
	roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
	if err != nil {
		return NewRoomNotFoundError(req.AlkemioRoomID.String()), nil
	}

	sender := domain.Actor{ID: req.SenderActorID.UUID()}
	err = h.service.RedactEvent(context.Background(), roomID, sender, id.EventID(req.MessageID), req.Reason)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.DeleteMessageResponse{BaseResponse: dto.NewSuccessResponse()}, nil
}

// ============================================================================
// Reaction Handlers (communication.reaction.*)
// ============================================================================

// HandleAddReaction handles communication.reaction.add topic.
func (h *RoomHandler) HandleAddReaction(payload []byte) (interface{}, error) {
	var req dto.AddReactionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}
	if req.MessageID == "" {
		return NewInvalidParamError("message_id is required"), nil
	}
	if req.SenderActorID.UUID() == uuid.Nil {
		return NewInvalidParamError("sender_actor_id is required"), nil
	}
	if req.Emoji == "" {
		return NewInvalidParamError("emoji is required"), nil
	}

	// Resolve room alias to Matrix room ID
	alias := fmt.Sprintf("#%s:%s", req.AlkemioRoomID.String(), h.matrix.HomeserverDomain())
	roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
	if err != nil {
		return NewRoomNotFoundError(req.AlkemioRoomID.String()), nil
	}

	sender := domain.Actor{ID: req.SenderActorID.UUID()}
	eventID, err := h.service.SendReaction(
		context.Background(),
		roomID,
		sender,
		id.EventID(req.MessageID),
		req.Emoji,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.AddReactionResponse{
		BaseResponse: dto.NewSuccessResponse(),
		ReactionID:   dto.ReactionID(eventID.String()),
	}, nil
}

// HandleRemoveReaction handles communication.reaction.remove topic.
func (h *RoomHandler) HandleRemoveReaction(payload []byte) (interface{}, error) {
	var req dto.RemoveReactionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}
	if req.ReactionID == "" {
		return NewInvalidParamError("reaction_id is required"), nil
	}
	if req.SenderActorID.UUID() == uuid.Nil {
		return NewInvalidParamError("sender_actor_id is required"), nil
	}

	// Resolve room alias to Matrix room ID
	alias := fmt.Sprintf("#%s:%s", req.AlkemioRoomID.String(), h.matrix.HomeserverDomain())
	roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
	if err != nil {
		return NewRoomNotFoundError(req.AlkemioRoomID.String()), nil
	}

	sender := domain.Actor{ID: req.SenderActorID.UUID()}
	// Redact the reaction event directly using its ID
	err = h.service.RedactEvent(
		context.Background(),
		roomID,
		sender,
		id.EventID(req.ReactionID),
		"Reaction removed",
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.RemoveReactionResponse{BaseResponse: dto.NewSuccessResponse()}, nil
}

// HandleGetReaction handles communication.reaction.get topic.
func (h *RoomHandler) HandleGetReaction(payload []byte) (interface{}, error) {
	var req dto.GetReactionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.AlkemioRoomID.UUID() == uuid.Nil {
		return NewInvalidParamError("alkemio_room_id is required"), nil
	}
	if req.ReactionID == "" {
		return NewInvalidParamError("reaction_id is required"), nil
	}

	// Resolve room alias to Matrix room ID
	alias := fmt.Sprintf("#%s:%s", req.AlkemioRoomID.String(), h.matrix.HomeserverDomain())
	roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
	if err != nil {
		return NewRoomNotFoundError(req.AlkemioRoomID.String()), nil
	}

	reaction, err := h.matrix.GetReaction(context.Background(), roomID, id.EventID(req.ReactionID))
	if err != nil {
		return MapServiceError(err), nil
	}

	reactionDTO := dto.ReactionDto{
		ID:            dto.ReactionID(reaction.ID.String()),
		Emoji:         reaction.Emoji,
		SenderActorID: dto.AlkemioActorID(reaction.SenderID),
		Timestamp:     reaction.Timestamp,
	}

	return dto.GetReactionResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Reaction:     reactionDTO,
	}, nil
}

// ============================================================================
// Batch Handlers (communication.room.member.batch.*)
// ============================================================================

// HandleBatchAddMember handles communication.room.member.batch.add topic.
func (h *RoomHandler) HandleBatchAddMember(payload []byte) (interface{}, error) {
	var req dto.BatchAddMemberRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.ActorID.UUID() == uuid.Nil {
		return NewInvalidParamError("actor_id is required"), nil
	}
	if len(req.AlkemioRoomIDs) == 0 {
		return NewInvalidParamError("alkemio_room_ids is required"), nil
	}

	results := make(map[string]dto.RoomOperationResult)
	actor := domain.Actor{ID: req.ActorID.UUID()}

	for _, alkemioRoomID := range req.AlkemioRoomIDs {
		// Resolve room alias to Matrix room ID
		alias := fmt.Sprintf("#%s:%s", alkemioRoomID.String(), h.matrix.HomeserverDomain())
		roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
		if err != nil {
			results[alkemioRoomID.String()] = MapToRoomOperationResult(fmt.Errorf("room not found"))
			continue
		}

		// Invite user to room (uses the bot to invite)
		err = h.matrix.InviteUser(context.Background(), roomID, domain.Actor{}, actor)
		results[alkemioRoomID.String()] = MapToRoomOperationResult(err)
	}

	return dto.BatchAddMemberResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Results:      results,
	}, nil
}

// HandleBatchRemoveMember handles communication.room.member.batch.remove topic.
func (h *RoomHandler) HandleBatchRemoveMember(payload []byte) (interface{}, error) {
	var req dto.BatchRemoveMemberRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if req.ActorID.UUID() == uuid.Nil {
		return NewInvalidParamError("actor_id is required"), nil
	}
	if len(req.AlkemioRoomIDs) == 0 {
		return NewInvalidParamError("alkemio_room_ids is required"), nil
	}

	results := make(map[string]dto.RoomOperationResult)
	actorMatrixID := id.NewUserID(req.ActorID.String(), h.matrix.HomeserverDomain())

	for _, alkemioRoomID := range req.AlkemioRoomIDs {
		// Resolve room alias to Matrix room ID
		alias := fmt.Sprintf("#%s:%s", alkemioRoomID.String(), h.matrix.HomeserverDomain())
		roomID, err := h.matrix.ResolveAlias(context.Background(), alias)
		if err != nil {
			results[alkemioRoomID.String()] = MapToRoomOperationResult(fmt.Errorf("room not found"))
			continue
		}

		// Kick user from room
		err = h.matrix.KickUser(context.Background(), roomID, actorMatrixID, req.Reason)
		results[alkemioRoomID.String()] = MapToRoomOperationResult(err)
	}

	return dto.BatchRemoveMemberResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Results:      results,
	}, nil
}
