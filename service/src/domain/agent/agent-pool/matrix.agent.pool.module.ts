import { Module } from '@nestjs/common';
import { MatrixAgentPool } from '@src/domain/agent/agent-pool/matrix.agent.pool';
import { MatrixAdminUserModule } from '@src/domain/matrix-admin/user/matrix.admin.user.module';

import { MatrixAgentFactoryModule } from '../agent-factory/matrix.agent.factory.module';

@Module({
  imports: [MatrixAdminUserModule, MatrixAgentFactoryModule],
  providers: [MatrixAgentPool],
  exports: [MatrixAgentPool],
})
export class MatrixAgentPoolModule {}
