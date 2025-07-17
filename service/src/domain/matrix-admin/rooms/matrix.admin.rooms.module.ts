import { Module } from '@nestjs/common';
import { MatrixAgentFactoryModule } from '@src/domain/agent/agent-factory/matrix.agent.factory.module';

import { CommunicationAdapterModule } from '../../../services/communication-adapter/communication-adapter.module';
import { MatrixUserAdapterModule } from '../../adapter-user/matrix.user.adapter.module';
import { MatrixAdminUserModule } from '../user/matrix.admin.user.module';
import { MatrixAdminUserElevatedModule } from '../user-elevated/matrix.admin.user.elevated.module';
import { MatrixAdminRoomsController } from './matrix.admin.rooms.controller';
import { MatrixAdminRoomsService } from './matrix.admin.rooms.service';

@Module({
  imports: [
    CommunicationAdapterModule,
    MatrixAgentFactoryModule,
    MatrixAdminUserModule,
    MatrixUserAdapterModule,
    MatrixAdminUserElevatedModule,
  ],
  providers: [MatrixAdminRoomsService],
  exports: [MatrixAdminRoomsService],
  controllers: [MatrixAdminRoomsController],
})
export class MatrixAdminRoomsModule {}
