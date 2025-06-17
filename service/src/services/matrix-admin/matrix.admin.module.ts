import { Module } from '@nestjs/common';
import { MatrixAdminService } from './matrix.admin.service.js';
import { CommunicationAdapterModule } from '../communication-adapter/communication-adapter.module.js';
import { MatrixAgentModule } from '../matrix/agent/matrix.agent.module.js';
import { MatrixUserManagementModule } from '../matrix/management/matrix.user.management.module.js';
import { MatrixUserAdapterModule } from '../matrix/adapter-user/matrix.user.adapter.module.js';
import { MatrixAdminController } from './matrix.admin.controller.js';
import { CommunicationAdminUserModule } from '../communication-admin-user/communication.admin.user.module.js';

@Module({
  imports: [
    CommunicationAdapterModule,
    MatrixAgentModule,
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    CommunicationAdminUserModule,
  ],
  providers: [MatrixAdminService],
  exports: [MatrixAdminService],
  controllers: [MatrixAdminController],
})
export class MatrixAdminModule {}
