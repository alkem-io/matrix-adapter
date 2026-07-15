package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// maxAttachmentsPerMessage caps how many media refs a single send may carry.
// The server is the authoritative validator; this is a defense-in-depth bound
// so a malformed/hostile request can't fan out into an unbounded number of
// Matrix events.
const maxAttachmentsPerMessage = 10

// RoomHandler handles queue messages related to room operations.
type RoomHandler struct {
	service  *service.RoomService
	matrix   ports.MatrixPort
	resolver *AliasResolver
	idMapper *domain.IDMapper
}

// NewRoomHandler creates a new instance of RoomHandler.
func NewRoomHandler(service *service.RoomService, matrix ports.MatrixPort, idMapper *domain.IDMapper) *RoomHandler {
	return &RoomHandler{
		service:  service,
		matrix:   matrix,
		resolver: NewAliasResolver(matrix, idMapper),
		idMapper: idMapper,
	}
}

// resolveSenderID resolves a Matrix user ID to an Alkemio actor UUID.
// Returns uuid.Nil if the matrix ID is empty or invalid.
func (h *RoomHandler) resolveSenderID(_ context.Context, matrixID string) uuid.UUID {
	if matrixID == "" {
		return uuid.Nil
	}
	return h.idMapper.AlkemioActorID(id.UserID(matrixID))
}

// ============================================================================
// DTO Conversion Helpers (DRY principle - single source of truth)
// ============================================================================

// convertReactionToDTO converts a domain.Reaction to dto.ReactionDto.
func convertReactionToDTO(r domain.Reaction) dto.ReactionDto {
	return dto.ReactionDto{
		ID:            dto.ReactionID(r.ID.String()),
		Emoji:         r.Emoji,
		SenderActorID: dto.AlkemioActorID(r.SenderID),
		Timestamp:     r.Timestamp.UnixMilli(),
	}
}

// convertMessageToDTO converts a domain.Message to dto.MessageDto.
func convertMessageToDTO(msg domain.Message) dto.MessageDto {
	msgDTO := dto.MessageDto{
		ID:            dto.MessageID(msg.ID),
		Content:       msg.Content,
		SenderActorID: dto.AlkemioActorID(msg.SenderID),
		Timestamp:     msg.Timestamp.UnixMilli(),
	}
	// Set ThreadID if present
	if msg.ThreadID != "" {
		tid := dto.MessageID(msg.ThreadID)
		msgDTO.ThreadID = &tid
	}
	// Convert reactions
	for _, r := range msg.Reactions {
		msgDTO.Reactions = append(msgDTO.Reactions, convertReactionToDTO(r))
	}
	// Convert attachments via the shared slice helper (single source of truth).
	msgDTO.Attachments = service.AttachmentsToReceivedDTO(msg.Attachments)
	return msgDTO
}

// convertAttachmentRefsToDomain converts inbound send attachment refs to domain attachments.
func convertAttachmentRefsToDomain(refs []dto.AttachmentRef) []domain.Attachment {
	if len(refs) == 0 {
		return nil
	}
	attachments := make([]domain.Attachment, 0, len(refs))
	for _, r := range refs {
		attachments = append(attachments, domain.Attachment{
			DocumentID:  r.DocumentID,
			DisplayName: r.DisplayName,
			MimeType:    r.MimeType,
			Size:        r.Size,
			Width:       r.Width,
			Height:      r.Height,
		})
	}
	return attachments
}

// convertMessagesToDTO converts a slice of domain.Message to []dto.MessageDto.
func convertMessagesToDTO(messages []domain.Message) []dto.MessageDto {
	result := make([]dto.MessageDto, 0, len(messages))
	for _, msg := range messages {
		result = append(result, convertMessageToDTO(msg))
	}
	return result
}

// convertMemberIDsToDTO converts a slice of uuid.UUID to []dto.AlkemioActorID.
func convertMemberIDsToDTO(memberIDs []uuid.UUID) []dto.AlkemioActorID {
	result := make([]dto.AlkemioActorID, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		result = append(result, dto.AlkemioActorID(memberID))
	}
	return result
}

// ============================================================================
// Room Handlers (communication.room.*)
// ============================================================================

