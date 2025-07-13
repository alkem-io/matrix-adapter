import { Module } from '@nestjs/common';
import { MatrixMessageAdapterModule } from '../adapter-message/matrix.message.adapter.module';
import { MatrixRoomAdapterModule } from '../adapter-room/matrix.room.adapter.module';
import { MatrixUserAdapterModule } from '../adapter-user/matrix.user.adapter.module';
import { MatrixUserManagementModule } from '@src/services/matrix-admin/user/matrix.admin.user.module';
import { MatrixAgentFactoryService } from './matrix.agent.factory.service';

@Module({
  imports: [
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    MatrixRoomAdapterModule,
    MatrixMessageAdapterModule,
  ],
  providers: [MatrixAgentFactoryService],
  exports: [MatrixAgentFactoryService],
})
export class MatrixAgentModule {}
