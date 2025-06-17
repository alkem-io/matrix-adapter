import { Module } from '@nestjs/common';
import { MatrixCryptographyService } from '@services/matrix/cryptography/matrix.cryptography.service.js';

@Module({
  providers: [MatrixCryptographyService],
  exports: [MatrixCryptographyService],
})
export class MatrixCryptographyModule {}
