import { LoggerService } from '@nestjs/common';
import { LogContext } from '@src/common/enums/logging.context';
import { Logger } from 'matrix-js-sdk/lib/logger';

export class AlkemioMatrixLogger implements Logger {
  private winstonLogger!: LoggerService;
  private prefix = '[alkemio]';

  constructor(winstonLogger: LoggerService) {
    this.winstonLogger = winstonLogger;
  }

  public debug(message: string) {
    this.winstonLogger.debug?.(`${this.prefix}: ${message}`, LogContext.MATRIX);
  }

  public info(message: string) {
    this.winstonLogger.warn?.(`${this.prefix}: ${message}`, LogContext.MATRIX);
  }

  public warn(message: string) {
    this.winstonLogger.warn?.(`${this.prefix}: ${message}`, LogContext.MATRIX);
  }

  public error(message: string) {
    this.winstonLogger.error?.(`${this.prefix}: ${message}`, LogContext.MATRIX);
  }
  public trace(message: string) {
    this.winstonLogger.verbose?.(
      `${this.prefix}: ${message}`,
      LogContext.MATRIX
    );
  }

  public getChild(namespace: string) {
    this.winstonLogger.debug?.(
      `${this.prefix}: ${namespace}`,
      LogContext.MATRIX
    );
    return this;
  }
}
