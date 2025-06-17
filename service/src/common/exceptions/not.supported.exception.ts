import { LogContext, AlkemioErrorStatus } from '../enums/index.js';
import { BaseException } from './base.exception.js';

export class NotSupportedException extends BaseException {
  constructor(error: string, context: LogContext) {
    super(error, context, AlkemioErrorStatus.NOT_SUPPORTED);
  }
}
