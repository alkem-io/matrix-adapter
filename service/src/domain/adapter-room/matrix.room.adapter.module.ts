import { Module } from '@nestjs/common';
import { MatrixMessageAdapterModule } from '../adapter-message/matrix.message.adapter.module';
import { MatrixRoomAdapter } from './matrix.room.adapter';
import { MatrixUserAdapterModule } from '../adapter-user/matrix.user.adapter.module';

@Module({
  imports: [MatrixUserAdapterModule, MatrixMessageAdapterModule],
  providers: [MatrixRoomAdapter],
  exports: [MatrixRoomAdapter],
})
export class MatrixRoomAdapterModule {}
