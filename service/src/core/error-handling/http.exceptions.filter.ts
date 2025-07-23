import { LogContext } from '@common/enums/index';
import { BaseException } from '@common/exceptions/base.exception';
import {
  ArgumentsHost,
  ExceptionFilter,
} from '@nestjs/common';
import pkg from '@nestjs/common';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
const { Catch, Injectable, Inject } = pkg;

@Injectable()
@Catch()
export class HttpExceptionsFilter implements ExceptionFilter {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService
  ) {}

  catch(exception: BaseException, _host: ArgumentsHost) {
    let context = LogContext.UNSPECIFIED;

    if (exception.getContext) context = exception.getContext();

    this.logger.error(`${exception.message} - ${_host}`, exception.stack, context);

    return exception;
  }
}
