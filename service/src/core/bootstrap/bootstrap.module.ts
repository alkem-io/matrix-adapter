import { Module } from '@nestjs/common';
import { BootstrapService } from './bootstrap.service.js';
import { CommunicationAdapterModule } from '@src/services/communication-adapter/communication-adapter.module.js';

@Module({
  imports: [CommunicationAdapterModule],
  providers: [BootstrapService],
  exports: [BootstrapService],
})
export class BootstrapModule {}
