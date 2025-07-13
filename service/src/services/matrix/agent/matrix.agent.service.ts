import { ConfigurationTypes, LogContext } from '@common/enums/index';
import { MatrixEntityNotFoundException } from '@common/exceptions/matrix.entity.not.found.exception';
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
import { MatrixRoom } from '../adapter-room/dto/matrix.room';
import { MatrixRoomAdapter } from '../adapter-room/matrix.room.adapter';
import { MatrixUserAdapter } from '../adapter-user/matrix.user.adapter';
import { IOperationalMatrixUser } from '../adapter-user/matrix.user.interface';
import { MatrixAgent } from './matrix.agent';
import { MatrixAgentMessageRequest } from './dto/matrix.agent.dto.message.request';
import { MatrixAgentMessageRequestDirect } from './dto/matrix.agent.dto.message.request.direct';
import { IMatrixAgent } from './matrix.agent.interface';
import { MatrixMessageAdapter } from '../adapter-message/matrix.message.adapter';
import { MatrixAgentMessageReply } from './dto/matrix.agent.dto.message.reply';
import { MatrixAgentMessageReaction } from './dto/matrix.agent.dto.message.reaction';
import {
  ReactionEventContent,
  RoomMessageEventContent,
} from 'matrix-js-sdk/lib/types';
import { AlkemioMatrixLogger } from '../types/matrix.logger';
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

  private async createMatrixClient(
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
}
