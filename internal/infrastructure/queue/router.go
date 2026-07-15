package queue

import (
	"context"
	"encoding/json"

	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// sendMessagePartitionKey extracts the target room from a SendMessageRequest
// payload so the ordered send pool can serialize same-room sends while running
// different rooms concurrently. Only the room id is decoded (not the full
// request with its attachment list) — the handler decodes the payload again
// anyway. A malformed payload (which the handler will reject) falls back to the
// empty key — still ordered, never dropped.
func sendMessagePartitionKey(payload []byte) string {
	var req struct {
		AlkemioRoomID dto.AlkemioRoomID `json:"alkemio_room_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ""
	}
	return req.AlkemioRoomID.String()
}

// RegisterRoutes registers all queue subscribers to their respective topics.
func RegisterRoutes(q ports.QueuePort, room *RoomHandler, actor *ActorHandler, space *SpaceHandler, readReceipt *ReadReceiptHandler, log ports.Logger) {
	routes := map[string]func(ctx context.Context, payload []byte) (interface{}, error){
		// Room Routes (communication.room.*)
		TopicRoomCreate:    room.HandleCreateRoom,
		TopicRoomGet:       room.HandleGetRoom,
		TopicRoomGetAsUser: room.HandleGetRoomAsUser,
		TopicRoomUpdate:    room.HandleUpdateRoom,
		TopicRoomDelete:    room.HandleDeleteRoom,
		TopicRoomList:      room.HandleListRooms,

		// Room Members Query (communication.room.members.*)
		TopicRoomMembersGet: room.HandleGetRoomMembers,

		// Message Routes (communication.message.*)
		// NOTE: TopicMessageSend is registered separately below via
		// SubscribeOrdered — a media send does a fetch + up to 10 uploads and
		// must not head-of-line-block other rooms' sends.
		TopicMessageGet:    room.HandleGetMessage,
		TopicMessageDelete: room.HandleDeleteMessage,

		// Thread Messages Query (communication.thread.*)
		TopicThreadMessagesGet: room.HandleGetThreadMessages,

		// Reaction Routes (communication.reaction.*)
		TopicReactionAdd:    room.HandleAddReaction,
		TopicReactionRemove: room.HandleRemoveReaction,
		TopicReactionGet:    room.HandleGetReaction,

		// Batch Member Routes (communication.room.member.batch.*)
		TopicRoomMemberBatchAdd:    room.HandleBatchAddMember,
		TopicRoomMemberBatchRemove: room.HandleBatchRemoveMember,

		// Actor Routes (communication.actor.*)
		TopicActorSync: actor.HandleSyncActor,

		// Space Routes (communication.space.*)
		TopicSpaceCreate: space.HandleCreateSpace,
		TopicSpaceGet:    space.HandleGetSpace,
		TopicSpaceUpdate: space.HandleUpdateSpace,
		TopicSpaceDelete: space.HandleDeleteSpace,
		TopicSpaceList:   space.HandleListSpaces,

		// Hierarchy Routes (communication.hierarchy.*)
		TopicHierarchySetParent: space.HandleSetParent,

		// Batch Space Member Routes (communication.space.member.batch.*)
		TopicSpaceMemberBatchAdd:    space.HandleBatchAddSpaceMember,
		TopicSpaceMemberBatchRemove: space.HandleBatchRemoveSpaceMember,

		// Read Receipt Routes (communication.message.read, communication.room.unread_counts.get)
		TopicMessageRead:     readReceipt.HandleMarkMessageRead,
		TopicUnreadCountsGet: readReceipt.HandleGetUnreadCounts,

		// Batch Query Routes (communication.room.batch.*, communication.room.last_message.*)
		TopicBatchUnreadCountsGet: readReceipt.HandleBatchGetUnreadCounts,
		TopicLastMessageGet:       room.HandleGetLastMessage,
		TopicBatchLastMessagesGet: room.HandleBatchGetLastMessages,

		// Custom State Routes (communication.{room,space}.state.*)
		TopicRoomStateSet:  room.HandleSetRoomState,
		TopicRoomStateGet:  room.HandleGetRoomState,
		TopicSpaceStateSet: space.HandleSetSpaceState,
		TopicSpaceStateGet: space.HandleGetSpaceState,
	}

	for topic, handler := range routes {
		if err := q.Subscribe(topic, handler); err != nil {
			log.Error("Failed to subscribe to topic", "topic", topic, "error", err)
		} else {
			log.Info("Subscribed to topic", "topic", topic)
		}
	}

	// The send topic gets room-partitioned concurrency: same-room sends stay
	// strictly ordered, different rooms progress in parallel, so one slow media
	// send can't block every other send.
	if err := q.SubscribeOrdered(TopicMessageSend, room.HandleSendMessage, sendMessagePartitionKey); err != nil {
		log.Error("Failed to subscribe to topic", "topic", TopicMessageSend, "error", err)
	} else {
		log.Info("Subscribed to topic (room-ordered)", "topic", TopicMessageSend)
	}
}
