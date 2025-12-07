package dto

// MessageReceivedPayload represents the event when a message is received.
type MessageReceivedPayload struct {
	RoomID      string  `json:"roomId"`
	RoomName    string  `json:"roomName"`
	Message     Message `json:"message"`
	ActorID     string  `json:"actorID"` // The actor who received the message (the bot)
	CommunityID *string `json:"communityId,omitempty"`
}

// Message represents a Matrix message event.
type Message struct {
	ID        string     `json:"id"`
	Message   string     `json:"message"`
	ThreadID  *string    `json:"threadID,omitempty"`
	Sender    string     `json:"sender"`
	Timestamp int64      `json:"timestamp"`
	Reactions []Reaction `json:"reactions"`
}

// Reaction represents a reaction to a message.
type Reaction struct {
	ID        string `json:"id"`
	Emoji     string `json:"emoji"`
	Sender    string `json:"sender"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
}

// ============================================================================
// Outgoing Events (Adapter → Server) - V4 Protocol Extension
// ============================================================================

// ReactionAddedEvent is published when a user adds a reaction to a message.
// Topic: communication.reaction.added
type ReactionAddedEvent struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	MessageID     MessageID      `json:"message_id"`
	ReactionID    ReactionID     `json:"reaction_id"`
	Emoji         string         `json:"emoji"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Timestamp     int64          `json:"timestamp"`
}

// ReactionRemovedEvent is published when a user removes a reaction from a message.
// Topic: communication.reaction.removed
type ReactionRemovedEvent struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	MessageID     MessageID      `json:"message_id"`
	ReactionID    ReactionID     `json:"reaction_id"`
	Emoji         string         `json:"emoji"`
	SenderActorID AlkemioActorID `json:"sender_actor_id"`
	Timestamp     int64          `json:"timestamp"`
}

// RoomMemberLeftEvent is published when a user leaves or is kicked from a room.
// Topic: communication.room.member.left
type RoomMemberLeftEvent struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	ActorID       AlkemioActorID `json:"actor_id"`
	Reason        string         `json:"reason,omitempty"`
	Timestamp     int64          `json:"timestamp"`
}
