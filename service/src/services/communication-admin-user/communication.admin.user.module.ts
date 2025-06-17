import { Module } from '@nestjs/common';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module.js';
import { MatrixAgentModule } from '@services/matrix/agent/matrix.agent.module.js';
import { MatrixUserAdapterModule } from '@services/matrix/adapter-user/matrix.user.adapter.module.js';
import { CommunicationAdminUserService } from './communication.admin.user.service.js';

@Module({
  imports: [
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    MatrixAgentModule,
  ],
  providers: [CommunicationAdminUserService],
  exports: [CommunicationAdminUserService],
})
export class CommunicationAdminUserModule {}
