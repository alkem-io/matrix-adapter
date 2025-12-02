package queue

import (
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// RegisterRoutes registers all queue subscribers to their respective topics.
func RegisterRoutes(q ports.QueuePort, room *RoomHandler, actor *ActorHandler, log ports.Logger) {
	routes := map[string]func([]byte) (interface{}, error){
		// Room Routes (communication.room.*)
		"communication.room.create": room.HandleCreateRoom,
		"communication.room.get":    room.HandleGetRoom,
		"communication.room.update": room.HandleUpdateRoom,
		"communication.room.delete": room.HandleDeleteRoom,
		"communication.room.list":   room.HandleListRooms,

		// Message Routes (communication.message.*)
		"communication.message.send":   room.HandleSendMessage,
		"communication.message.get":    room.HandleGetMessage,
		"communication.message.delete": room.HandleDeleteMessage,

		// Reaction Routes (communication.reaction.*)
		"communication.reaction.add":    room.HandleAddReaction,
		"communication.reaction.remove": room.HandleRemoveReaction,
		"communication.reaction.get":    room.HandleGetReaction,

		// Batch Member Routes (communication.room.member.batch.*)
		"communication.room.member.batch.add":    room.HandleBatchAddMember,
		"communication.room.member.batch.remove": room.HandleBatchRemoveMember,

		// Actor Routes (communication.actor.*)
		"communication.actor.sync": actor.HandleSyncActor,
	}

	for topic, handler := range routes {
		if err := q.Subscribe(topic, handler); err != nil {
			log.Error("Failed to subscribe to topic", "topic", topic, "error", err)
		} else {
			log.Info("Subscribed to topic", "topic", topic)
		}
	}
}
