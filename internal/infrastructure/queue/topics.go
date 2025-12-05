// Package queue provides RabbitMQ message queue infrastructure.
package queue

import "github.com/alkem-io/matrix-adapter-go/pkg/dto"

// Topic aliases for internal use - imported from dto (single source of truth).
const (
	// TopicRoomCreate is the topic for room creation commands.
	TopicRoomCreate = dto.TopicRoomCreate
	// TopicRoomGet is the topic for room retrieval commands.
	TopicRoomGet = dto.TopicRoomGet
	// TopicRoomUpdate is the topic for room update commands.
	TopicRoomUpdate = dto.TopicRoomUpdate
	// TopicRoomDelete is the topic for room deletion commands.
	TopicRoomDelete = dto.TopicRoomDelete
	// TopicRoomList is the topic for room listing commands.
	TopicRoomList = dto.TopicRoomList

	// TopicMessageSend is the topic for message send commands.
	TopicMessageSend = dto.TopicMessageSend
	// TopicMessageGet is the topic for message retrieval commands.
	TopicMessageGet = dto.TopicMessageGet
	// TopicMessageDelete is the topic for message deletion commands.
	TopicMessageDelete = dto.TopicMessageDelete

	// TopicReactionAdd is the topic for reaction add commands.
	TopicReactionAdd = dto.TopicReactionAdd
	// TopicReactionRemove is the topic for reaction remove commands.
	TopicReactionRemove = dto.TopicReactionRemove
	// TopicReactionGet is the topic for reaction retrieval commands.
	TopicReactionGet = dto.TopicReactionGet

	// TopicRoomMemberBatchAdd is the topic for batch adding room members.
	TopicRoomMemberBatchAdd = dto.TopicRoomMemberBatchAdd
	// TopicRoomMemberBatchRemove is the topic for batch removing room members.
	TopicRoomMemberBatchRemove = dto.TopicRoomMemberBatchRemove

	// TopicActorSync is the topic for actor synchronization commands.
	TopicActorSync = dto.TopicActorSync

	// TopicSpaceCreate is the topic for space creation commands.
	TopicSpaceCreate = dto.TopicSpaceCreate
	// TopicSpaceGet is the topic for space retrieval commands.
	TopicSpaceGet = dto.TopicSpaceGet
	// TopicSpaceUpdate is the topic for space update commands.
	TopicSpaceUpdate = dto.TopicSpaceUpdate
	// TopicSpaceDelete is the topic for space deletion commands.
	TopicSpaceDelete = dto.TopicSpaceDelete
	// TopicSpaceList is the topic for space listing commands.
	TopicSpaceList = dto.TopicSpaceList

	// TopicHierarchySetParent is the topic for setting space hierarchy parent.
	TopicHierarchySetParent = dto.TopicHierarchySetParent

	// TopicSpaceMemberBatchAdd is the topic for batch adding space members.
	TopicSpaceMemberBatchAdd = dto.TopicSpaceMemberBatchAdd
	// TopicSpaceMemberBatchRemove is the topic for batch removing space members.
	TopicSpaceMemberBatchRemove = dto.TopicSpaceMemberBatchRemove
)
