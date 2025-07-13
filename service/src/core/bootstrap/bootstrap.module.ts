import { Module } from '@nestjs/common';
import { BootstrapService } from './bootstrap.service';
import { MatrixAdminUserElevatedModule } from '@src/domain/matrix-admin/user-elevated/matrix.admin.user.elevated.module';
import { HttpModule } from '@nestjs/axios';

@Module({
  imports: [MatrixAdminUserElevatedModule, HttpModule],
  providers: [BootstrapService],
  exports: [BootstrapService],
})
export class BootstrapModule {}
