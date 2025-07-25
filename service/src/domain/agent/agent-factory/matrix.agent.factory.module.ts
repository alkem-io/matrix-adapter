import { Module } from '@nestjs/common';
import { MatrixAdminUserModule } from '@src/domain/matrix-admin/user/matrix.admin.user.module';

import { MatrixMessageAdapterModule } from '../../adapter-message/matrix.message.adapter.module';
import { MatrixRoomAdapterModule } from '../../adapter-room/matrix.room.adapter.module';
import { MatrixUserAdapterModule } from '../../adapter-user/matrix.user.adapter.module';
import { MatrixAgentFactoryService } from './matrix.agent.factory.service';

@Module({
  imports: [
    MatrixAdminUserModule,
    MatrixUserAdapterModule,
    MatrixRoomAdapterModule,
    MatrixMessageAdapterModule,
  ],
  providers: [MatrixAgentFactoryService],
  exports: [MatrixAgentFactoryService],
})
export class MatrixAgentFactoryModule {}
