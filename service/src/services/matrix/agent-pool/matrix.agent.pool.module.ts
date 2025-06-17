import { Module } from '@nestjs/common';
import { MatrixAgentPool } from '@services/matrix/agent-pool/matrix.agent.pool.js';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module.js';
import { MatrixAgentModule } from '../agent/matrix.agent.module.js';

@Module({
  imports: [MatrixUserManagementModule, MatrixAgentModule],
  providers: [MatrixAgentPool],
  exports: [MatrixAgentPool],
})
export class MatrixAgentPoolModule {}
