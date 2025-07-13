import { Module } from '@nestjs/common';
import { HttpModule } from '@nestjs/axios';
import { MatrixCryptographyModule } from '@services/matrix/cryptography/matrix.cryptography.module';
import { MatrixUserAdapterModule } from '@src/services/matrix/adapter-user/matrix.user.adapter.module';
import { MatrixUserManagementService } from './matrix.admin.user.service';

@Module({
  imports: [MatrixUserAdapterModule, MatrixCryptographyModule, HttpModule],
  providers: [MatrixUserManagementService],
  exports: [MatrixUserManagementService],
})
export class MatrixUserManagementModule {}
