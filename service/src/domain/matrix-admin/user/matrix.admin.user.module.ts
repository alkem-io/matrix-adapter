import { HttpModule } from '@nestjs/axios';
import { Module } from '@nestjs/common';
import { MatrixUserAdapterModule } from '@src/domain/adapter-user/matrix.user.adapter.module';
import { MatrixCryptographyModule } from '@src/services/cryptography/matrix.cryptography.module';

import { MatrixAdminUserService } from './matrix.admin.user.service';

@Module({
  imports: [MatrixUserAdapterModule, MatrixCryptographyModule, HttpModule],
  providers: [MatrixAdminUserService],
  exports: [MatrixAdminUserService],
})
export class MatrixAdminUserModule {}
