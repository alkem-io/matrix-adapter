import { ConfigurationTypes } from '@common/enums/configuration.type';
import { Injectable } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import * as logform from 'logform';
import { utilities as nestWinstonModuleUtilities } from 'nest-winston';
import * as winston from 'winston';
import * as WinstonElasticsearch from 'winston-elasticsearch';

const LOG_LABEL = 'matrix-adapter';

const consoleLoggingStandardFormat: logform.Format[] = [
  winston.format.timestamp(),
  nestWinstonModuleUtilities.format.nestLike(),
];

const consoleLoggingProdFormat: logform.Format[] = [
  winston.format.timestamp(),
  winston.format.label({ label: LOG_LABEL }),
  winston.format.json({ deterministic: true }),
];

@Injectable()
export class WinstonConfigService {
  constructor(private configService: ConfigService) {}

  createWinstonModuleOptions() {
    const consoleEnabled: boolean = this.configService.get(
      ConfigurationTypes.MONITORING
    )?.logging?.console_logging_enabled;
    const transports: any[] = [
      new winston.transports.Console({
        format: winston.format.combine(
          ...(this.configService.get(ConfigurationTypes.MONITORING)?.logging
            ?.json
            ? consoleLoggingProdFormat
            : consoleLoggingStandardFormat)
        ),
        level: this.configService
          .get(ConfigurationTypes.MONITORING)
          ?.logging?.level.toLowerCase(),
        silent: !consoleEnabled,
      }),
    ];

    if (
      this.configService.get(ConfigurationTypes.MONITORING)?.elastic?.enabled
    ) {
      transports.push(
        new WinstonElasticsearch.ElasticsearchTransport({
          level: this.configService.get(ConfigurationTypes.MONITORING)?.elastic
            ?.logging_level,
          transformer: logData => {
            return {
              '@timestamp': new Date().getTime(),
              severity: logData.level,
              message: `[${logData.level}] LOG Message: ${logData.message}`,
              environment: this.configService.get(ConfigurationTypes.HOSTING)
                ?.environment as string,
              fields: { ...logData.meta },
            };
          },
          clientOpts: {
            cloud: {
              id: this.configService.get(ConfigurationTypes.MONITORING)?.elastic
                ?.cloud?.id,
            },
            auth: {
              username: this.configService.get(ConfigurationTypes.MONITORING)
                ?.elastic?.cloud?.username,
              password: this.configService.get(ConfigurationTypes.MONITORING)
                ?.elastic?.cloud?.password,
            },
          },
        })
      );
    }

    return {
      transports: transports,
    };
  }
}
