import pkg from '@nestjs/common';
const { Inject, Injectable } = pkg;
import { HttpService } from '@nestjs/axios';
import { ConfigService } from '@nestjs/config';
import { SynapseEndpoint } from '@src/common/enums/synapse.endpoint.js';
import { MatrixUserRegistrationException } from '@src/common/exceptions/matrix.registration.exception.js';
import { MatrixAdminUserElevatedService } from '@src/domain/matrix-admin/user-elevated/matrix.admin.user.elevated.service.js';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';

import { ConfigurationTypes, LogContext } from '../../common/enums/index.js';
import { BootstrapException } from '../../common/exceptions/bootstrap.exception.js';

@Injectable()
export class BootstrapService {
  constructor(
    private configService: ConfigService,
    private userElevatedManagementService: MatrixAdminUserElevatedService,
    private httpService: HttpService,
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
      const version = await this.getServerVersion();
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

  public async getServerVersion(): Promise<string> {
    const baseUrl = this.configService.get(ConfigurationTypes.MATRIX)?.server
      ?.url;
    const url = new URL(SynapseEndpoint.SERVER_VERSION, baseUrl);
    const response = await this.httpService
      .get<{ server_version: string }>(url.href)
      .toPromise();
    if (!response)
      throw new MatrixUserRegistrationException(
        'Invalid response!',
        LogContext.COMMUNICATION
      );

    const version = response.data['server_version'];
    return JSON.stringify(version);
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
