package dto

// ============================================================================
// Read Receipt Commands & Responses
// ============================================================================

// MarkMessageReadRequest is the command to mark a message as read.
// Topic: communication.message.read
type MarkMessageReadRequest struct {
	ActorID       AlkemioActorID `json:"actor_id"`
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	MessageID     MessageID      `json:"message_id"`
	ThreadID      *MessageID     `json:"thread_id,omitempty"` // Optional: for thread-specific receipts
}

// GetUnreadCountsRequest is the command to get unread counts for a user in a room.
// Topic: communication.room.unread_counts.get
type GetUnreadCountsRequest struct {
	ActorID       AlkemioActorID `json:"actor_id"`
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	ThreadIDs     []MessageID    `json:"thread_ids,omitempty"` // Optional: specific threads to query
}

// GetUnreadCountsResponse is the response for GetUnreadCountsRequest.
type GetUnreadCountsResponse struct {
	BaseResponse       `tstype:",extends"`
	RoomUnreadCount    int            `json:"room_unread_count"`
	ThreadUnreadCounts map[string]int `json:"thread_unread_counts,omitempty"` // Map[ThreadID]Count
}

// ============================================================================
// Read Receipt Events (Adapter → Server)
// ============================================================================

// ReadReceiptUpdatedEvent is published when a user's read receipt is updated.
// Topic: matrix.room.receipt.updated
type ReadReceiptUpdatedEvent struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	ActorID       AlkemioActorID `json:"actor_id"`
	EventID       MessageID      `json:"event_id"`            // Event ID that was marked as read
	ThreadID      *MessageID     `json:"thread_id,omitempty"` // Thread root event ID (if thread-level)
	Timestamp     int64          `json:"timestamp"`
}

// ============================================================================
// Message Event Notifications (Adapter → Server)
// ============================================================================

// MessageEditedEvent is published when a message is edited (m.replace).
// Topic: matrix.room.message.edited
type MessageEditedEvent struct {
	AlkemioRoomID     AlkemioRoomID  `json:"alkemio_room_id"`
	SenderActorID     AlkemioActorID `json:"sender_actor_id"`
	OriginalMessageID MessageID      `json:"original_message_id"` // Event ID of original message
	NewMessageID      MessageID      `json:"new_message_id"`      // Event ID of the edit event
	NewContent        string         `json:"new_content"`
	ThreadID          *MessageID     `json:"thread_id,omitempty"` // Thread root event ID (if in thread)
	Timestamp         int64          `json:"timestamp"`
}

// MessageRedactedEvent is published when a message is redacted.
// Topic: matrix.room.message.redacted
type MessageRedactedEvent struct {
	AlkemioRoomID      AlkemioRoomID  `json:"alkemio_room_id"`
	RedactorActorID    AlkemioActorID `json:"redactor_actor_id"`
	RedactedMessageID  MessageID      `json:"redacted_message_id"`  // Event ID of the redacted message
	RedactionMessageID MessageID      `json:"redaction_message_id"` // Event ID of the redaction event
	Reason             string         `json:"reason,omitempty"`
	ThreadID           *MessageID     `json:"thread_id,omitempty"` // Thread root event ID (if in thread)
	Timestamp          int64          `json:"timestamp"`
}

// RoomCreatedEvent is published when a room is created.
// Topic: matrix.room.created
type RoomCreatedEvent struct {
	AlkemioRoomID  AlkemioRoomID  `json:"alkemio_room_id"`
	CreatorActorID AlkemioActorID `json:"creator_actor_id"`
	RoomType       string         `json:"room_type"` // "room", "space"
	Name           string         `json:"name,omitempty"`
	Topic          string         `json:"topic,omitempty"`
	Timestamp      int64          `json:"timestamp"`
}

// RoomMemberUpdatedEvent is published when a user's membership status changes.
// Topic: matrix.room.member.updated
type RoomMemberUpdatedEvent struct {
	AlkemioRoomID AlkemioRoomID  `json:"alkemio_room_id"`
	MemberActorID AlkemioActorID `json:"member_actor_id"` // Actor whose membership changed
	SenderActorID AlkemioActorID `json:"sender_actor_id"` // Actor who performed the action
	Membership    string         `json:"membership"`      // join, leave, invite, ban, knock
	Timestamp     int64          `json:"timestamp"`
}
