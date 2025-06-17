import { Module } from '@nestjs/common';
import { MatrixMessageAdapter } from './matrix.message.adapter.js';

@Module({
  imports: [],
  providers: [MatrixMessageAdapter],
  exports: [MatrixMessageAdapter],
})
export class MatrixMessageAdapterModule {}
