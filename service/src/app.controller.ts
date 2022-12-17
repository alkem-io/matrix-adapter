import { Controller, Inject, LoggerService } from '@nestjs/common';
import {
  Ctx,
  MessagePattern,
  Payload,
  RmqContext,
  RpcException,
} from '@nestjs/microservices';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { LogContext } from './common/enums';
import {
  MatrixAdapterEventType,
  RoomDetailsResponsePayload,
} from '@alkemio/matrix-adapter-lib';
import { RoomDetailsPayload } from '@alkemio/matrix-adapter-lib';
import { CommunicationAdapter } from './services/communication-adapter/communication.adapter';
import {
  RoomSendMessagePayload,
  RoomSendMessageResponsePayload,
  RoomDeleteMessagePayload,
  RoomDeleteMessageResponsePayload,
} from '@alkemio/matrix-adapter-lib';
import { RoomMessageSenderResponsePayload } from '@alkemio/matrix-adapter-lib';
import { RoomMessageSenderPayload } from '@alkemio/matrix-adapter-lib';
import { CreateGroupPayload } from '@alkemio/matrix-adapter-lib';
import { CreateGroupResponsePayload } from '@alkemio/matrix-adapter-lib';
import { CreateRoomPayload } from '@alkemio/matrix-adapter-lib';
import { CreateRoomResponsePayload } from '@alkemio/matrix-adapter-lib';
import { RoomsUserResponsePayload } from '@alkemio/matrix-adapter-lib';
import { RoomsUserPayload } from '@alkemio/matrix-adapter-lib';
import { RoomsPayload } from '@alkemio/matrix-adapter-lib';
import { RoomsResponsePayload } from '@alkemio/matrix-adapter-lib';
import { RemoveUserFromRoomsResponsePayload } from '@alkemio/matrix-adapter-lib';
import { RemoveUserFromRoomsPayload } from '@alkemio/matrix-adapter-lib';
import { ReplicateRoomMembershipPayload } from '@alkemio/matrix-adapter-lib';
import { ReplicateRoomMembershipResponsePayload } from '@alkemio/matrix-adapter-lib';
import { AddUserToRoomsPayload } from '@alkemio/matrix-adapter-lib';
import { AddUserToRoomsResponsePayload } from '@alkemio/matrix-adapter-lib';
import { SendMessageToUserPayload } from '@alkemio/matrix-adapter-lib';
import { SendMessageToUserResponsePayload } from '@alkemio/matrix-adapter-lib';
import { RoomsUserDirectResponsePayload } from '@alkemio/matrix-adapter-lib';
import { RoomsUserDirectPayload } from '@alkemio/matrix-adapter-lib';
import { AddUserToRoomResponsePayload } from '@alkemio/matrix-adapter-lib';
import { AddUserToRoomPayload } from '@alkemio/matrix-adapter-lib';
import { RegisterNewUserPayload } from '@alkemio/matrix-adapter-lib';
import { RegisterNewUserResponsePayload } from '@alkemio/matrix-adapter-lib';

