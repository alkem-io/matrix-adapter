export enum MatrixAdapterEventType {
  ROOM_DETAILS = 'roomDetails',
  ROOM_JOIN_RULE = 'roomJoinRule',
  ROOM_MEMBERS = 'roomMembers',
  ROOM_SEND_MESSAGE = 'roomSendMessage',
  ROOM_SEND_MESSAGE_REPLY = 'roomSendMessageReply',
  ROOM_ADD_REACTION_TO_MESSAGE = 'roomAddReactionToMessage',
  ROOM_REMOVE_REACTION_TO_MESSAGE = 'roomRemoveReactionToMessage',
  ROOM_DELETE_MESSAGE = 'roomDeleteMessage',
  ROOM_MESSAGE_SENDER = 'roomMessageSender',
  ROOM_REACTION_SENDER = 'roomReactionSender',
  ROOMS_USER = 'roomsUser',
  ROOMS_USER_DIRECT = 'roomsUserDirect',
  ROOMS = 'rooms',
  REMOVE_USER_FROM_ROOMS = 'removeUserFromRooms',
  REPLICATE_ROOM_MEMBERSHIP = 'replicateRoomMembership',
  CREATE_GROUP = 'createGroup',
  CREATE_ROOM = 'createRoom',
  ADD_USER_TO_ROOMS = 'addUserToRooms',
  ADD_USER_TO_ROOM = 'addUserToRoom',
  REMOVE_ROOM = 'removeRoom',
  UPDATE_ROOMS_GUEST_ACCESS = 'updateRoomsGuestAccess',
  SEND_MESSAGE_TO_USER = 'sendMessageToUser',
  REGISTER_NEW_USER = 'registerNewUser',
}
