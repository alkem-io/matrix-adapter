import { Module } from '@nestjs/common';
import { HttpModule } from '@nestjs/axios';
import { MatrixCryptographyModule } from '@services/matrix/cryptography/matrix.cryptography.module.js';
import { MatrixUserManagementService } from '@services/matrix/management/matrix.user.management.service.js';
import { MatrixUserAdapterModule } from '../adapter-user/matrix.user.adapter.module.js';

@Module({
  imports: [MatrixUserAdapterModule, MatrixCryptographyModule, HttpModule],
  providers: [MatrixUserManagementService],
  exports: [MatrixUserManagementService],
})
export class MatrixUserManagementModule {}
