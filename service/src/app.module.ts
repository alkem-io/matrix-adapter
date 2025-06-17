import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { APP_FILTER } from '@nestjs/core';
import { WinstonModule } from 'nest-winston';
import { HealthModule } from '@services/health/health.module.js';
import { AppController } from './app.controller.js';
import { WinstonConfigService } from './config/winston.config.js';
import configuration from './config/configuration.js';
import { HttpExceptionsFilter } from './core/error-handling/http.exceptions.filter.js';
import { BootstrapModule } from './core/bootstrap/bootstrap.module.js';
import { CommunicationAdapterModule } from './services/communication-adapter/communication-adapter.module.js';
import { MatrixAdminModule } from './services/matrix-admin/matrix.admin.module.js';

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
