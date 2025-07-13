import { Module } from '@nestjs/common';
import { MatrixAdminRoomsService } from './matrix.admin.rooms.service';
import { CommunicationAdapterModule } from '../../../services/communication-adapter/communication-adapter.module';
import { MatrixAgentFactoryModule } from '../../matrix/agent-factory/matrix.agent.factory.module';
import { MatrixUserAdapterModule } from '../../matrix/adapter-user/matrix.user.adapter.module';
import { MatrixAdminRoomsController } from './matrix.admin.rooms.controller';
import { MatrixAdminUserElevatedModule } from '../user-elevated/matrix.admin.user.elevated.module';
import { MatrixAdminUserModule } from '../user/matrix.admin.user.module';

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
