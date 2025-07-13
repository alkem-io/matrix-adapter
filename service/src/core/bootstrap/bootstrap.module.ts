import { Module } from '@nestjs/common';
import { BootstrapService } from './bootstrap.service';
import { MatrixAdminUserElevatedModule } from '@src/domain/matrix-admin/user-elevated/matrix.admin.user.elevated.module';
import { MatrixAdminUserModule } from '@src/domain/matrix-admin/user/matrix.admin.user.module';

@Module({
  imports: [MatrixAdminUserElevatedModule, MatrixAdminUserModule],
  providers: [BootstrapService],
  exports: [BootstrapService],
})
export class BootstrapModule {}
