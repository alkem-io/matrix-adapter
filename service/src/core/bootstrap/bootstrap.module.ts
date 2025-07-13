import { Module } from '@nestjs/common';
import { BootstrapService } from './bootstrap.service';
import { MatrixAdminUserElevatedModule } from '@src/domain/matrix-admin/user-elevated/matrix.admin.user.elevated.module';

@Module({
  imports: [MatrixAdminUserElevatedModule],
  providers: [BootstrapService],
  exports: [BootstrapService],
})
export class BootstrapModule {}
