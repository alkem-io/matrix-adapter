package dto

// ============================================================================
// Topic Constants - Single Source of Truth
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
}

// OutgoingEventRegistry defines events emitted by the adapter (not commands).
//
//nolint:gochecknoglobals // Registry is intentionally global for code generation
var OutgoingEventRegistry = []CommandDef{
	{Topic: TopicMessageReceived, RequestType: "", ResponseType: "MessageReceivedPayload"},
}
