package dto

// ============================================================================
// Topic Constants - Single Source of Truth
// ============================================================================

// Room command topics
const (
	TopicRoomCreate    = "communication.room.create"
	TopicRoomGet       = "communication.room.get"
	TopicRoomGetAsUser = "communication.room.get.as_user"
	TopicRoomUpdate    = "communication.room.update"
	TopicRoomDelete    = "communication.room.delete"
	TopicRoomList      = "communication.room.list"
)

// Message command topics
const (
	TopicMessageSend     = "communication.message.send"
	TopicMessageGet      = "communication.message.get"
	TopicMessageDelete   = "communication.message.delete"
	TopicMessageReceived = "communication.message.received" // Outbound event
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

// TopicActorSync is the topic for actor synchronization commands.
const TopicActorSync = "communication.actor.sync"

// Space command topics
const (
	TopicSpaceCreate = "communication.space.create"
	TopicSpaceGet    = "communication.space.get"
	TopicSpaceUpdate = "communication.space.update"
	TopicSpaceDelete = "communication.space.delete"
	TopicSpaceList   = "communication.space.list"
)

// TopicHierarchySetParent is the topic for setting space hierarchy parent commands.
const TopicHierarchySetParent = "communication.hierarchy.set_parent"

// Space batch member command topics
const (
	TopicSpaceMemberBatchAdd    = "communication.space.member.batch.add"
	TopicSpaceMemberBatchRemove = "communication.space.member.batch.remove"
)

// Custom state API topics (io.alkemio.* state events)
const (
	TopicRoomStateSet  = "communication.room.state.set"
	TopicRoomStateGet  = "communication.room.state.get"
	TopicSpaceStateSet = "communication.space.state.set"
	TopicSpaceStateGet = "communication.space.state.get"
)

// TopicRoomDMRequested is the topic for DM room request events (outbound to Server).
const TopicRoomDMRequested = "communication.room.dm.requested"

// Outgoing Event Topics (Adapter → Server) - V4 Protocol Extension
const (
	// TopicReactionAdded is the topic for reaction added events.
	TopicReactionAdded = "communication.reaction.added"
	// TopicReactionRemoved is the topic for reaction removed events.
	TopicReactionRemoved = "communication.reaction.removed"
	// TopicRoomMemberLeft is the topic for room member left events.
	TopicRoomMemberLeft = "communication.room.member.left"
)

// Command Topics (Server → Adapter) - V4 Protocol Extension
const (
	// TopicRoomMembersGet is the topic for getting room members.
	TopicRoomMembersGet = "communication.room.members.get"
	// TopicThreadMessagesGet is the topic for getting thread messages.
	TopicThreadMessagesGet = "communication.thread.messages.get"
)

// Read Receipt Command Topics (Server → Adapter)
const (
	// TopicMessageRead is the topic for marking a message as read.
	TopicMessageRead = "communication.message.read"
	// TopicUnreadCountsGet is the topic for getting unread counts.
	TopicUnreadCountsGet = "communication.room.unread_counts.get"
)

// Batch Query Command Topics (Server → Adapter)
const (
	// TopicBatchUnreadCountsGet is the topic for getting unread counts for multiple rooms.
	TopicBatchUnreadCountsGet = "communication.room.batch.unread_counts.get"
	// TopicLastMessageGet is the topic for getting the last message in a room.
	TopicLastMessageGet = "communication.room.last_message.get"
	// TopicBatchLastMessagesGet is the topic for getting last messages for multiple rooms.
	TopicBatchLastMessagesGet = "communication.room.batch.last_messages.get"
)

// Room Check Topics (Adapter → Server, request-reply)
const (
	// TopicRoomCheck is the topic for room creation check commands (consent, dedup).
	TopicRoomCheck = "communication.room.check"
	// TopicRoomInfo is the topic for retrieving server-side room info during reconciliation.
	TopicRoomInfo = "communication.room.info"
)

// Read Receipt & Message Event Topics (Adapter → Server)
const (
	// TopicReadReceiptUpdated is the topic for read receipt update events.
	TopicReadReceiptUpdated = "communication.room.receipt.updated"
	// TopicMessageEdited is the topic for message edited events.
	TopicMessageEdited = "communication.message.edited"
	// TopicMessageRedacted is the topic for message redacted events.
	TopicMessageRedacted = "communication.message.redacted"
	// TopicRoomCreated is the topic for room created events.
	TopicRoomCreated = "communication.room.created"
	// TopicRoomMemberUpdated is the topic for room member updated events.
	TopicRoomMemberUpdated = "communication.room.member.updated"
	// TopicRoomUpdated is the topic for room property updated events.
	TopicRoomUpdated = "communication.room.updated"
	// TopicSpaceUpdated is the topic for space property updated events.
	TopicSpaceUpdated = "communication.space.updated"
)

// ============================================================================
// Command Registry - Topic/Request/Response Mapping for Code Generation
// ============================================================================

// CommandDef defines a command with its topic, request type name, and response type name.
// This is used by the TypeScript generator to create type-safe command definitions.
type CommandDef struct {
	Topic        string
	RequestType  string
	ResponseType string
}

// CommandRegistry defines all commands supported by the Matrix Adapter.
// The generator reads this to produce TypeScript command definitions.
//
//nolint:gochecknoglobals // Registry is intentionally global for code generation
var CommandRegistry = []CommandDef{
	// Room commands
	{Topic: TopicRoomCreate, RequestType: "CreateRoomRequest", ResponseType: "BaseResponse"},
	{Topic: TopicRoomGet, RequestType: "GetRoomRequest", ResponseType: "GetRoomResponse"},
	{Topic: TopicRoomGetAsUser, RequestType: "GetRoomAsUserRequest", ResponseType: "GetRoomAsUserResponse"},
	{Topic: TopicRoomUpdate, RequestType: "UpdateRoomRequest", ResponseType: "BaseResponse"},
	{Topic: TopicRoomDelete, RequestType: "DeleteRoomRequest", ResponseType: "BaseResponse"},
	{Topic: TopicRoomList, RequestType: "ListRoomsRequest", ResponseType: "ListRoomsResponse"},

	// Message commands
	{Topic: TopicMessageSend, RequestType: "SendMessageRequest", ResponseType: "SendMessageResponse"},
	{Topic: TopicMessageGet, RequestType: "GetMessageRequest", ResponseType: "GetMessageResponse"},
	{Topic: TopicMessageDelete, RequestType: "DeleteMessageRequest", ResponseType: "BaseResponse"},

	// Reaction commands
	{Topic: TopicReactionAdd, RequestType: "AddReactionRequest", ResponseType: "AddReactionResponse"},
	{Topic: TopicReactionRemove, RequestType: "RemoveReactionRequest", ResponseType: "BaseResponse"},
	{Topic: TopicReactionGet, RequestType: "GetReactionRequest", ResponseType: "GetReactionResponse"},

	// Room batch membership commands
	{Topic: TopicRoomMemberBatchAdd, RequestType: "BatchAddMemberRequest", ResponseType: "BatchAddMemberResponse"},
	{Topic: TopicRoomMemberBatchRemove, RequestType: "BatchRemoveMemberRequest", ResponseType: "BatchRemoveMemberResponse"},

	// Actor commands
	{Topic: TopicActorSync, RequestType: "SyncActorRequest", ResponseType: "BaseResponse"},

	// Space commands
	{Topic: TopicSpaceCreate, RequestType: "CreateSpaceRequest", ResponseType: "BaseResponse"},
	{Topic: TopicSpaceGet, RequestType: "GetSpaceRequest", ResponseType: "GetSpaceResponse"},
	{Topic: TopicSpaceUpdate, RequestType: "UpdateSpaceRequest", ResponseType: "BaseResponse"},
	{Topic: TopicSpaceDelete, RequestType: "DeleteSpaceRequest", ResponseType: "BaseResponse"},
	{Topic: TopicSpaceList, RequestType: "ListSpacesRequest", ResponseType: "ListSpacesResponse"},

	// Hierarchy commands
	{Topic: TopicHierarchySetParent, RequestType: "SetParentRequest", ResponseType: "BaseResponse"},

	// Space batch membership commands
	{Topic: TopicSpaceMemberBatchAdd, RequestType: "BatchAddSpaceMemberRequest", ResponseType: "BatchAddSpaceMemberResponse"},
	{Topic: TopicSpaceMemberBatchRemove, RequestType: "BatchRemoveSpaceMemberRequest", ResponseType: "BatchRemoveSpaceMemberResponse"},

	// Room members query (V4)
	{Topic: TopicRoomMembersGet, RequestType: "GetRoomMembersRequest", ResponseType: "GetRoomMembersResponse"},

	// Thread messages query (V4)
	{Topic: TopicThreadMessagesGet, RequestType: "GetThreadMessagesRequest", ResponseType: "GetThreadMessagesResponse"},

	// Read Receipt commands
	{Topic: TopicMessageRead, RequestType: "MarkMessageReadRequest", ResponseType: "BaseResponse"},
	{Topic: TopicUnreadCountsGet, RequestType: "GetUnreadCountsRequest", ResponseType: "GetUnreadCountsResponse"},

	// Batch query commands
	{Topic: TopicBatchUnreadCountsGet, RequestType: "BatchGetUnreadCountsRequest", ResponseType: "BatchGetUnreadCountsResponse"},
	{Topic: TopicLastMessageGet, RequestType: "GetLastMessageRequest", ResponseType: "GetLastMessageResponse"},
	{Topic: TopicBatchLastMessagesGet, RequestType: "BatchGetLastMessagesRequest", ResponseType: "BatchGetLastMessagesResponse"},

	// Room check commands (adapter-initiated request-reply)
	{Topic: TopicRoomCheck, RequestType: "CheckRoomRequest", ResponseType: "CheckRoomResponse"},
	{Topic: TopicRoomInfo, RequestType: "GetRoomInfoRequest", ResponseType: "GetRoomInfoResponse"},

	// Custom state API (io.alkemio.*)
	{Topic: TopicRoomStateSet, RequestType: "SetRoomStateRequest", ResponseType: "BaseResponse"},
	{Topic: TopicRoomStateGet, RequestType: "GetRoomStateRequest", ResponseType: "GetRoomStateResponse"},
	{Topic: TopicSpaceStateSet, RequestType: "SetSpaceStateRequest", ResponseType: "BaseResponse"},
	{Topic: TopicSpaceStateGet, RequestType: "GetSpaceStateRequest", ResponseType: "GetSpaceStateResponse"},
}

// OutgoingEventRegistry defines events emitted by the adapter (not commands).
//
//nolint:gochecknoglobals // Registry is intentionally global for code generation
var OutgoingEventRegistry = []CommandDef{
	{Topic: TopicMessageReceived, RequestType: "", ResponseType: "MessageReceivedPayload"},
	{Topic: TopicRoomDMRequested, RequestType: "", ResponseType: "DMRequestedEvent"},
	// V4 Protocol Extension - New Outgoing Events
	{Topic: TopicReactionAdded, RequestType: "", ResponseType: "ReactionAddedEvent"},
	{Topic: TopicReactionRemoved, RequestType: "", ResponseType: "ReactionRemovedEvent"},
	{Topic: TopicRoomMemberLeft, RequestType: "", ResponseType: "RoomMemberLeftEvent"},
	// Read Receipt & Message Events (008-read-receipts)
	{Topic: TopicReadReceiptUpdated, RequestType: "", ResponseType: "ReadReceiptUpdatedEvent"},
	{Topic: TopicMessageEdited, RequestType: "", ResponseType: "MessageEditedEvent"},
	{Topic: TopicMessageRedacted, RequestType: "", ResponseType: "MessageRedactedEvent"},
	{Topic: TopicRoomCreated, RequestType: "", ResponseType: "RoomCreatedEvent"},
	{Topic: TopicRoomMemberUpdated, RequestType: "", ResponseType: "RoomMemberUpdatedEvent"},
	// Room state change events (010-room-state-events)
	{Topic: TopicRoomUpdated, RequestType: "", ResponseType: "RoomUpdatedEvent"},
	// Space state change events (013-space-room-params)
	{Topic: TopicSpaceUpdated, RequestType: "", ResponseType: "SpaceUpdatedEvent"},
}
