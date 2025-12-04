package dto

// ============================================================================
// Command Registry - Single Source of Truth for Topic/Request/Response Mapping
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
	{Topic: "communication.room.create", RequestType: "CreateRoomRequest", ResponseType: "BaseResponse"},
	{Topic: "communication.room.get", RequestType: "GetRoomRequest", ResponseType: "GetRoomResponse"},
	{Topic: "communication.room.update", RequestType: "UpdateRoomRequest", ResponseType: "BaseResponse"},
	{Topic: "communication.room.delete", RequestType: "DeleteRoomRequest", ResponseType: "BaseResponse"},
	{Topic: "communication.room.list", RequestType: "ListRoomsRequest", ResponseType: "ListRoomsResponse"},

	// Message commands
	{Topic: "communication.message.send", RequestType: "SendMessageRequest", ResponseType: "SendMessageResponse"},
	{Topic: "communication.message.get", RequestType: "GetMessageRequest", ResponseType: "GetMessageResponse"},
	{Topic: "communication.message.delete", RequestType: "DeleteMessageRequest", ResponseType: "BaseResponse"},

	// Reaction commands
	{Topic: "communication.reaction.add", RequestType: "AddReactionRequest", ResponseType: "AddReactionResponse"},
	{Topic: "communication.reaction.remove", RequestType: "RemoveReactionRequest", ResponseType: "BaseResponse"},
	{Topic: "communication.reaction.get", RequestType: "GetReactionRequest", ResponseType: "GetReactionResponse"},

	// Room batch membership commands
	{Topic: "communication.room.member.batch.add", RequestType: "BatchAddMemberRequest", ResponseType: "BatchAddMemberResponse"},
	{Topic: "communication.room.member.batch.remove", RequestType: "BatchRemoveMemberRequest", ResponseType: "BatchRemoveMemberResponse"},

	// Actor commands
	{Topic: "communication.actor.sync", RequestType: "SyncActorRequest", ResponseType: "BaseResponse"},

	// Space commands
	{Topic: "communication.space.create", RequestType: "CreateSpaceRequest", ResponseType: "BaseResponse"},
	{Topic: "communication.space.get", RequestType: "GetSpaceRequest", ResponseType: "GetSpaceResponse"},
	{Topic: "communication.space.update", RequestType: "UpdateSpaceRequest", ResponseType: "BaseResponse"},
	{Topic: "communication.space.delete", RequestType: "DeleteSpaceRequest", ResponseType: "BaseResponse"},
	{Topic: "communication.space.list", RequestType: "ListSpacesRequest", ResponseType: "ListSpacesResponse"},

	// Hierarchy commands
	{Topic: "communication.hierarchy.set_parent", RequestType: "SetParentRequest", ResponseType: "BaseResponse"},

	// Space batch membership commands
	{Topic: "communication.space.member.batch.add", RequestType: "BatchAddSpaceMemberRequest", ResponseType: "BatchAddSpaceMemberResponse"},
	{Topic: "communication.space.member.batch.remove", RequestType: "BatchRemoveSpaceMemberRequest", ResponseType: "BatchRemoveSpaceMemberResponse"},
}

// OutgoingEventRegistry defines events emitted by the adapter (not commands).
//
//nolint:gochecknoglobals // Registry is intentionally global for code generation
var OutgoingEventRegistry = []CommandDef{
	{Topic: "communication.message.received", RequestType: "", ResponseType: "MessageReceivedPayload"},
}
