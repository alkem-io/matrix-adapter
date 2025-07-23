import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { APP_FILTER } from '@nestjs/core';
import { WinstonModule } from 'nest-winston';

import { AppController } from './app.controller';
import configuration from './config/configuration';
import { WinstonConfigService } from './config/winston.config';
import { BootstrapModule } from './core/bootstrap/bootstrap.module';
import { HttpExceptionsFilter } from './core/error-handling/http.exceptions.filter';
import { MatrixAdminRoomsModule } from './domain/matrix-admin/rooms/matrix.admin.rooms.module';
import { CommunicationAdapterModule } from './services/communication-adapter/communication-adapter.module';
import { HealthModule } from './services/health/health.module';

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
    MatrixAdminRoomsModule,
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
