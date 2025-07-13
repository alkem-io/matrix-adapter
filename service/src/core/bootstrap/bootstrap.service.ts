import pkg from '@nestjs/common';
const { Inject, Injectable } = pkg;
import { ConfigService } from '@nestjs/config';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { ConfigurationTypes, LogContext } from '../../common/enums/index.js';
import { BootstrapException } from '../../common/exceptions/bootstrap.exception.js';
import { CommunicationAdapter } from '../../services/communication-adapter/communication.adapter.js';
import { MatrixAdminUserElevatedService } from '@src/domain/matrix-admin/user-elevated/matrix.admin.user.elevated.service.js';
import { MatrixAdminUserService } from '@src/domain/matrix-admin/user/matrix.admin.user.service.js';

@Injectable()
export class BootstrapService {
  constructor(
    private configService: ConfigService,
    private userManagementService: MatrixAdminUserService,
    private userElevatedManagementService: MatrixAdminUserElevatedService,
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
      await this.initiateMatrixConnection();
    } catch (error: any) {
      throw new BootstrapException(error.message);
    }
  }

  public async initiateMatrixConnection() {
    await this.logServerVersion();
    this.logger.verbose?.(
      `Matrix admin identifier: ${this.userElevatedManagementService.adminCommunicationsID}`,
      LogContext.BOOTSTRAP
    );
    await this.userElevatedManagementService.getMatrixAgentElevated();
  }

  private async logServerVersion() {
    try {
      const version = await this.userManagementService.getServerVersion();
      this.logger.verbose?.(
        `Synapse server version: ${version}`,
        LogContext.BOOTSTRAP
      );
    } catch (error: any) {
      this.logger.verbose?.(
        `Unable to get synapse server version: ${error}`,
        LogContext.BOOTSTRAP
      );
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
