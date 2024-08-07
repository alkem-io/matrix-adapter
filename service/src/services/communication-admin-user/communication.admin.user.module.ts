import { Module } from '@nestjs/common';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module';
import { MatrixAgentModule } from '@services/matrix/agent/matrix.agent.module';
import { MatrixUserAdapterModule } from '@services/matrix/adapter-user/matrix.user.adapter.module';
import { CommunicationAdminUserService } from './communication.admin.user.service';

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
