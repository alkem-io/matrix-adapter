import pkg  from '@nestjs/common';
const { Inject, Controller } = pkg;
import {
  Ctx,
  MessagePattern,
  Payload,
  RmqContext,
  RpcException,
} from '@nestjs/microservices';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { LogContext } from './common/enums/logging.context.js';
import { CommunicationAdapter } from './services/communication-adapter/communication.adapter.js';
import alkemioMatrixAdapterLib from '@alkemio/matrix-adapter-lib';

@Controller()
export class AppController {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private communicationAdapter: CommunicationAdapter
  ) {}

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_DETAILS })
  async roomDetails(
    @Payload() data: alkemioMatrixAdapterLib.RoomDetailsPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomDetailsResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_DETAILS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const room = await this.communicationAdapter.getCommunityRoom(
        data.roomID
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomDetailsResponsePayload = {
        room: room,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when getting room details: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_MEMBERS })
  async roomMembers(
    @Payload() data: alkemioMatrixAdapterLib.RoomMembersPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomMembersResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_MEMBERS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const userIDs = await this.communicationAdapter.getRoomMembers(
        data.roomID
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomMembersResponsePayload = {
        userIDs: userIDs,
      };

      return response;
    } catch (error: any) {
      const errorMessage = `Error when getting room members: ${error}`;
      this.logger.error(errorMessage, error?.stack, LogContext.COMMUNICATION);
      channel.ack(originalMsg);

      throw error;
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_SEND_MESSAGE })
  async roomSendMessage(
    @Payload() data: alkemioMatrixAdapterLib.RoomSendMessagePayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomSendMessageResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_SEND_MESSAGE} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const message = await this.communicationAdapter.sendMessage(data);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomSendMessageResponsePayload = {
        message: message,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when sending message to room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_SEND_MESSAGE_REPLY })
  async roomSendMessageReply(
    @Payload() data: alkemioMatrixAdapterLib.RoomSendMessageReplyPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomSendMessageResponsePayload> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_SEND_MESSAGE_REPLY
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const message = await this.communicationAdapter.sendMessageReply(data);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomSendMessageResponsePayload = {
        message: message,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when sending message to room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_ADD_REACTION_TO_MESSAGE })
  async roomAddReactionToMessage(
    @Payload() data: alkemioMatrixAdapterLib.RoomAddMessageReactionPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomAddMessageReactionResponsePayload> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_ADD_REACTION_TO_MESSAGE
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const reaction =
        await this.communicationAdapter.addReactionToMessage(data);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomAddMessageReactionResponsePayload = {
        reaction,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when adding reaction to message in room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({
    cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_REMOVE_REACTION_TO_MESSAGE,
  })
  async roomRemoveReactionToMessage(
    @Payload() data: alkemioMatrixAdapterLib.RoomRemoveMessageReactionPayload,
    @Ctx() context: RmqContext
  ): Promise<boolean> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_REMOVE_REACTION_TO_MESSAGE
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      await this.communicationAdapter.removeReactionToMessage(data);
      channel.ack(originalMsg);

      return true;
    } catch (error) {
      const errorMessage = `Error when removing reaction to message in room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_DELETE_MESSAGE })
  async roomDeleteMessage(
    @Payload() data: alkemioMatrixAdapterLib.RoomDeleteMessagePayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomDeleteMessageResponsePayload> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_DELETE_MESSAGE
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const messageID = await this.communicationAdapter.deleteMessage(data);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomDeleteMessageResponsePayload = {
        messageID: messageID,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when deleting message on room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_MESSAGE_SENDER })
  async roomMessageSender(
    @Payload() data: alkemioMatrixAdapterLib.RoomMessageSenderPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomMessageSenderResponsePayload> {
    this.logger.verbose?.(
      `${
       alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_MESSAGE_SENDER
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const senderID = await this.communicationAdapter.getMessageSender(
        data.roomID,
        data.messageID
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomMessageSenderResponsePayload = {
        senderID: senderID,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when getting message sender on room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_REACTION_SENDER })
  async roomReactionSender(
    @Payload() data: alkemioMatrixAdapterLib.RoomReactionSenderPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomReactionSenderResponsePayload> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOM_MESSAGE_SENDER
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const senderID = await this.communicationAdapter.getReactionSender(
        data.roomID,
        data.reactionID
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomReactionSenderResponsePayload = {
        senderID: senderID,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when getting reaction sender on room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.UPDATE_ROOM_STATE })
  async updateRoomState(
    @Payload() data: alkemioMatrixAdapterLib.UpdateRoomStatePayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomDetailsResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.UPDATE_ROOM_STATE} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.updateRoomState(data);
      channel.ack(originalMsg);

      return result;
    } catch (error) {
      const errorMessage = `Error when getting join rule on room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.CREATE_ROOM })
  async createRoom(
    @Payload() data: alkemioMatrixAdapterLib.CreateRoomPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.CreateRoomResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.CREATE_ROOM} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const roomID = await this.communicationAdapter.createCommunityRoom(
        data.roomName,
        data.metadata
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.CreateRoomResponsePayload = {
        roomID: roomID,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when creating group: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOMS_USER })
  async roomsUser(
    @Payload() data: alkemioMatrixAdapterLib.RoomsUserPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomsUserResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOMS_USER} - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const rooms = await this.communicationAdapter.getCommunityRooms(
        data.userID
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomsUserResponsePayload = {
        rooms: rooms,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when getting rooms for user: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOMS_USER_DIRECT })
  async roomsUserDirect(
    @Payload() data: alkemioMatrixAdapterLib.RoomsUserDirectPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomsUserDirectResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOMS_USER} - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const rooms = await this.communicationAdapter.getDirectRooms(data.userID);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomsUserDirectResponsePayload = {
        rooms: rooms,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when getting direct rooms for user: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOMS_USER })
  async rooms(
    @Payload() data: alkemioMatrixAdapterLib.RoomsPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RoomsResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ROOMS} - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const rooms = await this.communicationAdapter.getAllRooms();
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RoomsResponsePayload = {
        rooms: rooms,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when getting all rooms: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.REMOVE_USER_FROM_ROOMS })
  async removeUserFromRooms(
    @Payload() data: alkemioMatrixAdapterLib.RemoveUserFromRoomsPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RemoveUserFromRoomsResponsePayload> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.REMOVE_USER_FROM_ROOMS
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.removeUserFromRooms(
        data.roomIDs,
        data.userID
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RemoveUserFromRoomsResponsePayload = {
        success: result,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when removing user from rooms: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.REMOVE_ROOM })
  async removeRoom(
    @Payload() data: alkemioMatrixAdapterLib.RemoveRoomPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RemoveRoomResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.REMOVE_ROOM} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.removeRoom(data.roomID);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RemoveRoomResponsePayload = {
        success: result,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when removing room '${data.roomID}': ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.REPLICATE_ROOM_MEMBERSHIP })
  async replicateRoomMembership(
    @Payload() data: alkemioMatrixAdapterLib.ReplicateRoomMembershipPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.ReplicateRoomMembershipResponsePayload> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.REPLICATE_ROOM_MEMBERSHIP
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.replicateRoomMembership(
        data.targetRoomID,
        data.sourceRoomID,
        data.userToPrioritize
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.ReplicateRoomMembershipResponsePayload = {
        success: result,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when replicating room membership: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ADD_USER_TO_ROOMS })
  async addUserToRooms(
    @Payload() data: alkemioMatrixAdapterLib.AddUserToRoomsPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.AddUserToRoomsResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ADD_USER_TO_ROOMS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.grantUserAccesToRooms(
        data.roomIDs,
        data.userID
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.AddUserToRoomsResponsePayload = {
        success: result,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when adding user to rooms: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.ADD_USER_TO_ROOM })
  async addUserToRoom(
    @Payload() data: alkemioMatrixAdapterLib.AddUserToRoomPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.AddUserToRoomResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.ADD_USER_TO_ROOMS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      await this.communicationAdapter.addUserToRoom(data.roomID, data.userID);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.AddUserToRoomResponsePayload = {
        success: true,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when adding user to rooms: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.SEND_MESSAGE_TO_USER })
  async sendMessageToUser(
    @Payload() data: alkemioMatrixAdapterLib.SendMessageToUserPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.SendMessageToUserResponsePayload> {
    this.logger.verbose?.(
      `${
        alkemioMatrixAdapterLib.MatrixAdapterEventType.SEND_MESSAGE_TO_USER
      } - payload: ${JSON.stringify(data)}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.sendMessageToUser(data);
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.SendMessageToUserResponsePayload = {
        messageID: result,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when sending message to user: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: alkemioMatrixAdapterLib.MatrixAdapterEventType.REGISTER_NEW_USER })
  async registerNewUser(
    @Payload() data: alkemioMatrixAdapterLib.RegisterNewUserPayload,
    @Ctx() context: RmqContext
  ): Promise<alkemioMatrixAdapterLib.RegisterNewUserResponsePayload> {
    this.logger.verbose?.(
      `${alkemioMatrixAdapterLib.MatrixAdapterEventType.REGISTER_NEW_USER} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.tryRegisterNewUser(
        data.email
      );
      channel.ack(originalMsg);
      const response: alkemioMatrixAdapterLib.RegisterNewUserResponsePayload = {
        userID: result,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when registering a new user: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }
}
