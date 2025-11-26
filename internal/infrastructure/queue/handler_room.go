package queue

import (
	"context"
	"encoding/json"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/core/service"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// RoomHandler handles queue messages related to room operations.
type RoomHandler struct {
	service *service.RoomService
}

// NewRoomHandler creates a new instance of RoomHandler.
func NewRoomHandler(service *service.RoomService) *RoomHandler {
	return &RoomHandler{
		service: service,
	}
}

// HandleCreate handles the message to create a new room.
func (h *RoomHandler) HandleCreate(payload []byte) (interface{}, error) {
	var req dto.RoomCreatePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.TriggeredBy)
	if err != nil {
		return nil, err
	}

	actor := domain.Actor{ID: actorID}

	roomID, err := h.service.CreateRoom(context.Background(), actor, req.RoomName, req.Metadata)
	if err != nil {
		return nil, err
	}

	return dto.RoomCreateResponse{
		Success: true,
		RoomID:  string(roomID),
	}, nil
}

// HandleInvite handles the message to invite a user to a room.
func (h *RoomHandler) HandleInvite(payload []byte) (interface{}, error) {
	var req dto.RoomInvitePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	inviterID, err := uuid.Parse(req.TriggeredBy)
	if err != nil {
		return nil, err
	}
	inviteeID, err := uuid.Parse(req.InviteeID)
	if err != nil {
		return nil, err
	}

	inviter := domain.Actor{ID: inviterID}
	invitee := domain.Actor{ID: inviteeID}
	roomID := id.RoomID(req.RoomID)

	return nil, h.service.InviteUser(context.Background(), roomID, inviter, invitee)
}

// HandleGetDetails handles the message to get details of a room.
func (h *RoomHandler) HandleGetDetails(payload []byte) (interface{}, error) {
	var req dto.RoomDetailsPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	room, err := h.service.GetRoomDetails(context.Background(), id.RoomID(req.RoomID))
	if err != nil {
		return nil, err
	}

	return dto.RoomDetailsResponse{
		RoomID: room.ID.String(),
		Name:   room.Name,
		Topic:  room.Topic,
		Alias:  room.Alias,
	}, nil
}

// HandleGetMembers handles the message to get members of a room.
func (h *RoomHandler) HandleGetMembers(payload []byte) (interface{}, error) {
	var req dto.RoomMembersPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	members, err := h.service.GetRoomMembers(context.Background(), id.RoomID(req.RoomID))
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, len(members))
	for i, m := range members {
		userIDs[i] = m.String()
	}

	return dto.RoomMembersResponse{
		RoomID:  req.RoomID,
		UserIDs: userIDs,
	}, nil
}

// HandleUpdateState handles the message to update the state of a room.
func (h *RoomHandler) HandleUpdateState(payload []byte) (interface{}, error) {
	var req dto.RoomUpdateStatePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.TriggeredBy)
	if err != nil {
		return nil, err
	}

	err = h.service.UpdateRoomState(
		context.Background(), id.RoomID(req.RoomID), domain.Actor{ID: actorID}, req.Name, req.Topic, req.Alias,
	)
	if err != nil {
		return nil, err
	}

	return dto.RoomUpdateStateResponse{Success: true}, nil
}

// HandleSendMessage handles the message to send a message to a room.
func (h *RoomHandler) HandleSendMessage(payload []byte) (interface{}, error) {
	var req dto.RoomMessageSendPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	senderID, err := uuid.Parse(req.SenderActorID)
	if err != nil {
		return nil, err
	}

	eventID, err := h.service.SendMessage(
		context.Background(), id.RoomID(req.RoomID), domain.Actor{ID: senderID}, req.Message,
	)
	if err != nil {
		return nil, err
	}

	return dto.RoomMessageSendResponse{EventID: eventID.String()}, nil
}

