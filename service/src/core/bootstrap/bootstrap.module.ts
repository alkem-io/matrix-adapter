import { Module } from '@nestjs/common';
import { BootstrapService } from './bootstrap.service';

@Module({
  imports: [],
  providers: [BootstrapService],
  exports: [BootstrapService],
})
export class BootstrapModule {}
