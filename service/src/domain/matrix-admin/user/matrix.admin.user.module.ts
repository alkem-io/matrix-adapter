import { Module } from '@nestjs/common';
import { HttpModule } from '@nestjs/axios';
import { MatrixCryptographyModule } from '@src/services/cryptography/matrix.cryptography.module';
import { MatrixUserAdapterModule } from '@src/domain/matrix/adapter-user/matrix.user.adapter.module';
import { MatrixAdminUserService } from './matrix.admin.user.service';

@Module({
  imports: [MatrixUserAdapterModule, MatrixCryptographyModule, HttpModule],
  providers: [MatrixAdminUserService],
  exports: [MatrixAdminUserService],
})
export class MatrixAdminUserModule {}
