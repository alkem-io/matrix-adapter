import { ConfigurationTypes, LogContext } from '@common/enums';
import { MatrixEntityNotFoundException } from '@common/exceptions';
import { Inject, Injectable, LoggerService } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { createClient, IContent, MatrixClient } from 'matrix-js-sdk';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { MatrixRoom } from '../adapter-room/matrix.room';
import { MatrixRoomAdapter } from '../adapter-room/matrix.room.adapter';
import { MatrixUserAdapter } from '../adapter-user/matrix.user.adapter';
import { IOperationalMatrixUser } from '../adapter-user/matrix.user.interface';
import { MatrixAgent } from './matrix.agent';
import { MatrixAgentMessageRequest } from './matrix.agent.dto.message.request';
import { MatrixAgentMessageRequestDirect } from './matrix.agent.dto.message.request.direct';
import { IMatrixAgent } from './matrix.agent.interface';
import { MatrixMessageAdapter } from '../adapter-message/matrix.message.adapter';
import { MatrixAgentMessageReply } from './matrix.agent.dto.message.reply';
import { MatrixAgentMessageReaction } from './matrix.agent.dto.message.reaction';

@Injectable()
export class MatrixAgentService {
  constructor(
    private configService: ConfigService,
    private matrixUserAdapter: MatrixUserAdapter,
    private matrixRoomAdapter: MatrixRoomAdapter,
    private matrixMessageAdapter: MatrixMessageAdapter,
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService
  ) {}

  async createMatrixAgent(
    operator: IOperationalMatrixUser
  ): Promise<MatrixAgent> {
    const matrixClient = await this.createMatrixClient(operator);
    return new MatrixAgent(
      matrixClient,
      this.matrixRoomAdapter,
      this.matrixMessageAdapter,
      this.logger
    );
  }

  async createMatrixClient(
    operator: IOperationalMatrixUser
  ): Promise<MatrixClient> {
    const idBaseUrl = this.configService.get(ConfigurationTypes.MATRIX)?.server
      ?.url;
    const baseUrl = this.configService.get(ConfigurationTypes.MATRIX)?.server
      ?.url;

    if (!idBaseUrl || !baseUrl) {
      throw new Error('Matrix configuration is not provided');
    }

    return createClient({
      baseUrl: baseUrl,
      idBaseUrl: idBaseUrl,
      userId: operator.username,
      accessToken: operator.accessToken,
    });
  }

  async getDirectRooms(matrixAgent: IMatrixAgent): Promise<MatrixRoom[]> {
    const matrixClient = matrixAgent.matrixClient;
    const rooms: MatrixRoom[] = [];

    // Direct rooms
    const dmRoomMap = await this.matrixRoomAdapter.getDirectMessageRoomsMap(
      matrixClient
    );
    for (const matrixUsername of Object.keys(dmRoomMap)) {
      const room = await this.getRoom(
        matrixAgent,
        dmRoomMap[matrixUsername][0]
      );
      room.receiverCommunicationsID =
        this.matrixUserAdapter.convertMatrixUsernameToMatrixID(matrixUsername);
      room.isDirect = true;
      rooms.push(room);
    }
    return rooms;
  }

  async getRoom(
    matrixAgent: IMatrixAgent,
    roomId: string
  ): Promise<MatrixRoom> {
    const matrixRoom = await matrixAgent.matrixClient.getRoom(roomId);
    if (!matrixRoom) {
      throw new MatrixEntityNotFoundException(
        `[User: ${matrixAgent.matrixClient.getUserId()}] Unable to access Room (${roomId}). Room either does not exist or user does not have access.`,
        LogContext.COMMUNICATION
      );
    }

    return matrixRoom;
  }

  async initiateMessagingToUser(
    matrixAgent: IMatrixAgent,
    messageRequest: MatrixAgentMessageRequestDirect
  ): Promise<string> {
    const directRoom = await this.getDirectRoomForMatrixID(
      matrixAgent,
      messageRequest.matrixID
    );
    if (directRoom) return directRoom.roomId;

    // Room does not exist, create...
    const targetRoomId = await this.matrixRoomAdapter.createRoom(
      matrixAgent.matrixClient,
      {
        dmUserId: messageRequest.matrixID,
      }
    );

    await this.matrixRoomAdapter.storeDirectMessageRoom(
      matrixAgent.matrixClient,
      targetRoomId,
      messageRequest.matrixID
    );

    return targetRoomId;
  }

