import pkg  from '@nestjs/common';
const { Inject, Injectable } = pkg;
import { ConfigService } from '@nestjs/config';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { ConfigurationTypes, LogContext } from '../../common/enums/index.js';
import { BootstrapException } from '../../common/exceptions/bootstrap.exception.js';
import { CommunicationAdapter } from '../../domain/communication-adapter/communication.adapter.js';

@Injectable()
export class BootstrapService {
  constructor(
    private configService: ConfigService,
    private communicationAdapter: CommunicationAdapter,
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService
  ) {}

  async bootstrap() {
    try {
      this.logger.verbose?.(
        'Bootstrapping Matrix Adapter...',
        LogContext.BOOTSTRAP
      );
      this.logConfiguration();
      await this.communicationAdapter.initiateMatrixConnection();
    } catch (error: any) {
      throw new BootstrapException(error.message);
    }
  }

  logConfiguration() {
    this.logger.verbose?.(
      '==== Configuration - Start ===',
      LogContext.BOOTSTRAP
    );

    const values = Object.values(ConfigurationTypes);
    for (const value of values) {
      this.logConfigLevel(value, this.configService.get(value));
    }
    this.logger.verbose?.('==== Configuration - End ===', LogContext.BOOTSTRAP);
  }

  logConfigLevel(key: any, value: any, indent = '', incrementalIndent = '  ') {
    if (typeof value === 'object') {
      const msg = `${indent}${key}:`;
      this.logger.verbose?.(`${msg}`, LogContext.BOOTSTRAP);
      Object.keys(value).forEach(childKey => {
        const childValue = value[childKey];
        const newIndent = `${indent}${incrementalIndent}`;
        this.logConfigLevel(childKey, childValue, newIndent, incrementalIndent);
      });
    } else {
      const msg = `${indent}==> ${key}: ${value}`;
      this.logger.verbose?.(`${msg}`, LogContext.BOOTSTRAP);
    }
  }
}
