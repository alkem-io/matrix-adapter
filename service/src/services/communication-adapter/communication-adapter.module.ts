import { Module } from '@nestjs/common';
import { MatrixAgentPoolModule } from '@services/matrix/agent-pool/matrix.agent.pool.module';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module';
import { MatrixRoomAdapterModule } from '@services/matrix/adapter-room/matrix.room.adapter.module';
import { MatrixAgentModule } from '@services/matrix/agent/matrix.agent.module';
import { MatrixUserAdapterModule } from '@services/matrix/adapter-user/matrix.user.adapter.module';
import { CommunicationAdapter } from './communication.adapter';
import { CommunicationAdminUserModule } from '../communication-admin-user/communication.admin.user.module';

@Module({
  imports: [
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    MatrixRoomAdapterModule,
    MatrixAgentModule,
    MatrixAgentPoolModule,
    CommunicationAdminUserModule,
  ],
  providers: [CommunicationAdapter],
  exports: [CommunicationAdapter],
})
export class CommunicationAdapterModule {}
