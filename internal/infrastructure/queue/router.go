package queue

import (
	"context"

	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// RegisterRoutes registers all queue subscribers to their respective topics.
func RegisterRoutes(q ports.QueuePort, room *RoomHandler, actor *ActorHandler, space *SpaceHandler, log ports.Logger) {
	routes := map[string]func(ctx context.Context, payload []byte) (interface{}, error){
		// Room Routes (communication.room.*)
		TopicRoomCreate: room.HandleCreateRoom,
		TopicRoomGet:    room.HandleGetRoom,
		TopicRoomUpdate: room.HandleUpdateRoom,
		TopicRoomDelete: room.HandleDeleteRoom,
		TopicRoomList:   room.HandleListRooms,

		// Room Members Query (communication.room.members.*)
		TopicRoomMembersGet: room.HandleGetRoomMembers,

		// Message Routes (communication.message.*)
		TopicMessageSend:   room.HandleSendMessage,
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
	}

	for topic, handler := range routes {
		if err := q.Subscribe(topic, handler); err != nil {
			log.Error("Failed to subscribe to topic", "topic", topic, "error", err)
		} else {
			log.Info("Subscribed to topic", "topic", topic)
		}
	}
}
