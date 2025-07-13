import { Module } from '@nestjs/common';
import { MatrixAgentModule } from '@src/domain/matrix/agent-factory/matrix.agent.factory.module';
import { MatrixUserAdapterModule } from '@src/domain/matrix/adapter-user/matrix.user.adapter.module';
import { MatrixAdminUserElevatedService } from './matrix.admin.user.elevated.service';
import { MatrixUserManagementModule } from '../user/matrix.admin.user.module';

@Module({
  imports: [
    MatrixUserManagementModule,
    MatrixUserAdapterModule,
    MatrixAgentModule,
  ],
  providers: [MatrixAdminUserElevatedService],
  exports: [MatrixAdminUserElevatedService],
})
export class MatrixAdminUserElevatedModule {}
