import { Module } from '@nestjs/common';
import { MatrixUserAdapterModule } from '@src/domain/adapter-user/matrix.user.adapter.module';
import { MatrixAgentFactoryModule } from '@src/domain/agent/agent-factory/matrix.agent.factory.module';

import { MatrixAdminUserModule } from '../user/matrix.admin.user.module';
import { MatrixAdminUserElevatedService } from './matrix.admin.user.elevated.service';

@Module({
  imports: [
    MatrixAdminUserModule,
    MatrixUserAdapterModule,
    MatrixAgentFactoryModule,
  ],
  providers: [MatrixAdminUserElevatedService],
  exports: [MatrixAdminUserElevatedService],
})
export class MatrixAdminUserElevatedModule {}
