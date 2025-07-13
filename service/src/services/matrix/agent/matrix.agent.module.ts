import { Module } from '@nestjs/common';
import { MatrixMessageAdapterModule } from '../adapter-message/matrix.message.adapter.module';
import { MatrixRoomAdapterModule } from '../adapter-room/matrix.room.adapter.module';
import { MatrixUserAdapterModule } from '../adapter-user/matrix.user.adapter.module';
import { MatrixAgentService } from './matrix.agent.service';
import { MatrixUserManagementModule } from '@src/services/matrix-admin/user/matrix.admin.user.module';

@Module({
  imports: [
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    MatrixRoomAdapterModule,
    MatrixMessageAdapterModule,
  ],
  providers: [MatrixAgentService],
  exports: [MatrixAgentService],
})
export class MatrixAgentModule {}
