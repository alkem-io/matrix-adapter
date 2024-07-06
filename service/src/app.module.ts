import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { APP_FILTER } from '@nestjs/core';
import { WinstonModule } from 'nest-winston';
import { HealthModule } from '@services/health';
import { AppController } from './app.controller';
import { WinstonConfigService } from './config';
import configuration from './config/configuration';
import { HttpExceptionsFilter } from './core';
import { BootstrapModule } from './core/bootstrap/bootstrap.module';
import { CommunicationAdapterModule } from './services/communication-adapter/communication-adapter.module';
import { MatrixAdminModule } from './services/matrix-admin/matrix.admin.module';

@Module({
  imports: [
    ConfigModule.forRoot({
      envFilePath: ['.env'],
      isGlobal: true,
      load: [configuration],
    }),
    WinstonModule.forRootAsync({
      useClass: WinstonConfigService,
    }),
    BootstrapModule,
    CommunicationAdapterModule,
    MatrixAdminModule,
    HealthModule,
  ],
  providers: [
    {
      provide: APP_FILTER,
      useClass: HttpExceptionsFilter,
    },
  ],
  controllers: [AppController],
})
export class AppModule {}
