import { Module } from '@nestjs/common';
import { MatrixCryptographyService } from '@src/services/cryptography/matrix.cryptography.service';

@Module({
  providers: [MatrixCryptographyService],
  exports: [MatrixCryptographyService],
})
export class MatrixCryptographyModule {}
