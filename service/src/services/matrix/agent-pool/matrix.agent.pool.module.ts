import { Module } from '@nestjs/common';
import { MatrixAgentPool } from '@services/matrix/agent-pool/matrix.agent.pool';
import { MatrixUserManagementModule } from '@services/matrix/management/matrix.user.management.module';
import { MatrixAgentModule } from '../agent/matrix.agent.module';

@Module({
  imports: [MatrixUserManagementModule, MatrixAgentModule],
  providers: [MatrixAgentPool],
  exports: [MatrixAgentPool],
})
export class MatrixAgentPoolModule {}