  /*
    the naming is really confusing
    what we attempt to solve is a race condition
    where two or more DM rooms are created between the two users
    we aim to always resolve the
  */
  async getDirectUserMatrixIDForRoomID(
    matrixAgent: MatrixAgent,
    matrixRoomId: string
  ): Promise<string | undefined> {
    // Need to implement caching for performance
    const dmRoomByUserMatrixIDMap =
      await this.matrixRoomAdapter.getDirectMessageRoomsMap(
        matrixAgent.matrixClient
      );
    const dmUserMatrixIDs = Object.keys(dmRoomByUserMatrixIDMap);
    const dmRoom = dmUserMatrixIDs.find(
      userID => dmRoomByUserMatrixIDMap[userID].indexOf(matrixRoomId) !== -1
    );
    return dmRoom;
  }

  async getDirectRoomForMatrixID(
    matrixAgent: IMatrixAgent,
    matrixUserId: string
  ): Promise<MatrixRoom | undefined> {
    const matrixUsername =
      this.matrixUserAdapter.convertMatrixIDToUsername(matrixUserId);
    // Need to implement caching for performance
    const dmRoomIds = this.matrixRoomAdapter.getDirectMessageRoomsMap(
      matrixAgent.matrixClient
    )[matrixUsername];

    if (!dmRoomIds || !Boolean(dmRoomIds[0])) {
      return undefined;
    }

    // Have a result
    const targetRoomId = dmRoomIds[0];
    return await this.getRoom(matrixAgent, targetRoomId);
  }

  async sendMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageRequest: MatrixAgentMessageRequest
  ): Promise<string> {
    const response = await matrixAgent.matrixClient.sendEvent(
      roomId,
      this.matrixMessageAdapter.EVENT_TYPE_MESSAGE,
      { body: messageRequest.text, msgtype: 'm.text' }
    );

    return response.event_id;
  }

  async sendReplyToMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageRequest: MatrixAgentMessageReply
  ): Promise<string> {
    const response = await matrixAgent.matrixClient.sendEvent(
      roomId,
      this.matrixMessageAdapter.EVENT_TYPE_MESSAGE,
      {
        msgtype: 'm.text',
        body: messageRequest.text,
        ['m.relates_to']: {
          rel_type: 'm.thread',
          event_id: messageRequest.threadID,
          is_falling_back: true,
          ['m.in_reply_to']: {
            event_id: messageRequest.lastMessageID,
          },
        },
      }
    );

    return response.event_id;
  }

  async addReactionOnMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageReaction: MatrixAgentMessageReaction
  ): Promise<string> {
    const response = await matrixAgent.matrixClient.sendEvent(
      roomId,
      this.matrixMessageAdapter.EVENT_TYPE_REACTION,
      {
        'm.relates_to': {
          rel_type: 'm.annotation',
          event_id: messageReaction.messageID,
          key: messageReaction.text,
        },
      }
    );

    return response.event_id;
  }

  async removeReactionOnMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    reactionID: string
  ) {
    await matrixAgent.matrixClient.redactEvent(roomId, reactionID);
  }

  // TODO - see if the js sdk supports message aggregation
  async editMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageId: string,
    messageRequest: MatrixAgentMessageRequest
  ) {
    const newContent: IContent = {
      msgtype: 'm.text',
      body: messageRequest.text,
    };
    await matrixAgent.matrixClient.sendMessage(
      roomId,
      Object.assign(
        {
          'm.new_content': newContent,
          'm.relates_to': {
            rel_type: 'm.replace',
            event_id: messageId,
          },
        },
        newContent
      )
    );
  }

  async deleteMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageId: string
  ) {
    await matrixAgent.matrixClient.redactEvent(roomId, messageId);
  }
}
