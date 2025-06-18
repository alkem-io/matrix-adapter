import { ConfigurationTypes, LogContext } from '@common/enums/index.js';
import { MatrixEntityNotFoundException } from '@common/exceptions/matrix.entity.not.found.exception.js';
import pkg  from '@nestjs/common';
const { Inject, Injectable } = pkg;
import { ConfigService } from '@nestjs/config';
import {
  createClient,
  MatrixClient,
  ICreateClientOpts,
  EventType,
  MsgType,
  RelationType,
} from 'matrix-js-sdk';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { MatrixRoom } from '../adapter-room/matrix.room.js';
import { MatrixRoomAdapter } from '../adapter-room/matrix.room.adapter.js';
import { MatrixUserAdapter } from '../adapter-user/matrix.user.adapter.js';
import { IOperationalMatrixUser } from '../adapter-user/matrix.user.interface.js';
import { MatrixAgent } from './matrix.agent.js';
import { MatrixAgentMessageRequest } from './matrix.agent.dto.message.request.js';
import { MatrixAgentMessageRequestDirect } from './matrix.agent.dto.message.request.direct.js';
import { IMatrixAgent } from './matrix.agent.interface.js';
import { MatrixMessageAdapter } from '../adapter-message/matrix.message.adapter.js';
import { MatrixAgentMessageReply } from './matrix.agent.dto.message.reply.js';
import { MatrixAgentMessageReaction } from './matrix.agent.dto.message.reaction.js';
import {
  ReactionEventContent,
  RoomMessageEventContent,
} from 'matrix-js-sdk/lib/types.js';
import { AlkemioMatrixLogger } from '../types/matrix.logger.js';
import { sleep } from 'matrix-js-sdk/lib/utils.js';

@Injectable()
export class MatrixAgentService {
  constructor(
    private configService: ConfigService,
    private matrixUserAdapter: MatrixUserAdapter,
    private matrixRoomAdapter: MatrixRoomAdapter,
    private matrixMessageAdapter: MatrixMessageAdapter,
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService
  ) {}

  async createMatrixAgent(
    operator: IOperationalMatrixUser
  ): Promise<MatrixAgent> {
    const matrixClient = await this.createMatrixClient(operator);
    return new MatrixAgent(
      matrixClient,
      this.matrixRoomAdapter,
      this.matrixMessageAdapter,
      this.configService,
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

    const timelineSupport: boolean = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.client.timelineSupport;

    this.logger.verbose?.(
      `Creating Matrix Client for ${operator.username} using timeline flag: ${timelineSupport}`,
      LogContext.MATRIX
    );

    const alkemioMatrixLogger = new AlkemioMatrixLogger(this.logger);

    const createClientInput: ICreateClientOpts = {
      baseUrl: baseUrl,
      idBaseUrl: idBaseUrl,
      userId: operator.username,
      accessToken: operator.accessToken,
      timelineSupport: timelineSupport,
      logger: alkemioMatrixLogger,
    };
    return createClient(createClientInput);
  }

  async getDirectRooms(matrixAgent: IMatrixAgent): Promise<MatrixRoom[]> {
    const matrixClient = matrixAgent.matrixClient;
    const rooms: MatrixRoom[] = [];

    // Direct rooms
    const dmRoomMap =
      this.matrixRoomAdapter.getDirectMessageRoomsMap(matrixClient);
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
    let matrixRoom = matrixAgent.matrixClient.getRoom(roomId);

    // TODO: temporary work around to allow room membership to complete
    const maxRetries = 10;
    let reTries = 0;
    while (!matrixRoom && reTries < maxRetries) {
      await sleep(100);
      matrixRoom = matrixAgent.matrixClient.getRoom(roomId);
      reTries++;
      this.logger.verbose?.(
        `Retrying to get room ${roomId}`,
        LogContext.COMMUNICATION
      );
    }
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
      {},
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
    const content: RoomMessageEventContent = {
      body: messageRequest.text,
      msgtype: MsgType.Text,
    };
    const response = await matrixAgent.matrixClient.sendEvent(
      roomId,
      EventType.RoomMessage,
      content
    );

    return response.event_id;
  }

  async sendReplyToMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageRequest: MatrixAgentMessageReply
  ): Promise<string> {
    const content: RoomMessageEventContent = {
      msgtype: MsgType.Text,
      body: messageRequest.text,
      ['m.relates_to']: {
        rel_type: RelationType.Thread,
        event_id: messageRequest.threadID,
        //is_falling_back: true,
        // when events need to be represented in an unthreaded client, this field makes the event a reply to the thread root event
        // ['m.in_reply_to']: {
        //   event_id: messageRequest.threadID,
        // },
      },
    };

    const response = await matrixAgent.matrixClient.sendEvent(
      roomId,
      EventType.RoomMessage,
      content
    );

    return response.event_id;
  }

  async addReactionOnMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageReaction: MatrixAgentMessageReaction
  ): Promise<string> {
    const content: ReactionEventContent = {
      'm.relates_to': {
        rel_type: RelationType.Annotation,
        event_id: messageReaction.messageID,
        key: messageReaction.emoji,
      },
    };

    const response = await matrixAgent.matrixClient.sendEvent(
      roomId,
      EventType.Reaction,
      content
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
    this.logger.verbose?.(
      `Editing message: ${messageId} ${matrixAgent} ${roomId} ${messageRequest}`,
      LogContext.COMMUNICATION
    );
    // const newContent: IContent = {
    //   msgtype: MsgType.Text,
    //   body: messageRequest.text,
    // };
    // const content: RoomMessageEventContent = {
    //   msgtype: MsgType.Text,
    //   'm.new_content': newContent,
    //   'm.relates_to': {
    //     rel_type: 'm.replace',
    //     event_id: messageId,
    //   },
    //   body: messageRequest.text,
    // };
    // await matrixAgent.matrixClient.sendMessage(roomId, content);
  }

  async deleteMessage(
    matrixAgent: IMatrixAgent,
    roomId: string,
    messageId: string
  ) {
    await matrixAgent.matrixClient.redactEvent(roomId, messageId);
  }
}
