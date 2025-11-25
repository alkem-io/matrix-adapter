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
