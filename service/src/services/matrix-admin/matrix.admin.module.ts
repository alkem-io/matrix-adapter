import { Module } from '@nestjs/common';
import { MatrixAdminService } from './matrix.admin.service';
import { CommunicationAdapterModule } from '../communication-adapter/communication-adapter.module';
import { MatrixAgentModule } from '../matrix/agent/matrix.agent.module';
import { MatrixUserManagementModule } from '../matrix/management/matrix.user.management.module';
import { MatrixUserAdapterModule } from '../matrix/adapter-user/matrix.user.adapter.module';
import { MatrixAdminController } from './matrix.admin.controller';
import { CommunicationAdminUserModule } from '../communication-admin-user/communication.admin.user.module';

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
