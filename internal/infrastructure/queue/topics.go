// Package queue provides RabbitMQ message queue infrastructure.
package queue

// ============================================================================
// Command Topics (Inbound from Alkemio Server)
// ============================================================================

// Room command topics
const (
	TopicRoomCreate = "communication.room.create"
	TopicRoomGet    = "communication.room.get"
	TopicRoomUpdate = "communication.room.update"
	TopicRoomDelete = "communication.room.delete"
	TopicRoomList   = "communication.room.list"
)

// Message command topics
const (
	TopicMessageSend   = "communication.message.send"
	TopicMessageGet    = "communication.message.get"
	TopicMessageDelete = "communication.message.delete"
)

// Reaction command topics
const (
	TopicReactionAdd    = "communication.reaction.add"
	TopicReactionRemove = "communication.reaction.remove"
	TopicReactionGet    = "communication.reaction.get"
)

// Room batch member command topics
const (
	TopicRoomMemberBatchAdd    = "communication.room.member.batch.add"
	TopicRoomMemberBatchRemove = "communication.room.member.batch.remove"
)

// Actor command topics
const (
	TopicActorSync = "communication.actor.sync"
)

// Space command topics
const (
	TopicSpaceCreate = "communication.space.create"
	TopicSpaceGet    = "communication.space.get"
	TopicSpaceUpdate = "communication.space.update"
	TopicSpaceDelete = "communication.space.delete"
	TopicSpaceList   = "communication.space.list"
)

// Hierarchy command topics
const (
	TopicHierarchySetParent = "communication.hierarchy.set_parent"
)

// Space batch member command topics
const (
	TopicSpaceMemberBatchAdd    = "communication.space.member.batch.add"
	TopicSpaceMemberBatchRemove = "communication.space.member.batch.remove"
)

// ============================================================================
// Event Topics (Outbound to Alkemio Server)
// ============================================================================

// Message event topics
const (
	TopicMessageReceived = "communication.message.received"
)
