import { Module } from '@nestjs/common';
import { MatrixAgentPool } from '@src/domain/matrix/agent-pool/matrix.agent.pool';
import { MatrixAgentModule } from '../agent-factory/matrix.agent.factory.module';
import { MatrixUserManagementModule } from '@src/domain/matrix-admin/user/matrix.admin.user.module';

@Module({
  imports: [MatrixUserManagementModule, MatrixAgentModule],
  providers: [MatrixAgentPool],
  exports: [MatrixAgentPool],
})
export class MatrixAgentPoolModule {}
