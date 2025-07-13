import { Module } from '@nestjs/common';
import { BootstrapService } from './bootstrap.service';
import { CommunicationAdapterModule } from '@src/services/communication-adapter/communication-adapter.module';

@Module({
  imports: [CommunicationAdapterModule],
  providers: [BootstrapService],
  exports: [BootstrapService],
})
export class BootstrapModule {}
