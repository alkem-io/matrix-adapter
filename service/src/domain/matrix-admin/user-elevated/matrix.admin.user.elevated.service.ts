import { ConfigurationTypes, LogContext } from '@common/enums/index';
import pkg  from '@nestjs/common';
const { Inject, Injectable } = pkg;
import { ConfigService } from '@nestjs/config';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { MatrixUserAdapter } from '@src/domain/matrix/adapter-user/matrix.user.adapter';
import { IOperationalMatrixUser } from '@src/domain/matrix/adapter-user/matrix.user.interface';
import { MatrixAgent } from '@src/domain/agent/matrix.agent';
import { MatrixAgentFactoryService } from '@src/domain/matrix/agent-factory/matrix.agent.factory.service';
import { MatrixAdminUserService } from '../user/matrix.admin.user.service';

@Injectable()
export class MatrixAdminUserElevatedService {
  private adminUser!: IOperationalMatrixUser;
  private matrixElevatedAgent!: MatrixAgent; // elevated as created with an admin account
  private adminEmail!: string;
  public adminCommunicationsID!: string;
  private adminPassword!: string;

  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private matrixAgentService: MatrixAgentFactoryService,
    private configService: ConfigService,
    private matrixUserManagementService: MatrixAdminUserService,
    private matrixUserAdapter: MatrixUserAdapter
  ) {
    this.adminEmail = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.admin?.username;
    this.adminPassword = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.admin?.password;

    this.adminCommunicationsID = this.matrixUserAdapter.convertEmailToMatrixID(
      this.adminEmail
    );
  }

  private async getElevatedUser() {
    if (this.adminUser) {
      return this.adminUser;
    }

    const adminExists = await this.matrixUserManagementService.isRegistered(
      this.adminCommunicationsID
    );
    if (adminExists) {
      this.logger.verbose?.(
        `Admin user is registered: ${this.adminEmail}, logging in...`,
        LogContext.COMMUNICATION
      );
      const adminUser = await this.matrixUserManagementService.login(
        this.adminCommunicationsID,
        this.adminPassword
      );
      this.adminUser = adminUser;
      return adminUser;
    }

    this.adminUser = await this.registerNewAdminUser();
    return this.adminUser;
  }

  public async getMatrixAgentElevated() {
    if (this.matrixElevatedAgent) {
      return this.matrixElevatedAgent;
    }

    const adminUser = await this.getElevatedUser();
    this.matrixElevatedAgent =
      await this.matrixAgentService.createMatrixAgent(adminUser);

    await this.matrixElevatedAgent.start({
      registerTimelineMonitor: false,
      registerRoomMonitor: false,
    });

    return this.matrixElevatedAgent;
  }

  private async registerNewAdminUser(): Promise<IOperationalMatrixUser> {
    return await this.matrixUserManagementService.register(
      this.adminCommunicationsID,
      this.adminPassword,
      true
    );
  }
}