// HandleSendReply handles the message to send a reply to a message in a room.
func (h *RoomHandler) HandleSendReply(payload []byte) (interface{}, error) {
	var req dto.RoomMessageSendReplyPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	senderID, err := uuid.Parse(req.SenderActorID)
	if err != nil {
		return nil, err
	}

	eventID, err := h.service.SendReply(
		context.Background(), id.RoomID(req.RoomID), domain.Actor{ID: senderID}, req.Message, id.EventID(req.ThreadID),
	)
	if err != nil {
		return nil, err
	}

	return dto.RoomMessageSendReplyResponse{EventID: eventID.String()}, nil
}

// HandleDeleteMessage handles the message to delete a message in a room.
func (h *RoomHandler) HandleDeleteMessage(payload []byte) (interface{}, error) {
	var req dto.RoomMessageDeletePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	senderID, err := uuid.Parse(req.SenderActorID)
	if err != nil {
		return nil, err
	}

	err = h.service.RedactEvent(
		context.Background(), id.RoomID(req.RoomID), domain.Actor{ID: senderID}, id.EventID(req.EventID), req.Reason,
	)
	if err != nil {
		return nil, err
	}

	return dto.RoomMessageDeleteResponse{Success: true}, nil
}

// HandleAddReaction handles the message to add a reaction to a message in a room.
func (h *RoomHandler) HandleAddReaction(payload []byte) (interface{}, error) {
	var req dto.RoomMessageAddReactionPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	senderID, err := uuid.Parse(req.SenderActorID)
	if err != nil {
		return nil, err
	}

	eventID, err := h.service.SendReaction(
		context.Background(), id.RoomID(req.RoomID), domain.Actor{ID: senderID}, id.EventID(req.MessageID), req.Emoji,
	)
	if err != nil {
		return nil, err
	}

	return dto.RoomMessageAddReactionResponse{EventID: eventID.String()}, nil
}

// HandleDelete handles the message to delete (forget) a room.
func (h *RoomHandler) HandleDelete(payload []byte) (interface{}, error) {
	var req dto.RoomDeletePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	actorID, err := uuid.Parse(req.TriggeredBy)
	if err != nil {
		return nil, err
	}

	err = h.service.ForgetRoom(context.Background(), id.RoomID(req.RoomID), domain.Actor{ID: actorID})
	if err != nil {
		return nil, err
	}

	return dto.RoomDeleteResponse{Success: true}, nil
}

// HandleMessageDetails handles the message to get message details.
func (h *RoomHandler) HandleMessageDetails(payload []byte) (interface{}, error) {
	var req dto.RoomMessageDetailsPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	msg, err := h.service.GetMessage(context.Background(), id.RoomID(req.RoomID), id.EventID(req.EventID))
	if err != nil {
		return nil, err
	}

	return dto.RoomMessageDetailsResponse{
		Message: dto.Message{
			ID:        msg.ID,
			Message:   msg.Content,
			Sender:    msg.SenderMatrixID,
			Timestamp: msg.Timestamp.UnixMilli(),
		},
	}, nil
}

// HandleRemoveReaction handles the message to remove a reaction.
func (h *RoomHandler) HandleRemoveReaction(payload []byte) (interface{}, error) {
	var req dto.RoomMessageRemoveReactionPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	senderID, err := uuid.Parse(req.SenderActorID)
	if err != nil {
		return nil, err
	}

	// Find the reaction event ID
	reactionEventID, err := h.service.GetReactionEventID(
		context.Background(), id.RoomID(req.RoomID), id.EventID(req.MessageID), req.Emoji, domain.Actor{ID: senderID},
	)
	if err != nil {
		return nil, err
	}

	// Redact the reaction
	err = h.service.RedactEvent(
		context.Background(), id.RoomID(req.RoomID), domain.Actor{ID: senderID}, reactionEventID, "Reaction removed",
	)
	if err != nil {
		return nil, err
	}

	return dto.RoomMessageRemoveReactionResponse{Success: true}, nil
}
