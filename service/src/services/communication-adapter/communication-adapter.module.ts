import { Module } from '@nestjs/common';
import { MatrixAgentPoolModule } from '@services/matrix/agent-pool/matrix.agent.pool.module';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module';
import { MatrixGroupAdapterModule } from '@services/matrix/adapter-group/matrix.group.adapter.module';
import { MatrixRoomAdapterModule } from '@services/matrix/adapter-room/matrix.room.adapter.module';
import { MatrixAgentModule } from '@services/matrix/agent/matrix.agent.module';
import { MatrixUserAdapterModule } from '@services/matrix/adapter-user/matrix.user.adapter.module';
import { CommunicationAdapter } from './communication.adapter';

@Module({
  imports: [
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    MatrixRoomAdapterModule,
    MatrixGroupAdapterModule,
    MatrixAgentModule,
    MatrixAgentPoolModule,
  ],
  providers: [CommunicationAdapter],
  exports: [CommunicationAdapter],
})
export class CommunicationAdapterModule {}
