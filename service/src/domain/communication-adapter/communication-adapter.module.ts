import { Module } from '@nestjs/common';
import { MatrixAgentPoolModule } from '@src/domain/matrix/agent-pool/matrix.agent.pool.module';
import { MatrixRoomAdapterModule } from '@src/domain/matrix/adapter-room/matrix.room.adapter.module';
import { MatrixAgentModule } from '@src/domain/matrix/agent-factory/matrix.agent.factory.module';
import { MatrixUserAdapterModule } from '@src/domain/matrix/adapter-user/matrix.user.adapter.module';
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
