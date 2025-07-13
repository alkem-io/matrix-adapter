import { Module } from '@nestjs/common';
import { MatrixAgentPool } from '@src/domain/matrix/agent-pool/matrix.agent.pool';
import { MatrixAgentModule } from '../agent-factory/matrix.agent.factory.module';
import { MatrixAdminUserModule } from '@src/domain/matrix-admin/user/matrix.admin.user.module';

@Module({
  imports: [MatrixAdminUserModule, MatrixAgentModule],
  providers: [MatrixAgentPool],
  exports: [MatrixAgentPool],
})
export class MatrixAgentPoolModule {}