@Controller()
export class AppController {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService,
    private communicationAdapter: CommunicationAdapter
  ) {}

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOM_DETAILS })
  async roomDetails(
    @Payload() data: RoomDetailsPayload,
    @Ctx() context: RmqContext
  ): Promise<RoomDetailsResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ROOM_DETAILS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const room = await this.communicationAdapter.getCommunityRoom(
        data.roomID
      );
      channel.ack(originalMsg);
      const response: RoomDetailsResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOM_SEND_MESSAGE })
  async roomSendMessage(
    @Payload() data: RoomSendMessagePayload,
    @Ctx() context: RmqContext
  ): Promise<RoomSendMessageResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ROOM_SEND_MESSAGE} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const message = await this.communicationAdapter.sendMessage(data);
      channel.ack(originalMsg);
      const response: RoomSendMessageResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOM_DELETE_MESSAGE })
  async roomDeleteMessage(
    @Payload() data: RoomDeleteMessagePayload,
    @Ctx() context: RmqContext
  ): Promise<RoomDeleteMessageResponsePayload> {
    this.logger.verbose?.(
      `${
        MatrixAdapterEventType.ROOM_DELETE_MESSAGE
      } - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const messageID = await this.communicationAdapter.deleteMessage(data);
      channel.ack(originalMsg);
      const response: RoomDeleteMessageResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOM_MESSAGE_SENDER })
  async roomMessageSender(
    @Payload() data: RoomMessageSenderPayload,
    @Ctx() context: RmqContext
  ): Promise<RoomMessageSenderResponsePayload> {
    this.logger.verbose?.(
      `${
        MatrixAdapterEventType.ROOM_MESSAGE_SENDER
      } - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const senderID = await this.communicationAdapter.getMessageSender(
        data.roomID,
        data.messageID
      );
      channel.ack(originalMsg);
      const response: RoomMessageSenderResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.CREATE_GROUP })
  async createGroup(
    @Payload() data: CreateGroupPayload,
    @Ctx() context: RmqContext
  ): Promise<CreateGroupResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.CREATE_GROUP} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const groupID = await this.communicationAdapter.createCommunityGroup(
        data.communityID,
        data.communityDisplayName
      );
      channel.ack(originalMsg);
      const response: CreateGroupResponsePayload = {
        groupID: groupID,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when creating group: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: MatrixAdapterEventType.CREATE_ROOM })
  async createRoom(
    @Payload() data: CreateRoomPayload,
    @Ctx() context: RmqContext
  ): Promise<CreateRoomResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.CREATE_ROOM} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const roomID = await this.communicationAdapter.createCommunityRoom(
        data.groupID,
        data.roomName,
        data.metadata
      );
      channel.ack(originalMsg);
      const response: CreateRoomResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOMS_USER })
  async roomsUser(
    @Payload() data: RoomsUserPayload,
    @Ctx() context: RmqContext
  ): Promise<RoomsUserResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ROOMS_USER} - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const rooms = await this.communicationAdapter.getCommunityRooms(
        data.userID
      );
      channel.ack(originalMsg);
      const response: RoomsUserResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOMS_USER_DIRECT })
  async roomsUserDirect(
    @Payload() data: RoomsUserDirectPayload,
    @Ctx() context: RmqContext
  ): Promise<RoomsUserDirectResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ROOMS_USER} - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const rooms = await this.communicationAdapter.getDirectRooms(data.userID);
      channel.ack(originalMsg);
      const response: RoomsUserDirectResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOMS_USER })
  async rooms(
    @Payload() data: RoomsPayload,
    @Ctx() context: RmqContext
  ): Promise<RoomsResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ROOMS} - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const rooms = await this.communicationAdapter.getAllRooms();
      channel.ack(originalMsg);
      const response: RoomsResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.REMOVE_USER_FROM_ROOMS })
  async removeUserFromRooms(
    @Payload() data: RemoveUserFromRoomsPayload,
    @Ctx() context: RmqContext
  ): Promise<RemoveUserFromRoomsResponsePayload> {
    this.logger.verbose?.(
      `${
        MatrixAdapterEventType.REMOVE_USER_FROM_ROOMS
      } - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.removeUserFromRooms(
        data.roomIDs,
        data.userID
      );
      channel.ack(originalMsg);
      const response: RemoveUserFromRoomsResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.REPLICATE_ROOM_MEMBERSHIP })
  async replicateRoomMembership(
    @Payload() data: ReplicateRoomMembershipPayload,
    @Ctx() context: RmqContext
  ): Promise<ReplicateRoomMembershipResponsePayload> {
    this.logger.verbose?.(
      `${
        MatrixAdapterEventType.REPLICATE_ROOM_MEMBERSHIP
      } - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
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
      const response: ReplicateRoomMembershipResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ADD_USER_TO_ROOMS })
  async addUserToRooms(
    @Payload() data: AddUserToRoomsPayload,
    @Ctx() context: RmqContext
  ): Promise<AddUserToRoomsResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ADD_USER_TO_ROOMS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.grantUserAccesToRooms(
        data.groupID,
        data.roomIDs,
        data.userID
      );
      channel.ack(originalMsg);
      const response: AddUserToRoomsResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.ADD_USER_TO_ROOM })
  async addUserToRoom(
    @Payload() data: AddUserToRoomPayload,
    @Ctx() context: RmqContext
  ): Promise<AddUserToRoomResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ADD_USER_TO_ROOMS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.addUserToRoom(
        data.roomID,
        data.userID
      );
      channel.ack(originalMsg);
      const response: AddUserToRoomResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.SEND_MESSAGE_TO_USER })
  async sendMessageToUser(
    @Payload() data: SendMessageToUserPayload,
    @Ctx() context: RmqContext
  ): Promise<SendMessageToUserResponsePayload> {
    this.logger.verbose?.(
      `${
        MatrixAdapterEventType.SEND_MESSAGE_TO_USER
      } - payload: ${JSON.stringify(data)}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.sendMessageToUser(data);
      channel.ack(originalMsg);
      const response: SendMessageToUserResponsePayload = {
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

  @MessagePattern({ cmd: MatrixAdapterEventType.REGISTER_NEW_USER })
  async registerNewUser(
    @Payload() data: RegisterNewUserPayload,
    @Ctx() context: RmqContext
  ): Promise<RegisterNewUserResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.REGISTER_NEW_USER} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const result = await this.communicationAdapter.tryRegisterNewUser(
        data.email
      );
      channel.ack(originalMsg);
      const response: RegisterNewUserResponsePayload = {
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