// HandleCreateRoom handles communication.room.create topic.
func (h *RoomHandler) HandleCreateRoom(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.CreateRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	// Validate required fields
	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	// Convert initial members to domain actors
	var initialMembers []domain.Actor
	for _, memberID := range req.InitialMembers {
		if memberID.UUID() != uuid.Nil {
			initialMembers = append(initialMembers, domain.NewActor(memberID.UUID()))
		}
	}

	err := h.service.CreateRoomWithAlkemioID(
		ctx,
		req.AlkemioRoomID.UUID(),
		string(req.Type),
		req.Name,
		req.Topic,
		req.AvatarURL,
		string(req.JoinRule),
		req.IsPublic,
		req.CustomState,
		initialMembers,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleGetRoom handles communication.room.get topic.
func (h *RoomHandler) HandleGetRoom(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	room, err := h.service.GetRoomWithMessages(ctx, req.AlkemioRoomID.UUID())
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.GetRoomResponse{
		BaseResponse:   dto.NewSuccessResponse(),
		AlkemioRoomID:  dto.AlkemioRoomID(room.AlkemioID),
		DisplayName:    room.Name,
		AvatarURL:      room.AvatarURL,
		CustomState:    room.CustomState,
		MemberActorIDs: convertMemberIDsToDTO(room.MemberIDs),
		Messages:       convertMessagesToDTO(room.Messages),
	}, nil
}

// HandleGetRoomAsUser handles communication.room.get.as_user topic.
// Returns room details from a specific user's perspective with read state information.
func (h *RoomHandler) HandleGetRoomAsUser(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetRoomAsUserRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}

	roomWithState, err := h.service.GetRoomAsUser(ctx, req.AlkemioRoomID.UUID(), req.ActorID.UUID())
	if err != nil {
		return MapServiceError(err), nil
	}

	room := roomWithState.Room

	// Convert messages to DTOs with read state using shared helper
	totalMessages := len(room.Messages)
	unreadStartIdx := totalMessages - roomWithState.UnreadCount
	messages := make([]dto.MessageWithReadStateDto, 0, totalMessages)

	for i, msg := range room.Messages {
		messages = append(messages, dto.MessageWithReadStateDto{
			MessageDto: convertMessageToDTO(msg),
			IsRead:     i < unreadStartIdx, // Messages before unreadStartIdx are read
		})
	}

	// Build response
	var lastReadEventID *dto.MessageID
	if roomWithState.LastReadEventID != "" {
		eventID := dto.MessageID(roomWithState.LastReadEventID)
		lastReadEventID = &eventID
	}

	return dto.GetRoomAsUserResponse{
		BaseResponse:    dto.NewSuccessResponse(),
		AlkemioRoomID:   dto.AlkemioRoomID(room.AlkemioID),
		DisplayName:     room.Name,
		AvatarURL:       room.AvatarURL,
		MemberActorIDs:  convertMemberIDsToDTO(room.MemberIDs),
		Messages:        messages,
		LastReadEventID: lastReadEventID,
		UnreadCount:     roomWithState.UnreadCount,
	}, nil
}

// HandleUpdateRoom handles communication.room.update topic.
func (h *RoomHandler) HandleUpdateRoom(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.UpdateRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	// Convert JoinRule pointer (following HandleUpdateSpace pattern)
	var joinRule *string
	if req.JoinRule != nil {
		jr := string(*req.JoinRule)
		joinRule = &jr
	}

	err := h.service.UpdateRoomMetadata(
		ctx,
		req.AlkemioRoomID.UUID(),
		req.Name,
		req.Topic,
		req.AvatarURL,
		joinRule,
		req.IsPublic,
		req.CustomState,
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleDeleteRoom handles communication.room.delete topic.
func (h *RoomHandler) HandleDeleteRoom(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.DeleteRoomRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	err := h.service.DeleteRoomFully(ctx, req.AlkemioRoomID.UUID(), req.Reason)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleListRooms handles communication.room.list topic.
func (h *RoomHandler) HandleListRooms(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.ListRoomsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	alkemioRoomIDs, nextCursor, err := h.service.ListRooms(ctx, req.Cursor)
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
func (h *RoomHandler) HandleSendMessage(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.SendMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.SenderActorID, "sender_actor_id"); errResp != nil {
		return *errResp, nil
	}
	// Content may be empty when the message carries only attachments.
	if len(req.Attachments) == 0 {
		if errResp := RequireNonEmpty(req.Content, "content"); errResp != nil {
			return *errResp, nil
		}
	}

	// Defense-in-depth: the server owns attachment validation, but cap the count
	// and reject empty document refs so a malformed request fails fast with a
	// clear error instead of part-way through emitting Matrix events.
	if len(req.Attachments) > maxAttachmentsPerMessage {
		return NewInvalidParamError(fmt.Sprintf(
			"too many attachments: %d (max %d)", len(req.Attachments), maxAttachmentsPerMessage)), nil
	}
	for i := range req.Attachments {
		if req.Attachments[i].DocumentID == "" {
			return NewInvalidParamError(fmt.Sprintf("attachment[%d] document_id is required", i)), nil
		}
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	sender := domain.NewActor(req.SenderActorID.UUID())
	attachments := convertAttachmentRefsToDomain(req.Attachments)

	var eventID id.EventID
	var err error
	if req.ParentMessageID != nil && *req.ParentMessageID != "" {
		// Send as reply/thread
		eventID, err = h.service.SendReply(
			ctx,
			roomID,
			sender,
			req.Content,
			id.EventID(*req.ParentMessageID),
			attachments,
			req.IdempotencyKey,
		)
	} else {
		// Send as regular message
		eventID, err = h.service.SendMessage(ctx, roomID, sender, req.Content, attachments, req.IdempotencyKey)
	}

	if err != nil {
		// Partial send: some events (text and/or earlier attachments) already
		// landed in the room before a later one failed. Report the delivered
		// message id alongside a partial-failure error so the server can record
		// what landed instead of treating the whole send as failed and
		// re-sending everything. Retries are idempotent when an idempotency_key
		// was supplied (per-event txn ids).
		var partial *domain.PartialSendError
		if errors.As(err, &partial) {
			resp := MapServiceError(partial.Err)
			resp.Error.Message = "partial send: " + resp.Error.Message
			return dto.SendMessageResponse{
				BaseResponse: resp,
				MessageID:    dto.MessageID(partial.PrimaryEventID),
				Timestamp:    time.Now().UnixMilli(),
			}, nil
		}
		return MapServiceError(err), nil
	}

	return dto.SendMessageResponse{
		BaseResponse: dto.NewSuccessResponse(),
		MessageID:    dto.MessageID(eventID.String()),
		Timestamp:    time.Now().UnixMilli(),
	}, nil
}

// HandleGetMessage handles communication.message.get topic.
func (h *RoomHandler) HandleGetMessage(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(string(req.MessageID), "message_id"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	msg, err := h.service.GetMessage(ctx, roomID, id.EventID(req.MessageID))
	if err != nil {
		return MapServiceError(err), nil
	}
	if msg == nil {
		return NewMessageNotFoundError(string(req.MessageID)), nil
	}

	return dto.GetMessageResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Message:      convertMessageToDTO(*msg),
	}, nil
}

// HandleDeleteMessage handles communication.message.delete topic.
func (h *RoomHandler) HandleDeleteMessage(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.DeleteMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(string(req.MessageID), "message_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.SenderActorID, "sender_actor_id"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	sender := domain.NewActor(req.SenderActorID.UUID())
	err := h.service.RedactEvent(ctx, roomID, sender, id.EventID(req.MessageID), req.Reason)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// ============================================================================
// Reaction Handlers (communication.reaction.*)
// ============================================================================

// HandleAddReaction handles communication.reaction.add topic.
func (h *RoomHandler) HandleAddReaction(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.AddReactionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(string(req.MessageID), "message_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.SenderActorID, "sender_actor_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(req.Emoji, "emoji"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	sender := domain.NewActor(req.SenderActorID.UUID())
	eventID, err := h.service.SendReaction(
		ctx,
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
		Timestamp:    time.Now().UnixMilli(),
	}, nil
}

// HandleRemoveReaction handles communication.reaction.remove topic.
func (h *RoomHandler) HandleRemoveReaction(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.RemoveReactionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(string(req.ReactionID), "reaction_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireUUID(req.SenderActorID, "sender_actor_id"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	sender := domain.NewActor(req.SenderActorID.UUID())
	// Redact the reaction event directly using its ID
	err := h.service.RedactEvent(
		ctx,
		roomID,
		sender,
		id.EventID(req.ReactionID),
		"Reaction removed",
	)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleGetReaction handles communication.reaction.get topic.
func (h *RoomHandler) HandleGetReaction(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetReactionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(string(req.ReactionID), "reaction_id"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	reaction, err := h.matrix.GetReaction(ctx, roomID, id.EventID(req.ReactionID))
	if err != nil {
		return MapServiceError(err), nil
	}
	if reaction == nil {
		return NewReactionNotFoundError(string(req.ReactionID)), nil
	}

	// Resolve SenderMatrixID to SenderID (Alkemio actor UUID) if needed
	if reaction.SenderID == uuid.Nil {
		reaction.SenderID = h.resolveSenderID(ctx, reaction.SenderMatrixID)
	}

	return dto.GetReactionResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Reaction:     convertReactionToDTO(*reaction),
	}, nil
}

// ============================================================================
// Batch Handlers (communication.room.member.batch.*)
// ============================================================================

// HandleBatchAddMember handles communication.room.member.batch.add topic.
func (h *RoomHandler) HandleBatchAddMember(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.BatchAddMemberRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if len(req.AlkemioRoomIDs) == 0 {
		return NewInvalidParamError("alkemio_room_ids is required"), nil
	}

	results := make(map[string]dto.BaseResponse)
	actor := domain.NewActor(req.ActorID.UUID())

	for _, alkemioRoomID := range req.AlkemioRoomIDs {
		roomID, err := h.resolver.ResolveRoomForBatch(ctx, alkemioRoomID)
		if err != nil {
			results[alkemioRoomID.String()] = MapToBatchResult(err)
			continue
		}

		// Invite user to room (uses the bot to invite)
		err = h.matrix.InviteUser(ctx, roomID, domain.Actor{}, actor)
		results[alkemioRoomID.String()] = MapToBatchResult(err)
	}

	return dto.BatchAddMemberResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Results:      results,
	}, nil
}

// HandleBatchRemoveMember handles communication.room.member.batch.remove topic.
func (h *RoomHandler) HandleBatchRemoveMember(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.BatchRemoveMemberRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.ActorID, "actor_id"); errResp != nil {
		return *errResp, nil
	}
	if len(req.AlkemioRoomIDs) == 0 {
		return NewInvalidParamError("alkemio_room_ids is required"), nil
	}

	results := make(map[string]dto.BaseResponse)
	actorMatrixID := h.idMapper.UserID(req.ActorID.UUID())

	for _, alkemioRoomID := range req.AlkemioRoomIDs {
		roomID, err := h.resolver.ResolveRoomForBatch(ctx, alkemioRoomID)
		if err != nil {
			results[alkemioRoomID.String()] = MapToBatchResult(err)
			continue
		}

		// Kick user from room
		err = h.matrix.KickUser(ctx, roomID, actorMatrixID, req.Reason)
		results[alkemioRoomID.String()] = MapToBatchResult(err)
	}

	return dto.BatchRemoveMemberResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Results:      results,
	}, nil
}

// ============================================================================
// Room Members Query (communication.room.members.*)
// ============================================================================

// HandleGetRoomMembers handles communication.room.members.get topic.
func (h *RoomHandler) HandleGetRoomMembers(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetRoomMembersRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	// Get room members from Matrix
	members, err := h.matrix.GetRoomMembers(ctx, roomID)
	if err != nil {
		return MapServiceError(err), nil
	}

	// Convert Matrix user IDs to Alkemio actor IDs
	memberActorIDs := make([]dto.AlkemioActorID, 0, len(members))
	for _, memberUserID := range members {
		actorID := h.idMapper.AlkemioActorID(memberUserID)
		// Skip non-ghost users (uuid.Nil indicates not a valid ghost user)
		if actorID == uuid.Nil {
			continue
		}
		memberActorIDs = append(memberActorIDs, dto.AlkemioActorID(actorID))
	}

	return dto.GetRoomMembersResponse{
		BaseResponse:   dto.NewSuccessResponse(),
		AlkemioRoomID:  req.AlkemioRoomID,
		MemberActorIDs: memberActorIDs,
	}, nil
}

// ============================================================================
// Thread Messages Query (communication.thread.*)
// ============================================================================

// HandleGetThreadMessages handles communication.thread.messages.get topic.
func (h *RoomHandler) HandleGetThreadMessages(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetThreadMessagesRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}
	if errResp := RequireNonEmpty(string(req.ThreadID), "thread_id"); errResp != nil {
		return *errResp, nil
	}

	// Resolve room alias to Matrix room ID
	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	// Get thread messages from Matrix
	messages, err := h.matrix.GetThreadMessages(ctx, roomID, id.EventID(req.ThreadID))
	if err != nil {
		return MapServiceError(err), nil
	}

	// Convert domain messages to DTOs
	messageDTOs := make([]dto.MessageDto, 0, len(messages))
	for i := range messages {
		// Resolve sender Matrix ID to Alkemio actor ID if needed
		if messages[i].SenderID == uuid.Nil {
			messages[i].SenderID = h.resolveSenderID(ctx, messages[i].SenderMatrixID)
		}
		messageDTOs = append(messageDTOs, convertMessageToDTO(messages[i]))
	}

	return dto.GetThreadMessagesResponse{
		BaseResponse:  dto.NewSuccessResponse(),
		AlkemioRoomID: req.AlkemioRoomID,
		ThreadID:      req.ThreadID,
		Messages:      messageDTOs,
	}, nil
}

// ============================================================================
// Last Message Handlers (communication.room.last_message.*, communication.room.batch.last_messages.*)
// ============================================================================

// HandleGetLastMessage handles communication.room.last_message.get topic.
func (h *RoomHandler) HandleGetLastMessage(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetLastMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	msg, err := h.matrix.GetLastMessage(ctx, roomID)
	if err != nil {
		return MapServiceError(err), nil
	}

	var msgDTO *dto.MessageDto
	if msg != nil {
		// Resolve sender Matrix ID to Alkemio actor ID if needed
		if msg.SenderID == uuid.Nil {
			msg.SenderID = h.resolveSenderID(ctx, msg.SenderMatrixID)
		}
		converted := convertMessageToDTO(*msg)
		msgDTO = &converted
	}

	return dto.GetLastMessageResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Message:      msgDTO,
	}, nil
}

// HandleBatchGetLastMessages handles communication.room.batch.last_messages.get topic.
// Uses parallel goroutines for efficient fetching from multiple rooms.
func (h *RoomHandler) HandleBatchGetLastMessages(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.BatchGetLastMessagesRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if len(req.AlkemioRoomIDs) == 0 {
		return NewInvalidParamError("alkemio_room_ids is required"), nil
	}

	// First, resolve all room aliases to Matrix room IDs
	matrixRoomIDs := make([]id.RoomID, 0, len(req.AlkemioRoomIDs))
	alkemioToMatrix := make(map[id.RoomID]dto.AlkemioRoomID) // Reverse mapping
	errors := make(map[string]dto.BaseResponse)

	for _, alkemioRoomID := range req.AlkemioRoomIDs {
		roomID, err := h.resolver.ResolveRoomForBatch(ctx, alkemioRoomID)
		if err != nil {
			errors[alkemioRoomID.String()] = MapToBatchResult(err)
			continue
		}
		matrixRoomIDs = append(matrixRoomIDs, roomID)
		alkemioToMatrix[roomID] = alkemioRoomID
	}

	// Use efficient batch method - parallel goroutines for all rooms
	batchResults, batchErrors := h.matrix.GetBatchLastMessages(ctx, matrixRoomIDs)

	// Convert results back to Alkemio room IDs
	messages := make(map[string]*dto.MessageDto)
	for matrixRoomID, msg := range batchResults {
		alkemioRoomID := alkemioToMatrix[matrixRoomID]
		var msgDTO *dto.MessageDto
		if msg != nil {
			// Resolve sender Matrix ID to Alkemio actor ID if needed
			if msg.SenderID == uuid.Nil {
				msg.SenderID = h.resolveSenderID(ctx, msg.SenderMatrixID)
			}
			converted := convertMessageToDTO(*msg)
			msgDTO = &converted
		}
		messages[alkemioRoomID.String()] = msgDTO
	}
	for matrixRoomID, err := range batchErrors {
		alkemioRoomID := alkemioToMatrix[matrixRoomID]
		errors[alkemioRoomID.String()] = MapToBatchResult(err)
	}

	resp := dto.BatchGetLastMessagesResponse{
		BaseResponse: dto.NewSuccessResponse(),
		Messages:     messages,
	}
	if len(errors) > 0 {
		resp.Errors = errors
	}
	return resp, nil
}

// ============================================================================
// Custom State Handlers (communication.room.state.*)
// ============================================================================

// HandleSetRoomState handles communication.room.state.set topic.
func (h *RoomHandler) HandleSetRoomState(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.SetRoomStateRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	if err := h.matrix.SetCustomState(ctx, roomID, req.State); err != nil {
		return MapServiceError(err), nil
	}

	return dto.NewSuccessResponse(), nil
}

// HandleGetRoomState handles communication.room.state.get topic.
func (h *RoomHandler) HandleGetRoomState(ctx context.Context, payload []byte) (interface{}, error) {
	var req dto.GetRoomStateRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewInvalidPayloadError(err), nil
	}

	if errResp := RequireUUID(req.AlkemioRoomID, "alkemio_room_id"); errResp != nil {
		return *errResp, nil
	}

	roomID, errResp := h.resolver.ResolveRoom(ctx, req.AlkemioRoomID)
	if errResp != nil {
		return *errResp, nil
	}

	state, err := h.matrix.GetCustomState(ctx, roomID, req.EventTypes)
	if err != nil {
		return MapServiceError(err), nil
	}

	return dto.GetRoomStateResponse{
		BaseResponse:  dto.NewSuccessResponse(),
		AlkemioRoomID: req.AlkemioRoomID,
		State:         state,
	}, nil
}
