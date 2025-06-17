import { Module } from '@nestjs/common';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module.js';
import { MatrixMessageAdapterModule } from '../adapter-message/matrix.message.adapter.module.js';
import { MatrixRoomAdapterModule } from '../adapter-room/matrix.room.adapter.module.js';
import { MatrixUserAdapterModule } from '../adapter-user/matrix.user.adapter.module.js';
import { MatrixAgentService } from './matrix.agent.service.js';

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
