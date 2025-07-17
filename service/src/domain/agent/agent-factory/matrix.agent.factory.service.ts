import { ConfigurationTypes, LogContext } from '@common/enums/index';
import pkg  from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { AlkemioMatrixLogger } from '@src/core/logger/matrix.logger';
import { IMatrixUser } from '@src/domain/user/matrix.user.interface';
import {
  createClient,
  ICreateClientOpts,
  MatrixClient,
} from 'matrix-js-sdk';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';

import { MatrixMessageAdapter } from '../../adapter-message/matrix.message.adapter';
import { MatrixAgent } from '../agent/matrix.agent';
const { Inject, Injectable } = pkg;

@Injectable()
export class MatrixAgentFactoryService {
  constructor(
    private configService: ConfigService,
    private matrixMessageAdapter: MatrixMessageAdapter,
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService
  ) {}

  createMatrixAgent(
    operator: IMatrixUser
  ): MatrixAgent {
    const matrixClient = this.createMatrixClient(operator);

    return new MatrixAgent(
      matrixClient,
      this.configService,
      this.matrixMessageAdapter,
      this.logger
    );
  }

  private createMatrixClient(
    operator: IMatrixUser
  ): MatrixClient {
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
