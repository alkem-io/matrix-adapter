import { Module } from '@nestjs/common';
import { MatrixCryptographyService } from '@services/matrix/cryptography/matrix.cryptography.service';

@Module({
  providers: [MatrixCryptographyService],
  exports: [MatrixCryptographyService],
})
export class MatrixCryptographyModule {}
