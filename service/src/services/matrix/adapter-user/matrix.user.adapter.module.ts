import { Module } from '@nestjs/common';
import { MatrixUserAdapter } from './matrix.user.adapter.js';

@Module({
  imports: [],
  providers: [MatrixUserAdapter],
  exports: [MatrixUserAdapter],
})
export class MatrixUserAdapterModule {}
