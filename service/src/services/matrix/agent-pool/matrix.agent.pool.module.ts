import { Module } from '@nestjs/common';
import { MatrixAgentPool } from '@services/matrix/agent-pool/matrix.agent.pool';
import { MatrixAgentModule } from '../agent/matrix.agent.module';
import { MatrixUserManagementModule } from '@src/services/matrix-admin/user/matrix.admin.user.module';

@Module({
  imports: [MatrixUserManagementModule, MatrixAgentModule],
  providers: [MatrixAgentPool],
  exports: [MatrixAgentPool],
})
export class MatrixAgentPoolModule {}
