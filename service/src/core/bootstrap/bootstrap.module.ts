import { HttpModule } from '@nestjs/axios';
import { Module } from '@nestjs/common';
import { MatrixAdminUserElevatedModule } from '@src/domain/matrix-admin/user-elevated/matrix.admin.user.elevated.module';

import { BootstrapService } from './bootstrap.service';

@Module({
  imports: [MatrixAdminUserElevatedModule, HttpModule],
  providers: [BootstrapService],
  exports: [BootstrapService],
})
export class BootstrapModule {}
