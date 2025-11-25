package dto

// RoomMessageSendPayload represents the payload to send a message.
type RoomMessageSendPayload struct {
	BaseMatrixAdapterEventPayload
	RoomID        string `json:"roomID"`
	SenderActorID string `json:"senderActorID"`
	Message       string `json:"message"`
}

// RoomMessageSendResponse represents the response for sending a message.
type RoomMessageSendResponse struct {
	EventID string `json:"eventID"`
}

// RoomMessageSendReplyPayload represents the payload to reply to a message/thread.
type RoomMessageSendReplyPayload struct {
	BaseMatrixAdapterEventPayload
	RoomID        string `json:"roomID"`
	SenderActorID string `json:"senderActorID"`
	ThreadID      string `json:"threadID"` // The Event ID of the parent message
	Message       string `json:"message"`
}

// RoomMessageSendReplyResponse represents the response for sending a reply.
type RoomMessageSendReplyResponse struct {
	EventID string `json:"eventID"`
}

// RoomMessageDeletePayload represents the payload to redact/delete a message.
type RoomMessageDeletePayload struct {
	BaseMatrixAdapterEventPayload
	RoomID        string `json:"roomID"`
	SenderActorID string `json:"senderActorID"` // Who is deleting it (must be sender or admin)
	EventID       string `json:"eventID"`
	Reason        string `json:"reason,omitempty"`
}

// RoomMessageDeleteResponse represents the response for message deletion.
type RoomMessageDeleteResponse struct {
	Success bool `json:"success"`
}

// RoomMessageAddReactionPayload represents the payload to add a reaction.
type RoomMessageAddReactionPayload struct {
	BaseMatrixAdapterEventPayload
	RoomID        string `json:"roomID"`
	SenderActorID string `json:"senderActorID"`
	MessageID     string `json:"messageID"` // Event ID to react to
	Emoji         string `json:"emoji"`
}

// RoomMessageAddReactionResponse represents the response for adding a reaction.
type RoomMessageAddReactionResponse struct {
	EventID string `json:"eventID"`
}

// RoomMessageRemoveReactionPayload represents the payload to remove a reaction.
type RoomMessageRemoveReactionPayload struct {
	BaseMatrixAdapterEventPayload
	RoomID        string `json:"roomID"`
	SenderActorID string `json:"senderActorID"`
	MessageID     string `json:"messageID"`
	Emoji         string `json:"emoji"` // Needed to find the reaction event to redact
}

// RoomMessageRemoveReactionResponse represents the response for removing a reaction.
type RoomMessageRemoveReactionResponse struct {
	Success bool `json:"success"`
}

// RoomMessageDetailsPayload represents the payload to get message details.
type RoomMessageDetailsPayload struct {
	BaseMatrixAdapterEventPayload
	RoomID  string `json:"roomID"`
	EventID string `json:"eventID"`
}

// RoomMessageDetailsResponse represents the message details.
type RoomMessageDetailsResponse struct {
	Message
}
