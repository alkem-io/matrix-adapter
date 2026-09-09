// Package queue provides RabbitMQ message queue infrastructure.
package queue

import "github.com/alkem-io/matrix-adapter/pkg/dto"

// Topic aliases for internal use - imported from dto (single source of truth).
const (
	// TopicRoomCreate is the topic for room creation commands.
	TopicRoomCreate = dto.TopicRoomCreate
	// TopicRoomGet is the topic for room retrieval commands.
	TopicRoomGet = dto.TopicRoomGet
	// TopicRoomGetAsUser is the topic for user-scoped room retrieval commands.
	TopicRoomGetAsUser = dto.TopicRoomGetAsUser
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
	// TopicHierarchySetChildren is the topic for declarative, parent-keyed hierarchy convergence.
	TopicHierarchySetChildren = dto.TopicHierarchySetChildren

	// TopicSpaceMemberBatchAdd is the topic for batch adding space members.
	TopicSpaceMemberBatchAdd = dto.TopicSpaceMemberBatchAdd
	// TopicSpaceMemberBatchRemove is the topic for batch removing space members.
	TopicSpaceMemberBatchRemove = dto.TopicSpaceMemberBatchRemove

	// TopicRoomMembersGet is the topic for getting room members (V4).
	TopicRoomMembersGet = dto.TopicRoomMembersGet
	// TopicThreadMessagesGet is the topic for getting thread messages (V4).
	TopicThreadMessagesGet = dto.TopicThreadMessagesGet

	// TopicReactionAdded is the topic for reaction added events (V4 outgoing).
	TopicReactionAdded = dto.TopicReactionAdded
	// TopicReactionRemoved is the topic for reaction removed events (V4 outgoing).
	TopicReactionRemoved = dto.TopicReactionRemoved
	// TopicRoomMemberLeft is the topic for room member left events (V4 outgoing).
	TopicRoomMemberLeft = dto.TopicRoomMemberLeft

	// Read Receipt Command Topics (008-read-receipts)
	// TopicMessageRead is the topic for marking a message as read.
	TopicMessageRead = dto.TopicMessageRead
	// TopicUnreadCountsGet is the topic for getting unread counts.
	TopicUnreadCountsGet = dto.TopicUnreadCountsGet

	// Batch Query Command Topics
	// TopicBatchUnreadCountsGet is the topic for getting unread counts for multiple rooms.
	TopicBatchUnreadCountsGet = dto.TopicBatchUnreadCountsGet
	// TopicLastMessageGet is the topic for getting the last message in a room.
	TopicLastMessageGet = dto.TopicLastMessageGet
	// TopicBatchLastMessagesGet is the topic for getting last messages for multiple rooms.
	TopicBatchLastMessagesGet = dto.TopicBatchLastMessagesGet

	// Read Receipt & Message Event Topics (008-read-receipts outgoing)
	// TopicReadReceiptUpdated is the topic for read receipt update events.
	TopicReadReceiptUpdated = dto.TopicReadReceiptUpdated
	// TopicMessageEdited is the topic for message edited events.
	TopicMessageEdited = dto.TopicMessageEdited
	// TopicMessageRedacted is the topic for message redacted events.
	TopicMessageRedacted = dto.TopicMessageRedacted
	// TopicRoomCreated is the topic for room created events.
	TopicRoomCreated = dto.TopicRoomCreated
	// TopicRoomMemberUpdated is the topic for room member updated events.
	TopicRoomMemberUpdated = dto.TopicRoomMemberUpdated

	// Room Check Topics (Adapter → Server, request-reply)
	// TopicRoomCheck is the topic for room creation check commands.
	TopicRoomCheck = dto.TopicRoomCheck
	// TopicRoomInfo is the topic for retrieving server-side room info.
	TopicRoomInfo = dto.TopicRoomInfo

	// Custom State Topics (io.alkemio.* state events)
	TopicRoomStateSet  = dto.TopicRoomStateSet
	TopicRoomStateGet  = dto.TopicRoomStateGet
	TopicSpaceStateSet = dto.TopicSpaceStateSet
	TopicSpaceStateGet = dto.TopicSpaceStateGet
)
