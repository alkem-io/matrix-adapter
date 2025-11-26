package queue

import (
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// RegisterRoutes registers all queue subscribers to their respective topics.
func RegisterRoutes(q ports.QueuePort, actor *ActorHandler, room *RoomHandler, admin *AdminHandler, log ports.Logger) {
	routes := map[string]func([]byte) (interface{}, error){
		// Room Routes
		"room.create":                 room.HandleCreate,
		"room.delete":                 room.HandleDelete,
		"room.details":                room.HandleGetDetails,
		"room.members":                room.HandleGetMembers,
		"room.updateState":            room.HandleUpdateState,
		"room.message.send":           room.HandleSendMessage,
		"room.message.details":        room.HandleMessageDetails,
		"room.message.sendReply":      room.HandleSendReply,
		"room.message.delete":         room.HandleDeleteMessage,
		"room.message.addReaction":    room.HandleAddReaction,
		"room.message.removeReaction": room.HandleRemoveReaction,

		// Actor Routes
		"actor.register":             actor.HandleRegister,
		"actor.addToRooms":           actor.HandleAddToRooms,
		"actor.removeFromRooms":      actor.HandleRemoveFromRooms,
		"actor.rooms":                actor.HandleGetRooms,
		"actor.rooms.direct":         actor.HandleGetDirectRooms,
		"actor.startDirectMessaging": actor.HandleStartDirectMessaging,
		"actor.stopDirectMessaging":  actor.HandleStopDirectMessaging,

		// Admin Routes
		"admin.allRooms":                admin.HandleGetAllRooms,
		"admin.replicateRoomMembership": admin.HandleReplicateRoomMembership,
	}

	for topic, handler := range routes {
		if err := q.Subscribe(topic, handler); err != nil {
			log.Error("Failed to subscribe to topic", "topic", topic, "error", err)
		} else {
			log.Info("Subscribed to topic", "topic", topic)
		}
	}
}
