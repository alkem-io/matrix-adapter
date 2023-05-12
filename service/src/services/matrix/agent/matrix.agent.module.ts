import { Module } from '@nestjs/common';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module';
import { MatrixMessageAdapterModule } from '../adapter-message/matrix.message.adapter.module';
import { MatrixRoomAdapterModule } from '../adapter-room/matrix.room.adapter.module';
import { MatrixUserAdapterModule } from '../adapter-user/matrix.user.adapter.module';
import { MatrixAgentService } from './matrix.agent.service';

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
