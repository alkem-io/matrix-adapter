import { Module } from '@nestjs/common';
import { MatrixAgentPoolModule } from '@src/domain/matrix/agent-pool/matrix.agent.pool.module';
import { MatrixRoomAdapterModule } from '@src/domain/matrix/adapter-room/matrix.room.adapter.module';
import { MatrixUserAdapterModule } from '@src/domain/matrix/adapter-user/matrix.user.adapter.module';
import { CommunicationAdapter } from './communication.adapter';
import { MatrixAdminUserElevatedModule } from '../../domain/matrix-admin/user-elevated/matrix.admin.user.elevated.module';
import { MatrixAdminUserModule } from '../../domain/matrix-admin/user/matrix.admin.user.module';

@Module({
  imports: [
    MatrixAdminUserModule,
    MatrixUserAdapterModule,
    MatrixRoomAdapterModule,
    MatrixAgentPoolModule,
    MatrixAdminUserElevatedModule,
  ],
  providers: [CommunicationAdapter],
  exports: [CommunicationAdapter],
})
export class CommunicationAdapterModule {}
