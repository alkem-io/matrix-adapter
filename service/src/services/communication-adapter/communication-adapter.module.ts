import { Module } from '@nestjs/common';
import { MatrixAgentPoolModule } from '@services/matrix/agent-pool/matrix.agent.pool.module';
import { MatrixRoomAdapterModule } from '@services/matrix/adapter-room/matrix.room.adapter.module';
import { MatrixAgentModule } from '@src/services/matrix/agent-factory/matrix.agent.factory.module';
import { MatrixUserAdapterModule } from '@services/matrix/adapter-user/matrix.user.adapter.module';
import { CommunicationAdapter } from './communication.adapter';
import { MatrixAdminUserElevatedModule } from '../matrix-admin/user-elevated/matrix.admin.user.elevated.module';
import { MatrixUserManagementModule } from '../matrix-admin/user/matrix.admin.user.module';

@Module({
  imports: [
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    MatrixRoomAdapterModule,
    MatrixAgentModule,
    MatrixAgentPoolModule,
    MatrixAdminUserElevatedModule,
  ],
  providers: [CommunicationAdapter],
  exports: [CommunicationAdapter],
})
export class CommunicationAdapterModule {}
