import { Module } from '@nestjs/common';
import { MatrixRoomAdapterModule } from '@src/domain/adapter-room/matrix.room.adapter.module';
import { MatrixUserAdapterModule } from '@src/domain/adapter-user/matrix.user.adapter.module';
import { MatrixAgentPoolModule } from '@src/domain/agent/agent-pool/matrix.agent.pool.module';

import { MatrixAdminUserModule } from '../../domain/matrix-admin/user/matrix.admin.user.module';
import { MatrixAdminUserElevatedModule } from '../../domain/matrix-admin/user-elevated/matrix.admin.user.elevated.module';
import { CommunicationAdapter } from './communication.adapter';

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
