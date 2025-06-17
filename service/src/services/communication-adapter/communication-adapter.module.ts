import { Module } from '@nestjs/common';
import { MatrixAgentPoolModule } from '@services/matrix/agent-pool/matrix.agent.pool.module.js';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module.js';
import { MatrixRoomAdapterModule } from '@services/matrix/adapter-room/matrix.room.adapter.module.js';
import { MatrixAgentModule } from '@services/matrix/agent/matrix.agent.module.js';
import { MatrixUserAdapterModule } from '@services/matrix/adapter-user/matrix.user.adapter.module.js';
import { CommunicationAdapter } from './communication.adapter.js';
import { CommunicationAdminUserModule } from '../communication-admin-user/communication.admin.user.module.js';

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
